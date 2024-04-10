// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package obj

import (
	"debug/dwarf"
	"debug/macho"
	"fmt"
	"io"
	"log"
	"sort"
	"sync"
	"syscall"

	"github.com/aclements/go-obj/arch"
)

// TODO: support relocations.

type machoFile struct {
	f        *macho.File
	arch     *arch.Arch
	sections []*machoSection
	symbols  []Sym
}

func (f *machoFile) Sym(i SymID) Sym { return f.symbols[i] }
func (f *machoFile) NumSyms() SymID  { return SymID(len(f.symbols)) }

func openMachO(r io.ReaderAt) (bool, File, error) {
	// Is this a MachO file?
	var magic [4]uint8 // MachO 64 = 0xFEEDFACF (LE)
	if _, err := r.ReadAt(magic[0:], 0); err != nil {
		return false, nil, err // file too short
	}
	if magic[3] != '\xFE' || magic[2] != '\xED' || magic[1] != '\xFA' || magic[0] != '\xCF' {
		return false, nil, nil // not MachO
	}

	// All errors after this point should return (true, _, err).

	// Parse MachO.
	ff, err := macho.NewFile(r)
	if err != nil {
		return true, nil, err
	}
	f := &machoFile{f: ff, arch: arch.ARM64}

	// Read section table.
	for rawID, machoSect := range ff.Sections {
		s := &Section{
			File:  f,
			Name:  machoSect.Name,
			ID:    SectionID(len(f.sections)), // 0-based
			RawID: rawID,                      // 0-based
			Addr:  machoSect.Addr,
			Size:  machoSect.Size,
		}

		ms := &machoSection{Section: s, macho: machoSect}
		f.sections = append(f.sections, ms)
	}

	// Read symbol table.
	if ff.Symtab != nil {
		const stabTypeMask = 0xE0

		// Build list of symbols, sort by Addr,
		// compute sizes by subtracting each previous Addr.
		var addrs []uint64
		for _, s := range f.f.Symtab.Syms {
			if s.Type&stabTypeMask != 0 {
				continue // Skip stab debug info.
			}
			addrs = append(addrs, s.Value)
		}
		sort.Slice(addrs, func(i, j int) bool { return addrs[i] < addrs[j] })

		var syms []Sym
		for _, s := range f.f.Symtab.Syms {
			if s.Type&stabTypeMask != 0 {
				continue // Skip stab debug info.
			}

			sym := Sym{
				Name:  s.Name,
				Value: s.Value,
				Kind:  SymUnknown, // (initially)
			}

			i := sort.Search(len(addrs), func(x int) bool { return addrs[x] > s.Value })
			if i < len(addrs) {
				sym.Size = uint64(addrs[i] - s.Value)
			}

			if s.Sect == 0 {
				sym.Kind = SymUndef
			} else if int(s.Sect) <= len(f.f.Sections) {
				sect := f.f.Sections[s.Sect-1]
				sym.Section = f.sections[s.Sect-1].Section
				switch sect.Seg {
				case "__TEXT":
					if sect.Name == "__rodata" {
						sym.Kind = SymData // (nm: R)
					} else {
						sym.Kind = SymText
					}

				case "__DATA":
					// section names:
					// __bss (nm: B)
					// __noptrbss (nm: B)
					// __data
					// __noptrdata
					// __go_buildinfo (nm: R)
					sym.Kind = SymData

				case "__DATA_CONST":
					// section names:
					// __rodata
					// __gopclntab
					// __gosymtab
					// __itablink
					// __typelink
					sym.Kind = SymData
				}
				if sym.Kind == SymUnknown {
					log.Printf("unknown symbol %s (Section.{Seg=%s,Name=%s})",
						s.Name, sect.Seg, sect.Name)
				}
			}
			syms = append(syms, sym)
		}
		f.symbols = syms
	}

	return true, f, nil
}

func (f *machoFile) Close() {
	// Release mmaps.
	for _, s := range f.sections {
		if s.mmapped != nil {
			mmapped := s.mmapped
			s.data = nil
			s.mmapped = nil
			syscall.Munmap(mmapped)
		}
	}
}

func (f *machoFile) Info() FileInfo {
	return FileInfo{Arch: f.arch}
}

func (f *machoFile) AsDebugDwarf() (*dwarf.Data, error) {
	return f.f.DWARF()
}

// Assert that machoFile implements AsDebugDwarf.
var _ AsDebugDwarf = (*machoFile)(nil)

// AsDebugMacho is implemented by File types that can return an underlying
// *debug/macho.File for format-specific access. AsDebugMacho may return
// nil, so the caller must both check that the type implements
// AsDebugMacho and check the result of calling AsDebugMacho.
type AsDebugMacho interface {
	File
	AsDebugMacho() *macho.File
}

func (f *machoFile) AsDebugMacho() *macho.File {
	return f.f
}

// Assert that machoFile implements AsDebugMacho.
var _ AsDebugMacho = (*machoFile)(nil)

type machoSection struct {
	// These fields are populated on loading.

	*Section

	macho *macho.Section

	dataOnce sync.Once
	data     []byte
	dataErr  error
	mmapped  []byte // if non-nil, original mmap of this section
}

func (s *machoSection) String() string {
	return fmt.Sprintf("%s [%d]", s.Name, s.RawID)
}

func (f *machoFile) Sections() []*Section {
	out := make([]*Section, len(f.sections))
	for i, ms := range f.sections {
		out[i] = ms.Section
	}
	return out
}

func (f *machoFile) Section(i SectionID) *Section {
	return f.sections[i].Section
}

func (f *machoFile) sectionData(s *Section, addr, size uint64, d *Data) (*Data, error) {
	err := f.machoSectionData(f.sections[s.ID], addr, size, d)
	if err != nil {
		return nil, err
	}
	return d, nil
}

func (f *machoFile) machoSectionData(s *machoSection, addr, size uint64, d *Data) error {
	ms := s.macho

	// Validate requested range.
	if addr+size < addr {
		panic("address overflow")
	}
	if addr < ms.Addr || addr+size > ms.Addr+ms.Size {
		panic(fmt.Sprintf("requested data [0x%x, 0x%x) is outside section [0x%x, 0x%x)", addr, addr+size, ms.Addr, ms.Addr+ms.Size))
	}

	// Read the section.
	bytes, err := f.sectionBytes(s)
	if err != nil {
		return s.dataErr
	}

	// Construct data.
	*d = Data{
		Addr:   addr,
		B:      bytes[addr-ms.Addr:][:size],
		Layout: f.arch.Layout,
	}

	return nil
}

func (f *machoFile) sectionBytes(s *machoSection) (data []byte, err error) {
	s.dataOnce.Do(func() {
		s.data, s.mmapped, s.dataErr = f.sectionBytesUncached(s)
	})
	return s.data, s.dataErr
}

func (f *machoFile) sectionBytesUncached(s *machoSection) (data []byte, mmapped []byte, err error) {
	// TODO: do the same mmap optimizations as ELF.
	ms := s.macho
	data, err = io.ReadAll(ms.Open())
	if err != nil {
		return nil, nil, err
	}
	if uint64(len(data)) != ms.Size {
		log.Fatalf("reading section got %d bytes, want %d", len(data), ms.Size)
	}
	return data, nil, nil
}

func (f *machoFile) ResolveAddr(addr uint64) *Section {
	for _, ms := range f.sections {
		if ms.Addr <= addr && addr-ms.Addr < ms.Size {
			return ms.Section
		}
	}
	return nil
}

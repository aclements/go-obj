// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package obj

import (
	"debug/elf"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"sync"
	"syscall"

	"github.com/aclements/go-obj/arch"
)

type elfFile struct {
	f *elf.File
	elfArch

	// fd is the mmap-able FD of this file, or ^0.
	fd uintptr
	// pageSize is the system page size for mmapping.
	pageSize uint64

	// elfLayout is the data layout of the ELF file itself (as opposed
	// to the architecture).
	elfLayout arch.Layout
	// symSize is the size of a SYMTAB entry in bytes.
	symSize uint64
	// relSize and relaSize are the sizes of REL and RELA entries in bytes.
	relSize, relaSize uint64

	// relocatable is true if this is a REL-type file. In this case,
	// there's no meaningful mapped address space and relocations store
	// section-relative offsets instead of virtual addresses.
	relocatable bool

	// sections contains the sections of this object file, indexed by
	// internal ID (not ELF section number).
	sections []*elfSection

	// shnToSection maps ELF section numbers to *elfSection objects.
	//
	// In general, prefer lookupShn, which performs checking.
	shnToSection []*elfSection

	// symTabs stores the static (index 0) and dynamic (index 1) symbol
	// tables, if they exist.
	//
	// [TIS ELF 1.2 Book III, p. 1-2] There may be at most one of each
	// type of symbol table section.
	symTabs [2]elfSymTab
}

type elfArch struct {
	arch *arch.Arch

	relClass relocClassID
}

var elfArches = map[elf.Machine]elfArch{
	elf.EM_X86_64: {arch.AMD64, rcElfX86_64},
	elf.EM_386:    {arch.I386, rcElf386},
}

func openElf(r io.ReaderAt) (bool, File, error) {
	// Is this an ELF file?
	var magic [4]uint8
	if _, err := r.ReadAt(magic[0:], 0); err != nil {
		return false, nil, err
	}
	if magic[0] != '\x7f' || magic[1] != 'E' || magic[2] != 'L' || magic[3] != 'F' {
		return false, nil, nil
	}
	// If there are errors past this point, we assume it's ELF and we
	// should report the error.

	ff, err := elf.NewFile(r)
	if err != nil {
		return true, nil, err
	}

	f := &elfFile{f: ff, elfArch: elfArches[ff.Machine]}

	// Is this a real file we can mmap?
	if file, ok := r.(*os.File); ok {
		f.fd = file.Fd()
		f.pageSize = uint64(syscall.Getpagesize())
	} else {
		f.fd = ^uintptr(0)
	}

	// Set per-class constants.
	var elfWordSize int
	switch ff.Class {
	default:
		return true, nil, fmt.Errorf("unknown ELF class %s", ff.Class)
	case elf.ELFCLASS32:
		elfWordSize = 4
		f.symSize = elf.Sym32Size
		f.relSize = 4 + 4
		f.relaSize = 4 + 4 + 4
	case elf.ELFCLASS64:
		elfWordSize = 8
		f.symSize = elf.Sym64Size
		f.relSize = 8 + 8
		f.relaSize = 8 + 8 + 8
	}
	f.elfLayout = arch.NewLayout(ff.ByteOrder, elfWordSize)
	f.relocatable = ff.Type == elf.ET_REL

	// Process section table.
	var relSections []*elfSection
	var relocatableSections []*elfSection
	f.shnToSection = make([]*elfSection, len(ff.Sections))
	for elfID, elfSect := range ff.Sections {
		if elfSect.Type == elf.SHT_NULL {
			continue
		}

		// Add to sections.
		s := &Section{
			File:  f,
			Name:  elfSect.Name,
			ID:    SectionID(len(f.sections)),
			RawID: elfID,
			Addr:  elfSect.Addr,
			Size:  elfSect.Size,
		}
		if !f.relocatable && elfSect.Flags&elf.SHF_ALLOC != 0 {
			// We ignore allocatable sections in reloctable objects:
			// these sections turn into mapped sections *after linking*,
			// but don't have meaningful addresses right now.
			s.SetMapped(true)
		}
		if elfSect.Flags&elf.SHF_WRITE == 0 {
			s.SetReadOnly(true)
		}
		if elfSect.Type == elf.SHT_NOBITS {
			s.SetZeroInitialized(true)
		}

		es := &elfSection{Section: s, elf: elfSect}
		f.sections = append(f.sections, es)
		f.shnToSection[elfID] = es

		// Track sections we're interested in.
		switch elfSect.Type {
		case elf.SHT_SYMTAB:
			f.symTabs[0].section = es
		case elf.SHT_DYNSYM:
			f.symTabs[1].section = es
		case elf.SHT_REL, elf.SHT_RELA:
			relSections = append(relSections, es)
		}
		if elfSect.Flags&elf.SHF_ALLOC != 0 && es.canHaveRelocs() {
			// Add to the list of sections to which section-less
			// relocations apply. Section-less relocations only get
			// applied to sections that are actually loaded
			// ("allocatable"). This is important because non-alloctable
			// sections may overlap the loadable address space, but may
			// have relocations of their own (e.g., DWARF sections).
			relocatableSections = append(relocatableSections, es)
		}
	}

	// Process relocation sections.
	for _, es := range relSections {
		// Find this section's symbol table.
		var symTab *elfSymTab
		shnSyms := elf.SectionIndex(es.elf.Link)
		if shnSyms == 0 {
			// The section number may be zero if none of the relocations
			// reference any symbols. You see this in, e.g., .rel.plt
			// sections.
			symTab = emptyElfSymTab
		} else {
			symSection, ok := f.lookupShn(shnSyms)
			if !ok {
				return true, nil, fmt.Errorf("relocation section %s references bad symbol section %d", es, shnSyms)
			}
			for i := range f.symTabs {
				if f.symTabs[i].section == symSection {
					symTab = &f.symTabs[i]
					break
				}
			}
			if symTab == nil {
				return true, nil, fmt.Errorf("relocation section %s references non-symbol section %s", es, symSection)
			}
		}
		es.rel = &elfSectionRel{symTab: symTab}

		// Relocation sections indicate which section they apply to.
		// Reverse this mapping so we can quickly find which relocations
		// apply to a section.
		shnTarget := elf.SectionIndex(es.elf.Info)
		if shnTarget == 0 {
			// This relocation section applies to all (loadable)
			// sections. This is common in non-relocatable objects, and
			// only makes sense in non-reloctable objects because the
			// relocations must be virtually indexed (in relocatable
			// objects, relocations are section-relative).
			if f.relocatable {
				return true, nil, fmt.Errorf("relocation section %s uses section offsets, but has no target section", es)
			}
			for _, ls := range relocatableSections {
				ls.relocSections = append(ls.relocSections, es)
			}
		} else {
			// This relocation section applies to a specific section.
			target, ok := f.lookupShn(shnTarget)
			if !ok {
				return true, nil, fmt.Errorf("relocation section %s references missing target section %d", es, shnTarget)
			}
			if target.canHaveRelocs() {
				target.relocSections = append(target.relocSections, es)
				es.rel.target = target
			}
		}
	}

	// For each symbol table, compute its global index range and get its
	// string section.
	var nSyms SymID
	for i := range f.symTabs {
		symTab := &f.symTabs[i]
		es := symTab.section
		if es == nil {
			// This file doesn't have this type of symbol table.
			symTab.start = nSyms
			symTab.end = symTab.start
			continue
		}

		// Load the section data.
		err := f.elfSectionData(es, es.Addr, es.Size, &symTab.data)
		if err != nil {
			return true, nil, fmt.Errorf("reading symbol table: %w", err)
		}
		symTab.data.Layout = f.elfLayout

		// Compute index range.
		count := SymID(int(es.Size/f.symSize) - 1)
		symTab.start = nSyms
		symTab.end = symTab.start + count
		nSyms += count

		// Loads strings section.
		if symTab.section == nil {
			continue
		}
		strShn := elf.SectionIndex(symTab.section.elf.Link)
		strSection, ok := f.lookupShn(strShn)
		if !ok || strSection.elf.Type != elf.SHT_STRTAB {
			return true, nil, fmt.Errorf("symbol table %s references bad string section %d", es, strShn)
		}
		strAddr, strSize := strSection.Bounds()
		err = f.elfSectionData(strSection, strAddr, strSize, &symTab.strings)
		if err != nil {
			return true, nil, fmt.Errorf("reading string table %s: %w", es, err)
		}
		symTab.strings.Layout = f.elfLayout
	}

	return true, f, nil
}

func (f *elfFile) Close() {
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

func (f *elfFile) Info() FileInfo {
	return FileInfo{f.arch}
}

// AsDebugElf is implemented by File types that can return an underlying
// *debug/elf.File for format-specific access. AsDebugElf may return
// nil, so the caller must both check that the type implements
// AsDebugElf and check the result of calling AsDebugElf.
type AsDebugElf interface {
	AsDebugElf() *elf.File
}

func (f *elfFile) AsDebugElf() *elf.File {
	return f.f
}

// Assert that elfFile implements AsDebugElf.
var _ AsDebugElf = (*elfFile)(nil)

type elfSection struct {
	// These fields are populated on loading.

	*Section

	elf *elf.Section

	relocSections []*elfSection // Relocation sections that modify this section

	rel *elfSectionRel // For relocation sections

	dataOnce sync.Once
	data     []byte
	dataErr  error
	mmapped  []byte // if non-nil, original mmap of this section

	relocsOnce sync.Once
	relocs     []Reloc // Relocations that apply to this section. Sorted by Addr.
	relocsErr  error
}

func (s *elfSection) String() string {
	return fmt.Sprintf("%s [%d]", s.Name, s.RawID)
}

// canHaveRelocs returns whether this section can have relocations
// applied.
//
// We narrow this down because otherwise its common to see, e.g., a
// relocation section that applies to itself (because it applies to all
// loadable sections), which tends to lead to infinite loops. We don't
// want to apply relocations to any ELF metadata sections.
func (s *elfSection) canHaveRelocs() bool {
	return s.elf.Type == elf.SHT_PROGBITS || s.elf.Type == elf.SHT_NOBITS || s.elf.Type >= elf.SHT_LOPROC
}

// lookupShn returns the *elfSection for a raw ELF section number and
// whether or not the section exists.
func (f *elfFile) lookupShn(shn elf.SectionIndex) (*elfSection, bool) {
	if shn < elf.SectionIndex(len(f.shnToSection)) {
		es := f.shnToSection[shn]
		return es, es != nil
	}
	return nil, false
}

func (f *elfFile) Sections() []*Section {
	out := make([]*Section, len(f.sections))
	for i, es := range f.sections {
		out[i] = es.Section
	}
	return out
}

func (f *elfFile) Section(i SectionID) *Section {
	return f.sections[i].Section
}

func (f *elfFile) sectionData(s *Section, addr, size uint64, d *Data) (*Data, error) {
	err := f.elfSectionData(f.sections[s.ID], addr, size, d)
	if err != nil {
		return nil, err
	}
	return d, nil
}

func (f *elfFile) elfSectionData(s *elfSection, addr, size uint64, d *Data) error {
	es := s.elf

	// Validate requested range.
	if addr+size < addr {
		panic("address overflow")
	}
	if addr < es.Addr || addr+size > es.Addr+es.Size {
		panic(fmt.Sprintf("requested data [0x%x, 0x%x) is outside section [0x%x, 0x%x)", addr, addr+size, es.Addr, es.Addr+es.Size))
	}

	// Read the section and its relocations.
	bytes, err := f.sectionBytes(s)
	if err != nil {
		return s.dataErr
	}
	relocs, err := f.sectionRelocs(s)
	if err != nil {
		return s.dataErr
	}

	// Construct data.
	//
	// TODO: Slice down relocs?
	*d = Data{Addr: addr, P: bytes[addr-es.Addr:][:size], R: relocs, Layout: f.arch.Layout}

	return nil
}

func (f *elfFile) sectionBytes(s *elfSection) (data []byte, err error) {
	s.dataOnce.Do(func() {
		s.data, s.mmapped, s.dataErr = f.sectionBytesUncached(s)
	})
	return s.data, s.dataErr
}

var testMmapSection func(bool)

func (f *elfFile) sectionBytesUncached(s *elfSection) (data []byte, mmaped []byte, err error) {
	es := s.elf

	// TODO: Make this cross-platform.
	if es.Type == elf.SHT_NOBITS {
		// There's no data to mmap. Create an anonymous zeroed mmap to
		// avoid bloating the Go heap.
		size := roundUp2(es.Size, f.pageSize)
		if size > 0 {
			data, err = syscall.Mmap(-1, 0, int(size), syscall.PROT_READ, syscall.MAP_SHARED|syscall.MAP_ANONYMOUS)
			if err == nil {
				if testMmapSection != nil {
					testMmapSection(true)
				}
				return data[:es.Size], data, nil
			}
		}
		// Just allocate on the heap.
		if testMmapSection != nil {
			testMmapSection(false)
		}
		return make([]byte, s.elf.Size), nil, nil
	}

	// Memory map the section when possible.
	if f.fd != ^uintptr(0) && es.Flags&elf.SHF_COMPRESSED == 0 && es.Size > 0 {
		start := roundDown2(es.Offset, f.pageSize)
		end := roundUp2(es.Offset+es.Size, f.pageSize)
		data, err = syscall.Mmap(int(f.fd), int64(start), int(end-start), syscall.PROT_READ, syscall.MAP_SHARED|syscall.MAP_FILE)
		if err == nil {
			if testMmapSection != nil {
				testMmapSection(true)
			}
			return data[es.Offset-start:][:es.Size], data, nil
		}
	}

	// Mmaping failed or wasn't possible. Read into the heap.
	data, err = ioutil.ReadAll(es.Open())
	if err != nil {
		return nil, nil, err
	}
	if uint64(len(data)) != es.Size {
		panic(fmt.Sprintf("reading section got %d bytes, want %d", len(data), es.Size))
	}
	if testMmapSection != nil {
		testMmapSection(false)
	}
	return data, nil, nil
}

func (f *elfFile) ResolveAddr(addr uint64) *Section {
	if f.relocatable {
		// Relocatable object files don't have any meaningful load
		// addresses (even though sections can be marked allocatable).
		return nil
	}

	for _, es := range f.sections {
		// Only consider sections that will be loaded into the address space.
		if es.elf.Flags&elf.SHF_ALLOC == 0 {
			continue
		}

		if es.Addr <= addr && addr-es.Addr < es.Size {
			return es.Section
		}
	}

	return nil
}

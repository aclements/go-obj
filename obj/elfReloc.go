// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package obj

import (
	"debug/elf"
	"fmt"
	"sort"
	"sync"
)

type elfReloc struct {
	size byte
}

var elfRelocsX86_64 = map[elf.R_X86_64]elfReloc{
	elf.R_X86_64_NONE:            {0},
	elf.R_X86_64_64:              {8},
	elf.R_X86_64_PC32:            {4},
	elf.R_X86_64_GOT32:           {4},
	elf.R_X86_64_PLT32:           {4},
	elf.R_X86_64_COPY:            {0},
	elf.R_X86_64_GLOB_DAT:        {8},
	elf.R_X86_64_JMP_SLOT:        {8},
	elf.R_X86_64_RELATIVE:        {8},
	elf.R_X86_64_GOTPCREL:        {4},
	elf.R_X86_64_32:              {4},
	elf.R_X86_64_32S:             {4},
	elf.R_X86_64_16:              {2},
	elf.R_X86_64_PC16:            {2},
	elf.R_X86_64_8:               {1},
	elf.R_X86_64_PC8:             {1},
	elf.R_X86_64_DTPMOD64:        {8},
	elf.R_X86_64_DTPOFF64:        {8},
	elf.R_X86_64_TPOFF64:         {8},
	elf.R_X86_64_TLSGD:           {4},
	elf.R_X86_64_TLSLD:           {4},
	elf.R_X86_64_DTPOFF32:        {4},
	elf.R_X86_64_GOTTPOFF:        {4},
	elf.R_X86_64_TPOFF32:         {4},
	elf.R_X86_64_PC64:            {8},
	elf.R_X86_64_GOTOFF64:        {8},
	elf.R_X86_64_GOTPC32:         {4},
	elf.R_X86_64_GOT64:           {8},
	elf.R_X86_64_GOTPCREL64:      {8},
	elf.R_X86_64_GOTPC64:         {8},
	elf.R_X86_64_GOTPLT64:        {8},
	elf.R_X86_64_PLTOFF64:        {8},
	elf.R_X86_64_SIZE32:          {4},
	elf.R_X86_64_SIZE64:          {8},
	elf.R_X86_64_GOTPC32_TLSDESC: {4},
	elf.R_X86_64_TLSDESC_CALL:    {0},
	elf.R_X86_64_TLSDESC:         {16},
	elf.R_X86_64_IRELATIVE:       {8},
	// See https://github.com/hjl-tools/x86-psABI/wiki/X86-psABI
	elf.R_X86_64_RELATIVE64:    {8}, // For x32
	elf.R_X86_64_PC32_BND:      {4}, // For x32; deprecated
	elf.R_X86_64_PLT32_BND:     {4}, // For x32; deprecated
	elf.R_X86_64_GOTPCRELX:     {4},
	elf.R_X86_64_REX_GOTPCRELX: {4},
}

type relocClassElfX86_64 struct{}

func (relocClassElfX86_64) String(val uint32) string {
	return elf.R_X86_64(val).String()
}

func (relocClassElfX86_64) Size(val uint32) int {
	r, ok := elfRelocsX86_64[elf.R_X86_64(val)]
	if ok {
		return int(r.size)
	}
	return -1
}

var elfRelocs386 = map[elf.R_386]elfReloc{
	elf.R_386_NONE:          {0},
	elf.R_386_32:            {4},
	elf.R_386_PC32:          {4},
	elf.R_386_GOT32:         {4},
	elf.R_386_PLT32:         {4},
	elf.R_386_COPY:          {0},
	elf.R_386_GLOB_DAT:      {4},
	elf.R_386_JMP_SLOT:      {4},
	elf.R_386_RELATIVE:      {4},
	elf.R_386_GOTOFF:        {4},
	elf.R_386_GOTPC:         {4},
	elf.R_386_TLS_TPOFF:     {4},
	elf.R_386_TLS_IE:        {4},
	elf.R_386_TLS_GOTIE:     {4},
	elf.R_386_TLS_LE:        {4},
	elf.R_386_TLS_GD:        {4},
	elf.R_386_TLS_LDM:       {4},
	elf.R_386_16:            {2},
	elf.R_386_PC16:          {2},
	elf.R_386_8:             {1},
	elf.R_386_PC8:           {1},
	elf.R_386_TLS_GD_32:     {4},
	elf.R_386_TLS_GD_PUSH:   {4},
	elf.R_386_TLS_GD_CALL:   {4},
	elf.R_386_TLS_GD_POP:    {4},
	elf.R_386_TLS_LDM_32:    {4},
	elf.R_386_TLS_LDM_PUSH:  {4},
	elf.R_386_TLS_LDM_CALL:  {4},
	elf.R_386_TLS_LDM_POP:   {4},
	elf.R_386_TLS_LDO_32:    {4},
	elf.R_386_TLS_IE_32:     {4},
	elf.R_386_TLS_LE_32:     {4},
	elf.R_386_TLS_DTPMOD32:  {4},
	elf.R_386_TLS_DTPOFF32:  {4},
	elf.R_386_TLS_TPOFF32:   {4},
	elf.R_386_SIZE32:        {4},
	elf.R_386_TLS_GOTDESC:   {4},
	elf.R_386_TLS_DESC_CALL: {0},
	elf.R_386_TLS_DESC:      {4},
	elf.R_386_IRELATIVE:     {4},
	elf.R_386_GOT32X:        {4},
}

type relocClassElf386 struct{}

func (relocClassElf386) String(val uint32) string {
	return elf.R_386(val).String()
}

func (relocClassElf386) Size(val uint32) int {
	r, ok := elfRelocs386[elf.R_386(val)]
	if ok {
		return int(r.size)
	}
	return -1
}

type elfSectionRel struct {
	target *elfSection // Target of relocations, or nil for global relocations
	symTab *elfSymTab

	once   sync.Once
	err    error
	relocs []Reloc // Decoded relocations in this section.
}

// readSectionRel parses a relocation (SHT_REL or SHT_RELA) section and caches
// the results.
func (f *elfFile) readSectionRel(s *elfSection) ([]Reloc, error) {
	s.rel.once.Do(func() {
		s.rel.relocs, s.rel.err = f.readSectionRelUncached(s)
	})
	return s.rel.relocs, s.rel.err
}

// readSectionRelUncached parses relocation section s.
func (f *elfFile) readSectionRelUncached(s *elfSection) ([]Reloc, error) {
	// Pre-size the slice.
	var nRelocs uint64
	switch s.elf.Type {
	case elf.SHT_REL:
		nRelocs = s.Size / f.relSize
	case elf.SHT_RELA:
		nRelocs = s.Size / f.relaSize
	}
	relocs := make([]Reloc, 0, nRelocs)

	data, err := s.Data(s.Bounds())
	if err != nil {
		return nil, err
	}
	data.Layout = f.elfLayout
	r := NewReader(data)

	symTab := s.rel.symTab
	typ, cls := s.elf.Type, f.f.Class
	switch {
	case typ == elf.SHT_REL && cls == elf.ELFCLASS32:
		relocs = elfReadRel32(relocs, r, symTab, f.relClass)
	case typ == elf.SHT_REL && cls == elf.ELFCLASS64:
		relocs = elfReadRel64(relocs, r, symTab, f.relClass)
	case typ == elf.SHT_RELA && cls == elf.ELFCLASS32:
		relocs = elfReadRela32(relocs, r, symTab, f.relClass)
	case typ == elf.SHT_RELA && cls == elf.ELFCLASS64:
		relocs = elfReadRela64(relocs, r, symTab, f.relClass)
	default:
		// We shouldn't be trying to read this as relocations.
		panic("unexpected relocation section type")
	}

	// Sort relocs by address.
	sort.Slice(relocs, func(i, j int) bool {
		return relocs[i].Addr < relocs[j].Addr
	})

	if f.relocatable && s.Addr != 0 {
		// [TIS ELF 1.2 Book I, p. 1-23] In relocatable files, relocations
		// store section offsets, but we always want absolute addresses.
		// Adjust accordingly. Often such sections have an address of 0
		// anyway, in which case we skip this because it would be a no-op.
		for i := range relocs {
			relocs[i].Addr += s.Addr
			if relocs[i].Addr < s.Addr {
				return nil, fmt.Errorf("relocation %d in section %s: address overflow", i, s)
			}
		}
	}

	if typ == elf.SHT_REL {
		if err := f.populateAddends(s, relocs); err != nil {
			return nil, err
		}
	}
	return relocs, nil
}

var nullSection = elfSection{Section: &Section{}}

// populateAddends populates the Addend fields of relocs for SHT_REL sections,
// which store their addends implicitly in section data. s must be an SHT_REL
// section and relocs its decoded relocations (without Addends).
func (f *elfFile) populateAddends(s *elfSection, relocs []Reloc) error {
	var bytes []byte
	layout := f.arch.Layout
	target := s.rel.target
	global := target == nil
	if global {
		// Set the initial target to a fake empty section so the bounds check
		// below will immediately fail and we'll look up the real target.
		target = &nullSection
	}

	for i := range relocs {
		size := relocs[i].Type.Size()
		if size == -1 {
			return fmt.Errorf("relocation %d in section %s: can't read addend for unknown relocation type %s", i, s, relocs[i].Type)
		}
		off := relocs[i].Addr - target.Addr
		if size != 0 && off >= target.Size || off+uint64(size) > target.Size {
			if global {
				// This is a global relocation section, so this just means we've
				// moved on to a different target section.
				t := f.ResolveAddr(relocs[i].Addr)
				if t == nil {
					return fmt.Errorf("relocation %d in section %s: address %#x is not in any section", i, s, relocs[i].Addr)
				}
				target, bytes = f.sections[t.ID], nil
				off = relocs[i].Addr - target.Addr
			} else {
				return fmt.Errorf("relocation %d in section %s: address %#x out of section bounds [%#x,%#x)", i, s, relocs[i].Addr, target.Addr, target.Addr+target.Size)
			}
		}

		// Load the section data once we've established the target section.
		if bytes == nil {
			var err error
			bytes, err = f.sectionBytes(target)
			if err != nil {
				return fmt.Errorf("reading target section %s of relocation %d in section %s: %v", target, i, s, err)
			}
		}

		// Fetch the addend.
		var addend int64
		switch size {
		case 0:
		case 1:
			addend = int64(int8(bytes[off]))
		case 2:
			addend = int64(layout.Int16(bytes[off:]))
		case 4:
			addend = int64(layout.Int32(bytes[off:]))
		case 8:
			addend = layout.Int64(bytes[off:])
		default:
			return fmt.Errorf("relocation %d in section %s: bad REL relocation size %d", i, s, size)
		}
		relocs[i].Addend = addend
	}
	return nil
}

// sectionRelocs returns the relocations that apply to section s. The results
// are cached.
func (f *elfFile) sectionRelocs(s *elfSection) ([]Reloc, error) {
	// Most of the time there is just one relocation section that applies (or
	// zero), in which case we can just return the slice for that section.
	switch len(s.relocSections) {
	case 0:
		return nil, nil
	case 1:
		// This does its own caching.
		return f.readSectionRel(s.relocSections[0])
	}

	// There are multiple relocation sections. Read them all and merge them.
	// This isn't particularly common, but does happen in practice when there's
	// a relocation section that applies to all loadable sections and another
	// that targets a specific section.
	//
	// Since this is more complicated, we add our own caching.
	s.relocsOnce.Do(func() {
		// Often only a single reloc section will actually apply to this
		// section's range, so we collect the ones that apply and then
		// merge/sort if necessary.
		todo := make([][]Reloc, 0, 1)
		for _, rs := range s.relocSections {
			r, err := f.readSectionRel(rs)
			if err != nil {
				s.relocsErr = err
				return
			}
			if len(r) == 0 {
				continue
			}
			if r[0].Addr <= s.Addr+s.Size && r[len(r)-1].Addr > s.Addr {
				todo = append(todo, r)
			}
		}
		var relocs []Reloc
		if len(todo) == 1 {
			relocs = todo[0]
		} else if len(todo) > 0 {
			// More than one reloc section applied. Merge and sort them.
			for _, t := range todo {
				relocs = append(relocs, t...)
			}
			sort.Slice(relocs, func(i, j int) bool {
				return relocs[i].Addr < relocs[j].Addr
			})
		}
		s.relocs = relocs
	})
	return s.relocs, s.relocsErr
}

func elfReadRel32(relocs []Reloc, r *Reader, symTab *elfSymTab, relClass relocClassID) []Reloc {
	for r.Avail() >= 8 {
		off := r.Uint32()
		info := r.Uint32()
		symID, _ := symTab.lookup(elf.R_SYM32(info))
		typ := elf.R_TYPE32(info)
		relocs = append(relocs, Reloc{uint64(off), makeRelocType(relClass, typ), symID, 0})
	}
	return relocs
}

func elfReadRel64(relocs []Reloc, r *Reader, symTab *elfSymTab, relClass relocClassID) []Reloc {
	for r.Avail() >= 16 {
		off := r.Uint64()
		info := r.Uint64()
		symID, _ := symTab.lookup(elf.R_SYM64(info))
		typ := elf.R_TYPE64(info)
		relocs = append(relocs, Reloc{off, makeRelocType(relClass, typ), symID, 0})
	}
	return relocs
}

func elfReadRela32(relocs []Reloc, r *Reader, symTab *elfSymTab, relClass relocClassID) []Reloc {
	for r.Avail() >= 12 {
		off := r.Uint32()
		info := r.Uint32()
		symID, _ := symTab.lookup(elf.R_SYM32(info))
		typ := elf.R_TYPE32(info)
		add := r.Uint32()
		relocs = append(relocs, Reloc{uint64(off), makeRelocType(relClass, typ), symID, int64(add)})
	}
	return relocs
}

func elfReadRela64(relocs []Reloc, r *Reader, symTab *elfSymTab, relClass relocClassID) []Reloc {
	for r.Avail() >= 24 {
		off := r.Uint64()
		info := r.Uint64()
		symID, _ := symTab.lookup(elf.R_SYM64(info))
		typ := elf.R_TYPE64(info)
		add := r.Int64()
		relocs = append(relocs, Reloc{off, makeRelocType(relClass, typ), symID, add})
	}
	return relocs
}

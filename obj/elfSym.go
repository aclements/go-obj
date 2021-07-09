// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package obj

import (
	"debug/elf"
	"fmt"
)

// TODO: I'm confused by TLS symbols. They have a section (which is also marked
// TLS), but their value is a small integer, even though the section has some
// reasonable address. I think the section must be a "template" for the TLS and
// the value of the symbol is actually an offset into TLS. But that means these
// look like data symbols because they have a section, but their "Value" isn't
// in their section.

type elfSymTab struct {
	start, end SymID // Excludes ELF symbol index 0: start maps to ELF symbol 1
	section    *elfSection
	data       Data
	strings    Data
}

var emptyElfSymTab = &elfSymTab{}

func (s *elfSymTab) lookup(elfSym uint32) (SymID, bool) {
	// Subtract 1 from the ELF symbol index since we don't represent the NULL
	// ELF symbol. If elfSym is 0 (meaning no symbol), this will wrap below
	// s.start and we'll fall through to returning NoSym.
	symID := SymID(elfSym) - 1 + s.start
	if s.start <= symID && symID < s.end {
		return symID, true
	}
	return NoSym, false
}

func (f *elfFile) NumSyms() SymID {
	return f.symTabs[len(f.symTabs)-1].end
}

func (f *elfFile) Sym(i SymID) Sym {
	tab := &f.symTabs[0]
	if i >= tab.end {
		tab = &f.symTabs[1]
		if i >= tab.end {
			panic(fmt.Sprintf("symbol index %d out of range [%d,%d)", i, 0, f.NumSyms()))
		}
	}

	// Set up to read.
	r := NewReader(&tab.data)
	rs := NewReader(&tab.strings)
	r.SetOffset(int(f.symSize * uint64(i-tab.start+1)))

	var sym Sym
	var (
		nameOff uint32
		info    uint8
		shn     elf.SectionIndex
	)
	switch f.f.Class {
	case elf.ELFCLASS32:
		nameOff = r.Uint32()
		sym.Value = uint64(r.Uint32())
		sym.Size = uint64(r.Uint32())
		info = r.Uint8()
		_ = r.Uint8() // st_other
		shn = elf.SectionIndex(r.Uint16())
	case elf.ELFCLASS64:
		nameOff = r.Uint32()
		info = r.Uint8()
		_ = r.Uint8() // st_other
		shn = elf.SectionIndex(r.Uint16())
		sym.Value = r.Uint64()
		sym.Size = r.Uint64()
	}

	es, ok := f.lookupShn(shn)
	if ok {
		sym.Section = es.Section
	}

	if elf.ST_TYPE(info) == elf.STT_SECTION && es != nil {
		// Section symbols don't have their own name, but tools conventionally
		// show the name of the section.
		sym.Name = es.Name
	} else {
		rs.SetOffset(int(nameOff))
		sym.Name = string(rs.CString())
	}

	kind := SymUnknown
	switch elf.ST_TYPE(info) {
	case elf.STT_SECTION:
		kind = SymSection
	default:
		switch shn {
		case elf.SHN_UNDEF:
			kind = SymUndef
		case elf.SHN_COMMON:
			kind = SymBSS
		case elf.SHN_ABS:
			kind = SymAbsolute
		default:
			if es == nil {
				// Leave unknown.
				break
			}
			// Determine kind by looking at section flags.
			switch es.elf.Flags & (elf.SHF_WRITE | elf.SHF_ALLOC | elf.SHF_EXECINSTR) {
			case elf.SHF_ALLOC | elf.SHF_EXECINSTR:
				kind = SymText
			case elf.SHF_ALLOC:
				kind = SymROData
			case elf.SHF_ALLOC | elf.SHF_WRITE:
				if es.elf.Type == elf.SHT_NOBITS {
					kind = SymBSS
				} else {
					kind = SymData
				}
			}
		}
	}
	sym.Kind = kind

	sym.SetLocal(elf.ST_BIND(info) == elf.STB_LOCAL)

	return sym
}

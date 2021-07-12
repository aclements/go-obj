// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package obj

import (
	"fmt"
	"strconv"
	"strings"
)

// A SymID uniquely identifies a symbol within an object file. Symbols within an
// object file are always numbered sequentially from 0.
//
// This does not necessarily correspond to the symbol indexing scheme used by a
// given object format.
//
// Some formats can have multiple symbol tables (e.g., ELF). These tables will
// be combined in a single global index space.
type SymID uint32

// NoSym is a placeholder SymID used to indicate "no symbol".
const NoSym = ^SymID(0)

func (id SymID) String() string {
	if id == NoSym {
		return "NoSym"
	}
	return strconv.FormatUint(uint64(id), 10)
}

// A Sym is a symbol in an object file.
type Sym struct {
	// Name is the string name of this symbol.
	Name string
	// Section is the section this symbol is defined in, or nil if this symbol
	// is not defined in any section. A symbol has data if and only if Section
	// is non-nil.
	Section *Section
	// Value is the value of this symbol. The interpretation of this differs
	// between different kinds of symbols. If this is a data symbol (Section is
	// non-nil), this is an absolute address within Section.
	Value uint64
	// Size is the size of this symbol in bytes, if it is a data symbol, or 0 if
	// unknown.
	Size uint64
	// Kind gives the general kind of this symbol.
	Kind SymKind
	// SymFlags stores flags for this symbol. This field is embedded so Sym
	// inherits the methods of SymFlags.
	SymFlags
}

// SymKind indicates the general kind of a symbol. The exact mappings
// from different object formats to these kinds is generally fuzzy, so
// different versions of the obj package may change how symbols are
// categorized.
type SymKind uint8

const (
	// SymUnknown indicates a symbol could not be categorized into one of the
	// supported kinds.
	SymUnknown SymKind = '?'
	// SymUndef symbols are not defined in this object (it will be resolved by
	// linking against other objects).
	SymUndef SymKind = 'U'
	// SymText symbols are in an executable code section.
	SymText SymKind = 'T'
	// SymData symbols are in a data section. This includes read-only
	// and zero-initialized (BSS) data.
	SymData SymKind = 'D'
	// SymAbsolute symbols have an absolute value that won't be changed by
	// linking. Generally, the "value" of an absolute symbol is not an address
	// like most symbols, but the actual value of the symbol.
	SymAbsolute SymKind = 'A'
	// SymSection symbols represent a section. Some object formats put sections
	// in the symbol table and others don't.
	SymSection SymKind = 'S'
)

// String returns a string representation of k. This is a single character in
// the style of "nm".
func (k SymKind) String() string {
	return string([]byte{byte(k)})
}

// SymFlags is a set of symbol flags.
type SymFlags struct {
	f symFlags
}

type symFlags uint8

const (
	symFlagLocal symFlags = 1 << iota
	symFlagSizeSynthesized

	// TODO: Indicate which symbol table this comes from for formats that
	// support more than one? (ELF has static and dynamic symbols.)
)

// Local indicates a symbol's name is only meaningful withing its defining
// compilation unit.
func (s SymFlags) Local() bool {
	return s.f&symFlagLocal != 0
}

// SetLocal sets the Local flag to v.
func (s *SymFlags) SetLocal(v bool) {
	if v {
		s.f |= symFlagLocal
	} else {
		s.f &^= symFlagLocal
	}
}

// SizeSynthesized indicates a symbol's size was synthensized using heuristics.
func (s SymFlags) SizeSynthesized() bool {
	return s.f&symFlagSizeSynthesized != 0
}

// SetSizeSynthesized set the SizeSynthesized flag to v.
func (s *SymFlags) SetSizeSynthesized(v bool) {
	if v {
		s.f |= symFlagSizeSynthesized
	} else {
		s.f &^= symFlagSizeSynthesized
	}
}

// String returns a string representation of the flags set in s.
func (s SymFlags) String() string {
	if s.f == 0 {
		return "{}"
	}
	var buf strings.Builder
	var sep byte = '{'
	if s.Local() {
		buf.WriteByte(sep)
		buf.WriteString("Local")
		sep = ','
	}
	if s.SizeSynthesized() {
		buf.WriteByte(sep)
		buf.WriteString("SizeSynthesized")
		sep = ','
	}
	buf.WriteByte('}')
	return buf.String()
}

// String returns the name of symbol s.
func (s *Sym) String() string {
	if s == nil {
		return "<nil>"
	}
	return s.Name
}

// Data reads size bytes of data from this symbol, starting at the given
// address. If s is an undefined symbol or otherwise not backed by data,
// it returns an ErrNoData error. It panics if the requested byte range
// is out of range for the section.
func (s *Sym) Data(addr, size uint64) (*Data, error) {
	if s.Section == nil {
		// We return an error rather than panic so that "Data" is useful
		// as a general-purpose interface.
		switch s.Kind {
		case SymUndef:
			return nil, &ErrNoData{"undefined symbol"}
		case SymAbsolute:
			return nil, &ErrNoData{"absolute symbol"}
		}
		return nil, &ErrNoData{"unknown reason"}
	}
	if addr < s.Value || addr+size > s.Value+s.Size {
		panic(fmt.Sprintf("requested data [0x%x, 0x%x) is outside symbol [0x%x, 0x%x)", addr, addr+size, s.Value, s.Value+s.Size))
	}
	return s.Section.Data(addr, size)
}

// Bounds returns the starting address and size in bytes of symbol s.
// For undefined symbols, it returns 0, 0.
func (s *Sym) Bounds() (addr, size uint64) {
	if s.Section == nil {
		return 0, 0
	}
	return s.Value, s.Size
}

// An ErrNoData error indicates that an entity is not backed by data.
type ErrNoData struct {
	Detail string
}

func (e *ErrNoData) Error() string {
	return fmt.Sprintf("no data: %s", e.Detail)
}

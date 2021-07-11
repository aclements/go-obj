// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package obj provide a common abstraction for working with different
// object formats.
package obj

import (
	"fmt"
	"io"

	"github.com/aclements/go-obj/arch"
)

// TODO: Raw file header bytes? Generic metadata representation for headers?

// Open attempts to open r as a known object file format.
func Open(r io.ReaderAt) (File, error) {
	if isElf, f, err := openElf(r); isElf {
		return f, err
	}
	// if isPE, f, err := openPE(r); isPE {
	// 	return f, err
	// }
	return nil, fmt.Errorf("unrecognized object file format")
}

// A File represents an object file.
type File interface {
	// Close closes this object file, releasing any OS resources used by it.
	//
	// It's possible that referencing a Data object returned from this File
	// after closing the File will panic.
	Close()

	// Info returns metadata about the whole object file.
	Info() FileInfo

	// Sections returns a slice of sections in this object file, indexed
	// by SectionID.
	//
	// All data in the object file (code, program data, etc) is stored
	// in sections. Often many metadata tables (e.g., symbol tables) are
	// as well.
	//
	// Each section has a name that generally follows a platform
	// convention, such as ".text" or ".data".
	//
	// All addresses within an object file, such as symbol addresses,
	// relocation targets, etc. are relative to some section of the
	// object file. Each section may or may not have a fixed base
	// address and sections may overlap. For example, in an executable
	// image, all of the loadable sections will have base addresses and
	// will not overlap, but there may be additional information in
	// non-loadable sections such as debugging info. In an unlinked
	// object file, typically none of the sections have base addresses.
	// And in a position-independent executable, sections may have base
	// addresses, but may be relocated to another address.
	Sections() []*Section

	// Section returns the i'th section. If i is out of range, it panics.
	Section(i SectionID) *Section

	// sectionData implements Section.Data. On success, it should
	// populate *d and return d, nil. If there's an error, it should
	// return nil and the error.
	sectionData(s *Section, addr, size uint64, d *Data) (*Data, error)

	// ResolveAddr finds the Section containing the given address in the
	// "loaded" address space. It returns nil if addr is not in the
	// loaded address space. Not all sections are loaded, and some types
	// of object files don't have any loaded address space at all (for
	// example, ELF relocatable objects).
	//
	// TODO: Reconsider this API. Eventually it would be nice to be able
	// to handle addresses from relocated PIE images and core files. Do
	// those just implement the File interface and present the
	// underlying file relocated to the target addresses given by the
	// PIE loading/core file?
	ResolveAddr(addr uint64) *Section

	// Sym returns i'th symbol. If i is our of range, it panics.
	Sym(i SymID) Sym

	// NumSyms returns the number of symbols.
	//
	// If an object file has more than one symbol table, they will be
	// concatenated. As a result, the "same" symbol may appear multiple times.
	NumSyms() SymID

	// TODO: AsDWARFData interface?
	//DWARF() (*dwarf.Data, error)
}

type FileInfo struct {
	// Arch is the machine architecture of this object file, or
	// nil if unknown.
	Arch *arch.Arch
}

// SectionID is an index for a section in an object file. These indexes
// are compact and start at 0.
//
// These may not correspond to any section numbering used by the object
// format itself; see Section.RawID for this. For example, ELF section
// number 0 is reserved, so this slice starts at section 1 in ELF
// objects.
type SectionID int

// A Section is a contiguous region of address space in an object file.
//
// An object file may have multiple sections whose addresses are not
// meaningfully related, so addresses within an object file must always
// be specified with respect to a given section.
type Section struct {
	// File is the object file containing this section.
	File File

	// Name is the name of this section. This typically follows platform
	// conventions, such as ".text" or ".data", but isn't necessarily
	// meaningful.
	Name string

	// ID is the obj-internal index of this section.
	ID SectionID

	// RawID is the index of this section in the underlying format's
	// representation, or -1 if this is not meaningful.
	RawID int

	// Addr is the virtual address at which this section begins in
	// memory, or 0 if either this section should not be loaded into
	// memory, or it has not yet been assigned a meaningful address.
	Addr uint64

	// Size is the size of this section in memory, in bytes.
	//
	// This may not be the size of the section on disk. For example, a
	// section that is all zeros may not be represented on disk at all,
	// or the section on disk may be compressed.
	Size uint64

	// SectionFlags stores flags for this section. This field is
	// embedded so Section inherits the methods of SectionFlags.
	SectionFlags
}

// Data reads size bytes of data from this section, starting at the
// given address. It panics if the requested byte range is out of range
// for the section.
func (s *Section) Data(addr, size uint64) (*Data, error) {
	// This approach allows the allocation of Data to be inlined into
	// the caller, where it can often be stack-allocated.
	var d Data
	return s.File.sectionData(s, addr, size, &d)
}

// Bounds returns the starting address and size in bytes of Section s.
func (s *Section) Bounds() (addr, size uint64) {
	return s.Addr, s.Size
}

// SectionFlags is a set of symbol flags.
type SectionFlags struct {
	f sectionFlags
}

type sectionFlags uint8

const (
	sectionFlagReadOnly sectionFlags = 1 << iota
	sectionFlagZeroInitialized
)

// ReadOnly indicates a section's data is read-only.
func (s SectionFlags) ReadOnly() bool {
	return s.f&sectionFlagReadOnly != 0
}

// SetReadOnly sets the ReadOnly flag to v.
func (s *SectionFlags) SetReadOnly(v bool) {
	if v {
		s.f |= sectionFlagReadOnly
	} else {
		s.f &^= sectionFlagReadOnly
	}
}

// ZeroInitialized indicates a section is in a zero-initialized section.
func (s SectionFlags) ZeroInitialize() bool {
	return s.f&sectionFlagReadOnly != 0
}

// SetZeroInitialized sets the ZeroInitialized flag to v.
func (s *SectionFlags) SetZeroInitialized(v bool) {
	if v {
		s.f |= sectionFlagZeroInitialized
	} else {
		s.f &^= sectionFlagZeroInitialized
	}
}

// roundDown2 to rounds x down to a multiple of y, where y must be a
// power of 2.
func roundDown2(x, y uint64) uint64 {
	if y&(y-1) != 0 {
		panic("y must be a power of 2")
	}
	return x &^ (y - 1)
}

// roundUp2 to rounds x up to a multiple of y, where y must be a power
// of 2.
func roundUp2(x, y uint64) uint64 {
	if y&(y-1) != 0 {
		panic("y must be a power of 2")
	}
	return (x + y - 1) &^ (y - 1)
}

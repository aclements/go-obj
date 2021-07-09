// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package obj

import "fmt"

// A Reloc is a relocation.
type Reloc struct {
	// Addr is the address where this Reloc is applied.
	//
	// This is an absolute address within some section. Hence, to compute the
	// offset of the Reloc within a Section, use Addr - Section.Addr, and to
	// compute the byte offset of the Reloc within a Data, use Addr - Data.Addr.
	Addr uint64
	// Type is the relocation type. This determines how to calculate the value
	// that would be stored at Offset.
	Type RelocType
	// Symbol is the target of this Reloc, or NoSym if Type does not have a
	// symbol as an input.
	Symbol SymID
	// Addend is the addend input to Type, if any.
	//
	// If the file format uses addends smaller than 64-bits, they will be sign
	// extended.
	//
	// Objects formats store addends either explicitly in the relocations table
	// or implicitly at the target of the relocation. The obj package hides this
	// difference and populate Addend in either case.
	Addend int64
}

// RelocType gives the type of a relocation. Relocations vary widely by
// architecture and operating system, so the interface to this is fairly opaque.
type RelocType struct {
	// n is the relocation type, encoded as a relocation class in the top 8 bits
	// and a relocation type within the class in the remaining 24 bits. We do
	// this rather than using an interface type to keep Reloc compact and
	// pointer-free, since we decode entire relocation sections into the heap.
	n uint32
}

// TODO: We may want a way to get the native relocation out for consumers that
// are willing to deal with them. That could just be the number. It would be
// nice if it were more type-safe and used the debug/* library types, though
// it's easy enough for a consumer to cast.

func (r RelocType) String() string {
	c, v := r.class()
	return c.String(v)
}

// Size returns the size of the relocation target in bytes, or -1 if unknown.
func (r RelocType) Size() int {
	c, v := r.class()
	return c.Size(v)
}

func (r RelocType) class() (relocClass, uint32) {
	c, v := r.n>>24, r.n&(1<<24-1)
	if c < uint32(len(relocClasses)) {
		return relocClasses[c], v
	}
	return relocClasses[rcUnknown], v
}

func makeRelocType(cls relocClassID, v uint32) RelocType {
	if v&(1<<24-1) != v {
		badRelocValue(v)
	}
	return RelocType{uint32(cls<<24) | v}
}

func badRelocValue(v uint32) {
	panic(fmt.Sprintf("relocation value %d too large to represent as a RelocType", v))
}

type relocClassID uint32

// Relocation classes.
const (
	rcUnknown relocClassID = iota
	rcElfX86_64
	rcElf386
)

var relocClasses = [...]relocClass{
	rcUnknown:   relocClassUnknown{},
	rcElfX86_64: relocClassElfX86_64{},
	rcElf386:    relocClassElf386{},
}

type relocClass interface {
	String(val uint32) string
	Size(val uint32) int
}

type relocClassUnknown struct{}

func (relocClassUnknown) String(val uint32) string {
	return fmt.Sprintf("unknown (0x%#x)", val)
}

func (relocClassUnknown) Size(val uint32) int {
	return -1
}

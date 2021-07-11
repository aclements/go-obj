// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package obj

import (
	"bytes"
	"fmt"

	"github.com/aclements/go-obj/arch"
)

// Data represents byte data in an object file.
type Data struct {
	// Addr is the address at which this data starts.
	//
	// If this Data is for a Section or a Sym, this is the base address
	// of the section or symbol.
	Addr uint64

	// B stores the raw byte data. Callers must not modify this.
	B []byte

	// R stores the relocations applied to this Data in increasing
	// address order.
	//
	// This may include relocations outside or partially outside of this
	// Data's address range.
	R []Reloc

	// Layout specifies the byte order and word size of this data. This
	// is inferred from the object file's architecture, and hence may
	// not be correct for sections or symbols that have a fixed byte
	// order regardless of the host order.
	Layout arch.Layout
}

type Reader struct {
	d *Data
	p int // Offset into P
}

func NewReader(d *Data) *Reader {
	return &Reader{d, 0}
}

// SetAddr moves r's cursor to the given address. If addr is out of
// range for r's Data, it panics.
func (r *Reader) SetAddr(addr uint64) {
	o := int(addr - r.d.Addr)
	if addr < r.d.Addr || o >= len(r.d.B) {
		panic(fmt.Sprintf("address 0x%x out of data's range [0x%x,0x%x)", addr, r.d.Addr, r.d.Addr+uint64(len(r.d.B))))
	}
	r.p = o
}

// Addr returns the current position of r's cursor as an address in r's Data.
func (r *Reader) Addr() uint64 {
	return r.d.Addr + uint64(r.p)
}

// SetOffset moves r's cursor to the given offset from the beginning of
// r's data.
func (r *Reader) SetOffset(offset int) {
	if offset < 0 || offset >= len(r.d.B) {
		r.badOffset(offset)
	}
	r.p = offset
}

func (r *Reader) badOffset(offset int) {
	panic(fmt.Sprintf("offset %d out of data's range [0,%d)", offset, len(r.d.B)))
}

// Avail returns the number of bytes remaining in r's Data.
func (r *Reader) Avail() int {
	return len(r.d.B) - r.p
}

func (r *Reader) Uint8() uint8 {
	o := r.p
	r.p++
	return r.d.B[o]
}

func (r *Reader) Uint16() uint16 {
	o := r.p
	r.p += 2
	return r.d.Layout.Uint16(r.d.B[o : o+2])
}

func (r *Reader) Uint32() uint32 {
	o := r.p
	r.p += 4
	return r.d.Layout.Uint32(r.d.B[o : o+4])
}

func (r *Reader) Uint64() uint64 {
	o := r.p
	r.p += 8
	return r.d.Layout.Uint64(r.d.B[o : o+8])
}

func (r *Reader) Int8() int8   { return int8(r.Uint8()) }
func (r *Reader) Int16() int16 { return int16(r.Uint16()) }
func (r *Reader) Int32() int32 { return int32(r.Uint32()) }
func (r *Reader) Int64() int64 { return int64(r.Uint64()) }

// Word reads a word from r using the word size from r's Data.
func (r *Reader) Word() uint64 {
	o := r.p
	r.p += r.d.Layout.WordSize()
	return r.d.Layout.Word(r.d.B[o:])
}

// CString reads a NULL-terminated string. The result omits the final
// NULL byte. If there is no NULL, this reads to the end of r's Data.
func (r *Reader) CString() []byte {
	s := r.d.B[r.p:]
	n := bytes.IndexByte(s, 0)
	if n < 0 {
		r.p = len(r.d.B)
		return s
	}
	r.p += n + 1
	return s[:n]
}

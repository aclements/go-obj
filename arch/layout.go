// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package arch

import (
	"encoding/binary"
	"fmt"
)

// Layout describes the data layout (byte order and word size) of an
// architecture.
type Layout struct {
	// order is 0 for little endian and 1 for big endian. We don't use
	// binary.ByteOrder directly for this because the interface call (and
	// inlining prevention) is costly.
	order    uint8
	wordSize uint8
}

// NewLayout returns a new Layout with the given byte order and word size.
//
// wordSize must be 1, 2, 4, or 8.
func NewLayout(order binary.ByteOrder, wordSize int) Layout {
	var l Layout
	switch order {
	case binary.LittleEndian:
		l.order = 0
	case binary.BigEndian:
		l.order = 1
	default:
		panic(fmt.Errorf("unknown byte order %v", order))
	}
	if wordSize < 1 || wordSize > 8 || (wordSize&(wordSize-1) != 0) {
		panic("word size must be 1, 2, 4, or 8")
	}
	l.wordSize = uint8(wordSize)
	return l
}

// Order returns the byte order of l.
func (l Layout) Order() binary.ByteOrder {
	if l.order == 0 {
		return binary.LittleEndian
	}
	return binary.BigEndian
}

// WordSize returns the word size of l.
func (l Layout) WordSize() int {
	return int(l.wordSize)
}

func (l Layout) Uint16(b []byte) uint16 {
	_ = b[1]
	if l.order == 0 {
		return uint16(b[0]) | uint16(b[1])<<8
	} else {
		return uint16(b[1]) | uint16(b[0])<<8
	}
}

func (l Layout) Int16(b []byte) int16 {
	return int16(l.Uint16(b))
}

func (l Layout) Uint32(b []byte) uint32 {
	_ = b[3]
	if l.order == 0 {
		return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
	} else {
		return uint32(b[3]) | uint32(b[2])<<8 | uint32(b[1])<<16 | uint32(b[0])<<24
	}
}

func (l Layout) Int32(b []byte) int32 {
	return int32(l.Uint32(b))
}

func (l Layout) Uint64(b []byte) uint64 {
	_ = b[7]
	if l.order == 0 {
		return uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24 |
			uint64(b[4])<<32 | uint64(b[5])<<40 | uint64(b[6])<<48 | uint64(b[7])<<56
	} else {
		return uint64(b[7]) | uint64(b[6])<<8 | uint64(b[5])<<16 | uint64(b[4])<<24 |
			uint64(b[3])<<32 | uint64(b[2])<<40 | uint64(b[1])<<48 | uint64(b[0])<<56
	}
}

func (l Layout) Int64(b []byte) int64 {
	return int64(l.Uint64(b))
}

func (l Layout) Word(b []byte) uint64 {
	switch l.wordSize {
	case 8:
		return l.Uint64(b)
	case 4:
		return uint64(l.Uint32(b))
	case 2:
		return uint64(l.Uint16(b))
	}
	return uint64(b[0])
}

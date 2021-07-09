// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package arch

import (
	"encoding/binary"
	"testing"
)

var randomData16K = generateData(16 << 10)

func generateData(size int) []byte {
	out := make([]byte, size)
	for i := 0; i < size; i += 8 {
		for j := 0; j < 8; j++ {
			out[i+j] = byte(i)
		}
	}
	return out
}

func TestLayoutOrder(t *testing.T) {
	data := []byte{0xff, 0xfe, 0xfd, 0xfc, 0xfb, 0xfa, 0xf9, 0xf8}
	check := func(layout Layout, label string, want, got interface{}) {
		t.Helper()
		if want != got {
			t.Errorf("for %s %s: want %v, got %v", layout.Order(), label, want, got)
		}
	}

	l := NewLayout(binary.LittleEndian, 1)
	check(l, "Uint16", l.Uint16(data), uint16(0xfeff))
	check(l, "Uint32", l.Uint32(data), uint32(0xfcfdfeff))
	check(l, "Uint64", l.Uint64(data), uint64(0xf8f9fafbfcfdfeff))
	check(l, "Int16", l.Int16(data), -int16(^uint16(0xfeff)+1))
	check(l, "Int32", l.Int32(data), -int32(^uint32(0xfcfdfeff)+1))
	check(l, "Int64", l.Int64(data), -int64(^uint64(0xf8f9fafbfcfdfeff)+1))

	l = NewLayout(binary.BigEndian, 1)
	check(l, "Uint16", l.Uint16(data), uint16(0xfffe))
	check(l, "Uint32", l.Uint32(data), uint32(0xfffefdfc))
	check(l, "Uint64", l.Uint64(data), uint64(0xfffefdfcfbfaf9f8))
	check(l, "Int16", l.Int16(data), -int16(^uint16(0xfffe)+1))
	check(l, "Int32", l.Int32(data), -int32(^uint32(0xfffefdfc)+1))
	check(l, "Int64", l.Int64(data), -int64(^uint64(0xfffefdfcfbfaf9f8)+1))
}

func TestLayoutWord(t *testing.T) {
	data := []byte{0xff, 0xfe, 0xfd, 0xfc, 0xfb, 0xfa, 0xf9, 0xf8}
	check := func(wordSize int, want uint64) {
		t.Helper()
		l := NewLayout(binary.LittleEndian, wordSize)
		got := l.Word(data)
		if want != got {
			t.Errorf("for word size %d: want %#x, got %#x", wordSize, want, got)
		}
	}
	check(1, 0xff)
	check(2, 0xfeff)
	check(4, 0xfcfdfeff)
	check(8, 0xf8f9fafbfcfdfeff)
}

func BenchmarkOrder(b *testing.B) {
	b.Run("size=16KiB/order=little/bits=32", func(b *testing.B) {
		benchmarkOrder32(b, NewLayout(binary.LittleEndian, 1))
	})
	b.Run("size=16KiB/order=big/bits=32", func(b *testing.B) {
		benchmarkOrder32(b, NewLayout(binary.BigEndian, 1))
	})
	b.Run("size=16KiB/order=little/bits=64", func(b *testing.B) {
		benchmarkOrder64(b, NewLayout(binary.LittleEndian, 1))
	})
	b.Run("size=16KiB/order=big/bits=64", func(b *testing.B) {
		benchmarkOrder64(b, NewLayout(binary.BigEndian, 1))
	})
}

func benchmarkOrder32(b *testing.B, order Layout) {
	data := randomData16K
	for i := 0; i < b.N; i++ {
		var sum uint32
		for off := 0; off < len(data); off += 4 {
			sum += order.Uint32(data[off:])
		}
		if sum != 3351756800 {
			b.Fatalf("bad sum %d", sum)
		}
	}
}

func benchmarkOrder64(b *testing.B, order Layout) {
	data := randomData16K
	for i := 0; i < b.N; i++ {
		var sum uint64
		for off := 0; off < len(data); off += 8 {
			sum += order.Uint64(data[off:])
		}
		if sum != 16421219234243403776 {
			b.Fatalf("bad sum %d", sum)
		}
	}
}

func BenchmarkBinaryOrder(b *testing.B) {
	b.Run("size=16KiB/order=little/bits=32", func(b *testing.B) {
		benchmarkBinaryOrder32(b, binary.LittleEndian)
	})
	b.Run("size=16KiB/order=big/bits=32", func(b *testing.B) {
		benchmarkBinaryOrder32(b, binary.BigEndian)
	})
	b.Run("size=16KiB/order=little/bits=64", func(b *testing.B) {
		benchmarkBinaryOrder64(b, binary.LittleEndian)
	})
	b.Run("size=16KiB/order=big/bits=64", func(b *testing.B) {
		benchmarkBinaryOrder64(b, binary.BigEndian)
	})
}

func benchmarkBinaryOrder32(b *testing.B, order binary.ByteOrder) {
	data := randomData16K
	for i := 0; i < b.N; i++ {
		var sum uint32
		for off := 0; off < len(data); off += 4 {
			sum += order.Uint32(data[off:])
		}
		if sum != 3351756800 {
			b.Fatalf("bad sum %d", sum)
		}
	}
}

func benchmarkBinaryOrder64(b *testing.B, order binary.ByteOrder) {
	data := randomData16K
	for i := 0; i < b.N; i++ {
		var sum uint64
		for off := 0; off < len(data); off += 8 {
			sum += order.Uint64(data[off:])
		}
		if sum != 16421219234243403776 {
			b.Fatalf("bad sum %d", sum)
		}
	}
}

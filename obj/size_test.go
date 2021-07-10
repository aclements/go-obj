// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package obj

import "testing"

func TestSynthesizeSizes(t *testing.T) {
	section := []Section{
		{Addr: 0, Size: 100},
		{Addr: 100, Size: 100},
		{Addr: 1000, Size: 100},
		{Addr: 2000, Size: 100},
	}
	type symTest struct {
		size int // -1 if non-syntheized
		sym  Sym
	}
	test := []symTest{
		{-1, Sym{Section: nil}}, // Non-data
		// Section symbols
		{-1, Sym{Section: &section[1], Kind: SymSection, Value: 100, Size: 100}}, // Has size
		{-1, Sym{Section: &section[1], Kind: SymSection, Value: 200, Size: 0}},   // Value doesn't match base
		{100, Sym{Section: &section[1], Kind: SymSection, Value: 100, Size: 0}},  // Synthesize
		// Data symbols
		{-1, Sym{Section: &section[0], Value: 100, Size: 100}}, // Has size
		{10, Sym{Section: &section[0], Value: 90}},             // To end of section
		{20, Sym{Section: &section[1], Value: 150}},            // To next symbol
		{-1, Sym{Section: &section[1], Value: 170, Size: 1}},
		// Multiple zero-sized symbols at the same address.
		{30, Sym{Section: &section[2], Value: 1000}},
		{30, Sym{Section: &section[2], Value: 1000}},
		{-1, Sym{Section: &section[2], Value: 1000, Size: 10}},
		{-1, Sym{Section: &section[2], Value: 1030, Size: 1}},
		// Symbols outside section.
		{150, Sym{Section: &section[3], Value: 1900}}, // To next symbol
		{50, Sym{Section: &section[3], Value: 2050}},  // Only to end of section
		{-1, Sym{Section: &section[3], Value: 2150}},  // Past end, ignored
	}

	var syms []Sym
	for _, t := range test {
		syms = append(syms, t.sym)
	}
	SynthesizeSizes(syms)

	for i, want := range test {
		got := syms[i]
		if want.size == -1 {
			// Size should be unchanged and it should not be marked
			// synthesized.
			if got.SizeSynthesized() {
				t.Errorf("symbol %d: incorrectly marked synthesized", i)
			} else if want.sym.Size != got.Size {
				t.Errorf("symbol %d: want non-synthetic size %d, got %d", i, want.sym.Size, got.Size)
			}
			continue
		}

		if !got.SizeSynthesized() {
			t.Errorf("symbol %d: incorrectly marked non-synthesized", i)
		} else if uint64(want.size) != got.Size {
			t.Errorf("symbol %d: want synthetic size %d, got %d", i, want.size, got.Size)
		}
	}
}

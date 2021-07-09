// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package obj

import (
	"bytes"
	"log"
	"testing"
)

var local = SymFlags{symFlagLocal}

var symTests = map[string]map[int]Sym{
	"hello-gcc10.3.0-AMD64-dyn": {
		// Text symbol.
		66: {"main", &Section{Name: ".text"}, 0x401136, 38, SymText, SymFlags{}},
		// Data symbol.
		52: {"data_start", &Section{Name: ".data"}, 0x404020, 0, SymData, SymFlags{}},
		// BSS symbol.
		38: {"completed.0", &Section{Name: ".bss"}, 0x404030, 1, SymBSS, local},
		// Undefined dynamic symbol.
		69 + 0: {"puts", nil, 0, 0, SymUndef, SymFlags{}},
		// Test section symbol's name.
		14: {".text", &Section{Name: ".text"}, 0x401050, 0, SymSection, local},
	},
	"hello-gcc10.3.0-I386-dyn": {
		69:     {"main", &Section{Name: ".text"}, 0x8049196, 64, SymText, SymFlags{}},
		73 + 0: {"puts", nil, 0, 0, SymUndef, SymFlags{}},
	},
}

// Value of "main" in hello-gcc10.3.0-AMD64-dyn.
var mainData = parseHex(`f30f1efa554889e54883ec10897dfc488975f0488d3db40e0000e8ebfeffffb800000000c9c3`)

func init() {
	// Attach symTests to elfTests.
	for _, elfTest := range elfTests {
		elfTest.syms = symTests[elfTest.path]
		delete(symTests, elfTest.path)
	}
	for path := range symTests {
		log.Fatalf("symTest[%q] does not match any elfTest", path)
	}
}

func TestElfSyms(t *testing.T) {
	forEachElfTest(t, func(t *testing.T, test *elfTest) {
		if test.nSyms == 0 {
			return
		}
		f := test.openOrSkip(t)

		// Check the count.
		nSyms := f.NumSyms()
		if nSyms != SymID(test.nSyms) {
			t.Errorf("want %d syms, got %d", test.nSyms, nSyms)
		}

		// Iterate over all the symbols just to check for crashes.
		var main Sym
		for i := SymID(0); i < nSyms; i++ {
			sym := f.Sym(i)
			if sym.Name == "main" {
				main = sym
			}
		}

		// Inspect specific symbols.
		for id, want := range test.syms {
			got := f.Sym(SymID(id))
			if !(want.Name == got.Name &&
				(want.Section == nil && got.Section == nil ||
					want.Section.Name == got.Section.Name) &&
				want.Value == got.Value &&
				want.Size == got.Size &&
				want.Kind == got.Kind &&
				want.SymFlags == got.SymFlags) {
				t.Errorf("symbol %d: want %#v, got %#v", id, want, got)
			}
		}

		// Test symbol data.
		if test.path == "hello-gcc10.3.0-AMD64-dyn" {
			data, err := main.Data(main.Bounds())
			if err != nil {
				t.Errorf("symbol main: error getting data: %v", err)
			} else if !bytes.Equal(mainData, data.P) {
				t.Errorf("symbol main: data not as expected")
			}
		}
	})
}

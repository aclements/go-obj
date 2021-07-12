// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package symtab

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/aclements/go-obj/obj"
)

var section1 = &obj.Section{Name: "section1", Addr: 1000, Size: 100} // Mapped
var section2 = &obj.Section{Name: "section2", Addr: 2000, Size: 100} // Mapped
var section3 = &obj.Section{Name: "section3", Addr: 3000, Size: 100} // NOT mapped

func init() {
	section1.SetMapped(true)
	section2.SetMapped(true)
}

func TestAddr(t *testing.T) {
	// Basic address lookup test.
	tab := NewTable([]obj.Sym{
		0: {Section: section1, Value: 1000, Size: 10},
		1: {Section: section1, Value: 1050, Size: 10},
		2: {Section: section2, Value: 2000, Size: 10},
		3: {Section: section3, Value: 3000, Size: 10},
	})
	check := func(label string, section *obj.Section, addr uint64, want obj.SymID) {
		t.Helper()
		got := tab.Addr(section, addr)
		if want != got {
			t.Errorf("%s: looking up (%s, %d) want %d, got %d", label, section, addr, want, got)
		}
	}
	check("beginning of symbol", section1, 1000, 0)
	check("beginning of symbol", section1, 1050, 1)
	check("beginning of symbol", section2, 2000, 2)
	check("beginning of symbol", section3, 3000, 3)

	check("end of symbol", section1, 1009, 0)
	check("end of symbol", section1, 1059, 1)
	check("just past end of symbol", section1, 1010, obj.NoSym)
	check("just past end of symbol", section1, 1060, obj.NoSym)

	check("any mapped section checks all mapped sections", section1, 2000, 2)
	check("nil section checks all mapped sections", nil, 2000, 2)
	check("mapped section does not check unmapped sections", section1, 3000, obj.NoSym)
	check("nil section does not checks unmapped sections", nil, 3000, obj.NoSym)

	check("before first symbol", section1, 100, obj.NoSym)
	check("before first symbol", nil, 100, obj.NoSym)

	sectionUnknown := &obj.Section{Name: "unknown"}
	check("unknown unmapped section", sectionUnknown, 1000, obj.NoSym)
	sectionUnknown.SetMapped(true)
	check("unknown mapped section", sectionUnknown, 1000, 0)
}

func TestName(t *testing.T) {
	var local obj.SymFlags
	local.SetLocal(true)
	tab := NewTable([]obj.Sym{
		0: {Section: section1, Name: "sym0", Value: 1000, Size: 10},
		1: {Section: section1, Name: "sym1", Value: 1001, Size: 0},
		2: {Section: section3, Name: "sym2", Value: 3000, Size: 0},
		3: {Section: section1, Name: "sym3", Value: 1002, Size: 10, SymFlags: local},
	})
	check := func(label string, name string, want obj.SymID) {
		t.Helper()
		got := tab.Name(name)
		if want != got {
			t.Errorf("%s: looking up %s want %d, got %d", label, name, want, got)
		}
	}

	check("mapped symbol with size", "sym0", 0)
	check("mapped symbol without size", "sym1", 1)
	check("unmapped symbol without size", "sym2", 2)
	check("local symbol", "sym3", obj.NoSym)
	check("unknown symbol", "sym100", obj.NoSym)
}

func TestSyms(t *testing.T) {
	syms := []obj.Sym{
		0: {Section: section1, Value: 1000, Size: 10},
		1: {Section: section1, Value: 1010, Size: 10},
	}
	tab := NewTable(syms)
	got := tab.Syms()
	if !reflect.DeepEqual(syms, got) {
		t.Fatalf("want %v, got %v", syms, got)
	}
}

func TestOverlap(t *testing.T) {
	const minAddr = 1000
	syms := []obj.Sym{
		// Strictly nested.
		{Value: 1000, Size: 3},
		{Value: 1001, Size: 1},
		// Same beginning. Smaller symbols should be preferred.
		{Value: 1010, Size: 5},
		{Value: 1010, Size: 4},
		{Value: 1010, Size: 3},
		// Same end.
		{Value: 1020, Size: 5},
		{Value: 1021, Size: 4},
		{Value: 1022, Size: 3},
		// Overlap in the middle with same size. Earlier symbol should be preferred.
		{Value: 1030, Size: 5},
		{Value: 1032, Size: 5},
		// Nested abutting symbols.
		{Value: 1040, Size: 5},
		{Value: 1041, Size: 1},
		{Value: 1042, Size: 1},
		// Same end nested in another symbol.
		{Value: 1050, Size: 5},
		{Value: 1051, Size: 2},
		{Value: 1052, Size: 1},
		// Totally overlapping. Lower SymIDs should be preferred.
		{Value: 1060, Size: 1},
		{Value: 1060, Size: 1},
	}
	const maxAddr = 1070
	for i := range syms {
		syms[i].Section = section1
		syms[i].Name = fmt.Sprintf("sym%d", i)
	}

	// For this test, we compare against a brute-force reference
	// implementation.
	prefer := func(a, b obj.SymID) bool {
		sa, sb := &syms[a], &syms[b]
		if sa.Value != sb.Value {
			return sa.Value > sb.Value
		}
		if sa.Size != sb.Size {
			return sa.Size < sb.Size
		}
		return a < b
	}
	slow := func(addr uint64) obj.SymID {
		best := obj.NoSym
		for i := range syms {
			i := obj.SymID(i)
			if syms[i].Value <= addr && addr < syms[i].Value+syms[i].Size {
				// Candidate.
				if best == obj.NoSym || prefer(i, best) {
					best = i
				}
			}
		}
		return best
	}

	tab := NewTable(syms)
	for addr := uint64(minAddr); addr < maxAddr; addr++ {
		want := slow(addr)
		got := tab.Addr(nil, addr)
		if want != got {
			t.Errorf("at address %d: want symbol %s, got %s", addr, want, got)
		}
	}
}

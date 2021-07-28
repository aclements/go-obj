// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dbg

import (
	"debug/dwarf"
	"debug/elf"
	"testing"
)

func open(t *testing.T, path string) *Data {
	f, err := elf.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	dw, err := f.DWARF()
	if err != nil {
		t.Fatal(err)
	}

	d, err := New(dw)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestAddrToCU(t *testing.T) {
	d := open(t, "testdata/inline")
	check := func(addr uint64, name string) {
		t.Helper()
		gotCU, ok := d.AddrToCU(addr)
		if !ok {
			t.Errorf("no CU found at %#x", addr)
			return
		}
		got := gotCU.Val(dwarf.AttrName).(string)
		if name != got {
			t.Errorf("CU at %#x: want name %s, got %s", addr, name, got)
		}
	}
	checkNone := func(addr uint64) {
		t.Helper()
		_, ok := d.AddrToCU(addr)
		if ok {
			t.Errorf("unexpectedly found CU at %#x", addr)
		}
	}
	check(0x1170, "inline.c")
	check(0x1170+0x68-1, "inline.c")
	checkNone(0x1170 + 0x68)
	check(0x1060, "inline.c")
	check(0x1060+0x14-1, "inline.c")
	checkNone(0x1060 + 0x14)
	check(0x11e0, "inline2.c")
	check(0x11e0+0x19-1, "inline2.c")
	checkNone(0x11e0 + 0x19)
}

func TestAddrToSubprogram(t *testing.T) {
	d := open(t, "testdata/inline")
	check := func(addr uint64, name string) {
		t.Helper()
		gotSubprogram, ok := d.AddrToSubprogram(addr, CU{})
		if !ok {
			t.Errorf("no subprogram found at %#x", addr)
			return
		}
		if gotSubprogram.Tag != dwarf.TagSubprogram {
			t.Errorf("subprogram at %#x has tag %v", addr, gotSubprogram.Tag)
		}
		got := gotSubprogram.Val(dwarf.AttrName).(string)
		if name != got {
			t.Errorf("subprogram at %#x: want name %s, got %s", addr, name, got)
		}
	}
	checkNone := func(addr uint64) {
		t.Helper()
		_, ok := d.AddrToSubprogram(addr, CU{})
		if ok {
			t.Errorf("unexpectedly found subprogram at %#x", addr)
		}
	}

	// First CU.
	check(0x1060, "main")
	check(0x1060+0x14-1, "main")
	check(0x11a0, "funcA")

	// Second CU.
	check(0x11e0, "print")

	// Outside any CU.
	checkNone(0xffff)

	// In a CU, but between funcC and funcB.
	checkNone(0x1170 + 8)
}

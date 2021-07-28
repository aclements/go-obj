// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dbg

import (
	"debug/dwarf"
	"fmt"
	"strings"
	"testing"
)

func TestInlineRanges(t *testing.T) {
	d := open(t, "testdata/inline")
	sub, _ := d.AddrToSubprogram(0x11a0, CU{}) // funcA
	ir := d.inlineRanges(sub)

	var got strings.Builder
	for it := ir.Iter(0); it.Valid(); it.Next() {
		fmt.Fprintln(&got, it.Key(), inlineString(it.Value().(*InlineSite), d))
	}

	// Expected ranges checked with `addr2line -Cfipe inline`
	want := `[0x11a0,0x11a8) funcA
[0x11a8,0x11b2) funcC /inline.c:10:13 funcB /inline.c:15:13 funcA
[0x11b2,0x11b7) funcB /inline.c:15:13 funcA
[0x11b7,0x11be) funcA
[0x11be,0x11c8) funcC /inline.c:10:13 funcB /inline.c:17:13 funcA
[0x11c8,0x11cd) funcB /inline.c:17:13 funcA
[0x11cd,0x11d8) funcA
`
	if want != got.String() {
		t.Fatalf("want:\n%sgot:\n%s", want, got.String())
	}
}

func inlineString(i *InlineSite, d *Data) string {
	dr := d.dw.Reader()
	var buf strings.Builder
	for i != nil {
		// Resolve name. It may be on this entry or its abstract origin.
		name, ok := i.Entry.Val(dwarf.AttrName).(string)
		if !ok {
			if ao, ok := i.Entry.Val(dwarf.AttrAbstractOrigin).(dwarf.Offset); ok {
				dr.Seek(ao)
				if ent, err := dr.Next(); err == nil {
					name, _ = ent.Val(dwarf.AttrName).(string)
				}
			}
		}

		buf.WriteString(name)
		if i.Caller != nil {
			fmt.Fprintf(&buf, " %s:%d:%d ", i.CallFile.Name, i.CallLine, i.CallColumn)
		}
		i = i.Caller
	}
	return buf.String()
}

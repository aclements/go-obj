// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dbg

import (
	"debug/dwarf"
	"fmt"
	"io"
	"math/rand"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func fprintLine(w io.Writer, r *LineReader) {
	if r.Line.EndSequence {
		fmt.Fprintf(w, "%#x end\n", r.Line.Address)
		return
	}
	fileName := "?"
	if r.Line.File != nil {
		fileName = r.Line.File.Name
	}
	fmt.Fprintf(w, "%#x %s:%d:%d %s\n", r.Line.Address, fileName, r.Line.Line, r.Line.Column, inlineString(r.Stack, r.d))
}

func TestLinesAllNext(t *testing.T) {
	// This binary has some interesting properties. inline.c's CU has
	// two discontiguous ranges, and the line table sequences are
	// actually out of address order.
	d := open(t, "testdata/inline")
	r := d.LineReader()
	if err := r.SeekPC(0); err != nil {
		t.Fatal(err)
	}

	// Record function boundaries.
	var fn *dwarf.Entry
	var got strings.Builder
	for {
		var lineFn *dwarf.Entry
		for i := r.Stack; i != nil; i = i.Caller {
			lineFn = i.Entry
		}
		if fn != lineFn || r.Line.EndSequence {
			fprintLine(&got, r)
			fn = lineFn
		}

		if err := r.Next(); err != nil {
			if err == io.EOF {
				break
			}
			t.Fatal(err)
		}
	}

	// This was checked against readelf --debug-dump=rawline.
	const want = `0x1060 /inline.c:21:33 main
0x1074 end
0x1170 /inline.c:5:20 funcC
0x1180 /inline.c:9:20 funcB
0x11a0 /inline.c:14:18 funcA
0x11d8 end
0x11e0 /inline2.c:3:19 print
0x11f9 end
`
	if want != got.String() {
		t.Fatalf("want:\n%sgot:\n%s", want, got.String())
	}
}

func TestLinesSubprogramNext(t *testing.T) {
	d := open(t, "testdata/inline")
	sub, _ := d.AddrToSubprogram(0x11a0, CU{}) // funcA
	r := d.LineReader()
	if err := r.SeekSubprogram(sub, 0); err != nil {
		t.Fatal(err)
	}
	var got strings.Builder
	for {
		fprintLine(&got, r)
		if err := r.Next(); err != nil {
			if err == io.EOF {
				break
			}
			t.Fatal(err)
		}
	}

	// This was checked against readelf and addr2line -Cfipe
	const want = `0x11a0 /inline.c:14:18 funcA
0x11a4 /inline.c:15:5 funcA
0x11a4 /inline.c:9:5 funcA
0x11a4 /inline.c:10:5 funcA
0x11a4 /inline.c:5:5 funcA
0x11a4 /inline.c:6:5 funcA
0x11a4 /inline.c:6:5 funcA
0x11a4 /inline.c:11:5 funcA
0x11a4 /inline.c:14:18 funcA
0x11a8 /inline.c:6:16 funcC /inline.c:10:13 funcB /inline.c:15:13 funcA
0x11b2 /inline.c:11:5 funcB /inline.c:15:13 funcA
0x11b7 /inline.c:11:5 funcA
0x11b7 /inline.c:16:5 funcA
0x11be /inline.c:17:5 funcC /inline.c:10:13 funcB /inline.c:17:13 funcA
0x11be /inline.c:9:5 funcC /inline.c:10:13 funcB /inline.c:17:13 funcA
0x11be /inline.c:10:5 funcC /inline.c:10:13 funcB /inline.c:17:13 funcA
0x11be /inline.c:5:5 funcC /inline.c:10:13 funcB /inline.c:17:13 funcA
0x11be /inline.c:6:5 funcC /inline.c:10:13 funcB /inline.c:17:13 funcA
0x11be /inline.c:6:5 funcC /inline.c:10:13 funcB /inline.c:17:13 funcA
0x11be /inline.c:11:5 funcC /inline.c:10:13 funcB /inline.c:17:13 funcA
0x11be /inline.c:6:16 funcC /inline.c:10:13 funcB /inline.c:17:13 funcA
0x11c8 /inline.c:6:16 funcB /inline.c:17:13 funcA
0x11c8 /inline.c:11:5 funcB /inline.c:17:13 funcA
0x11cd /inline.c:11:5 funcA
0x11cd /inline.c:18:5 funcA
0x11cf /inline.c:19:1 funcA
0x11d3 /inline.c:18:5 funcA
0x11d8 end
`
	if want != got.String() {
		t.Fatalf("want:\n%sgot:\n%s", want, got.String())
	}
}

type testLine struct {
	line  dwarf.LineEntry
	stack *InlineSite
}

type testLines []testLine

func newTestLines(t *testing.T, r *LineReader) testLines {
	var lines []testLine
	for {
		lines = append(lines, testLine{r.Line, r.Stack})
		if err := r.Next(); err != nil {
			if err == io.EOF {
				break
			}
			t.Fatal(err)
		}
	}
	return testLines(lines)
}

func (l testLines) find(pc uint64) (testLine, bool) {
	n := sort.Search(len(l), func(i int) bool {
		return pc < l[i].line.Address
	}) - 1
	if n < 0 {
		n = 0
	}
	if l[n].line.EndSequence {
		// pc isn't a "valid" address, so move on to the next valid
		// address.
		n++
	}
	if n >= len(l) {
		return testLine{}, false
	}
	return l[n], true
}

func checkSeek(t *testing.T, r *LineReader, pc uint64, lines testLines, gotErr error) {
	t.Helper()
	want, wantOK := lines.find(pc)
	if !wantOK {
		if gotErr != dwarf.ErrUnknownPC {
			t.Errorf("seeking to %#x: want ErrUnknownPC, got error %v", pc, gotErr)
		}
		return
	}
	if gotErr != nil {
		t.Errorf("seeking to %#x failed: %v", pc, gotErr)
		return
	}
	if !reflect.DeepEqual(want.line, r.Line) {
		t.Errorf("seeking to %#x: want line %+v, got %+v", pc, want.line, r.Line)
	} else if want.stack != r.Stack {
		t.Errorf("seeking to %#x: want stack %v, got %v", pc, inlineString(want.stack, r.d), inlineString(r.Stack, r.d))
	}
}

func TestLineAllSeek(t *testing.T) {
	// Iterate over all the lines, then seek randomly and check that we
	// get the right results. This assumes iterating over everything is
	// correct, but TestLineAllNext checked that.
	d := open(t, "testdata/inline")
	r := d.LineReader()
	if err := r.SeekPC(0); err != nil {
		t.Fatal(err)
	}

	lines := newTestLines(t, r)
	lo, hi := int64(lines[0].line.Address-10), int64(lines[len(lines)-1].line.Address+10)
	for i := 0; i < 1000; i++ {
		pc := uint64(rand.Int63n(hi-lo) + lo)
		err := r.SeekPC(pc)
		checkSeek(t, r, pc, lines, err)
	}
}

func TestLineSubprogramSeek(t *testing.T) {
	// Iterate over all the lines, then seek randomly and check that we
	// get the right results.
	d := open(t, "testdata/inline")
	r := d.LineReader()
	sub, _ := d.AddrToSubprogram(0x11a0, CU{}) // funcA
	if err := r.SeekSubprogram(sub, 0); err != nil {
		t.Fatal(err)
	}

	lines := newTestLines(t, r)
	lo, hi := int64(lines[0].line.Address-10), int64(lines[len(lines)-1].line.Address+10)
	for i := 0; i < 1000; i++ {
		pc := uint64(rand.Int63n(hi-lo) + lo)
		err := r.SeekSubprogram(sub, pc)
		checkSeek(t, r, pc, lines, err)
	}
}

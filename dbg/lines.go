// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dbg

import (
	"debug/dwarf"
	"fmt"
	"io"
	"sort"
	"sync"

	"github.com/aclements/go-obj/internal/imap"
)

// LineReader maps from PCs to source line information, including stacks
// of inlined functions.
//
// It is efficient both for single-PC lookups (using SeekPC or
// SeekSubprogram) and for iterating over ranges of addresses (using
// Seek* followed by Next).
//
// Iteration is always in order of increasing PC. The reader can be
// scoped to the entire binary, in which case it will iterate forward
// over all PCs, jumping between compilation units as necessary; or it
// can be scoped to a single subprogram DIE, in which case it will
// iterate only over PCs within that function (which may be
// discontiguous).
type LineReader struct {
	// The implementation of LineReader is shockingly complex and
	// requires many layers of caching to be efficient. Line tables are
	// attached to compilation units. Each compilation unit can cover
	// multiple discontiguous address ranges. Each line table can
	// consist of multiple sequences. Within a sequence, the entries
	// must be in increasing address order, but the sequences themselves
	// don't have to be in order, so we have to read the entire table to
	// figure out if it covers a PC. Subprograms exist within
	// compilation units, and can themselves cover discontiguous address
	// ranges. Inlining information isn't in the line tables, but rather
	// attached in the DIE tree of each subprogram.
	//
	// The general approach is to cache at every articulation point in
	// this mess, and to validate each cache before you use it.

	d *Data

	// dlr is the line reader for the current CU. It is positioned just after Line.
	dlr *dwarf.LineReader
	// cu is the CU of dlr, used to determine whether we need to
	// construct a new LineReader when seeking to a new PC.
	cu CU

	// ranges is the set of PC ranges this reader is scoped to. The
	// values are CUs.
	ranges *entryMap
	// riter is positioned on the current PC range in ranges.
	rIter imap.Iter

	// subprogram is the TagSubprogram this reader is scoped to, or nil
	// if this reader is not scoped.
	subprogram *dwarf.Entry

	// inlineRanges is the PC -> *InlineSite cache for the current
	// subprogram. This is populated even if subprogram == nil.
	inlineRanges imap.Imap
	// stackValid is the PC interval from inlineRanges for which Stack
	// is valid.
	stackValid imap.Interval

	// Line is the line metadata for the current position of the
	// LineReader. If the current PC is inside an inlined function, this
	// will be the line metadata for the innermost frame.
	Line dwarf.LineEntry

	// Stack is the inline call stack at Line.Address, starting with the
	// innermost frame.
	//
	// This may be nil if the current inlining stack cannot be
	// determined.
	Stack *InlineSite
}

type lineTableCache struct {
	once sync.Once
	err  error

	// In order to bound the lookup cost in the line table, we cache the
	// line table state every Nth row, so we can just seek to the
	// closest ancestor and decode at most N rows.
	waypoints []lineTableWaypoint

	// files is this CU's file table after the entire line table has
	// been read.
	files []*dwarf.LineFile
}

const lineCacheFreq = 32

type lineTableWaypoint struct {
	pc  uint64
	pos dwarf.LineReaderPos
}

// ensure populates lineTableCache if necessary. lr must be a line
// reader positioned at the beginning of the CU's line table.
func (lc *lineTableCache) ensure(dw *dwarf.Data, cu CU) (*lineTableCache, error) {
	lc.once.Do(func() {
		lr, err := dw.LineReader(cu.Entry)
		if err != nil {
			lc.err = fmt.Errorf("decoding line table header: %w", err)
			return
		}

		var line dwarf.LineEntry
		var pos dwarf.LineReaderPos
		for i := 0; ; i++ {
			save := i%lineCacheFreq == 0
			if save {
				// Save this waypoint.
				pos = lr.Tell()
			}

			if err := lr.Next(&line); err != nil {
				if err == io.EOF {
					break
				}
				lc.err = err
				return
			}

			if line.EndSequence {
				// We never place a waypoint on EndSequence because its
				// address isn't meaningfully seekable (it *closes* an
				// interval). However, we always want to place a
				// waypoint at the beginning of the next sequence.  A
				// line table can consistent of multiple sequences. Each
				// sequence needs to be in address order, but the
				// sequences don't have to be.
				i = -1
			} else if save {
				lc.waypoints = append(lc.waypoints, lineTableWaypoint{line.Address, pos})
			}
		}

		// Sort the waypoints since the sequences may be out of order.
		sort.Slice(lc.waypoints, func(i, j int) bool {
			return lc.waypoints[i].pc < lc.waypoints[j].pc
		})

		// We've read the whole table, so we can get the full file list.
		lc.files = lr.Files()
	})
	if lc.err != nil {
		return nil, lc.err
	}
	return lc, nil
}

// LineReader returns a new unpositioned line table reader. The caller
// must call one of the Seek methods to position the line table reader
// before using it.
func (d *Data) LineReader() *LineReader {
	return &LineReader{d: d, Line: dwarf.LineEntry{EndSequence: true}}
}

// SeekSubprogram positions the line table reader at the line entry
// containing pc within subprogram, or the first entry in subprogram
// after pc, and scopes the line table reader to iterate only within
// subprogram. A subprogram may consist of multiple discontiguous
// address ranges. If there are no valid addresses after pc in
// subprogram, it returns dwarf.ErrUnknownPC. To seek to the beginning
// of subprogram, pass 0 for pc.
func (lr *LineReader) SeekSubprogram(subprogram Subprogram, pc uint64) error {
	if subprogram.Entry == lr.subprogram {
		// Seeking within the current subprogram, so there's no need to
		// get the ranges again.
		return lr.seek(lr.ranges, pc, subprogram.Entry)
	}

	// Get subprogram's ranges and create a map where all values are set
	// to the CU (since they're all the same value, this will also merge
	// ranges if necessary).
	ranges, err := lr.d.dw.Ranges(subprogram.Entry)
	if err != nil {
		return fmt.Errorf("decoding subprogram ranges: %w", err)
	}
	var m entryMap
	for _, r := range ranges {
		m.m.Insert(imap.Interval{Low: r[0], High: r[1]}, subprogram.CU.Entry)
	}
	return lr.seek(&m, pc, subprogram.Entry)
}

// SeekPC positions the line table reader at the line entry containing
// pc, or the first entry after pc, and scopes the line table reader to
// iterate over all code in all compilation units. If there are no valid
// addresses after pc, it returns dwarf.ErrUnknownPC.
//
// Each line entry covers the address range from Line.Address up to but
// not including the address of the next line entry. Line entries that
// have EndSequence do not cover any addresses, but merely provide the
// end address for the previous line entry.
func (lr *LineReader) SeekPC(pc uint64) error {
	// Use all CUs as the PC ranges to iterate over.
	return lr.seek(&lr.d.cuRanges, pc, nil)
}

// Next advances to the next entry in the line table for the reader's
// scope and updates lr.Line and lr.Stack. If there are no more entries,
// it returns io.EOF.
//
// Rows are always in increasing order of Line.Address, but Line.Line
// may go forward or backward. Line.Address may be the same for entries.
//
// In contrast with the underlying DWARF line tables, this will iterate
// across compilation units as necessary, and ensures increasing order
// of address.
func (lr *LineReader) Next() error {
	if lr.Line.EndSequence {
		// It never makes sense to Next past an EndSequence because
		// sequences aren't necessarily in address order.
		err := lr.seek(lr.ranges, lr.Line.Address, lr.subprogram)
		// Seeking to the EndSequence's own address should get us to the
		// next sequence, but just in case it didn't, seek one more byte
		// to ensure progress. I don't think this should ever happen.
		if err == nil && lr.Line.EndSequence {
			err = lr.seek(lr.ranges, lr.Line.Address+1, lr.subprogram)
		}
		if err == dwarf.ErrUnknownPC {
			// We went past the end. Turn this into an EOF.
			err = io.EOF
		}
		return err
	}

	// We're still in the sequence. Advance the line table reader.
	endAddress := lr.Line.Address + 1
	err := lr.dlr.Next(&lr.Line)
	if err == nil {
		// Check if we're still within scope.
		pcRange := lr.rIter.Key()
		if lr.Line.Address < pcRange.High {
			// All good.
			lr.updateStack()
			return nil
		}
		// Act like we reached the end.
		endAddress = pcRange.High
	} else if err != io.EOF {
		return fmt.Errorf("reading next record from line table: %w", err)
	}

	// We reached the end of a table without an EndSequence.
	// Synthesize an EndSequence. The next call to Next will see
	// this and seek to the next range.
	lr.Line = dwarf.LineEntry{Address: endAddress, EndSequence: true}
	lr.updateStack()
	return nil
}

func (lr *LineReader) seek(ranges *entryMap, pc uint64, subprogram *dwarf.Entry) error {
	lr.ranges = ranges
	lr.subprogram = subprogram

	if !lr.rIter.Valid() || !lr.rIter.Key().Contains(pc) {
		// Find the range containing pc.
		lr.rIter = ranges.m.Iter(pc)
		if !lr.rIter.Valid() {
			return dwarf.ErrUnknownPC
		}
		// Round pc up to the beginning of the range we found, since it
		// may be before it.
		if low := lr.rIter.Key().Low; pc < low {
			pc = low
		}
	}

	cu := CU{lr.rIter.Value().(*dwarf.Entry)}
	cuData := lr.d.cus[cu]

	if cu != lr.cu {
		lr.cu = cu
		// Construct a new DWARF line reader.
		var err error
		lr.dlr, err = lr.d.dw.LineReader(cu.Entry)
		if err != nil {
			return fmt.Errorf("decoding line table header: %w", err)
		}
	}

	// Get CU's line table cache.
	ltc, err := cuData.lineTable.ensure(lr.d.dw, cu)
	if err != nil {
		return err
	}

	// Search the line table cache.
	n := sort.Search(len(ltc.waypoints), func(i int) bool {
		return ltc.waypoints[i].pc > pc
	}) - 1
	if n < 0 {
		n = 0
	}

	// Position the line reader. SeekPC (as of Go 1.16) doesn't handle
	// out-of-order sequences, but that's okay because the waypoints
	// ensure we seek to the right sequence.
	lr.dlr.Seek(ltc.waypoints[n].pos)
	if err := lr.dlr.SeekPC(pc, &lr.Line); err != nil {
		return fmt.Errorf("seeking in line table: %w", err)
	}

	lr.updateStack()

	return nil
}

func (lr *LineReader) updateStack() {
	if lr.Line.EndSequence {
		// lr.Line.Address isn't logically in a function.
		lr.clearStack()
		return
	}

	// Are we in the current stack-valid range?
	if lr.stackValid.Contains(lr.Line.Address) {
		return
	}

	// Try looking up the PC in the current inline map.
	stackValid, inl1 := lr.inlineRanges.Find(lr.Line.Address)
	if inl1 == nil {
		// We must have moved out of lr.inlineRanges' subprogram. Find
		// the current subprogram and its inline ranges.
		subprogram := Subprogram{lr.subprogram, lr.cu}
		if subprogram.Entry == nil {
			s, ok := lr.d.AddrToSubprogram(lr.Line.Address, lr.cu)
			if !ok {
				lr.clearStack()
				return
			}
			subprogram = s
		}
		lr.inlineRanges = lr.d.inlineRanges(subprogram)
		stackValid, inl1 = lr.inlineRanges.Find(lr.Line.Address)
		if inl1 == nil {
			// Nope, we still have no idea.
			lr.clearStack()
			return
		}
	}

	lr.stackValid, lr.Stack = stackValid, inl1.(*InlineSite)
}

func (lr *LineReader) clearStack() {
	lr.Stack = nil
	lr.stackValid = imap.Interval{}
}

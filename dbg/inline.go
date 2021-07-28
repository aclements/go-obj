// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dbg

import (
	"debug/dwarf"

	"github.com/aclements/go-obj/internal/imap"
)

// InlineSite describes a call site at which a function is inlined.
//
// Inline sites form a tree within a given top-level function.
// InlineSite objects are immutable once created.
type InlineSite struct {
	// Entry is the TagSubprogram or TagInlinedSubroutine DWARF entry of
	// the function that was inlined at this site.
	Entry *dwarf.Entry

	// Caller is the frame at which this inlined function was called, or
	// nil if this is the top of the inlining stack.
	Caller *InlineSite

	// CallLine, CallColumn, and CallFile give the location at which
	// Entry was inlined into Outer. These are the zero value if Outer
	// == nil or if the call site is unknown.
	CallLine, CallColumn int
	CallFile             *dwarf.LineFile
}

// inlineRanges returns a map from PCs within subprogram to the inlining
// hierarchy at each PC. The values in the returned map are of type
// *inline and give the innermost TagSubprogram or TagInlinedSubroutine
// at that PC.
func (d *Data) inlineRanges(subprogram Subprogram) imap.Imap /*[*InlineSite]*/ {
	// Check the cache.
	if m, ok := d.inlineRangesCache.Load(subprogram.Entry); ok {
		return m.(imap.Imap)
	}

	// Fetch the file table for this CU.
	cuData := d.cus[subprogram.CU]
	ltc, err := cuData.lineTable.ensure(d.dw, subprogram.CU)
	if err != nil {
		return imap.Imap{}
	}

	dr := d.dw.Reader()
	dr.Seek(subprogram.Offset)

	var m imap.Imap
	var stack []*InlineSite
	var outer *InlineSite
	for {
		// Consume the next entity and its children.
		ent, err := dr.Next()
		if err != nil || ent == nil {
			break
		}
		if ent.Tag == 0 {
			if len(stack) > 0 {
				if stack[len(stack)-1] != nil {
					outer = stack[len(stack)-1].Caller
				}
				stack = stack[:len(stack)-1]
			}
		}

		if outer == nil && ent.Tag == dwarf.TagSubprogram ||
			ent.Tag == dwarf.TagInlinedSubroutine {
			// Construct the inline record.
			line, _ := ent.Val(dwarf.AttrCallLine).(int64)
			col, _ := ent.Val(dwarf.AttrCallColumn).(int64)
			inner := &InlineSite{Caller: outer, Entry: ent, CallLine: int(line), CallColumn: int(col)}
			callFile, _ := ent.Val(dwarf.AttrCallFile).(int64)
			if callFile > 0 && callFile < int64(len(ltc.files)) {
				inner.CallFile = ltc.files[callFile]
			}
			stack = append(stack, inner)
			// Add it to the map. (Ignore errors.)
			addRanges(&m, d.dw, ent, inner)
			outer = inner
			continue
		} else if outer != nil && ent.Tag == dwarf.TagSubprogram {
			// If there are nested functions, don't look into them.
			dr.SkipChildren()
			continue
		}

		// Enter this entry. We enter everything because
		// TagInlinedSubroutines can appear in surprising places. For
		// example, it can be nested in TagLexicalBlock.
		if ent.Children {
			stack = append(stack, nil)
		}

		if outer == nil {
			// We've popped back to the top level.
			break
		}
	}

	// Update the cache.
	if m, ok := d.inlineRangesCache.LoadOrStore(subprogram.Entry, m); ok {
		// Someone beat us to it. Use the cached copy.
		return m.(imap.Imap)
	}

	return m
}

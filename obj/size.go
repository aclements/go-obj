// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package obj

import "sort"

// SynthesizeSizes assigns sizes to syms that don't have sizes using
// heuristics.
func SynthesizeSizes(syms []Sym) {
	// Gather symbols with data and sort by section then address
	// (without destroying order).
	todo := []int{}
	for i := range syms {
		if syms[i].Section == nil {
			// Only assign sizes to symbols with data.
			continue
		}
		if syms[i].Kind == SymSection {
			if syms[i].Value == syms[i].Section.Addr && syms[i].Size == 0 {
				syms[i].Size = syms[i].Section.Size
				syms[i].SetSizeSynthesized(true)
			}
			continue
		}
		// If the symbol is past the end of its section, leave it out
		// because we can't give it a meaningful address and it may
		// throw off earlier symbols in the section.
		if syms[i].Value > syms[i].Section.Addr+syms[i].Section.Size {
			continue
		}
		todo = append(todo, i)
	}
	sort.Slice(todo, func(i, j int) bool {
		si, sj := &syms[todo[i]], &syms[todo[j]]
		if si.Section != sj.Section {
			return si.Section.ID < sj.Section.ID
		}
		return si.Value < sj.Value
	})

	// Assign addresses to zero-sized symbols within each section.
	for len(todo) != 0 {
		// Collect symbols that have the same value and
		// section. Most of the time we'll get groups of 1,
		// but sometimes there are multiple names for the same
		// address (especially in shared objects).
		s1 := &syms[todo[0]]
		group := 1
		anyZero := s1.Size == 0
		for group < len(todo) {
			s2 := &syms[todo[group]]
			if s1.Value != s2.Value || s1.Section != s2.Section {
				break
			}
			if s1.Size == 0 {
				anyZero = true
			}
			group++
		}
		if !anyZero {
			// They all have sizes. Move on.
			todo = todo[group:]
			continue
		}

		// Compute the size of these symbols.
		var size uint64
		// Cap symbols at the end of the section.

		if group == len(todo) || s1.Section != syms[todo[group]].Section {
			// Cap the symbols at the end of the section.
			size = s1.Section.Addr + s1.Section.Size - s1.Value
		} else {
			size = syms[todo[group]].Value - s1.Value
		}

		// Apply this size to all zero-sized symbols in this group.
		for _, symi := range todo[:group] {
			if syms[symi].Size == 0 {
				syms[symi].Size = size
				syms[symi].SetSizeSynthesized(true)
			}
		}
		todo = todo[group:]
	}
}

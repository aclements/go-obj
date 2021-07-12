// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package symtab implements symbol table lookup by name and address.
package symtab

import (
	"sort"

	"github.com/aclements/go-obj/obj"
)

// Table facilitates fast symbol lookup by name and address.
type Table struct {
	// syms is the original syms slice, by SymID
	syms []obj.Sym

	// sections contains the address to symbol mapping for each section.
	// Mappable sections are all indexed under the nil key.
	sections map[*obj.Section]sectionTable

	// name indexes non-local symbols by name.
	name map[string]obj.SymID
}

type sectionTable struct {
	// addr contains boundaries of symbols in Table.syms, ordered by
	// address. The boundary from symbol to NoSym is not explicitly
	// represented, since lookup can check the size of the symbol.
	//
	// If symbols overlap, this may contain the same symbol multiple
	// times. E.g., given one symbol strictly nested in another, the
	// outer symbol will appear both at its beginning address and at the
	// end address of the inner symbol.
	addr []symAddr
}

type symAddr struct {
	// addr is the address of this symbol boundary. Usually this is
	// beginning of the symbol, except in the case of overlapping
	// symbols.
	addr uint64
	id   obj.SymID
}

// NewTable creates a new table for syms. syms must be indexed by obj.SymID.
//
// NewTable uses sizes as they appear in syms, so the caller may wish to
// first call obj.SynthesizeSizes.
func NewTable(syms []obj.Sym) *Table {
	// Index symbols by name and break them up by section for address
	// indexing.
	name := make(map[string]obj.SymID)
	sectionSyms := map[*obj.Section][]obj.SymID{nil: {}}
	for i, s := range syms {
		if !s.Local() {
			name[s.Name] = obj.SymID(i)
		}
		// Add symbols that have data to the address list. We omit
		// symbols of size 0 because they can't be the result of a
		// lookup and mess up the algorithm that computes the index.
		if s.Section != nil && s.Size != 0 {
			section := s.Section
			if section.Mapped() {
				// All mapped sections are indexed undef "nil".
				section = nil
			}
			sectionSyms[section] = append(sectionSyms[section], obj.SymID(i))
		}
	}

	// Create each section tables.
	sections := make(map[*obj.Section]sectionTable)
	for section, symIDs := range sectionSyms {
		sections[section] = sectionTable{makeAddrIndex(syms, symIDs)}
	}

	return &Table{syms, sections, name}
}

func makeAddrIndex(syms []obj.Sym, ids []obj.SymID) []symAddr {
	// Sort by starting address then priority, with low priority symbols
	// before higher priority so the higher priority ones override the
	// lower priority as we loop over the slice.
	sort.Slice(ids, func(i, j int) bool {
		si, sj := &syms[ids[i]], &syms[ids[j]]

		// Sort by symbol address.
		if si.Value != sj.Value {
			return si.Value < sj.Value
		}

		// Then size, preferring smaller symbols.
		if si.Size != sj.Size {
			return si.Size > sj.Size
		}

		// Then by index, which is gauranteed to be unique. This is
		// particularly important when there are multiple symbol tables,
		// such as in ELF files that have both static and dynamic
		// tables. This will prefer static symbols.
		return ids[i] > ids[j]
	})

	// Create the address index. This would be trivial except that
	// symbols can and do overlap. See Addr for the rules of
	// disambiguation. We iterate through each symbol *boundary*
	// (beginning and end) and keep a stack of symbols at the current
	// address (lowest end address at top of stack). Typically this
	// stack will be very shallow, so we don't bother with more
	// sophisticated data structures.
	var out []symAddr
	stack := make([]symAddr, 0, 8) // addr is *end* address
	drainStack := func(addr uint64) {
		for len(stack) > 0 {
			// Do any symbols end before addr?
			endAddr := stack[len(stack)-1].addr
			if endAddr > addr {
				// No, nothing to do.
				return
			}
			// Pop all of the symbols that end at the next boundary.
			// There may be more than one.
			for len(stack) > 0 && stack[len(stack)-1].addr == endAddr {
				stack = stack[:len(stack)-1]
			}
			// At endAddr, we drop to the symbol at top of stack. If the
			// stack is empty now, we drop to NoSym, which doesn't have
			// an explicit marker.
			if len(stack) > 0 {
				out = append(out, symAddr{endAddr, stack[len(stack)-1].id})
			}
		}
	}
	for _, id := range ids {
		// Drain symbols that end before this symbol starts. Usually
		// there's just one symbol in the stack and it ends before this
		// symbol so we optimize for that case.
		sym := syms[id]
		if len(stack) == 1 { // XXX
			if stack[0].addr <= sym.Value {
				// Pop the symbol. No boundary because we're returning
				// to NoSym.
				stack = stack[:0]
			}
		} else if len(stack) > 0 {
			drainStack(sym.Value)
		}
		// Transition to sym at sym.Value.
		start := symAddr{sym.Value, id}
		if len(out) > 0 && out[len(out)-1].addr == sym.Value {
			// Replace the last boundary.
			out[len(out)-1] = start
		} else {
			out = append(out, start)
		}
		// Add symbol to the stack, keeping it ordered by end address.
		stack = append(stack, symAddr{sym.Value + sym.Size, id})
		if len(stack) > 1 {
			// Insertion sort from the back. Usually this won't take any steps.
			for i := len(stack) - 1; i >= 1 && stack[i].addr > stack[i-1].addr; i-- {
				stack[i], stack[i-1] = stack[i-1], stack[i]
			}
		}
	}
	// Drain anything that's left in the stack.
	drainStack(^uint64(0))

	return out
}

// Syms returns all symbols in Table. The returned slice can be
// indexed by SymID. The caller must not modify the returned slice.
func (t *Table) Syms() []obj.Sym {
	return t.syms
}

// Name returns the (global) symbol with the given name, or obj.NoSym.
// This symbol may not be unique.
func (t *Table) Name(name string) obj.SymID {
	if i, ok := t.name[name]; ok {
		return i
	}
	return obj.NoSym
}

// Addr returns the symbol containing addr in section, or obj.NoSym.
//
// If section is nil or a mapped section, Addr considers symbols in all
// mapped sections.
//
// This symbol may not be unique, in which case Addr prioritizes the
// symbol with the latest starting address, followed by the symbol with
// the smallest size.
func (t *Table) Addr(section *obj.Section, addr uint64) obj.SymID {
	if section != nil && section.Mapped() {
		section = nil
	}
	tab, ok := t.sections[section]
	if !ok {
		return obj.NoSym
	}
	i := sort.Search(len(tab.addr), func(i int) bool {
		return addr < tab.addr[i].addr
	}) - 1
	if i < 0 {
		return obj.NoSym
	}
	id := tab.addr[i].id
	sym := &t.syms[id]
	if sym.Value+sym.Size <= addr {
		// The symbol ends before addr.
		return obj.NoSym
	}
	return id
}

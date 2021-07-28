// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dbg

import (
	"debug/dwarf"

	"github.com/aclements/go-obj/internal/imap"
)

func addRanges(m *imap.Imap, dw *dwarf.Data, ent *dwarf.Entry, val interface{}) error {
	rs, err := dw.Ranges(ent)
	if err != nil {
		return err
	}
	for _, r := range rs {
		m.Insert(imap.Interval{Low: r[0], High: r[1]}, val)
	}
	return nil
}

type entryMap struct {
	m imap.Imap
}

func (m *entryMap) add(dw *dwarf.Data, ent *dwarf.Entry) error {
	return addRanges(&m.m, dw, ent, ent)
}

func (m *entryMap) find(addr uint64) *dwarf.Entry {
	_, val := m.m.Find(addr)
	if val == nil {
		return nil
	}
	return val.(*dwarf.Entry)
}

// cuRanges indexes the PC ranges in dw.
func cuRanges(dw *dwarf.Data) (entryMap, map[CU]*cuData, error) {
	var out entryMap
	cuMap := make(map[CU]*cuData)
	dr := dw.Reader()
	for {
		ent, err := dr.Next()
		if err != nil {
			return entryMap{}, nil, err
		}
		if ent == nil {
			break
		}
		// Only read the top level.
		dr.SkipChildren()

		if ent.Tag != dwarf.TagCompileUnit {
			continue
		}
		if err := out.add(dw, ent); err != nil {
			return entryMap{}, nil, err
		}
		cuMap[CU{ent}] = new(cuData)
	}

	return out, cuMap, nil
}

// CU is a DWARF compilation unit entry.
type CU struct {
	*dwarf.Entry
}

// AddrToCU returns the DWARF compilation unit containing address addr,
// or CU{}, false if no CU contains addr.
func (d *Data) AddrToCU(addr uint64) (CU, bool) {
	entry := d.cuRanges.find(addr)
	if entry == nil {
		return CU{}, false
	}
	return CU{entry}, true
}

// Subprogram is a DWARF subprogram entry. That is, a top-level
// function.
type Subprogram struct {
	*dwarf.Entry
	CU CU // The compilation unit containing Subprogram.
}

// AddrToSubprogram returns the dwarf.TagSubprogram entry containing
// address addr. cu may be CU{} or the CU containing addr.
func (d *Data) AddrToSubprogram(addr uint64, cu CU) (Subprogram, bool) {
	if cu.Entry == nil {
		var ok bool
		if cu, ok = d.AddrToCU(addr); !ok {
			return Subprogram{}, false
		}
	}

	cuData := d.cus[cu]
	cuData.subprograms.once.Do(func() {
		// Index the subprogram DIEs in this CU.
		dr := d.dw.Reader()
		dr.Seek(cu.Offset)
		// Enter the CU.
		ent, err := dr.Next()
		if err != nil || ent == nil {
			return
		}
		if !ent.Children {
			return
		}

		// Read the children of the CU.
		m := &cuData.subprograms.ranges
		for {
			ent, err := dr.Next()
			if err != nil || ent == nil || ent.Tag == 0 {
				break
			}
			dr.SkipChildren()

			if ent.Tag != dwarf.TagSubprogram {
				continue
			}
			m.add(d.dw, ent)
		}
	})
	entry := cuData.subprograms.ranges.find(addr)
	if entry == nil {
		return Subprogram{}, false
	}
	return Subprogram{entry, cu}, true
}

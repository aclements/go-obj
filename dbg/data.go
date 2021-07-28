// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package dbg contains tools for interpreting DWARF debug info.
package dbg

import (
	"debug/dwarf"
	"sync"
)

// Data wraps dwarf.Data to provide utilities for interpreting DWARF
// debug info. This uses various caches internally to speed up lookups.
type Data struct {
	dw *dwarf.Data

	cuRanges entryMap

	cus map[CU]*cuData

	inlineRangesCache sync.Map /*[*dwarf.Entry, imap.Imap]*/
}

type cuData struct {
	subprograms struct {
		once   sync.Once
		ranges entryMap
	}
	lineTable lineTableCache
}

// New returns a new Data wrapping dw.
func New(dw *dwarf.Data) (*Data, error) {
	// Index the CU ranges eagerly. This is pretty cheap, almost
	// everything else depends on this, and it will catch basic encoding
	// errors right away.
	cuRanges, cus, err := cuRanges(dw)
	if err != nil {
		return nil, err
	}
	return &Data{dw: dw, cuRanges: cuRanges, cus: cus}, nil
}

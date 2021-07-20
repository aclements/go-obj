// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package imap

// An Iter iterates over an Imap in order.
type Iter struct {
	n *avlNode
}

// Iter returns an iterator positioned on the interval containing addr
// or the lowest interval following addr.
func (m *Imap) Iter(addr uint64) Iter {
	n := m.tree.Search(func(n *avlNode) bool {
		return addr < n.high
	})
	return Iter{n}
}

func (i *Iter) Valid() bool {
	return i.n != nil
}

func (i *Iter) Key() Interval {
	if i.n == nil {
		panic("iterator not valid")
	}
	return i.n.interval()
}

func (i *Iter) Value() interface{} {
	if i.n == nil {
		panic("iterator not valid")
	}
	return i.n.value
}

func (i *Iter) Next() {
	if i.n == nil {
		panic("iterator out of bounds")
	}
	i.n = i.n.Next()
}

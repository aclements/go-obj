// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package imap

type Imap struct {
	tree avlTree
}

type avlNode struct {
	key         uint64 // Interval low
	left, right *avlNode
	parent      *avlNode
	heightCache int

	high  uint64
	value interface{}
}

func (n *avlNode) interval() Interval {
	return Interval{n.key, n.high}
}

func (m *Imap) Insert(key Interval, value interface{}) {
	if key.Empty() {
		return
	}
	low, high := key.Low, key.High

	// Find the node that overlaps or just abuts the new range. If an
	// existing range abuts the new range, we'll extend the existing
	// range.
	n := m.tree.Search(func(n *avlNode) bool {
		return low <= n.high
	})
	pred := n

	// Split intervals that intersect low or high (one interval could do
	// both) and delete fully overlapping intervals.
	for n != nil && n.key < high {
		// Fetch the next node in case we delete this node.
		nNext := n.Next()

		// Make room for our new interval.
		l, h := n.interval().Subtract(Interval{low, high})
		lok := !l.Empty()
		hok := !h.Empty()
		if lok && !hok {
			// n overlaps the low end of the new interval. Adjust n's
			// high. Order doesn't change.
			n.high = l.High
		} else if !lok && hok {
			// n overlaps the high end of the new interval. Adjust n's
			// low. Order doesn't change.
			n.key = h.Low
			break
		} else if lok && hok {
			// The new interval falls in the middle of an existing
			// interval. Split the existing interval.
			if n.value == value {
				// Nothing needs to be done.
				return
			}
			n.high = l.High
			n2 := m.tree.Insert(h.Low)
			n2.high, n2.value = h.High, n.value
			n = n2
			break
		} else {
			// The new interval covers this interval. Delete it.
			m.tree.Delete(n)
		}

		n = nNext
	}

	// Merge with existing intervals if possible. We already handled the
	// completely overlapping case above.
	if pred != nil && pred.high == low && pred.value == value {
		// Extend the predecessor over the new range.
		pred.high = high
		if n != nil && n.key == high && n.value == value {
			// We merged right into the successor. Extend the
			// predecessor and delete the successor.
			pred.high = n.high
			m.tree.Delete(n)
		}
		return
	}
	if n != nil && n.key == high && n.value == value {
		// Extend the successor over the new range.
		n.key = low
		return
	}

	// We should now have space for the new interval.
	n = m.tree.Insert(low)
	n.high, n.value = high, value
}

// Find returns the value at addr and the interval over which value is
// the same (which may be smaller than the interval originally
// inserted). If no interval contains value, it returns Interval{}, nil.
func (m *Imap) Find(addr uint64) (key Interval, value interface{}) {
	n := m.tree.Search(func(n *avlNode) bool {
		return addr < n.high
	})
	if n != nil && n.key <= addr {
		return n.interval(), n.value
	}
	return Interval{}, nil
}

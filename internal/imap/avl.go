// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package imap

// This is a completely generic AVL tree implementation. We keep this
// separate because ideally we'll be able to replace this with a
// type-parameterized version eventually.

type avlTree struct {
	root *avlNode
}

func (t *avlTree) Insert(key uint64) *avlNode {
	// Find the insertion point and its parent node.
	var p *avlNode
	np, n := &t.root, t.root
	for n != nil {
		p = n
		if key < n.key {
			np, n = &n.left, n.left
		} else if key > n.key {
			np, n = &n.right, n.right
		} else {
			return n
		}
	}

	n = &avlNode{key: key, parent: p, heightCache: 1}
	*np = n
	t.rebalance(p)
	return n
}

func (t *avlTree) Delete(node *avlNode) {
	nodeP := t.nodeP(node)

	if node.left != nil && node.right != nil {
		// Two children. We need to move node to where it has at most
		// one child. Find node's in-order successor.
		succP, succ := &node.right, node.right
		for succ.left != nil {
			succP, succ = &succ.left, succ.left
		}

		// Transpose node and succ. This messes up the tree order, of
		// course, but after we delete node the tree will be back in
		// order. Often, this is presented as just swapping their
		// values, which is *much* easier, but doing so messes up
		// iterators.
		//
		// Between node and succ, we have six relations to update.
		// succ.left == nil and thus doesn't have a parent, so that's up
		// to 11 total links.
		parent, nl, nr, sp, sr := node.parent, node.left, node.right, succ.parent, succ.right
		*nodeP = succ
		if succ == node.right {
			// When succ and node are linked to each other, two of the
			// six relations are actually the same, so we have to handle
			// that link differently.
			succ.right = node
			nodeP = &succ.right
		} else {
			succ.right, node.parent, *succP = nr, sp, node
			nodeP = succP
		}
		node.left, node.right, succ.left, succ.parent = nil, sr, nl, parent
		node.heightCache, succ.heightCache = succ.heightCache, node.heightCache
		// Fix parent pointers.
		if succ.left != nil {
			succ.left.parent = succ
		}
		if succ.right != nil {
			succ.right.parent = succ
		}
		if node.right != nil {
			node.right.parent = node
		}
		// Now node has at most one child.
	}
	// Node has at most one child, so we can just remove node.
	if node.left == nil {
		*nodeP = node.right
		if node.right != nil {
			node.right.parent = node.parent
		}
	} else if node.right == nil {
		*nodeP = node.left
		node.left.parent = node.parent
	}

	// Walk up the tree and rebalance.
	t.rebalance(node)
}

// Search returns the first node in m's sort order for which pred
// returns true, or nil if pred is false for all nodes.
func (t *avlTree) Search(pred func(n *avlNode) bool) *avlNode {
	var best *avlNode
	n := t.root
	for n != nil {
		if pred(n) {
			// Try going smaller.
			best = n
			n = n.left
		} else {
			// Try going larger.
			n = n.right
		}
	}
	return best
}

func (n *avlNode) Next() *avlNode {
	if n.right == nil {
		// Go up left until we can go up right.
		for n.parent != nil && n.parent.right == n {
			n = n.parent
		}
		return n.parent
	}
	// Go right, and then left as much as we can.
	n = n.right
	for n.left != nil {
		n = n.left
	}
	return n
}

func (n *avlNode) Prev() *avlNode {
	if n.left == nil {
		// Go up right until we can go up left.
		for n.parent != nil && n.parent.left == n {
			n = n.parent
		}
		return n.parent
	}
	// Go left, and then right as much as we can.
	n = n.left
	for n.right != nil {
		n = n.right
	}
	return n
}

// rebalance fixes out-of-balance nodes in the path from t.root to node.
func (t *avlTree) rebalance(node *avlNode) {
	for ; node != nil; node = node.parent {
		node.updateHeight()
		b := node.balance()
		if b > 1 {
			if node.left.balance() < 0 {
				rotateLeft(&node.left)
			}
			rotateRight(t.nodeP(node))
		} else if b < -1 {
			if node.right.balance() > 0 {
				rotateRight(&node.right)
			}
			rotateLeft(t.nodeP(node))
		}
	}
}

// nodeP returns the pointer to n from n's parent.
func (t *avlTree) nodeP(n *avlNode) **avlNode {
	if n.parent == nil {
		return &t.root
	} else if n.parent.left == n {
		return &n.parent.left
	}
	return &n.parent.right
}

func (n *avlNode) height() int {
	if n == nil {
		return 0
	}
	return n.heightCache
}

func (n *avlNode) updateHeight() {
	l, r := n.left.height(), n.right.height()
	if l > r {
		n.heightCache = l + 1
	} else {
		n.heightCache = r + 1
	}
}

func (n *avlNode) balance() int {
	return n.left.height() - n.right.height()
}

func rotateLeft(np **avlNode) {
	n := *np
	nr, nrl := n.right, n.right.left
	n.parent, n.right, nr.parent, nr.left = nr, nrl, n.parent, n
	if nrl != nil {
		nrl.parent = n
	}
	n.updateHeight()
	nr.updateHeight()
	*np = nr
}

func rotateRight(np **avlNode) {
	n := *np
	nl, nlr := n.left, n.left.right
	n.parent, n.left, nl.parent, nl.right = nl, nlr, n.parent, n
	if nlr != nil {
		nlr.parent = n
	}
	n.updateHeight()
	nl.updateHeight()
	*np = nl
}

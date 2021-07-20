// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package imap

import (
	"fmt"
	"math/rand"
	"reflect"
	"sort"
	"testing"
)

func (n *avlNode) String() string {
	return fmt.Sprintf("%d (h %d)", n.value, n.heightCache)
}

func TestAVLTree(t *testing.T) {
	for i := 0; i < 1000 && !t.Failed(); i++ {
		var tree avlTree

		elts := insertRandom(t, &tree)
		check(t, &tree, elts)

		deleteRandom(t, &tree, elts)
		check(t, &tree, elts)
	}
}

func insertRandom(t *testing.T, tree *avlTree) map[uint64]bool {
	have := map[uint64]bool{}
	for j := 0; j < 100; j++ {
		val := rand.Uint64()
		tree.Insert(val)
		have[val] = true
	}
	return have
}

func deleteRandom(t *testing.T, tree *avlTree, vals map[uint64]bool) {
	i := 0
	for k := range vals {
		i++
		// TODO: It's probably worth testing deleting everything.
		if i == 50 {
			break
		}
		n := tree.Search(func(n *avlNode) bool { return n.key >= k })
		tree.Delete(n)
		delete(vals, k)
	}
}

func check(t *testing.T, tree *avlTree, want map[uint64]bool) {
	var wantOrder []uint64
	for k := range want {
		wantOrder = append(wantOrder, k)
	}
	sort.Slice(wantOrder, func(i, j int) bool { return wantOrder[i] < wantOrder[j] })

	var got []uint64
	var walk func(n, parent *avlNode) int
	walk = func(n, parent *avlNode) int {
		if n == nil {
			return 0
		}

		// Check parent.
		if n.parent != parent {
			t.Errorf("node %v has wrong parent %v, want %v", n, n.parent, parent)
		}

		lh := walk(n.left, n)
		// Collect values.
		got = append(got, n.key)
		rh := walk(n.right, n)

		// Check AVL balance.
		height := lh + 1
		if rh > lh {
			height = rh + 1
		}
		balance := lh - rh
		if n.heightCache != height {
			t.Errorf("node height: want %d, got %d", height, n.heightCache)
		} else if balance < -1 || balance > 1 {
			t.Errorf("node out of balance: left %d, right %d", lh, rh)
		}

		return height
	}

	walk(tree.root, nil)
	// Check values.
	if !reflect.DeepEqual(wantOrder, got) {
		t.Errorf("tree has wrong values\nwant: %v\ngot:  %v", wantOrder, got)
	}
}

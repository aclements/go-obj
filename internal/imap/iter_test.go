// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package imap

import "fmt"

func ExampleIter() {
	var m Imap
	for i := uint64(0); i < 5; i++ {
		m.Insert(Interval{i * 0x10, i*0x10 + 8}, i)
	}
	for it := m.Iter(0x29); it.Valid(); it.Next() {
		fmt.Printf("%v %v\n", it.Key(), it.Value())
	}
	// Output:
	// [0x30,0x38) 3
	// [0x40,0x48) 4
}

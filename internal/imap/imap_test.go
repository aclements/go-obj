// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package imap

import (
	"math/rand"
	"testing"
)

func TestImapRandom(t *testing.T) {
	var m Imap
	const max = 16
	want := make([]int, max)
	for i := 0; i < 1000; i++ {
		low := rand.Intn(max)
		high := low + rand.Intn(max-low)
		val := 1 + rand.Intn(10)
		t.Logf("insert %v@%v", val, Interval{uint64(low), uint64(high)})
		m.Insert(Interval{uint64(low), uint64(high)}, val)

		for i := low; i < high; i++ {
			want[i] = val
		}
		t.Log(want)

		// Break want into ranges.
		i := 0
		for i < len(want) {
			j := i
			for j < len(want) && want[j] == want[i] {
				j++
			}

			// Check lookup
			wantVal := want[i]
			wantInterval := Interval{uint64(i), uint64(j)}
			for k := i; k < j; k++ {
				interval, val := m.Find(uint64(k))
				if want[i] == 0 {
					if val != nil || interval != (Interval{}) {
						t.Errorf("at %#x, want none, got %v@%v", k, val, interval)
					}
				} else {
					if val != wantVal || interval != wantInterval {
						t.Errorf("at %#x, want %v@%v, got %v@%v", k, wantVal, wantInterval, val, interval)
					}
				}
			}

			i = j
		}
	}
}

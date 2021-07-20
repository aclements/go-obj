// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package imap

import "fmt"

type Interval struct {
	Low, High uint64
}

func (i Interval) String() string {
	if i.Empty() {
		return "∅"
	}
	return fmt.Sprintf("[%#x,%#x)", i.Low, i.High)
}

func (i Interval) Empty() bool {
	return i.High <= i.Low
}

func (i Interval) Contains(addr uint64) bool {
	return i.Low <= addr && addr < i.High
}

// Subtract removes interval o from interval i and returns the part of i
// (if any) that falls below o and the part of i (if any) that falls
// above o. This could result in 0, 1, or 2 new intervals.
func (i Interval) Subtract(o Interval) (below Interval, above Interval) {
	if i.Low < o.Low {
		below = Interval{i.Low, o.Low}
	}
	if o.High < i.High {
		above = Interval{o.High, i.High}
	}
	return
}

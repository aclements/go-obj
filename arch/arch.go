// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package arch provides basic descriptions of CPU architectures.
package arch

// An Arch describes a CPU architecture.
type Arch struct {
	// Layout is the byte order and word size of this architecture.
	Layout Layout

	// GoArch is the GOARCH value for this architecture.
	GoArch string

	// MinFrameSize is the number of bytes at the bottom of every
	// stack frame except for empty leaf frames. This includes,
	// for example, space for a saved LR (because that space is
	// always reserved), but does not include the return PC pushed
	// on x86 by CALL (because that is added only on a call).
	MinFrameSize int
}

var (
	AMD64 = &Arch{Layout{0, 8}, "amd64", 0}
	I386  = &Arch{Layout{0, 4}, "386", 0}
)

// String returns the GOARCH value of a.
func (a *Arch) String() string {
	if a == nil {
		return "<nil>"
	}
	return a.GoArch
}

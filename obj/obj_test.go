// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package obj

import (
	"bytes"
	"strconv"
	"testing"
)

func parseHex(hex string) []byte {
	var out []byte
	for len(hex) > 0 {
		x, err := strconv.ParseUint(hex[:2], 16, 8)
		if err != nil {
			panic(err)
		}
		out = append(out, byte(x))
		hex = hex[2:]
	}
	return out
}

func TestOpenNonObject(t *testing.T) {
	ident := []byte("AAA")
	f := bytes.NewReader(ident[:])
	_, err := Open(f)
	if err == nil {
		t.Fatalf("Open succeeded unexpectedly")
	}
	want := "unrecognized object file format"
	if err.Error() != want {
		t.Fatalf("want error %q, got %q", want, err.Error())
	}
}

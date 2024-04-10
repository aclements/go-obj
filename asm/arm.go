// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package asm

import (
	"io"

	"golang.org/x/arch/arm64/arm64asm"
)

func disasmARM64(text []byte, pc uint64) Seq {
	var out arm64Seq
	for len(text) > 0 {
		inst, err := arm64asm.Decode(text)
		if err != nil || inst.Op == 0 {
			inst = arm64asm.Inst{}
		}
		out = append(out, arm64Inst{inst, pc})

		const size = 4
		text = text[size:]
		pc += uint64(size)
	}
	return out

}

type arm64Seq []arm64Inst

func (s arm64Seq) Len() int {
	return len(s)
}

func (s arm64Seq) Get(i int) Inst {
	return &s[i]
}

type arm64Inst struct {
	arm64asm.Inst
	pc uint64
}

func (i *arm64Inst) GoSyntax(symname func(uint64) (string, uint64)) string {
	if i.Op == 0 {
		return "?"
	}

	var text io.ReaderAt = nil // TODO: populate
	return arm64asm.GoSyntax(i.Inst, i.pc, symname, text)
}

func (i *arm64Inst) PC() uint64 {
	return i.pc
}

func (i *arm64Inst) Len() int { return 4 }

func (i *arm64Inst) Control() Control {
	var c Control
	c.TargetPC = ^uint64(0)

	// Handle explicit control flow instructions.
	switch i.Op {
	case arm64asm.B:
		c.Type = ControlJump
	case arm64asm.BL, arm64asm.SYSL, arm64asm.SYS:
		c.Type = ControlCall
	case arm64asm.RET, arm64asm.ERET:
		c.Type = ControlRet
	}

	for _, arg := range i.Args {
		switch arg := arg.(type) {
		case arm64asm.Cond:
			c.Conditional = true
		case arm64asm.PCRel:
			c.TargetPC = uint64(int64(i.pc) + int64(arg))
		}
	}

	return c
}

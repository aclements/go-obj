// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package asm

import (
	"fmt"

	"golang.org/x/arch/x86/x86asm"
)

func disasmX86(text []byte, pc uint64, bits int) Seq {
	var out x86Seq
	for len(text) > 0 {
		inst, err := x86asm.Decode(text, bits)
		size := inst.Len
		if err != nil || size == 0 || inst.Op == 0 {
			inst = x86asm.Inst{}
		}
		if size == 0 {
			size = 1
		}
		out = append(out, x86Inst{inst, pc})

		text = text[size:]
		pc += uint64(size)
	}
	return out

}

type x86Seq []x86Inst

func (s x86Seq) Len() int {
	return len(s)
}

func (s x86Seq) Get(i int) Inst {
	return &s[i]
}

type x86Inst struct {
	x86asm.Inst
	pc uint64
}

func (i *x86Inst) GoSyntax(symname func(uint64) (string, uint64)) string {
	if i.Op == 0 {
		return "?"
	}
	return x86asm.GoSyntax(i.Inst, i.pc, symname)
}

func (i *x86Inst) PC() uint64 {
	return i.pc
}

func (i *x86Inst) Len() int {
	return i.Inst.Len
}

func (i *x86Inst) Control() Control {
	var c Control

	// Handle REP-prefixed instructions.
	for _, pfx := range i.Inst.Prefix {
		if pfx == 0 {
			break
		}
		if pfx == x86asm.PrefixREP || pfx == x86asm.PrefixREPN {
			c.Type = ControlJump
			c.Conditional = true
			c.TargetPC = i.pc
			return c
		}
	}

	// Handle explicit control flow instructions.
	switch i.Op {
	default:
		return c
	case x86asm.CALL, x86asm.LCALL, x86asm.SYSCALL, x86asm.SYSENTER:
		c.Type = ControlCall
	case x86asm.RET, x86asm.LRET, x86asm.SYSRET, x86asm.SYSEXIT:
		c.Type = ControlRet
		return c // No argument
	case x86asm.UD1, x86asm.UD2:
		c.Type = ControlExit
		return c // no argument
	case x86asm.JMP, x86asm.LJMP:
		c.Type = ControlJump
	case x86asm.JA, x86asm.JAE, x86asm.JB, x86asm.JBE, x86asm.JCXZ, x86asm.JE, x86asm.JECXZ, x86asm.JG, x86asm.JGE, x86asm.JL, x86asm.JLE, x86asm.JNE, x86asm.JNO, x86asm.JNP, x86asm.JNS, x86asm.JO, x86asm.JP, x86asm.JRCXZ, x86asm.JS,
		x86asm.LOOP, x86asm.LOOPE, x86asm.LOOPNE,
		x86asm.XBEGIN:
		c.Type = ControlJump
		c.Conditional = true
	}
	if i.Args[0] == nil || i.Args[1] != nil {
		panic(fmt.Sprintf("expected one argument, got %s", i))
	}
	if rel, ok := i.Args[0].(x86asm.Rel); ok {
		c.TargetPC = uint64(int64(i.pc) + int64(i.Inst.Len) + int64(rel))
	}
	c.Target = i.Args[0]
	return c
}

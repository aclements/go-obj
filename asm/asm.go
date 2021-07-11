// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package asm abstracts disassembling machine code from various
// architectures.
package asm

import (
	"fmt"

	"github.com/aclements/go-obj/arch"
)

// Disasm disassembles machine code for the given architecture. pc is
// the program counter at which text begins.
func Disasm(arch *arch.Arch, text []byte, pc uint64) (Seq, error) {
	switch arch.GoArch {
	case "amd64":
		return disasmX86(text, pc, 64), nil
	case "386":
		return disasmX86(text, pc, 32), nil
	}
	return nil, fmt.Errorf("unsupported assembly architecture: %s", arch)
}

// Seq is a sequence of instructions.
type Seq interface {
	Len() int
	Get(i int) Inst
}

// Inst is a single machine instruction.
type Inst interface {
	// GoSyntax returns the Go assembler syntax representation of
	// this instruction. symname, if non-nil, must return the name
	// and base of the symbol containing address addr, or "" if
	// symbol lookup fails.
	GoSyntax(symName func(addr uint64) (string, uint64)) string

	// PC returns the address of this instruction.
	PC() uint64

	// Len returns the length of this instruction in bytes.
	Len() int

	// Control returns the control-flow effects of this
	// instruction.
	Control() Control
}

// Control captures control-flow effects of an instruction.
type Control struct {
	Type        ControlType
	Conditional bool
	TargetPC    uint64
	Target      Arg
}

type ControlType uint8

const (
	ControlNone ControlType = iota
	ControlJump
	ControlCall
	ControlRet

	// ControlJumpUnknown is a jump with an unknown target. This
	// means the control analysis could be incomplete, since this
	// could jump to an instruction in the analyzed function.
	ControlJumpUnknown

	// ControlExit is like a call that never returns.
	ControlExit
)

// Arg is an argument to an instruction.
type Arg interface {
}

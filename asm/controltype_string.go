// Code generated by "stringer -type=ControlType"; DO NOT EDIT.

package asm

import "strconv"

func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	// Re-run the stringer command to generate them again.
	var x [1]struct{}
	_ = x[ControlNone-0]
	_ = x[ControlJump-1]
	_ = x[ControlCall-2]
	_ = x[ControlRet-3]
	_ = x[ControlJumpUnknown-4]
	_ = x[ControlExit-5]
}

const _ControlType_name = "ControlNoneControlJumpControlCallControlRetControlJumpUnknownControlExit"

var _ControlType_index = [...]uint8{0, 11, 22, 33, 43, 61, 72}

func (i ControlType) String() string {
	if i >= ControlType(len(_ControlType_index)-1) {
		return "ControlType(" + strconv.FormatInt(int64(i), 10) + ")"
	}
	return _ControlType_name[_ControlType_index[i]:_ControlType_index[i+1]]
}

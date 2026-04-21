package pyc

// CPython 3.14 opcode constants. Only the subset gopygo v0.1
// understands is listed; encountering any other opcode at transpile
// time is a hard error.
const (
	CACHE                 uint8 = 0
	END_FOR               uint8 = 9
	END_SEND              uint8 = 10
	FORMAT_SIMPLE         uint8 = 12
	NOT_TAKEN             uint8 = 28
	UNARY_NEGATIVE        uint8 = 41
	UNARY_NOT             uint8 = 42
	GET_ITER              uint8 = 16
	INTERPRETER_EXIT      uint8 = 20
	MAKE_FUNCTION         uint8 = 23
	NOP                   uint8 = 27
	POP_ITER              uint8 = 30
	POP_TOP               uint8 = 31
	PUSH_NULL             uint8 = 33
	RETURN_VALUE          uint8 = 35
	TO_BOOL               uint8 = 39
	BINARY_OP             uint8 = 44
	BUILD_LIST            uint8 = 46
	BUILD_STRING          uint8 = 50
	BUILD_TUPLE           uint8 = 51
	CALL                  uint8 = 52
	COMPARE_OP            uint8 = 56
	COPY                  uint8 = 59
	COPY_FREE_VARS        uint8 = 60
	FOR_ITER              uint8 = 70
	JUMP_BACKWARD         uint8 = 75
	JUMP_FORWARD          uint8 = 77
	LOAD_CONST            uint8 = 82
	LOAD_FAST             uint8 = 84
	LOAD_FAST_AND_CLEAR   uint8 = 85
	LOAD_FAST_BORROW      uint8 = 86
	LOAD_FAST_BORROW_LOAD_FAST_BORROW uint8 = 87
	LOAD_FAST_CHECK       uint8 = 88
	LOAD_FAST_LOAD_FAST   uint8 = 89
	LOAD_GLOBAL           uint8 = 92
	LOAD_NAME             uint8 = 93
	LOAD_SMALL_INT        uint8 = 94
	POP_JUMP_IF_FALSE     uint8 = 100
	POP_JUMP_IF_NONE      uint8 = 101
	POP_JUMP_IF_NOT_NONE  uint8 = 102
	POP_JUMP_IF_TRUE      uint8 = 103
	STORE_FAST            uint8 = 112
	STORE_FAST_STORE_FAST uint8 = 114
	STORE_NAME            uint8 = 116
	SWAP                  uint8 = 117
	RESUME                uint8 = 128
)

// Cache slots that follow each opcode. An opcode byte pair
// [op, arg] is always two bytes; cached ops additionally consume
// 2*Cache[op] bytes of inline cache storage that the compiler
// pre-allocated.
var Cache = [256]uint8{
	LOAD_GLOBAL:          4,
	BINARY_OP:            5,
	COMPARE_OP:           1,
	FOR_ITER:             1,
	CALL:                 3,
	JUMP_BACKWARD:        1,
	TO_BOOL:              3,
	POP_JUMP_IF_TRUE:     1,
	POP_JUMP_IF_FALSE:    1,
	POP_JUMP_IF_NONE:     1,
	POP_JUMP_IF_NOT_NONE: 1,
}

// BINARY_OP argument encoding (NB_* in CPython 3.14).
const (
	NB_ADD          = 0
	NB_AND          = 1
	NB_FLOOR_DIVIDE = 2
	NB_LSHIFT       = 3
	NB_MATRIX_MUL   = 4
	NB_MULTIPLY     = 5
	NB_REMAINDER    = 6
	NB_OR           = 7
	NB_POWER        = 8
	NB_RSHIFT       = 9
	NB_SUBTRACT     = 10
	NB_TRUE_DIVIDE  = 11
	NB_XOR          = 12
	// Inplace variants follow at 13..25, same op semantics for v0.1.
	NB_INPLACE_ADD  = 13
	NB_INPLACE_SUB  = 23
	NB_INPLACE_MUL  = 18
	NB_INPLACE_TDIV = 24
	NB_INPLACE_FDIV = 15
	NB_INPLACE_REM  = 19
)

// COMPARE_OP argument decoding. In CPython 3.14 the argument is a
// 16-bit value where the top 4 bits hold the comparison kind and
// bit 4 (0x10) signals "cast result to bool". gopygo always casts
// to bool because Python comparisons are already bool-typed.
const (
	CMP_LT = 0
	CMP_LE = 1
	CMP_EQ = 2
	CMP_NE = 3
	CMP_GT = 4
	CMP_GE = 5
)

// OpName maps an opcode byte to a mnemonic for diagnostics.
func OpName(op uint8) string {
	if n, ok := opNames[op]; ok {
		return n
	}
	return "OP?"
}

var opNames = map[uint8]string{
	CACHE:                 "CACHE",
	RESUME:                "RESUME",
	NOP:                   "NOP",
	POP_ITER:              "POP_ITER",
	POP_TOP:               "POP_TOP",
	PUSH_NULL:             "PUSH_NULL",
	RETURN_VALUE:          "RETURN_VALUE",
	COPY:                  "COPY",
	SWAP:                  "SWAP",
	COPY_FREE_VARS:        "COPY_FREE_VARS",
	LOAD_CONST:            "LOAD_CONST",
	LOAD_SMALL_INT:        "LOAD_SMALL_INT",
	LOAD_FAST:             "LOAD_FAST",
	LOAD_FAST_CHECK:       "LOAD_FAST_CHECK",
	LOAD_FAST_LOAD_FAST:   "LOAD_FAST_LOAD_FAST",
	LOAD_FAST_AND_CLEAR:   "LOAD_FAST_AND_CLEAR",
	STORE_FAST:            "STORE_FAST",
	STORE_FAST_STORE_FAST: "STORE_FAST_STORE_FAST",
	LOAD_GLOBAL:           "LOAD_GLOBAL",
	LOAD_NAME:             "LOAD_NAME",
	STORE_NAME:            "STORE_NAME",
	BINARY_OP:             "BINARY_OP",
	COMPARE_OP:            "COMPARE_OP",
	CALL:                  "CALL",
	MAKE_FUNCTION:         "MAKE_FUNCTION",
	GET_ITER:              "GET_ITER",
	FOR_ITER:              "FOR_ITER",
	END_FOR:               "END_FOR",
	END_SEND:              "END_SEND",
	NOT_TAKEN:             "NOT_TAKEN",
	UNARY_NEGATIVE:        "UNARY_NEGATIVE",
	UNARY_NOT:             "UNARY_NOT",
	LOAD_FAST_BORROW:      "LOAD_FAST_BORROW",
	LOAD_FAST_BORROW_LOAD_FAST_BORROW: "LOAD_FAST_BORROW_LOAD_FAST_BORROW",
	JUMP_FORWARD:          "JUMP_FORWARD",
	JUMP_BACKWARD:         "JUMP_BACKWARD",
	POP_JUMP_IF_FALSE:     "POP_JUMP_IF_FALSE",
	POP_JUMP_IF_TRUE:      "POP_JUMP_IF_TRUE",
	POP_JUMP_IF_NONE:      "POP_JUMP_IF_NONE",
	POP_JUMP_IF_NOT_NONE:  "POP_JUMP_IF_NOT_NONE",
	BUILD_TUPLE:           "BUILD_TUPLE",
	BUILD_LIST:            "BUILD_LIST",
	BUILD_STRING:          "BUILD_STRING",
	FORMAT_SIMPLE:         "FORMAT_SIMPLE",
	TO_BOOL:               "TO_BOOL",
	INTERPRETER_EXIT:      "INTERPRETER_EXIT",
}

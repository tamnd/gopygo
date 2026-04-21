// Package pyc models CPython 3.14 code objects and decodes the marshal
// stream produced by `python3.14 -m py_compile`. It carries just enough
// structure for the transpiler; it is not a general Python runtime.
package pyc

import "math/big"

// Object is a constant value carried inside a Code.Consts slot or
// produced anywhere else the marshal format returns a Python value.
// Kept deliberately minimal; the transpiler pattern-matches on the
// concrete type and emits the corresponding runtime call.
type Object interface{ pyConst() }

// Int wraps an arbitrary-precision Python int.
type Int struct{ V big.Int }

func (*Int) pyConst() {}

// Str is a Python str. Stored as a Go string; UTF-8.
type Str struct{ V string }

func (*Str) pyConst() {}

// Bytes is a Python bytes literal.
type Bytes struct{ V []byte }

func (*Bytes) pyConst() {}

// Float is a Python float.
type Float struct{ V float64 }

func (*Float) pyConst() {}

// Bool is a Python bool.
type Bool struct{ V bool }

func (*Bool) pyConst() {}

// NoneType is the type of None. A single value (None) exists.
type NoneType struct{}

func (*NoneType) pyConst() {}

// None is the Python None singleton.
var None = &NoneType{}

// True / False are the canonical booleans exposed by marshal.
var (
	True  = &Bool{V: true}
	False = &Bool{V: false}
)

// Tuple is an immutable sequence.
type Tuple struct{ V []Object }

func (*Tuple) pyConst() {}

// Local-plus kind bits from CPython's symtable.
const (
	FastLocal = 0x20
	FastCell  = 0x40
	FastFree  = 0x80
)

// Code is a single compiled Python code object.
type Code struct {
	ArgCount        int
	PosOnlyArgCount int
	KwOnlyArgCount  int
	Stacksize       int
	Flags           int
	Bytecode        []byte
	Consts          []Object
	Names           []string
	LocalsPlusNames []string
	LocalsPlusKinds []byte
	Filename        string
	Name            string
	QualName        string
	FirstLineNo     int
	LineTable       []byte
	ExceptionTable  []byte

	NLocals  int
	NCells   int
	NFrees   int
	CellVars []string
	FreeVars []string
}

func (*Code) pyConst() {}

// IntFromBig wraps a *big.Int as *Int.
func IntFromBig(x *big.Int) *Int {
	i := &Int{}
	i.V.Set(x)
	return i
}

package pyc

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/big"
	"os"
)

// Magic314 is the 4-byte magic for CPython 3.14 .pyc files.
var Magic314 = [4]byte{0x2b, 0x0e, 0x0d, 0x0a}

// LoadPyc reads the .pyc at path and returns the top-level code object.
func LoadPyc(path string) (*Code, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Decode(b)
}

// Decode parses a .pyc file (16-byte header + marshal stream).
func Decode(b []byte) (*Code, error) {
	if len(b) < 16 {
		return nil, fmt.Errorf("pyc: too short (%d bytes)", len(b))
	}
	for i := 0; i < 4; i++ {
		if b[i] != Magic314[i] {
			return nil, fmt.Errorf("pyc: unsupported magic %02x%02x%02x%02x (need 3.14 = %02x%02x%02x%02x)",
				b[0], b[1], b[2], b[3], Magic314[0], Magic314[1], Magic314[2], Magic314[3])
		}
	}
	o, err := Unmarshal(b[16:])
	if err != nil {
		return nil, err
	}
	c, ok := o.(*Code)
	if !ok {
		return nil, fmt.Errorf("pyc: top-level object is %T, want code", o)
	}
	return c, nil
}

// Type tags (subset used by python3.14 -m py_compile).
const (
	TYPE_NULL                 = '0'
	TYPE_NONE                 = 'N'
	TYPE_FALSE                = 'F'
	TYPE_TRUE                 = 'T'
	TYPE_INT                  = 'i'
	TYPE_INT64                = 'I'
	TYPE_FLOAT                = 'f'
	TYPE_BINARY_FLOAT         = 'g'
	TYPE_LONG                 = 'l'
	TYPE_STRING               = 's'
	TYPE_INTERNED             = 't'
	TYPE_REF                  = 'r'
	TYPE_TUPLE                = '('
	TYPE_CODE                 = 'c'
	TYPE_UNICODE              = 'u'
	TYPE_ASCII                = 'a'
	TYPE_ASCII_INTERNED       = 'A'
	TYPE_SMALL_TUPLE          = ')'
	TYPE_SHORT_ASCII          = 'z'
	TYPE_SHORT_ASCII_INTERNED = 'Z'

	FLAG_REF = 0x80
)

// Reader holds decode state.
type Reader struct {
	buf  []byte
	pos  int
	refs []Object
}

// Unmarshal decodes one object from b.
func Unmarshal(b []byte) (Object, error) {
	r := &Reader{buf: b}
	return r.ReadObject()
}

func (r *Reader) need(n int) error {
	if r.pos+n > len(r.buf) {
		return fmt.Errorf("marshal: unexpected EOF (need %d, have %d)", n, len(r.buf)-r.pos)
	}
	return nil
}

func (r *Reader) readByte() (byte, error) {
	if err := r.need(1); err != nil {
		return 0, err
	}
	b := r.buf[r.pos]
	r.pos++
	return b, nil
}

func (r *Reader) readI32() (int32, error) {
	if err := r.need(4); err != nil {
		return 0, err
	}
	v := int32(binary.LittleEndian.Uint32(r.buf[r.pos:]))
	r.pos += 4
	return v, nil
}

func (r *Reader) readU32() (uint32, error) {
	if err := r.need(4); err != nil {
		return 0, err
	}
	v := binary.LittleEndian.Uint32(r.buf[r.pos:])
	r.pos += 4
	return v, nil
}

func (r *Reader) readU64() (uint64, error) {
	if err := r.need(8); err != nil {
		return 0, err
	}
	v := binary.LittleEndian.Uint64(r.buf[r.pos:])
	r.pos += 8
	return v, nil
}

func (r *Reader) readBytes(n int) ([]byte, error) {
	if err := r.need(n); err != nil {
		return nil, err
	}
	out := make([]byte, n)
	copy(out, r.buf[r.pos:r.pos+n])
	r.pos += n
	return out, nil
}

func (r *Reader) reserve(flag bool) int {
	if !flag {
		return -1
	}
	r.refs = append(r.refs, nil)
	return len(r.refs) - 1
}

func (r *Reader) set(idx int, o Object) Object {
	if idx >= 0 {
		r.refs[idx] = o
	}
	return o
}

// ReadObject decodes the next object from the stream.
func (r *Reader) ReadObject() (Object, error) {
	t, err := r.readByte()
	if err != nil {
		return nil, err
	}
	flag := t&FLAG_REF != 0
	t &^= FLAG_REF
	switch t {
	case TYPE_NULL:
		return nil, nil
	case TYPE_NONE:
		return None, nil
	case TYPE_FALSE:
		return False, nil
	case TYPE_TRUE:
		return True, nil
	case TYPE_INT:
		v, err := r.readI32()
		if err != nil {
			return nil, err
		}
		return r.set(r.reserve(flag), IntFromBig(big.NewInt(int64(v)))), nil
	case TYPE_INT64:
		v, err := r.readU64()
		if err != nil {
			return nil, err
		}
		return r.set(r.reserve(flag), IntFromBig(new(big.Int).SetInt64(int64(v)))), nil
	case TYPE_LONG:
		return r.readLong(flag)
	case TYPE_BINARY_FLOAT:
		v, err := r.readU64()
		if err != nil {
			return nil, err
		}
		return r.set(r.reserve(flag), &Float{V: math.Float64frombits(v)}), nil
	case TYPE_FLOAT:
		n, err := r.readByte()
		if err != nil {
			return nil, err
		}
		b, err := r.readBytes(int(n))
		if err != nil {
			return nil, err
		}
		var f float64
		_, _ = fmt.Sscan(string(b), &f)
		return r.set(r.reserve(flag), &Float{V: f}), nil
	case TYPE_STRING:
		n, err := r.readU32()
		if err != nil {
			return nil, err
		}
		b, err := r.readBytes(int(n))
		if err != nil {
			return nil, err
		}
		return r.set(r.reserve(flag), &Bytes{V: b}), nil
	case TYPE_UNICODE, TYPE_INTERNED, TYPE_ASCII, TYPE_ASCII_INTERNED:
		n, err := r.readU32()
		if err != nil {
			return nil, err
		}
		b, err := r.readBytes(int(n))
		if err != nil {
			return nil, err
		}
		return r.set(r.reserve(flag), &Str{V: string(b)}), nil
	case TYPE_SHORT_ASCII, TYPE_SHORT_ASCII_INTERNED:
		n, err := r.readByte()
		if err != nil {
			return nil, err
		}
		b, err := r.readBytes(int(n))
		if err != nil {
			return nil, err
		}
		return r.set(r.reserve(flag), &Str{V: string(b)}), nil
	case TYPE_TUPLE:
		n, err := r.readU32()
		if err != nil {
			return nil, err
		}
		return r.readTuple(int(n), flag)
	case TYPE_SMALL_TUPLE:
		n, err := r.readByte()
		if err != nil {
			return nil, err
		}
		return r.readTuple(int(n), flag)
	case TYPE_CODE:
		return r.readCode(flag)
	case TYPE_REF:
		ix, err := r.readU32()
		if err != nil {
			return nil, err
		}
		if int(ix) >= len(r.refs) {
			return nil, fmt.Errorf("marshal: bad TYPE_REF index %d", ix)
		}
		return r.refs[ix], nil
	}
	return nil, fmt.Errorf("marshal: type byte %q (0x%02x) at pos %d not supported in v0.1",
		t, t, r.pos-1)
}

func (r *Reader) readTuple(n int, flag bool) (Object, error) {
	t := &Tuple{V: make([]Object, n)}
	idx := r.reserve(flag)
	r.set(idx, t)
	for i := 0; i < n; i++ {
		o, err := r.ReadObject()
		if err != nil {
			return nil, err
		}
		t.V[i] = o
	}
	return t, nil
}

func (r *Reader) readLong(flag bool) (Object, error) {
	n, err := r.readI32()
	if err != nil {
		return nil, err
	}
	neg := false
	size := int(n)
	if n < 0 {
		neg = true
		size = int(-n)
	}
	result := new(big.Int)
	shift := uint(0)
	for i := 0; i < size; i++ {
		if err := r.need(2); err != nil {
			return nil, err
		}
		d := uint16(r.buf[r.pos]) | uint16(r.buf[r.pos+1])<<8
		r.pos += 2
		chunk := new(big.Int).SetUint64(uint64(d))
		chunk.Lsh(chunk, shift)
		result.Or(result, chunk)
		shift += 15
	}
	if neg {
		result.Neg(result)
	}
	return r.set(r.reserve(flag), IntFromBig(result)), nil
}

func (r *Reader) readCode(flag bool) (Object, error) {
	c := &Code{}
	r.set(r.reserve(flag), c)

	var v int32
	var err error
	ri32 := func(dst *int) error {
		v, err = r.readI32()
		if err != nil {
			return err
		}
		*dst = int(v)
		return nil
	}
	if err := ri32(&c.ArgCount); err != nil {
		return nil, err
	}
	if err := ri32(&c.PosOnlyArgCount); err != nil {
		return nil, err
	}
	if err := ri32(&c.KwOnlyArgCount); err != nil {
		return nil, err
	}
	if err := ri32(&c.Stacksize); err != nil {
		return nil, err
	}
	if err := ri32(&c.Flags); err != nil {
		return nil, err
	}

	code, err := r.ReadObject()
	if err != nil {
		return nil, err
	}
	b, ok := code.(*Bytes)
	if !ok {
		return nil, fmt.Errorf("marshal: co_code is %T", code)
	}
	c.Bytecode = b.V

	consts, err := r.ReadObject()
	if err != nil {
		return nil, err
	}
	if t, ok := consts.(*Tuple); ok {
		c.Consts = t.V
	}

	names, err := r.ReadObject()
	if err != nil {
		return nil, err
	}
	if t, ok := names.(*Tuple); ok {
		c.Names = strSlice(t.V, "co_names")
	}

	lpn, err := r.ReadObject()
	if err != nil {
		return nil, err
	}
	if t, ok := lpn.(*Tuple); ok {
		c.LocalsPlusNames = strSlice(t.V, "co_localsplusnames")
	}

	kinds, err := r.ReadObject()
	if err != nil {
		return nil, err
	}
	if b, ok := kinds.(*Bytes); ok {
		c.LocalsPlusKinds = b.V
	}

	fn, err := r.ReadObject()
	if err != nil {
		return nil, err
	}
	if s, ok := fn.(*Str); ok {
		c.Filename = s.V
	}

	nm, err := r.ReadObject()
	if err != nil {
		return nil, err
	}
	if s, ok := nm.(*Str); ok {
		c.Name = s.V
	}

	qn, err := r.ReadObject()
	if err != nil {
		return nil, err
	}
	if s, ok := qn.(*Str); ok {
		c.QualName = s.V
	}

	if err := ri32(&c.FirstLineNo); err != nil {
		return nil, err
	}

	lt, err := r.ReadObject()
	if err != nil {
		return nil, err
	}
	if b, ok := lt.(*Bytes); ok {
		c.LineTable = b.V
	}

	et, err := r.ReadObject()
	if err != nil {
		return nil, err
	}
	if b, ok := et.(*Bytes); ok {
		c.ExceptionTable = b.V
	}

	for i, k := range c.LocalsPlusKinds {
		switch {
		case k&FastFree != 0:
			c.NFrees++
			c.FreeVars = append(c.FreeVars, c.LocalsPlusNames[i])
		case k&FastCell != 0:
			c.NCells++
			c.CellVars = append(c.CellVars, c.LocalsPlusNames[i])
		default:
			c.NLocals++
		}
	}

	return c, nil
}

func strSlice(v []Object, what string) []string {
	out := make([]string, len(v))
	for i, x := range v {
		s, ok := x.(*Str)
		if !ok {
			panic(errors.New(what + ": non-string entry"))
		}
		out[i] = s.V
	}
	return out
}

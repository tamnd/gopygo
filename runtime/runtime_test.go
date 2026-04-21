package runtime

import (
	"bytes"
	"math/big"
	"testing"
)

func TestAddIntInt(t *testing.T) {
	r, err := Add(NewIntInt(7), NewIntInt(5))
	if err != nil {
		t.Fatal(err)
	}
	if r.(*Int).V.Cmp(big.NewInt(12)) != 0 {
		t.Fatalf("got %v", r)
	}
}

func TestAddStrStr(t *testing.T) {
	r, err := Add(NewStr("hi "), NewStr("there"))
	if err != nil {
		t.Fatal(err)
	}
	if r.(*Str).V != "hi there" {
		t.Fatalf("got %q", r.(*Str).V)
	}
}

func TestFloorDivNegative(t *testing.T) {
	// Python's // floors toward negative infinity, not toward zero.
	r, err := FloorDiv(NewIntInt(-7), NewIntInt(2))
	if err != nil {
		t.Fatal(err)
	}
	if r.(*Int).V.Cmp(big.NewInt(-4)) != 0 {
		t.Fatalf("want -4, got %v", r)
	}
}

func TestCompareLess(t *testing.T) {
	r, err := Compare(0, NewIntInt(3), NewIntInt(5))
	if err != nil {
		t.Fatal(err)
	}
	if r.(*Bool).V != true {
		t.Fatal("3 < 5 should be true")
	}
}

func TestPrint(t *testing.T) {
	var buf bytes.Buffer
	prev := Stdout
	Stdout = &buf
	defer func() { Stdout = prev }()
	_, err := pyPrint([]Value{NewStr("a"), NewIntInt(1)})
	if err != nil {
		t.Fatal(err)
	}
	if buf.String() != "a 1\n" {
		t.Fatalf("got %q", buf.String())
	}
}

func TestRange(t *testing.T) {
	v, err := pyRange([]Value{NewIntInt(3)})
	if err != nil {
		t.Fatal(err)
	}
	it := v.(*Iter)
	var got []int64
	for {
		x, ok := it.Next()
		if !ok {
			break
		}
		got = append(got, x.(*Int).V.Int64())
	}
	if len(got) != 3 || got[0] != 0 || got[1] != 1 || got[2] != 2 {
		t.Fatalf("got %v", got)
	}
}

func TestTruthy(t *testing.T) {
	cases := []struct {
		v    Value
		want bool
	}{
		{NewIntInt(0), false},
		{NewIntInt(1), true},
		{NewStr(""), false},
		{NewStr("x"), true},
		{None, false},
		{True, true},
		{False, false},
	}
	for _, c := range cases {
		if got := Truthy(c.v); got != c.want {
			t.Errorf("Truthy(%v)=%v, want %v", c.v, got, c.want)
		}
	}
}

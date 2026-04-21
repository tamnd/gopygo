package gen

import (
	"bytes"
	"fmt"

	"github.com/tamnd/gopygo/pyast"
	"github.com/tamnd/gopygo/types"
)

// emitModule walks the Module node in two passes:
//   1. Collect every top-level FunctionDef's signature from its
//      annotations. This lets module-level code and other functions
//      call them without forward-reference gymnastics.
//   2. Emit each function body in source order, followed by main().
func (g *gen) emitModule(mod pyast.Node) error {
	if mod.Type() != "Module" {
		return fmt.Errorf("gopygo: expected Module at root, got %s", mod.Type())
	}
	body := mod.Children("body")

	modScope := newScope(nil)
	g.scope = modScope

	// Pass 1: function signatures.
	for _, s := range body {
		if s.Type() != "FunctionDef" {
			continue
		}
		sig, err := g.funcSignature(s)
		if err != nil {
			return err
		}
		modScope.names[s.Str("name")] = sig
	}

	// Pass 2: function bodies.
	for _, s := range body {
		if s.Type() != "FunctionDef" {
			continue
		}
		if err := g.emitFunction(s); err != nil {
			return err
		}
	}

	// Pass 3: main()  — everything that is not a def.
	fmt.Fprintf(&g.out, "func main() {\n")
	for _, s := range body {
		if s.Type() == "FunctionDef" {
			continue
		}
		if err := g.emitStmt(s, "\t"); err != nil {
			return err
		}
	}
	fmt.Fprintf(&g.out, "}\n")
	return nil
}

func (g *gen) funcSignature(n pyast.Node) (types.TFunc, error) {
	name := n.Str("name")
	args := n.Child("args")
	if args == nil {
		return types.TFunc{}, g.errf(n, "FunctionDef without args")
	}
	if len(args.Children("posonlyargs")) > 0 ||
		len(args.Children("kwonlyargs")) > 0 ||
		args.Child("vararg") != nil ||
		args.Child("kwarg") != nil ||
		len(args.Children("defaults")) > 0 {
		return types.TFunc{}, g.errf(n, "v0.1 supports only positional parameters, no defaults/*args/**kwargs")
	}
	var params []types.Type
	for _, a := range args.Children("args") {
		ann := a.Child("annotation")
		if ann == nil {
			return types.TFunc{}, g.errf(a, "parameter %q needs a type annotation", a.Str("arg"))
		}
		t, err := g.parseAnnotation(ann)
		if err != nil {
			return types.TFunc{}, err
		}
		params = append(params, t)
	}
	retNode := n.Child("returns")
	var ret types.Type = types.TNone{}
	if retNode != nil {
		t, err := g.parseAnnotation(retNode)
		if err != nil {
			return types.TFunc{}, err
		}
		ret = t
	}
	return types.TFunc{Name: name, Params: params, Return: ret}, nil
}

// parseAnnotation maps a Python annotation expression node to a
// gopygo Type. Accepted shapes: bare Name (int, float, bool, str,
// None, Any), Subscript (list[T], dict[K,V], tuple[T1,T2,...]).
func (g *gen) parseAnnotation(n pyast.Node) (types.Type, error) {
	switch n.Type() {
	case "Name":
		switch n.Str("id") {
		case "int":
			return types.TInt{}, nil
		case "float":
			return types.TFloat{}, nil
		case "bool":
			return types.TBool{}, nil
		case "str":
			return types.TStr{}, nil
		case "None":
			return types.TNone{}, nil
		case "Any":
			return types.TAny{}, nil
		}
		return nil, g.errf(n, "unknown type annotation: %s", n.Str("id"))

	case "Constant":
		if n.Raw("value") == nil {
			return types.TNone{}, nil
		}
		return nil, g.errf(n, "unsupported constant in annotation")

	case "Subscript":
		outer := n.Child("value")
		if outer == nil || outer.Type() != "Name" {
			return nil, g.errf(n, "unsupported annotation shape")
		}
		inner := n.Child("slice")
		switch outer.Str("id") {
		case "list":
			t, err := g.parseAnnotation(inner)
			if err != nil {
				return nil, err
			}
			return types.TList{Elem: t}, nil
		case "dict":
			if inner.Type() != "Tuple" {
				return nil, g.errf(n, "dict[K, V] needs two type arguments")
			}
			elts := inner.Children("elts")
			if len(elts) != 2 {
				return nil, g.errf(n, "dict[K, V] needs exactly two type arguments")
			}
			kt, err := g.parseAnnotation(elts[0])
			if err != nil {
				return nil, err
			}
			vt, err := g.parseAnnotation(elts[1])
			if err != nil {
				return nil, err
			}
			return types.TDict{K: kt, V: vt}, nil
		case "tuple":
			if inner.Type() != "Tuple" {
				t, err := g.parseAnnotation(inner)
				if err != nil {
					return nil, err
				}
				return types.TTuple{Elems: []types.Type{t}}, nil
			}
			var elems []types.Type
			for _, e := range inner.Children("elts") {
				t, err := g.parseAnnotation(e)
				if err != nil {
					return nil, err
				}
				elems = append(elems, t)
			}
			return types.TTuple{Elems: elems}, nil
		}
	}
	return nil, g.errf(n, "unsupported annotation node: %s", n.Type())
}

func (g *gen) emitFunction(n pyast.Node) error {
	sig, _ := g.funcSignature(n)
	name := n.Str("name")
	args := n.Child("args")
	params := args.Children("args")

	// Go signature header.
	var goParams bytes.Buffer
	for i, p := range params {
		if i > 0 {
			goParams.WriteString(", ")
		}
		fmt.Fprintf(&goParams, "%s %s", p.Str("arg"), sig.Params[i].Go())
	}
	retPart := ""
	if _, ok := sig.Return.(types.TNone); !ok {
		retPart = " " + sig.Return.Go()
	}
	fmt.Fprintf(&g.out, "func %s(%s)%s {\n", name, goParams.String(), retPart)

	fnScope := newScope(g.scope)
	for i, p := range params {
		fnScope.names[p.Str("arg")] = sig.Params[i]
	}
	saved := g.scope
	g.scope = fnScope
	// Stash current expected return type so Return can check.
	g.returnType = sig.Return

	for _, s := range n.Children("body") {
		if err := g.emitStmt(s, "\t"); err != nil {
			g.scope = saved
			return err
		}
	}
	g.scope = saved
	fmt.Fprintf(&g.out, "}\n\n")
	return nil
}

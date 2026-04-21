// Package pyast is the Python source frontend for gopygo. Parsing is
// delegated to python3.14 itself: a small helper script invokes
// ast.parse and emits a compact JSON tree that Go decodes here.
//
// The Node type is a thin wrapper around the decoded map so the
// generator can walk it without a hand-written struct per AST node.
// Accessors on Node panic rather than return errors; the generator
// is the sole caller and the JSON shape is stable across a run.
package pyast

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

//go:embed helper.py
var helperPy []byte

// Node is one AST node. The "_t" key holds the Python class name
// (e.g. "Assign", "FunctionDef"). Other keys mirror the node's
// _fields attribute; "lineno" / "col" are present on most nodes.
type Node map[string]any

// Parse runs the embedded helper against src on disk and returns the
// root Module node.
func Parse(path string) (Node, error) {
	tmp, err := os.CreateTemp("", "gopygo-helper-*.py")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(helperPy); err != nil {
		tmp.Close()
		return nil, err
	}
	tmp.Close()

	cmd := exec.Command("python3.14", tmp.Name(), path)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("python3.14 ast parse failed: %w", err)
	}
	var root any
	if err := json.Unmarshal(out, &root); err != nil {
		return nil, fmt.Errorf("decode ast json: %w", err)
	}
	m, ok := root.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("ast json root is not an object")
	}
	return Node(m), nil
}

// Type returns the "_t" tag.
func (n Node) Type() string { s, _ := n["_t"].(string); return s }

// Line returns the 1-based source line if known.
func (n Node) Line() int {
	if v, ok := n["lineno"]; ok {
		if f, ok := v.(float64); ok {
			return int(f)
		}
	}
	return 0
}

// Col returns the 0-based source column if known.
func (n Node) Col() int {
	if v, ok := n["col"]; ok {
		if f, ok := v.(float64); ok {
			return int(f)
		}
	}
	return 0
}

// Child returns a child node by field name, or nil if absent.
func (n Node) Child(field string) Node {
	v, ok := n[field]
	if !ok || v == nil {
		return nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	return Node(m)
}

// Children returns a list-valued field as a []Node.
func (n Node) Children(field string) []Node {
	v, ok := n[field]
	if !ok || v == nil {
		return nil
	}
	xs, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]Node, 0, len(xs))
	for _, e := range xs {
		if m, ok := e.(map[string]any); ok {
			out = append(out, Node(m))
		}
	}
	return out
}

// Str returns a string-valued field.
func (n Node) Str(field string) string {
	v, ok := n[field]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// Raw returns a field value untouched. Used for Constant.value which
// can be a string, number, bool, or nil.
func (n Node) Raw(field string) any { return n[field] }

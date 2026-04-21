// Package pyast parses Python 3.14 source by shelling out to
// CPython. The embedded helper script walks ast.parse's output and
// emits a compact JSON tree; this package decodes that into a
// generic Node map that the rest of gopygo walks.
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

// Node is one AST node. The "_t" key carries the Python class name.
type Node map[string]any

// Parse runs the embedded helper against path and returns the root
// Module node.
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

func (n Node) Type() string { s, _ := n["_t"].(string); return s }

func (n Node) Line() int {
	if v, ok := n["lineno"]; ok {
		if f, ok := v.(float64); ok {
			return int(f)
		}
	}
	return 0
}

func (n Node) Col() int {
	if v, ok := n["col"]; ok {
		if f, ok := v.(float64); ok {
			return int(f)
		}
	}
	return 0
}

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

func (n Node) Str(field string) string {
	v, ok := n[field]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

func (n Node) Raw(field string) any { return n[field] }

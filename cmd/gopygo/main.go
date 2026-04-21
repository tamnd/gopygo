// gopygo transpiles a Python source file to Go. The emitted Go
// imports only the standard library; there is no gopygo runtime.
package main

import (
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/tamnd/gopygo/gen"
	"github.com/tamnd/gopygo/pyast"
)

const version = "0.3.0"

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
    gopygo transpile <in.py> -o <out.go> [-pkg main]
    gopygo run       <in.py>
    gopygo version`)
	os.Exit(2)
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "version":
		fmt.Println("gopygo", version)
	case "transpile":
		cmdTranspile(os.Args[2:])
	case "run":
		cmdRun(os.Args[2:])
	default:
		usage()
	}
}

func cmdTranspile(args []string) {
	in, out, pkg := "", "", "main"
	i := 0
	for i < len(args) {
		a := args[i]
		switch a {
		case "-o":
			i++
			if i >= len(args) {
				usage()
			}
			out = args[i]
		case "-pkg":
			i++
			if i >= len(args) {
				usage()
			}
			pkg = args[i]
		default:
			if in != "" {
				usage()
			}
			in = a
		}
		i++
	}
	if in == "" || out == "" {
		usage()
	}
	src, err := transpile(in, pkg)
	if err != nil {
		die(err)
	}
	if err := os.WriteFile(out, src, 0o644); err != nil {
		die(err)
	}
}

func cmdRun(args []string) {
	if len(args) != 1 {
		usage()
	}
	src, err := transpile(args[0], "main")
	if err != nil {
		die(err)
	}
	dir, err := os.MkdirTemp("", "gopygo-run-*")
	if err != nil {
		die(err)
	}
	defer os.RemoveAll(dir)
	goFile := filepath.Join(dir, "main.go")
	if err := os.WriteFile(goFile, src, 0o644); err != nil {
		die(err)
	}
	cmd := exec.Command("go", "run", goFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.Exit(1)
	}
}

func transpile(path, pkg string) ([]byte, error) {
	mod, err := pyast.Parse(path)
	if err != nil {
		return nil, err
	}
	raw, err := gen.Compile(mod, pkg)
	if err != nil {
		return nil, err
	}
	pretty, err := format.Source(raw)
	if err != nil {
		// Return the raw source so the user can see what failed.
		return raw, fmt.Errorf("gofmt: %w\n--- emitted ---\n%s", err, raw)
	}
	return pretty, nil
}

func die(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

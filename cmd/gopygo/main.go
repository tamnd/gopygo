// Command gopygo transpiles Python 3.14 source into Go source and
// optionally builds & runs the result. Parsing is delegated to
// python3.14's ast module via pyast; code generation is in gen.
package main

import (
	"flag"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/tamnd/gopygo/gen"
	"github.com/tamnd/gopygo/pyast"
)

const version = "0.2.0-py"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "transpile":
		if err := cmdTranspile(os.Args[2:]); err != nil {
			die(err)
		}
	case "run":
		if err := cmdRun(os.Args[2:]); err != nil {
			die(err)
		}
	case "version":
		fmt.Println(version)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: gopygo transpile <input.py> -o <out.go> [--pkg main]")
	fmt.Fprintln(os.Stderr, "       gopygo run <input.py>")
	fmt.Fprintln(os.Stderr, "       gopygo version")
}

func die(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func cmdTranspile(args []string) error {
	fs := flag.NewFlagSet("transpile", flag.ExitOnError)
	out := fs.String("o", "", "output .go file")
	pkg := fs.String("pkg", "main", "package name for the output")
	fs.Parse(args)
	if fs.NArg() != 1 || *out == "" {
		return fmt.Errorf("transpile: need input and -o")
	}
	src := fs.Arg(0)
	code, err := transpileFile(src, *pkg)
	if err != nil {
		return err
	}
	return os.WriteFile(*out, code, 0o644)
}

func transpileFile(path, pkg string) ([]byte, error) {
	tree, err := pyast.Parse(path)
	if err != nil {
		return nil, err
	}
	raw, err := gen.Compile(tree, pkg)
	if err != nil {
		return nil, err
	}
	formatted, ferr := format.Source(raw)
	if ferr != nil {
		// Emit the unformatted source so the user can see the bug.
		fmt.Fprintln(os.Stderr, "gopygo: generated code failed gofmt:", ferr)
		return raw, nil
	}
	return formatted, nil
}

func cmdRun(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("run: need exactly one .py file")
	}
	src := args[0]
	code, err := transpileFile(src, "main")
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "gopygo-run-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	if err := os.WriteFile(filepath.Join(dir, "main.go"), code, 0o644); err != nil {
		return err
	}

	gopygoSrc := os.Getenv("GOPYGO_SRC")
	if gopygoSrc == "" {
		// Fall back to the directory of this binary's source during `go run`.
		// Resolved relative to module root via the caller's cwd.
		cwd, _ := os.Getwd()
		gopygoSrc = cwd
	}
	gomod := fmt.Sprintf(`module gopygorun

go 1.26

require github.com/tamnd/gopygo v0.0.0
replace github.com/tamnd/gopygo => %s
`, gopygoSrc)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644); err != nil {
		return err
	}

	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

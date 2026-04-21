// Command gopygo transpiles a CPython 3.14 .pyc into a standalone
// Go program and either writes the source (`transpile`) or compiles
// and runs it (`run`).
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/tamnd/gopygo/pyc"
	"github.com/tamnd/gopygo/transpile"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "transpile":
		doTranspile(os.Args[2:])
	case "run":
		doRun(os.Args[2:])
	case "version":
		fmt.Println("gopygo v0.1")
	case "-h", "--help", "help":
		usage()
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  gopygo transpile <input.pyc> -o <output.go> [--pkg main]")
	fmt.Fprintln(os.Stderr, "  gopygo run <input.pyc>")
	fmt.Fprintln(os.Stderr, "  gopygo version")
}

func doTranspile(args []string) {
	if len(args) < 1 {
		usage()
		os.Exit(2)
	}
	input := args[0]
	output := ""
	pkg := "main"
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "-o":
			i++
			if i >= len(args) {
				usage()
				os.Exit(2)
			}
			output = args[i]
		case "--pkg":
			i++
			if i >= len(args) {
				usage()
				os.Exit(2)
			}
			pkg = args[i]
		default:
			fmt.Fprintln(os.Stderr, "unknown flag:", args[i])
			os.Exit(2)
		}
	}
	code, err := pyc.LoadPyc(input)
	if err != nil {
		die("load:", err)
	}
	src, err := transpile.Compile(code, pkg)
	if err != nil {
		die("transpile:", err)
	}
	if output == "" {
		os.Stdout.Write(src)
		return
	}
	if err := os.WriteFile(output, src, 0o644); err != nil {
		die("write:", err)
	}
}

func doRun(args []string) {
	if len(args) < 1 {
		usage()
		os.Exit(2)
	}
	input := args[0]
	code, err := pyc.LoadPyc(input)
	if err != nil {
		die("load:", err)
	}
	src, err := transpile.Compile(code, "main")
	if err != nil {
		die("transpile:", err)
	}
	dir, err := os.MkdirTemp("", "gopygo-run-*")
	if err != nil {
		die("tmp:", err)
	}
	defer os.RemoveAll(dir)

	mainFile := filepath.Join(dir, "main.go")
	if err := os.WriteFile(mainFile, src, 0o644); err != nil {
		die("write:", err)
	}
	// Write a go.mod that inherits the gopygo runtime via replace if
	// we are running from inside the gopygo checkout, otherwise rely
	// on the network resolve.
	mod := "module gopygorun\n\ngo 1.26\n\nrequire github.com/tamnd/gopygo v0.0.0\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644); err != nil {
		die("write go.mod:", err)
	}
	// Use replace directive pointing at GOPYGO_SRC (test harness sets
	// this to the checkout root).
	if src := os.Getenv("GOPYGO_SRC"); src != "" {
		abs, _ := filepath.Abs(src)
		appendReplace(filepath.Join(dir, "go.mod"), abs)
		if err := runCmd(dir, "go", "mod", "tidy"); err != nil {
			die("tidy:", err)
		}
	}
	if err := runCmd(dir, "go", "run", "."); err != nil {
		die("run:", err)
	}
}

func appendReplace(path, target string) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		die("append:", err)
	}
	defer f.Close()
	fmt.Fprintf(f, "\nreplace github.com/tamnd/gopygo => %s\n", target)
}

func runCmd(dir, bin string, args ...string) error {
	c := exec.Command(bin, args...)
	c.Dir = dir
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func die(ctx string, err error) {
	fmt.Fprintln(os.Stderr, ctx, err)
	os.Exit(1)
}

# gopygo

Transpile Python 3.14 source into standalone Go programs.

`gopygo` reads a `.py` file, parses it with CPython's own `ast`
module, emits a `.go` file whose block structure mirrors the
original Python, and hands it to the Go toolchain. The resulting
binary reproduces the program's observable behaviour with no Python
interpreter involved at runtime. No cgo, no libpython.

## Quick start

```bash
cat > hello.py <<'EOF'
print("hi from gopygo")
EOF

go run ./cmd/gopygo run hello.py
```

Expected output:

```
hi from gopygo
```

To write the generated Go to disk instead of running it:

```bash
go run ./cmd/gopygo transpile -o hello.go hello.py
```

## How it works

Parsing is delegated to CPython: a small embedded helper invokes
`ast.parse` and hands the tree back as JSON. Go walks that tree and
emits one Go `func` per Python `def` plus a `main()` for module-
level code. Python names stay Go names. Python `if`/`while`/`for`
become Go `if`/`for` loops over a runtime iterator. Fallible
operations (`a + b`, `f(x)`, `a < b`) are wrapped in `rt.Must` so
they slot into any Go expression position and panic with a
Python-style error on failure; the generated `main` recovers and
exits non-zero.

Every generated program imports `rt
"github.com/tamnd/gopygo/runtime"`, which supplies `rt.Value`, the
arithmetic, comparison, iteration, and built-in call surface.

You can see the output shape by reading `tests/fixtures/*.go` — each
fixture has its generated Go checked in next to the `.py` (under
`//go:build ignore` so it does not interfere with `go build ./...`).

## Requirements

- Go 1.26
- CPython 3.14 on PATH (the frontend shells to `python3.14` for
  parsing; the test harness compares stdout against it)

## What works today

v0.1 of the source transpiler, verified against CPython 3.14 on the
fixtures under `tests/fixtures/`.

| Area | Status |
|---|---|
| `int` (arbitrary precision via `math/big`), `float`, `bool`, `str`, `None` | works |
| Arithmetic: `+ - * / // % **`, unary `-`, unary `not` | works |
| Comparisons including chained (`a < b < c`), `and` / `or`, `is` / `is not`, `in` / `not in` | works |
| Control flow: `if` / `elif` / `else`, `while`, `for`, `break`, `continue` | works |
| Functions, positional args, recursion, mutual calls | works |
| Module-level names and cross-function lookup via `globals` + builtins | works |
| Tuples, lists, dicts (literal, subscript, assignment, `in`) | works |
| f-strings (no format spec) | works |
| Built-ins: `print`, `range`, `len`, `abs`, `min`, `max`, `str`, `int`, `bool`, `list` | works |
| Classes, inheritance, `__init__`, `super` | not yet |
| Generators, `yield`, `async` / `await`, `with` | not yet |
| Exceptions, `try` / `except` / `finally` | not yet |
| Decorators, `*args`, `**kwargs`, defaults, closures over locals | not yet |
| `import` of user modules | not yet |
| C extensions | out of scope forever |

Any unsupported AST node is a hard transpile-time error with a
source location. No silent fallback to an interpreter.

## Tests

```bash
go test ./runtime/    # runtime unit tests
tests/run.sh          # end-to-end fixtures: python3.14 vs gopygo stdout diff
```

`tests/run.sh` iterates every `NN_topic.py` under `tests/fixtures/`,
transpiles it, runs the resulting binary, and diffs stdout
byte-for-byte against `python3.14 NN_topic.py`. Any diff fails.

## Project layout

- `pyast/` is the frontend: an embedded Python helper that runs
  `ast.parse` and emits JSON, plus a Go decoder and generic `Node`
  walker.
- `gen/` walks the AST and emits Go source.
- `runtime/` is the support library every generated program imports.
  `rt.Value`, arithmetic, comparisons, iteration, built-ins.
- `cmd/gopygo/` is the CLI with `transpile`, `run`, and `version`
  subcommands.
- `tests/fixtures/` is the numbered fixture corpus with checked-in
  transpiler output.

## Relationship to goipy

[`goipy`](https://github.com/tamnd/goipy) is an interpreter for the
same bytecode dialect; it walks `.pyc` one op at a time. gopygo
takes the opposite bet: work from source, emit Go ahead of time,
keep the output readable. Use goipy when you want to load new Python
at runtime without recompiling. Use gopygo when you want Python
logic as part of a Go build.

## License

MIT.

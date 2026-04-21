# gopygo

Transpile CPython 3.14 `.pyc` files into standalone Go programs.

`gopygo` reads a `.pyc` produced by `python3.14 -m py_compile`, emits a `.go` source file, and hands it to the Go toolchain. The resulting binary reproduces the original program's observable behaviour with no Python interpreter involved at runtime. No cgo, no libpython, no JIT.

## Quick start

```bash
cat > hello.py <<'EOF'
print("hi from gopygo")
EOF

python3.14 -m py_compile hello.py
go run ./cmd/gopygo run __pycache__/hello.cpython-314.pyc
```

Expected output:

```
hi from gopygo
```

To write the generated Go to disk instead of running it:

```bash
go run ./cmd/gopygo transpile __pycache__/hello.cpython-314.pyc -o hello.go
```

## How it works

`gopygo` translates each Python code object to one Go function. Python locals become Go variables; the evaluation stack becomes a fixed-size Go array sized from CPython's `co_stacksize`; bytecode jumps become `goto` statements between labels. Built-in operations delegate to the `runtime` package (`rt.Add`, `rt.Compare`, `rt.Call`, ...), which is a small support library that every generated program imports.

## What works today

v0.1, verified against CPython 3.14.4 on the fixtures under `tests/fixtures/`.

| Area | Status |
|---|---|
| `int` (arbitrary precision via `math/big`), `float`, `bool`, `str`, `None` | works |
| Arithmetic: `+`, `-`, `*`, `/`, `//`, `%`, unary `-`, unary `not` | works |
| Comparisons: `<`, `<=`, `==`, `!=`, `>`, `>=` | works |
| Control flow: `if`/`else`, `while`, `for i in range(N)`, simple f-strings | works |
| Functions, positional args, recursion, calls across functions | works |
| Module-level `STORE_NAME` / `LOAD_NAME` | works |
| Built-ins: `print`, `range`, `len`, `abs`, `True`, `False`, `None` | works |
| Classes, closures, `*args`/`**kwargs`, defaults | not yet |
| Generators, `async`/`await`, `with` | not yet |
| Exceptions, `try`/`except`/`finally` | not yet |
| `import` of anything other than the entry file | not yet |
| C extensions | out of scope forever |

Encountering any opcode outside the v0.1 set is a hard transpile-time error naming the opcode and offset. No silent fallback to an interpreter.

## Tests

```bash
go test ./runtime/        # runtime unit tests
tests/run.sh              # end-to-end fixtures: python3.14 vs gopygo stdout diff
```

`tests/run.sh` compiles every `.py` under `tests/fixtures/` with `python3.14 -m py_compile`, transpiles with gopygo, builds and runs the result, and diffs stdout byte-for-byte against `python3.14 fixture.py`. Any diff fails.

## Project layout

- `pyc/` reads `.pyc` files (marshal decoder) and holds the CPython 3.14 opcode table.
- `runtime/` is the support library every generated program imports. `rt.Value`, `rt.Int`, arithmetic, comparisons, built-ins.
- `transpile/` walks a `pyc.Code` and emits Go source.
- `cmd/gopygo/` is the CLI with `transpile`, `run`, and `version` subcommands.
- `tests/fixtures/` is the golden-output test corpus.

## Relationship to goipy

[`goipy`](https://github.com/tamnd/goipy) is an interpreter for the same bytecode; it shares the interpretation loop but not the transpilation strategy. Use goipy when you want to load new `.pyc` at runtime without recompiling the host binary. Use gopygo when you want to ship Python logic as part of your Go build.

## License

MIT. Bytecode input produced by CPython remains under the PSF license.

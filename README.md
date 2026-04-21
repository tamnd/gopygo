# gopygo

Transpile a typed subset of Python to Go. The emitted Go imports
only the Go standard library — there is no gopygo runtime package
to link against.

## How it works

1. `pyast` shells to CPython 3.14 to run `ast.parse` and reads back a
   compact JSON tree.
2. `types` is a small type lattice (`int`, `float`, `bool`, `str`,
   `list[T]`, `dict[K,V]`, `tuple[...]`, `func(...) -> T`).
3. `gen` walks the AST in a fused inference + emission pass. Function
   signatures come from PEP 484 annotations; inside function bodies
   and at module scope, local types are inferred from initializers.
   Numeric widening matches Python (`int + float → float`,
   `/` always returns float).

## Usage

```
gopygo transpile hello.py -o hello.go
gopygo run hello.py
gopygo version
```

## Subset supported (v0.3)

- Module-level code and top-level `def` (no classes, no nested defs).
- Function parameters and returns must be annotated.
- `int` → `int64`, `float` → `float64`, `bool`, `str`, `list[T]`,
  `dict[K,V]`.
- Arithmetic, comparisons, boolean ops, f-strings.
- `if/elif/else`, `while`, `for x in range(...)`, `for x in seq`.
- Builtins: `print`, `len`, `abs`, `min`, `max`, `int`, `float`,
  `str`, `bool`, `range`.

Anything outside the subset is a transpile-time error with a Python
source location.

## Tests

```
./tests/run.sh
```

Each fixture runs under `python3.14` and under `gopygo run`; outputs
must match byte-for-byte. The runner also fails if any emitted Go
source imports the `github.com/tamnd/gopygo` module.

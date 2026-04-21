<h1 align="center">gopygo</h1>

<p align="center">
  <b>Typed Python 3 &rarr; idiomatic, stdlib-only Go.</b><br>
  <sub>Source-to-source. No runtime. The output reads like Go you would write by hand.</sub>
</p>

<p align="center">
  <a href="#quick-start">Quick start</a> ·
  <a href="#example">Example</a> ·
  <a href="#supported-subset">Subset</a> ·
  <a href="ARCHITECTURE.md">Architecture</a> ·
  <a href="#faq">FAQ</a>
</p>

---

```python
def add(a: int, b: int) -> int:
    return a + b

print(add(2, 3))
```

```go
func add(a int64, b int64) int64 {
    return (a + b)
}

func main() {
    pyPrintln(add(2, 3))
}
```

Concrete types. Real Go signatures. `fmt` / `strconv` / `strings` only.
Drop it into any Go module.

## Why

Other Python-to-Go tools pick one of two paths: a boxed-value
runtime that runs everything but reads like assembly, or a narrow
compiler that falls back to a runtime for the rest. gopygo takes
a third path. Annotate your Python with PEP 484, and gopygo emits
Go you would be comfortable merging into a real codebase.

The rule is strict: every value has a concrete type. When
inference stalls, gopygo stops at the Python line and tells you
which annotation to add. Programs already written with `mypy`
in the loop tend to fit as-is.

## Quick start

Requires **Go 1.26** and **Python 3.14** on `PATH`.

```sh
git clone https://github.com/tamnd/gopygo
cd gopygo
go build -o gopygo ./cmd/gopygo

./gopygo transpile hello.py -o hello.go   # emit Go
./gopygo run hello.py                     # transpile and go run
./gopygo version
```

Run the snapshot suite:

```sh
./tests/run.sh
```

Every fixture transpiles, executes under CPython and under the
emitted Go, and byte-diffs the two outputs. A grep guard fails the
suite if any emitted Go imports `github.com/tamnd/gopygo` &mdash;
that is how the stdlib-only promise stays honest.

## Example

`hello.py`:

```python
def greet(name: str) -> str:
    return "hello, " + name

for who in ["alice", "bob"]:
    print(greet(who))
```

`gopygo transpile hello.py -o hello.go`:

```go
package main

import (
    "fmt"
    "strconv"
    "strings"
)

func greet(name string) string {
    return "hello, " + name
}

func main() {
    for _, who := range []string{"alice", "bob"} {
        pyPrintln(greet(who))
    }
}

// pyRepr, pyPrintln helpers emitted once per program, only when print is used.
```

More fixtures live under [`tests/fixtures/`](tests/fixtures/) &mdash;
each one ships the `.py`, the expected `.go`, and the expected stdout.

## Supported subset

| Area                   | Supported                                                    |
|------------------------|--------------------------------------------------------------|
| Types in annotations   | `int`, `float`, `bool`, `str`, `None`, `Any`, `list[T]`, `dict[K,V]`, `tuple[...]` |
| Functions              | Top-level `def` with PEP 484 parameters and return type      |
| Expressions            | Arithmetic, comparisons, bool ops, subscript, f-strings, ternary, list/dict literals |
| Statements             | `if`/`elif`/`else`, `while`, `for` over `range`/list/str/dict, `return`/`break`/`continue`/`pass`, augmented assignment |
| Builtins               | `print`, `len`, `abs`, `min`, `max`, `int`, `float`, `str`, `bool`, `range` |
| Inference              | First-assignment declares; numeric widening; reassignments typecheck |
| Classes / exceptions / generators / imports / `async` | Out of scope &mdash; rejected at transpile time with a line number |

The [architecture doc](ARCHITECTURE.md#the-supported-subset) has
the full emission reference and diagnostics catalogue.

## Project layout

```
cmd/gopygo/   CLI: transpile, run, version
pyast/        CPython parser bridge and JSON AST wrapper
types/        Type lattice and Python-to-Go mapping
gen/          Fused inference and emission
tests/        Fixtures (.py/.go/.expected.txt) and snapshot runner
```

A few thousand lines total. Readable in an afternoon.

## FAQ

**Can I use `any` to paper over an unknown type?** Only by
annotating with `typing.Any`. Inference stays concrete, by design.

**Why `int64` instead of `int`?** Python ints are unbounded; Go's
`int` is platform-dependent. `int64` sets an explicit upper bound
that stays stable across 32- and 64-bit builds.

**Why route `print` through a helper?** Python formats `True` and
`3.0` differently from Go's `fmt.Println`. The `pyPrintln` helper
keeps stdout byte-identical to CPython, which is what makes the
snapshot tests work.

**Why CPython for parsing?** Reusing the reference parser keeps
gopygo honest as Python evolves and frees the project to focus on
typing and emission.

**Does it work on Windows?** Untested. `pyast` shells `python3.14`;
if that is on `PATH` it should work, but fixture snapshots use LF
line endings.

See [ARCHITECTURE.md &sect; FAQ](ARCHITECTURE.md#faq) for the full list.

## Learn more

- [ARCHITECTURE.md](ARCHITECTURE.md) &mdash; pipeline stages, type
  lattice, inference rules, design trade-offs, emission reference.
- [`tests/fixtures/`](tests/fixtures/) &mdash; every feature exercised
  as a runnable triplet.
- Releases [v0.1](https://github.com/tamnd/gopygo/releases/tag/v0.1),
  [v0.2](https://github.com/tamnd/gopygo/releases/tag/v0.2),
  [v0.3](https://github.com/tamnd/gopygo/releases/tag/v0.3) &mdash;
  the path from bytecode dispatch to typed source translation.

## License

MIT. See [LICENSE](LICENSE).

# gopygo

A source-to-source compiler that turns a typed subset of Python 3 into
readable, stdlib-only Go. No runtime library. No boxed values. No
reflection. If the type system cannot pin a concrete Go type on every
value in your program, the compiler refuses to emit code and points at
the Python line that defeated it.

This document is long on purpose. It is equal parts tour, tutorial,
reference, and design diary. If you only want to try the tool, jump to
[Quick start](#quick-start). Everything after that exists so a reader
who has never seen the project can understand not just *what* the code
does, but *why* it does it that way.

---

## Table of contents

1. [Motivation](#1-motivation)
2. [What gopygo is and is not](#2-what-gopygo-is-and-is-not)
3. [Quick start](#3-quick-start)
4. [A worked example](#4-a-worked-example)
5. [Architecture overview](#5-architecture-overview)
6. [The pipeline, stage by stage](#6-the-pipeline-stage-by-stage)
   1. [Stage 1: parsing with CPython](#61-stage-1-parsing-with-cpython)
   2. [Stage 2: the type lattice](#62-stage-2-the-type-lattice)
   3. [Stage 3: fused inference and emission](#63-stage-3-fused-inference-and-emission)
   4. [Stage 4: formatting and optional execution](#64-stage-4-formatting-and-optional-execution)
7. [The supported subset](#7-the-supported-subset)
8. [Emission reference](#8-emission-reference)
9. [Diagnostics](#9-diagnostics)
10. [Testing philosophy](#10-testing-philosophy)
11. [Design decisions and trade-offs](#11-design-decisions-and-trade-offs)
12. [What is missing and why](#12-what-is-missing-and-why)
13. [Extending gopygo](#13-extending-gopygo)
14. [Project layout](#14-project-layout)
15. [FAQ](#15-faq)
16. [A short history of the project](#16-a-short-history-of-the-project)
17. [License](#17-license)

---

## 1. Motivation

Most "Python to Go" projects fall into one of two buckets.

The first bucket treats Go as an interpreter target. They emit Go that
pulls in a runtime library, then represents every Python value as a
boxed struct with a type tag. Integers become pointers. Lists become
doubly-indirected slices of interface values. Calls dispatch through a
vtable. The output runs, sort of, but it is neither readable as Go nor
fast. If you open the generated source you see a wall of
`rt.Call(rt.GetAttr(...))` and shrug.

The second bucket is pragmatic and narrow. Tools like Grumpy (which
Google eventually archived) picked a hybrid: compile what you can,
fall back to a runtime for the rest. That works. It is also not what
you want when your goal is to read the generated code and believe it.

gopygo belongs to a third bucket: **typed ahead-of-time translation,
no runtime**. It gives up on translating every Python program. In
exchange, the programs it *does* translate become real Go: concrete
types, ordinary function signatures, `int64`s and `float64`s and
`map[string]int64`s, imports from the Go standard library only. You
can read it. You can debug it. You can link it into another Go binary
the same way you link anything else.

The constraint that makes this possible is simple: **every value must
have a concrete type the compiler can infer at compile time**. Where
Python is flexible, gopygo is strict. Function parameters and returns
need PEP 484 annotations. Local variables get their type from their
first assignment and keep it. A name that could be two different types
along two branches is an error, not an `any`. The transpiler will
never paper over ambiguity by reaching for Go's `any`; it refuses, and
tells you the Python line to fix.

This sounds harsh. In practice it is not. Scripts that use Python as a
typed language, the kind a working engineer writes with `mypy` in the
loop, are already within the subset. Everything else stays in Python.

## 2. What gopygo is and is not

**gopygo is:**

- A source-to-source compiler from a typed subset of Python 3 to Go.
- A frontend (the Python `ast` module) plus a small type system plus a
  single emission pass.
- A tool for moving small, well-typed Python programs into Go
  codebases when you want the result to look like hand-written Go.

**gopygo is not:**

- A Python VM in Go. There is no interpreter here. The runtime is Go's
  runtime; there is no gopygo-specific one.
- A complete Python implementation. Classes, decorators, generators,
  exceptions, imports, metaclasses, async, and the rest of the wide
  dynamic surface area are deliberately out of scope.
- A one-click porting tool. If your Python program uses the full
  language, gopygo will fail on almost every file. That is the
  expected behaviour.

Think of it the way you think of `mypyc` or `shedskin`: a restricted
dialect that gives up expressivity in exchange for guarantees on the
output.

## 3. Quick start

You need Go 1.26 and Python 3.14 on your `PATH`.

```sh
git clone https://github.com/tamnd/gopygo
cd gopygo
go build -o gopygo ./cmd/gopygo

# Translate a single .py file to a .go file.
./gopygo transpile hello.py -o hello.go

# Or run it end-to-end (writes Go to a temp dir, shells `go run`).
./gopygo run hello.py

# Print the version.
./gopygo version
```

The CLI is intentionally minimal. Three verbs, no config file, no
plugin system.

Run the test suite:

```sh
./tests/run.sh
```

This transpiles every fixture, executes both the Python original and
the generated Go, and diffs their output byte-for-byte. It also greps
the emitted Go to confirm that no fixture imports the gopygo module,
which is the guard-rail that keeps the "no runtime" promise honest.

## 4. A worked example

Start with a tiny Python file:

```python
# hello.py
def add(a: int, b: int) -> int:
    return a + b


def greet(name: str) -> str:
    return "hello, " + name


print(add(2, 3))
print(greet("alice"))
```

Run `gopygo transpile hello.py -o hello.go` and you get:

```go
// Code generated by gopygo. DO NOT EDIT.

package main

import (
    "fmt"
    "strconv"
    "strings"
)

func add(a int64, b int64) int64 {
    return (a + b)
}

func greet(name string) string {
    return "hello, " + name
}

func main() {
    pyPrintln(add(2, 3))
    pyPrintln(greet("alice"))
}

func pyRepr(v any) string {
    switch x := v.(type) {
    case bool:
        if x {
            return "True"
        }
        return "False"
    case float64:
        if x == float64(int64(x)) {
            return strconv.FormatFloat(x, 'f', 1, 64)
        }
        return strconv.FormatFloat(x, 'g', -1, 64)
    case string:
        return x
    }
    return fmt.Sprint(v)
}

func pyPrintln(args ...any) {
    parts := make([]string, len(args))
    for i, a := range args {
        parts[i] = pyRepr(a)
    }
    fmt.Println(strings.Join(parts, " "))
}
```

Notice what is *not* there. No import from `github.com/tamnd/...`. No
reflection. No interface{} holding a Python object. `add` is a
function from `(int64, int64)` to `int64`; that is the actual Go
signature, not a facade. `greet` takes and returns a `string`. The
`pyPrintln` and `pyRepr` helpers exist because `print` in Python
renders `True` and `3.0` differently from how Go's `fmt.Println` would
by default; the helpers are plain Go, emitted once per program, and
only when `print` is actually called.

A consumer who reads this file does not need to know anything about
gopygo. It is Go.

## 5. Architecture overview

The project has four packages and a CLI:

```
pyast/   -> parse Python source into a generic AST tree
types/   -> gopygo's type lattice and the Python <-> Go mapping
gen/     -> fused inference and emission
cmd/     -> the gopygo binary (transpile / run / version)
tests/   -> fixtures and the snapshot-diff runner
```

The data flow is linear:

```
hello.py
   |
   v   pyast.Parse
tree of map[string]any nodes
   |
   v   gen.Compile
[]byte Go source
   |
   v   go/format.Source
formatted Go source
   |
   v   -o hello.go   or   go run
```

There are no cross-cutting concerns. There is no plugin mechanism.
Each stage has a single job and a single entry point.

## 6. The pipeline, stage by stage

### 6.1 Stage 1: parsing with CPython

gopygo does not reimplement a Python parser. It shells out to the one
Python already ships. The `pyast` package embeds a tiny Python helper
script, writes it to a temp file, invokes `python3.14` on the user's
input, and reads back a JSON representation of the AST.

Why CPython? Two reasons. First, correctness: the only implementation
that agrees with the Python language specification on every edge case
of the grammar is CPython itself. Reimplementing the parser in Go
would mean tracking Python's evolving syntax forever. Second, scope:
the parser is not where the interesting work lives. The interesting
work is the typing and emission stages. Offloading parsing to CPython
lets the rest of the codebase stay small.

The helper walks the `ast.AST` tree and dumps a compact dictionary
form. For every node it records the class name under the key `_t`, all
fields declared by the class, and `lineno` / `col` when present. For
`ast.Constant` specifically it also records a `_vkind` of `int`,
`float`, `bool`, `str`, or `none`. That extra tag exists because
`json.dump` turns both `3` and `3.0` into the same JSON number, and
Go's default `json.Unmarshal` into `any` lifts every number to
`float64`. Without `_vkind` the Go side cannot tell an int literal
from a float literal, and picking the wrong one changes the emitted
code.

On the Go side, `pyast.Node` is a thin wrapper around
`map[string]any`. It exposes `Type()`, `Line()`, `Col()`, and
accessors for fields (`Str`, `Child`, `Children`, `Raw`). The rest of
gopygo talks to the AST through this interface; it never cares about
the JSON shape directly.

### 6.2 Stage 2: the type lattice

The `types` package defines the set of types gopygo can reason about.
It is small on purpose.

| Python                    | gopygo internal | Go                 |
|---------------------------|-----------------|--------------------|
| `int`                     | `TInt`          | `int64`            |
| `float`                   | `TFloat`        | `float64`          |
| `bool`                    | `TBool`         | `bool`             |
| `str`                     | `TStr`          | `string`           |
| `None`                    | `TNone`         | `struct{}`         |
| `Any`                     | `TAny`          | `any`              |
| `list[T]`                 | `TList{Elem}`   | `[]T.Go()`         |
| `dict[K, V]`              | `TDict{K, V}`   | `map[K.Go()]V.Go()`|
| `tuple[T1, T2, ...]`      | `TTuple{Elems}` | a named struct (*) |
| function                  | `TFunc{...}`    | `func(...) R`      |

(*) Tuples are represented internally but not currently emitted in
expression position. The codebase exposes the type so that future work
on tuple unpacking and multi-return can use it without another
lattice change.

Each type answers two questions. `Go()` returns the Go source string
for that type, which is what the emitter splices into the output.
`String()` returns a Python-flavoured name for use in error messages,
so a diagnostic reads `"cannot compare int and str"` instead of
`"cannot compare int64 and string"`.

Two helpers do the rest of the work:

- `Equal(a, b)` is structural equality. Lists are equal if their
  element types are equal, dicts if both key and value types match,
  functions if arity and every parameter and return type match.
- `Widen(a, b)` is Python's numeric tower in three lines: if either
  side is `float`, the result is `float`; if both are `int`, the
  result is `int`; otherwise `nil`, which the emitter treats as a
  type error.

`TAny` deserves a note. It never appears from inference. The only way
a value gets typed as `Any` is if the user explicitly annotated it
with `Any` from Python's `typing` module. The transpiler will not
invent `any` to paper over an unknown. If it cannot figure out a
concrete type, it stops and tells you.

### 6.3 Stage 3: fused inference and emission

The `gen` package is where the real work happens. It walks the AST
from `Module` downward and, at each node, *simultaneously* infers the
type and writes the Go source. These two jobs are fused rather than
separated because they feed each other: the Go fragment for `a + b`
depends on the inferred types of `a` and `b` (to decide whether to
emit an `int` add, a `float` add, or a `string` concat), and the
inferred type of the whole expression depends on what you emitted.

Emission happens in three passes over the top-level statements of a
module:

1. **Signature pass.** For every top-level `def`, parse the PEP 484
   annotations into `TFunc`s and enter them into the module scope.
   This lets module-level code and other functions call them without
   forward-reference gymnastics.
2. **Body pass.** Emit each `def`'s body. Every function gets a child
   scope seeded with its parameters. The emitter stashes the declared
   return type so that `return` statements can be type-checked
   against it.
3. **main pass.** Wrap every non-`def` top-level statement in a
   synthetic `func main() { ... }`.

Inside a function or in the main body, the heart of the emitter is
two mutually recursive methods:

- `emitExpr(n) (string, types.Type, error)` for expressions: given an
  AST node, return the Go fragment, its inferred type, and any error.
- `emitStmt(s, indent) error` for statements: write Go directly to the
  output buffer, updating the current scope as names are introduced.

The scope is a classic linked list of maps. Lookup walks parent
scopes; declarations write to the current scope. Each function body,
`if` branch, `while` body, and `for` body gets its own scope *for new
declarations*, but lookups still walk outward. That is what makes

```python
total = 0
for x in xs:
    total = total + x
```

compile to a Go for-loop that reassigns the outer `total` instead of
shadowing it with a new inner variable. A naive design that only
checks the current scope's map would silently shadow and leave
`total` unused, which Go rejects.

Four inference rules are worth spelling out because they show up
everywhere:

1. **First assignment declares.** `x = 7` emits `var x int64 = 7`
   the first time; `x = 8` emits `x = 8` thereafter, with a check
   that the new value's type matches the declared type. Numeric
   widening (`int -> float`) is allowed implicitly; anything else is
   an error.
2. **`/` always floats.** Python 3's true division always returns
   float, even for two ints. gopygo emits `float64(a) / float64(b)`.
   For floor division (`//`) the result stays int if both operands
   are int; otherwise `math.Floor(a / b)` with a float result.
3. **Compare chains fan out.** `a < b < c` becomes `(a < b) && (b < c)`.
   For v0.3 the sub-expressions are simply re-emitted; there are no
   side-effecting calls inside chained compares in the supported
   subset, so double-evaluation is safe.
4. **`print` is special.** Python's `print` renders `True` as `True`
   (not `true`) and `3.0` as `3.0` (not `3`). Emitting a literal
   `fmt.Println` would drift from Python's output immediately. So
   when a program calls `print`, gopygo emits a program-local
   `pyPrintln` helper and routes the call through it. That keeps
   stdouts byte-identical between the original Python and the Go
   translation, which in turn makes the snapshot-diff tests possible.

A small number of builtins need program-local helpers for similar
reasons: `absInt` for integer absolute value, `minInt` / `maxInt` for
variadic integer min/max, `mustAtoi64` for `int("42")`. Each helper
is emitted once per program, and only when the corresponding builtin
is actually used. If your program never calls `abs`, the `absInt`
helper is not in the output.

### 6.4 Stage 4: formatting and optional execution

The emitter writes raw Go with `fmt.Fprintf`. The CLI runs the result
through `go/format.Source` before writing it to disk, so the final
output has canonical gofmt spacing and indentation. If `go/format`
rejects the emitted source, that is a bug in the emitter, not a user
problem. The CLI still writes out the unformatted source so you can
see what gopygo produced, and returns the formatter's error.

`gopygo run` does the same work but drops the Go into a temp
directory and shells `go run` on it.

## 7. The supported subset

### Top-level structure

- A module is a sequence of top-level statements.
- Top-level `def`s are functions with annotated parameters and an
  annotated return type. They become package-level Go functions.
- Every other top-level statement is collected into a synthetic
  `main` in source order.
- Classes, nested defs, `import`, `from ... import`, `try`, `with`,
  and `raise` are all rejected with a source-located error. They are
  not silently dropped.

### Types in annotations

- `int`, `float`, `bool`, `str`, `None`, `Any`.
- `list[T]`, `dict[K, V]`, `tuple[T1, T2, ...]` where each `T` is
  itself a supported annotation.
- Bare forward-referenced names (strings) are not supported.

### Expressions

- Integer, float, string, bool, and `None` literals.
- Names (local and module scope).
- Binary operators: `+`, `-`, `*`, `/`, `//`, `%`, `**`.
  - `+` on two strings is concat; on numerics it widens.
  - `/` is always float.
  - `//` on two ints is Go `/`; on floats it is `math.Floor`.
  - `%` on two ints is Go `%`; on floats it is `math.Mod`.
  - `**` is `math.Pow`, cast back to int when both operands are int.
- Unary operators: `-`, `+`, `not`.
- Boolean operators: `and`, `or`. Operands must be `bool`.
- Comparisons: `==`, `!=`, `<`, `<=`, `>`, `>=`. Chains fan out to
  `&&` as described above.
- Function and builtin calls.
- Subscript on `list[T]` (int index), `str` (int index, yields a
  one-character string), and `dict[K, V]` (typed key).
- f-strings (`JoinedStr`). Each interpolation picks a format verb
  based on the inferred type (`%d`, `%g`, `%t`, `%s`, `%v`).
- Conditional expressions (`a if cond else b`). Emitted as an inline
  closure because Go has no ternary.
- List and dict literals. Element types must agree (with numeric
  widening for lists).

### Statements

- `x = expr`: declares on first use, reassigns thereafter.
- `x: T = expr` and `x: T`: annotated assignment and declaration.
- `x += expr` and other augmented assignments.
- `expr` as a statement (typically a call).
- `if`, `elif`, `else`.
- `while cond:`.
- `for x in range(...)`: becomes a C-style Go loop. Supports the
  one-, two-, and three-argument forms. A literal negative step
  flips the comparison to `>`.
- `for x in seq:`: supports `list`, `str` (iterates one-character
  strings, not runes), and `dict` (iterates keys, like Python).
- `return`, `break`, `continue`, `pass`.

### Builtins

- `print(*args)`: Python-style, via the `pyPrintln` helper.
- `len(x)`: on `str`, `list`, `dict`. Returns `int64`.
- `abs(x)`: int or float.
- `min(*xs)`, `max(*xs)`: variadic, ints only in v0.3.
- `int(x)`: from int, float, bool, or `str` (via `strconv.ParseInt`).
- `float(x)`: from any numeric.
- `str(x)`: via `fmt.Sprint`.
- `bool(x)`: from bool, int, float, or str.
- `range(...)`: valid only as a `for` iterable.

## 8. Emission reference

This section shows, for each Python construct, the Go gopygo emits.
These are patterns, not guarantees against formatting drift.

**Annotated function:**

```python
def add(a: int, b: int) -> int:
    return a + b
```
```go
func add(a int64, b int64) int64 {
    return (a + b)
}
```

**Module-level code:**

```python
print("hi")
```
```go
func main() {
    pyPrintln("hi")
}
```

**Assignment with inference:**

```python
xs = [1, 2, 3]
xs = [4, 5]
```
```go
var xs []int64 = []int64{1, 2, 3}
xs = []int64{4, 5}
```

**for-range with a count:**

```python
for i in range(5):
    print(i)
```
```go
for i := int64(0); i < 5; i++ {
    pyPrintln(i)
}
```

**for-range with start, stop, step:**

```python
for k in range(10, 0, -2):
    print(k)
```
```go
for k := int64(10); k > 0; k += -2 {
    pyPrintln(k)
}
```

**for over a list:**

```python
total = 0
for x in xs:
    total = total + x
```
```go
var total int64 = 0
for _, x := range xs {
    total = total + x
}
```

**if / elif / else:**

```python
if n < 5:
    print("small")
elif n < 10:
    print("medium")
else:
    print("large")
```
```go
if n < 5 {
    pyPrintln("small")
} else if n < 10 {
    pyPrintln("medium")
} else {
    pyPrintln("large")
}
```

**f-string:**

```python
print(f"n = {n}, name = {name}")
```
```go
pyPrintln(fmt.Sprintf("n = %d, name = %s", n, name))
```

**Recursive function:**

```python
def fib(n: int) -> int:
    if n < 2:
        return n
    return fib(n - 1) + fib(n - 2)
```
```go
func fib(n int64) int64 {
    if n < 2 {
        return n
    }
    return (fib((n - 1)) + fib((n - 2)))
}
```

The parentheses around sub-expressions are intentional: the emitter
parenthesises around every binary operator so it never has to reason
about precedence in the generator. `gofmt` is free to drop or keep
them; in practice the output is readable either way.

## 9. Diagnostics

Every error carries a Python source location. A typical message looks
like:

```
gopygo: line 7 col 4: parameter "x" needs a type annotation
```

There are four broad error categories:

1. **Unsupported construct.** Classes, imports, `try`, etc. The fix
   is to remove the construct or keep that program in Python.
2. **Missing annotation.** Function parameters and returns must be
   annotated. Local variables are inferred; parameters are not.
3. **Type mismatch.** `return` types must match the declared return;
   reassignments must match the declared local type (with numeric
   widening); comparisons and arithmetic must have compatible
   operands.
4. **Undefined name.** Names used before they are bound.

The transpiler does not produce warnings. Every diagnostic is fatal.
A file either compiles cleanly or does not compile at all.

## 10. Testing philosophy

The test suite is a set of fixtures under `tests/fixtures/`. Each
fixture is a three-file triplet:

- `NN_topic.py`: the source program.
- `NN_topic.go`: the expected emission, pre-formatted with
  `go/format`, headed with `//go:build ignore` so `go build ./...`
  skips it.
- `NN_topic.expected.txt`: the expected stdout.

`tests/run.sh` runs each fixture three ways:

1. Transpile the `.py` and diff the output against the committed
   `.go` snapshot. If the emitter drifts, the test fails and shows
   the diff.
2. Execute the transpiled program via `gopygo run` and diff the
   output against `.expected.txt`.
3. `grep` the emitted `.go` for `github.com/tamnd/gopygo`. If any
   fixture ever imports the gopygo module the test fails. This is
   the guard-rail that keeps the "no runtime" promise honest across
   refactors.

Regenerate every snapshot with `UPDATE=1 tests/run.sh`. Review the
resulting diff in git carefully; that diff is the real specification
of what the emitter produces.

Adding a fixture is deliberately cheap. Drop three files into
`tests/fixtures/`, run `UPDATE=1 tests/run.sh`, commit.

## 11. Design decisions and trade-offs

**CPython for parsing.** A pure-Go Python parser would avoid the
python3.14 dependency, at the cost of maintaining a parser that
tracks Python's grammar forever. Not worth it for a project at this
scope. If a user cannot install Python they probably cannot develop
against the output either.

**JSON over gob or protobuf for the AST.** JSON is a degenerate
representation (it loses int vs float), but it is debuggable: you can
pipe the helper script output to `jq` and look at it. The `_vkind`
tag fixes the one place where the degeneracy bites.

**Fused inference and emission.** Separating the two would mean
building a typed IR between them. For the current feature surface,
the IR buys nothing: each AST node has a single emission shape, and
the types flow bottom-up naturally. When gopygo grows richer features
(generics emission, polymorphism beyond Python's numeric tower), a
typed IR becomes the right move.

**Scopes that nest but lookups that walk.** This is how Python
actually treats names inside `for` and `if` bodies. The alternative,
Go-style block scoping, would require hoisting every assignment to
the enclosing function which is exactly what the emitter does, by
writing to the nearest scope that already has the name.

**Program-local helpers instead of a stdlib addition.** Functions
like `pyPrintln` and `absInt` are emitted inline in each generated
file when needed. That keeps the output self-contained: you can copy
the `.go` file into a fresh Go module with only the standard library
and it still compiles.

**Parenthesise everything.** The emitter wraps every binary
expression in parentheses. That makes precedence a non-concern at
emission time and makes the output `gofmt`-stable.

**Integers are `int64`.** Not `int`. Python ints are arbitrary
precision, but within the transpiled subset they must fit in a
machine word. Picking `int64` is the honest upper bound for a
64-bit platform; picking `int` would silently change semantics when
porting between 32- and 64-bit Go.

## 12. What is missing and why

Everything in this list is deliberately out of scope for v0.3. None
of it is ruled out forever; each is missing because a correct
translation without a runtime is either hard or requires design work.

- **Classes.** Mapping Python's single-inheritance method resolution
  order and duck typing to Go's struct+interface model is a
  non-trivial design. A future version will probably accept a
  dataclass-style subset.
- **Exceptions.** `try`, `except`, and `raise` have no direct Go
  equivalent. Go's `error` values are the right shape for
  `raise ValueError(...)`-style programs, but mapping `except`
  blocks needs a full design pass.
- **Generators and `yield`.** Coroutine-backed iteration is possible
  in Go via channels or callbacks, but the choice of encoding
  matters for readability and performance.
- **Arbitrary-precision ints.** Python ints do not overflow. Go's
  `int64` does. A future version could emit `math/big.Int` on
  request; by default v0.3 accepts the overflow risk in exchange
  for clean output.
- **Imports and the Python standard library.** Every external Python
  module would need a Go mapping. The design question is whether
  users can register their own mappings or only a curated set ships.
- **Tuple unpacking in expressions.** `a, b = f()` is useful; the
  emitter has the type infrastructure (`TTuple`) but has not yet
  wired it up to unpacking assignment.
- **List, dict, and set comprehensions.** Desugarable to loops but
  not yet done.
- **Decorators and `*args` / `**kwargs`.** These are dynamic by
  nature. A reasonable subset (`@staticmethod`, fixed-arity
  forwarders) could be added later.

If you see a construct here you need, that is probably the next
thing worth adding.

## 13. Extending gopygo

The path to teaching gopygo a new feature is:

1. **Write the smallest possible fixture** under
   `tests/fixtures/NN_topic.py` that demonstrates it. Run
   `UPDATE=1 tests/run.sh` and confirm you get a reasonable
   `.go` and `.expected.txt`. The diff is your design review.
2. **If the feature needs a new type**, extend `types/types.go`
   with the new case, its `Go()` and `String()` methods, its
   membership in `Equal` and any relevant helpers.
3. **If the feature is a new expression**, add a case to
   `gen/expr.go`'s `emitExpr` dispatch and implement the emission.
   Remember that the method returns the inferred type alongside
   the Go fragment.
4. **If the feature is a new statement**, add a case to
   `gen/stmt.go`'s `emitStmt` dispatch.
5. **If the feature needs a helper in the output**, add a flag on
   the `gen` struct (for example `needPyPrint`), emit the helper
   in `emitHelpers`, and register any imports the helper needs at
   the point where you set the flag, not when you emit the helper.
   The header has already been written by then.
6. **Run the full test suite** and confirm no fixture drifted. If
   a fixture did drift, look at the diff: it is either the intended
   effect of your change, or a regression.

Every new feature should come with at least one fixture. A feature
without a fixture is indistinguishable from a feature that does not
work yet.

## 14. Project layout

```
cmd/gopygo/main.go     CLI: transpile, run, version.
gen/gen.go             gen struct, scope, helper emission, assembly.
gen/module.go          module walker and annotation parsing.
gen/expr.go            expression emitter; infers types bottom-up.
gen/stmt.go            statement emitter; updates scope.
pyast/pyast.go         Node wrapper around the JSON AST.
pyast/helper.py        Embedded Python script that emits JSON AST.
types/types.go         The type lattice.
tests/fixtures/        NN_topic.{py,go,expected.txt} triplets.
tests/run.sh           Snapshot diff runner.
README.md              This file.
```

Total Go source is a few thousand lines. You can read the whole
codebase in an afternoon, which is the point.

## 15. FAQ

**Why does the output parenthesise every binary expression?**

So the emitter never has to know Python's or Go's operator
precedence. `gofmt` is free to keep or drop the parentheses; in
practice the result is readable either way.

**Why is `print` routed through a helper?**

Python and Go format booleans and floats differently out of the box.
`True` vs `true`, `3.0` vs `3`. Routing through `pyPrintln` keeps
output byte-identical, which makes the snapshot test runner
possible.

**Why `int64` and not `int`?**

Python ints are unbounded; Go's `int` is platform-dependent. Picking
`int64` makes the upper bound explicit and the semantics identical
on 32- and 64-bit Go.

**Can I use `any` to paper over an unknown type?**

Only by annotating a value as `Any` explicitly. Inference will not
produce `Any`. If the compiler cannot figure out a concrete type it
will stop and point at the Python line.

**Can the transpiler talk to my favourite Python library?**

No. There is no import mechanism in v0.3, and no runtime for the
imported module to live in. Rewriting the Python program to not
depend on the library is the path.

**What Python version does the frontend need?**

Python 3.14 on the `PATH`. Older 3.x may work, but CI pins 3.14.

**Does it work on Windows?**

Untested. The `pyast` package shells `python3.14`; if your Windows
install has that on `PATH` it will probably work, but the snapshots
in `tests/fixtures/*.expected.txt` use LF line endings.

**Why is the project called gopygo?**

It started as a different tool (py -> go via compiled bytecode) and
the name stuck through a full rewrite. Think of it as "Go, pytho-n,
Go", or as a pun on the children's game. The name is not load
bearing.

## 16. A short history of the project

- **v0.1 (pyc -> Go).** The original attempt read CPython bytecode
  (`.pyc`) and emitted Go that drove a gopygo runtime. It worked,
  but the output was a dispatch table and a forest of
  `rt.Call(...)`. No one would read it for fun.
- **v0.2 (py -> Go with a runtime).** A full rewrite moved to
  source-level parsing via `ast.parse`, but still leaned on a
  runtime package and boxed `rt.Value`s. Output was easier to read
  than v0.1 but still not Go that a human would write.
- **v0.3 (py -> Go, no runtime, typed subset).** The current
  project. The runtime is gone. The type system is small but
  strict. The output is Go you would write.

Each rewrite narrowed the subset and raised the quality bar on the
output. The current version is the first where the generated code is
something the project would recommend merging into a real Go
codebase.

## 17. License

MIT. See `LICENSE`.

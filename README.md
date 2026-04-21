# gopygo

Transpile CPython 3.14 `.pyc` files into standalone Go programs.

`gopygo` reads a `.pyc` produced by `python3.14 -m py_compile`, emits a `.go` source file, and hands that file to the Go toolchain. The resulting binary reproduces the original program's observable behaviour with no Python interpreter involved at runtime.

This repository is a work in progress. See [spec 0975](https://github.com/tamnd/notes) for the design.

## Relationship to goipy

[`goipy`](https://github.com/tamnd/goipy) is an interpreter for the same bytecode. `gopygo` shares goipy's `.pyc` frontend and diverges after that. Pick goipy for scripting inside a Go service; pick gopygo for ahead-of-time compilation.

## License

MIT.

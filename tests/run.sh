#!/bin/bash
# Golden-output harness. For every fixture:
#   1. compile with python3.14 -m py_compile
#   2. transpile + build + run via `gopygo run`
#   3. diff stdout byte-for-byte against `python3.14 fixture.py`
# Any diff fails the run with a non-zero exit status.
set -eu

cd "$(dirname "$0")/.."
SRC="$(pwd)"
export GOPYGO_SRC="$SRC"

fail=0
for f in tests/fixtures/*.py; do
    base=$(basename "$f" .py)
    python3.14 -m py_compile "$f"
    pyc="tests/fixtures/__pycache__/${base}.cpython-314.pyc"
    pyout=$(python3.14 "$f")
    goout=$(go run ./cmd/gopygo run "$pyc")
    if [ "$pyout" = "$goout" ]; then
        printf 'OK   %s\n' "$base"
    else
        printf 'FAIL %s\n' "$base"
        diff <(printf '%s\n' "$pyout") <(printf '%s\n' "$goout") | head -20
        fail=1
    fi
done
exit "$fail"

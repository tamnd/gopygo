#!/bin/bash
# For every tests/fixtures/NN_*.py:
#   1. transpile + build + run via `gopygo run`
#   2. diff stdout byte-for-byte against `python3.14 fixture.py`.
# Any diff fails.
set -eu

cd "$(dirname "$0")/.."
SRC="$(pwd)"
export GOPYGO_SRC="$SRC"

fail=0
for f in tests/fixtures/[0-9]*.py; do
    base=$(basename "$f" .py)
    pyout=$(python3.14 "$f")
    goout=$(go run ./cmd/gopygo run "$f")
    if [ "$pyout" = "$goout" ]; then
        printf 'OK   %s\n' "$base"
    else
        printf 'FAIL %s\n' "$base"
        diff <(printf '%s\n' "$pyout") <(printf '%s\n' "$goout") | head -40
        fail=1
    fi
done
exit "$fail"

#!/usr/bin/env bash
# Run every fixture under tests/fixtures, comparing the output of
# `gopygo run` against the output of `python3.14`. Also verifies that
# no emitted Go source imports the gopygo module — the whole point of
# v0.3 is stdlib-only output.
set -eu

cd "$(dirname "$0")/.."

go build -o /tmp/gopygo ./cmd/gopygo

pass=0
fail=0
for py in tests/fixtures/*.py; do
    name="$(basename "$py" .py)"
    want="$(python3.14 "$py" 2>&1 || true)"
    got="$(/tmp/gopygo run "$py" 2>&1 || true)"
    if [ "$want" = "$got" ]; then
        echo "ok   $name"
        pass=$((pass + 1))
    else
        echo "FAIL $name"
        diff <(printf '%s\n' "$want") <(printf '%s\n' "$got") || true
        fail=$((fail + 1))
    fi

    # Stdlib-only check.
    out="$(/tmp/gopygo transpile "$py" -o /tmp/_stdlib_check.go 2>&1 || true)"
    if grep -q 'github.com/tamnd/gopygo' /tmp/_stdlib_check.go; then
        echo "FAIL $name: emitted Go imports gopygo runtime"
        fail=$((fail + 1))
    fi
done

echo
echo "$pass passed, $fail failed"
[ "$fail" = 0 ]

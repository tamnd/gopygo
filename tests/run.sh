#!/usr/bin/env bash
# Run every fixture under tests/fixtures:
#
#   - transpile .py to Go and diff against the committed .go snapshot
#     (headed by //go:build ignore so `go build ./...` ignores it),
#   - execute the emitted program via `gopygo run` and diff against
#     the committed .expected.txt snapshot,
#   - grep the emitted Go for 'github.com/tamnd/gopygo' to catch any
#     accidental runtime dependency — v0.3 must stay stdlib-only.
#
# Regenerate snapshots with: UPDATE=1 tests/run.sh
set -eu

cd "$(dirname "$0")/.."
go build -o /tmp/gopygo ./cmd/gopygo

pass=0
fail=0
for py in tests/fixtures/*.py; do
    name="$(basename "$py" .py)"
    snap_go="tests/fixtures/${name}.go"
    snap_out="tests/fixtures/${name}.expected.txt"

    tmp_go="$(mktemp -t gopygo-go.XXXX).go"
    /tmp/gopygo transpile "$py" -o "$tmp_go"
    # Snapshots carry a build tag so `go build ./...` skips them.
    tagged="$(mktemp -t gopygo-go-tagged.XXXX)"
    printf '//go:build ignore\n\n' > "$tagged"
    cat "$tmp_go" >> "$tagged"

    got_out="$(/tmp/gopygo run "$py" 2>&1 || true)"

    if [ "${UPDATE:-0}" = 1 ]; then
        cp "$tagged" "$snap_go"
        printf '%s\n' "$got_out" > "$snap_out"
    fi

    ok=1
    if ! diff -q "$tagged" "$snap_go" >/dev/null 2>&1; then
        echo "FAIL $name: emitted Go drifted from $snap_go"
        diff "$snap_go" "$tagged" || true
        ok=0
    fi
    if ! diff -q <(printf '%s\n' "$got_out") "$snap_out" >/dev/null 2>&1; then
        echo "FAIL $name: stdout drifted from $snap_out"
        diff "$snap_out" <(printf '%s\n' "$got_out") || true
        ok=0
    fi
    if grep -q 'github.com/tamnd/gopygo' "$snap_go"; then
        echo "FAIL $name: emitted Go imports the gopygo module"
        ok=0
    fi

    if [ "$ok" = 1 ]; then
        echo "ok   $name"
        pass=$((pass + 1))
    else
        fail=$((fail + 1))
    fi
done

echo
echo "$pass passed, $fail failed"
[ "$fail" = 0 ]

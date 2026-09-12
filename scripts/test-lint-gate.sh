#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
root=$(cd "$script_dir/.." && pwd)
test_root=$(mktemp -d "${TMPDIR:-/tmp}/arcc-lint-gate-test.XXXXXX")
trap 'rm -rf -- "$test_root"' EXIT

mkdir -p "$test_root/go"
cp "$script_dir/testdata/lint-gate/go.mod" "$test_root/go/go.mod"
cp "$script_dir/testdata/lint-gate/main.go" "$test_root/go/main.go"
cp "$root/justfile" "$test_root/justfile"

fake_bin="$test_root/bin"
mkdir -p "$fake_bin"
true_bin=$(type -P true)
ln -s "$true_bin" "$fake_bin/protoc"
ln -s "$true_bin" "$fake_bin/jj"

gofmt_output=$(cd "$test_root/go" && gofmt -l .)
grep -Fq "main.go" <<<"$gofmt_output"

ci_log="$test_root/ci.log"
if (cd "$test_root" && PATH="$fake_bin:$PATH" just ci) >"$ci_log" 2>&1; then
	echo "ci unexpectedly accepted an unformatted Go file" >&2
	cat "$ci_log" >&2
	exit 1
fi
grep -Fq "gofmt -l ." "$ci_log"

ci_commands=$(cd "$test_root" && just --dry-run ci 2>&1)
grep -Fq "cd go && go vet ./..." <<<"$ci_commands"
grep -Fq "gofmt -l ." <<<"$ci_commands"

echo "lint gate rejects an unformatted Go file and is reachable from ci"

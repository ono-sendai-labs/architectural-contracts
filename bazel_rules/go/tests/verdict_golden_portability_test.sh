#!/usr/bin/env bash
# Keep semantic verdict goldens host-independent. This guard is intentionally
# file-oriented rather than a report parser: a verdict golden is only the
# stable pass/fail contract, so any layout, SDK, diagnostic, source, or import
# namespace content is contamination and names its content class.
set -euo pipefail

scan_file() {
  local file="$1"
  local match=""
  if [[ ! -f "${file}" ]]; then
    echo "verdict golden portability: ${file} is not a regular file" >&2
    return 1
  fi

  check_class() {
    local class="$1"
    local pattern="$2"
    if match="$(grep -Eio -- "${pattern}" "${file}" | head -n 1)" && [[ -n "${match}" ]]; then
      echo "verdict golden portability: ${file} contains ${class}: ${match}" >&2
      return 1
    fi
    return 0
  }

  check_class "absolute path" '(^|[[:space:]\"])/[^[:space:]\"]+|(^|[[:space:]\"])[A-Za-z]:[\\/][^[:space:]\"]*'
  check_class "package closure" '(^|[[:space:]\"])(packages?|package_closure|roots|closure|import_closure)([[:space:]\"]|:|=)'
  check_class "SDK/export metadata" '(go_sdk_root|stdlib_export_data|export_file|export_roots|sdk_key|toolchain_version|goos|goarch|build_tags|goexperiment)'
  check_class "diagnostic input metrics" '(non_member_export_(artifact_count|bytes)|input_?(artifact_)?(count|bytes)|diagnostic)'
  check_class "configured host import prefix" '(^|[^[:alnum:]_])([[:alnum:]-]+\.)+[[:alpha:]]{2,}/[[:alnum:]_.+/-]+'
  check_class "source location" '(source_path|location|sites|[[:alnum:]_.-]+\.go([[:space:]\"}]|$))'

  local contents
  contents="$(<"${file}")"
  if [[ "${contents}" != "pass" && "${contents}" != "fail" ]]; then
    echo "verdict golden portability: ${file} contains non-verdict content" >&2
    return 1
  fi
}

if [[ "${1:-}" == "--scan-only" ]]; then
  shift
  if [[ "$#" -ne 1 ]]; then
    echo "usage: $0 [--scan-only] <verdict-golden>..." >&2
    exit 2
  fi
  scan_file "$1"
  exit 0
fi

if [[ "$#" -eq 0 ]]; then
  echo "usage: $0 <verdict-golden>..." >&2
  exit 2
fi

for file in "$@"; do
  scan_file "${file}"
done

# Exercise the guard against each forbidden content class. These probes are
# deliberate test data, kept in TEST_TMPDIR rather than the canonical golden
# inventory, and ensure a future contamination reports the file and class.
probe_dir="${TEST_TMPDIR:-/tmp}"
probe_cases=(
  'absolute path|{"file":"/workspace/member.go"}'
  'package closure|{"packages":["example.com/member"]}'
  'SDK/export metadata|{"go_sdk_root":"sdk/src"}'
  'diagnostic input metrics|{"non_member_export_bytes":12}'
  'configured host import prefix|example.com/host/member'
  'source location|{"location":{"file":"member.go"}}'
)
for case in "${probe_cases[@]}"; do
  class="${case%%|*}"
  contents="${case#*|}"
  probe="$(mktemp "${probe_dir}/verdict-golden-contamination.XXXXXX")"
  printf '%s\n' "${contents}" > "${probe}"
  if diagnostic="$("$0" --scan-only "${probe}" 2>&1)"; then
    echo "verdict golden portability: contamination probe for ${class} unexpectedly passed" >&2
    rm -f "${probe}"
    exit 1
  fi
  if [[ "${diagnostic}" != *"${probe}"* || "${diagnostic}" != *"${class}"* ]]; then
    echo "verdict golden portability: contamination probe for ${class} lacked file/class diagnostic: ${diagnostic}" >&2
    rm -f "${probe}"
    exit 1
  fi
  rm -f "${probe}"
done

echo "OK: all verdict goldens are portable and contamination diagnostics are actionable"

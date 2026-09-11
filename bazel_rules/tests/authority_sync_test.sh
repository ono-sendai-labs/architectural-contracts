#!/usr/bin/env bash
# The authority taxonomy exists twice: as Starlark constants consumers write
# into `declared_authority`, and as arcc's own `KnownCapabilities` set, which
# rejects anything it does not recognise at manifest-parse time. A capability
# added to one and not the other turns into a confusing parse error at check
# time, so the two sets are compared here instead. Component verification
# selectors (`DECLARED` and `UNKNOWN`) share the language-neutral file but are
# intentionally outside this capability taxonomy.
#
# Args: <authority.bzl> <capabilities.go>
set -euo pipefail

authority_bzl="$1"
capabilities_go="$2"

# The ALL_AUTHORITIES list is the source of the capability constant names. It
# excludes the separate DECLARED/UNKNOWN verification-status selectors.
constants="$(
  sed -n '/^ALL_AUTHORITIES = \[/,/^\]/p' "${authority_bzl}" |
    grep -oE '^    [A-Z][A-Z_]*,' | tr -d ' ,' | sort
)"

# The ALL_AUTHORITIES list, which tooling iterates and the rule validates against.
all_authorities="$(
  sed -n '/^ALL_AUTHORITIES = \[/,/^\]/p' "${authority_bzl}" |
    grep -oE '^    [A-Z][A-Z_]*,' | tr -d ' ,' | sort
)"

# `"FILES": true,` entries in arcc's schema.KnownCapabilities map.
known_capabilities="$(
  sed -n '/^var KnownCapabilities = map\[string\]bool{/,/^}/p' "${capabilities_go}" |
    grep -oE '"[A-Z_]+":' | tr -d '"' | tr -d ':' | sort
)"

status=0

if [[ -z "${constants}" || -z "${all_authorities}" || -z "${known_capabilities}" ]]; then
  echo "FAIL: extracted an empty authority set — the parsing above no longer matches the sources" >&2
  exit 1
fi

if [[ "${constants}" != "${all_authorities}" ]]; then
  echo "FAIL: authority.bzl constants and ALL_AUTHORITIES disagree:" >&2
  diff <(echo "${constants}") <(echo "${all_authorities}") >&2 || true
  status=1
fi

if [[ "${constants}" != "${known_capabilities}" ]]; then
  echo "FAIL: authority.bzl and arcc's KnownCapabilities disagree" >&2
  echo "      (< only in authority.bzl, > only in capabilities.go):" >&2
  diff <(echo "${constants}") <(echo "${known_capabilities}") >&2 || true
  status=1
fi

if [[ "${status}" -eq 0 ]]; then
  echo "OK: $(echo "${constants}" | wc -l) authorities in sync"
fi

exit "${status}"

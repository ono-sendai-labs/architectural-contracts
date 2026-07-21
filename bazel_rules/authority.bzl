"""Ambient-authority taxonomy shared by every arcc component rule.

These constants name the capabilities arcc understands; a component's
`declared_authority` is a subset of them. The taxonomy is language-neutral —
`go_component` re-exports it for convenience, and a future `rust_component`
would use it unchanged.

The Go source of truth is `KnownCapabilities` in
`go/internal/manifest/manifest.go`; `//bazel_rules/tests:authority_sync_test`
fails if the two sets drift apart.
"""

FILES = "FILES"
NETWORK = "NETWORK"
READ_SYSTEM_STATE = "READ_SYSTEM_STATE"
MODIFY_SYSTEM_STATE = "MODIFY_SYSTEM_STATE"
OPERATING_SYSTEM = "OPERATING_SYSTEM"
SYSTEM_CALLS = "SYSTEM_CALLS"
EXEC = "EXEC"
RUNTIME = "RUNTIME"
ARBITRARY_EXECUTION = "ARBITRARY_EXECUTION"
CGO = "CGO"
UNSAFE_POINTER = "UNSAFE_POINTER"
REFLECT = "REFLECT"
UNANALYZED = "UNANALYZED"

# Every authority arcc knows about, for tooling and for validating
# `declared_authority` at analysis time.
ALL_AUTHORITIES = [
    FILES,
    NETWORK,
    READ_SYSTEM_STATE,
    MODIFY_SYSTEM_STATE,
    OPERATING_SYSTEM,
    SYSTEM_CALLS,
    EXEC,
    RUNTIME,
    ARBITRARY_EXECUTION,
    CGO,
    UNSAFE_POINTER,
    REFLECT,
    UNANALYZED,
]

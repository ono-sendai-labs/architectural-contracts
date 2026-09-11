"""Ambient-authority taxonomy shared by every arcc component rule.

These constants name the capabilities arcc understands; a component's
`declared_authority` is a subset of them. The taxonomy is language-neutral —
`go_component` re-exports it for convenience, and a future `rust_component`
would use it unchanged. The component-authority selectors below are a separate
axis: DECLARED means the component is checked, while UNKNOWN means its
package-level surface is asserted without analysis.

The Go source of truth is `KnownCapabilities` in
`go/internal/manifest/manifest.go`; `//bazel_rules/tests:authority_sync_test`
fails if the two sets drift apart.
"""

# Component verification-status selectors. These are intentionally not members
# of ALL_AUTHORITIES: UNKNOWN is not an ambient capability and DECLARED is not
# a capability declaration.
DECLARED = "DECLARED"
UNKNOWN = "UNKNOWN"
ALL_COMPONENT_AUTHORITIES = [DECLARED, UNKNOWN]

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

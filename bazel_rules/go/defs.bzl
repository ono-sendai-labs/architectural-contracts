"""Public Go rules for Architectural Contracts.

    load("@rules_arcc//bazel_rules/go:defs.bzl", "go_component", "FILES")

The authority constants are re-exported here so a Go consumer needs one load
statement; they are equally available from their language-neutral home,
`@rules_arcc//bazel_rules:authority.bzl`.
"""

load(
    "//bazel_rules:authority.bzl",
    _ALL_AUTHORITIES = "ALL_AUTHORITIES",
    _ARBITRARY_EXECUTION = "ARBITRARY_EXECUTION",
    _CGO = "CGO",
    _EXEC = "EXEC",
    _FILES = "FILES",
    _MODIFY_SYSTEM_STATE = "MODIFY_SYSTEM_STATE",
    _NETWORK = "NETWORK",
    _OPERATING_SYSTEM = "OPERATING_SYSTEM",
    _READ_SYSTEM_STATE = "READ_SYSTEM_STATE",
    _REFLECT = "REFLECT",
    _RUNTIME = "RUNTIME",
    _SYSTEM_CALLS = "SYSTEM_CALLS",
    _UNANALYZED = "UNANALYZED",
    _UNSAFE_POINTER = "UNSAFE_POINTER",
)

FILES = _FILES
NETWORK = _NETWORK
READ_SYSTEM_STATE = _READ_SYSTEM_STATE
MODIFY_SYSTEM_STATE = _MODIFY_SYSTEM_STATE
OPERATING_SYSTEM = _OPERATING_SYSTEM
SYSTEM_CALLS = _SYSTEM_CALLS
EXEC = _EXEC
RUNTIME = _RUNTIME
ARBITRARY_EXECUTION = _ARBITRARY_EXECUTION
CGO = _CGO
UNSAFE_POINTER = _UNSAFE_POINTER
REFLECT = _REFLECT
UNANALYZED = _UNANALYZED
ALL_AUTHORITIES = _ALL_AUTHORITIES

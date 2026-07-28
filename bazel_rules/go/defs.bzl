"""Public Go rules for Architectural Contracts.

    load("@rules_arcc//bazel_rules/go:defs.bzl", "go_component", "FILES")

The authority constants are re-exported here so a Go consumer needs one load
statement; they are equally available from their language-neutral home,
`@rules_arcc//bazel_rules:authority.bzl`.
"""

load("//bazel_rules/go/private:check.bzl", "arcc_check_test")
load("//bazel_rules/go/private:component.bzl", "go_component_rule")
load(
    "//bazel_rules/go/private:go_adapter.bzl",
    "go_infra_deps",
)
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

def validate_component_shape(name, kwargs):
    _validate_component_shape(name, kwargs)
PACKAGE_SURFACE = "PACKAGE_SURFACE"

def _is_label(s):
    return s.startswith("//") or s.startswith(":") or s.startswith("@")

def _has_wildcards(s):
    for c in ["*", "?", "[", "]", "\\"]:
        if c in s:
            return True
    return False

def _validate_component_shape(name, kwargs):
    style = kwargs.get("interface_style")
    interface = kwargs.get("interface")
    members = kwargs.get("members") or []

    if style == None or style == "":
        declared_style = True
    elif style == PACKAGE_SURFACE:
        declared_style = False
    else:
        fail("component %s: unknown interface_style %r; accepted values are unset (declared style) and %s." % (
            name,
            style,
            PACKAGE_SURFACE,
        ))

    if declared_style:
        if interface == None:
            fail("component %s: declared interface_style requires interface." % name)
        for m in members:
            m_str = str(m)
            if not _is_label(m_str) or _has_wildcards(m_str):
                fail("component %s: declared-style members must be literal target labels: %r" % (name, m_str))
        return

    if interface != None:
        fail("component %s: interface_style %s does not allow interface; remove interface." % (
            name,
            PACKAGE_SURFACE,
        ))
    if not members:
        fail("component %s: interface_style %s requires non-empty members." % (
            name,
            PACKAGE_SURFACE,
        ))

def _go_component_impl(name, visibility, **kwargs):
    # Validate the authoring shape before dropping unset inherited attributes
    # or invoking the private rule, so errors point at the component declaration
    # rather than at a later artifact-generation assumption.
    _validate_component_shape(name, kwargs)

    # Unset inherited attributes arrive as None; the rule wants its own
    # defaults for those, not a null.
    set_kwargs = {key: value for key, value in kwargs.items() if value != None}

    raw_members = set_kwargs.get("members", [])
    target_members = []
    pattern_members = []
    for m in raw_members:
        m_str = str(m)
        if _is_label(m_str) and not _has_wildcards(m_str):
            target_members.append(m_str)
        else:
            pattern_members.append(m_str)

    set_kwargs["members"] = target_members
    set_kwargs["member_patterns"] = pattern_members
    if "infra_deps" not in set_kwargs:
        set_kwargs["infra_deps"] = go_infra_deps()

    go_component_rule(
        name = name,
        visibility = visibility,
        **set_kwargs
    )

    # Every component gets a hermetic `.check` (design §4.2). It inherits the
    # component's `tags` and `testonly`, so a component tagged `manual` — the
    # deliberate analysis-failure fixtures — keeps its check out of
    # `bazel test //...` too, and never fails there for reasons the component
    # target already covers.
    check_kwargs = {key: set_kwargs[key] for key in ("tags", "testonly") if key in set_kwargs}
    arcc_check_test(
        name = name + ".check",
        component = ":" + name,
        # The check runs in a few seconds; "small" keeps Bazel from warning that
        # the default "medium" size overshoots its runtime (design §4.2).
        size = "small",
        visibility = visibility,
        **check_kwargs
    )

go_component = macro(
    implementation = _go_component_impl,
    inherit_attrs = "common",
    attrs = {
        "interface": attr.label(
            mandatory = False,
            configurable = False,
            doc = "The single go_library holding the component's public surface. " +
                  "Required for declared style and omitted for PACKAGE_SURFACE.",
        ),
        "interface_style": attr.string(
            configurable = False,
            doc = "Interface shape: unset for declared style, or PACKAGE_SURFACE for " +
                 "components whose complete surface is their concrete members.",
        ),
        "members": attr.string_list(
            configurable = False,
            doc = "Concrete go_library labels or import-path patterns (under PACKAGE_SURFACE) declared as component members.",
        ),
        "component_deps": attr.label_list(
            configurable = False,
            doc = "Other go_component targets this component depends on. Their packages are " +
                  "covered by them, and so are neither members of this component nor absorbed.",
        ),
        "absorbed_deps": attr.label_list(
            configurable = False,
            doc = "Libraries this component absorbs as implementation details, taking " +
                  "responsibility for their ambient authority. No reasons are recorded: " +
                  "a BUILD comment is the place for a note.",
        ),
        "contract": attr.label_list(
            allow_files = True,
            configurable = False,
            doc = "Contract documents. Bazel-only metadata: arcc never reads them.",
        ),
        "declared_authority": attr.string_list(
            configurable = False,
            doc = "The ambient authority this component declares, as constants from this file " +
                  "(FILES, NETWORK, ...). Empty means the component claims to be authority-free.",
        ),
    },
    doc = """Declares a checkable arcc component around a Go interface library.

Expands to:

  * `name` — generates `name.component.textproto` and
    `name.package-layout.json`, and forwards the interface library's Go
    providers, so the component target can be used as a `deps` entry.
  * `name.check` — a hermetic test that runs `arcc check` on the generated
    manifest and layout. `bazel test` it to enforce the component's contract.

Example:

    go_component(
        name = "svc_component",
        interface = ":svc",
        component_deps = ["//other:other_component"],
        absorbed_deps = ["//third_party/csvparse"],
        declared_authority = [FILES],
        visibility = ["//visibility:public"],
    )
""",
)

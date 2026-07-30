"""Testing macro for go_component test targets in analysis tests."""

load("//bazel_rules/go/private:check.bzl", "arcc_check_test")
load(
    "//bazel_rules/go/private:component.bzl",
    "GO_COMPONENT_ATTRS",
    "go_component_impl",
)
load(
    "//bazel_rules/go/private:go_adapter.bzl",
    "GO_TOOLCHAINS",
    "go_attach_infra",
    "go_attached_infra",
)
load("//bazel_rules/go/private:paths.bzl", "match_path")
load("//bazel_rules:providers.bzl", "ArccComponentInfo")
load("//bazel_rules/go:providers.bzl", "ArccPackageInfo")
load("//bazel_rules/go:defs.bzl", "validate_component_shape")

def _test_attach_predicate(roots, entry):
    """Implements attachment modes for fixtures without modifying the host seam."""
    attach_mode = entry.attach_mode
    if attach_mode == "ALWAYS":
        return go_attach_infra(roots, entry)
    if attach_mode == "NEVER":
        return False
    if attach_mode == "ROOTS":
        # This mode pins the host seam's first argument: M9 requires the
        # interface and every declared member to arrive as one root list.
        if len(roots) != 2:
            return False
        for root in roots:
            if ArccPackageInfo not in root:
                return False
        return go_attach_infra(roots, entry)
    if attach_mode == "CLOSURE":
        search_patterns = entry.import_path_patterns
        if not search_patterns:
            search_patterns = ["*runtime*", "*injected*", "*member*"]
        for root in roots:
            if ArccPackageInfo in root:
                for pkg in root[ArccPackageInfo].packages.to_list():
                    for pattern in search_patterns:
                        if match_path(pattern, pkg.importpath):
                            return True
        return False
    return go_attach_infra(roots, entry)

def _test_infra_registry(ctx):
    entries = []
    for dep in ctx.attr.infra_deps:
        info = dep[ArccComponentInfo]
        entries.append(struct(
            name = info.component_name,
            component = str(dep.label),
            import_path_patterns = ctx.attr.test_infra_patterns,
            attach_mode = ctx.attr.test_infra_attach,
            attach_predicate = _test_attach_predicate,
        ))
    return entries

def _testing_attachment_fn(ctx, roots, infra_deps):
    return go_attached_infra(
        ctx,
        roots,
        infra_deps,
        registry = _test_infra_registry(ctx),
    )

def _testing_go_component_impl(ctx):
    return go_component_impl(ctx, attachment_fn = _testing_attachment_fn)

_TEST_COMPONENT_ATTRS = dict(GO_COMPONENT_ATTRS)
_TEST_COMPONENT_ATTRS.update({
    "test_infra_patterns": attr.string_list(
        doc = "Test-only import-path patterns used by the attachment fixture harness.",
    ),
    "test_infra_attach": attr.string(
        doc = "Test-only attachment mode: ALWAYS, NEVER, CLOSURE, or ROOTS.",
    ),
})

testing_go_component_rule = rule(
    implementation = _testing_go_component_impl,
    attrs = _TEST_COMPONENT_ATTRS,
    toolchains = GO_TOOLCHAINS,
    provides = [ArccComponentInfo],
    doc = "Test-only component rule with configurable infrastructure attachment.",
)

def testing_go_component(name, visibility = None, **kwargs):
    validate_component_shape(name, kwargs)

    set_kwargs = {key: value for key, value in kwargs.items() if value != None}

    raw_members = set_kwargs.get("members", [])
    target_members = []
    pattern_members = []
    for m in raw_members:
        m_str = str(m)
        if m_str.startswith("//") or m_str.startswith(":") or m_str.startswith("@"):
            if not any([c in m_str for c in ["*", "?", "[", "]", "\\"]]):
                target_members.append(m_str)
                continue
        pattern_members.append(m_str)

    set_kwargs["members"] = target_members
    set_kwargs["member_patterns"] = pattern_members
    # Keep the analysis-test macro's generated manifest aligned with its
    # manual-aware `.check` target, just like the public go_component macro.
    set_kwargs["own_check_runs"] = "manual" not in set_kwargs.get("tags", [])

    testing_go_component_rule(
        name = name,
        visibility = visibility,
        **set_kwargs
    )

    check_kwargs = {key: set_kwargs[key] for key in ("tags", "testonly") if key in set_kwargs}
    arcc_check_test(
        name = name + ".check",
        component = ":" + name,
        size = "small",
        visibility = visibility,
        **check_kwargs
    )

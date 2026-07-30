"""Testing macro for go_component test targets in analysis tests."""

load("//bazel_rules/go/private:check.bzl", "arcc_check_test")
load("//bazel_rules/go/private:component.bzl", "go_component_rule")
load("//bazel_rules/go:defs.bzl", "validate_component_shape")

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

    go_component_rule(
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

"""Testing macro for go_component test targets in analysis tests."""

load("//bazel_rules/go/private:check.bzl", "arcc_check_test")
load(
    "//bazel_rules/go/private:component.bzl",
    "GO_COMPONENT_ATTRS",
    "go_component_impl",
)
load("//bazel_rules/go/private:aspect.bzl", "arcc_deps_aspect")
load(
    "//bazel_rules/go/private:go_adapter.bzl",
    "GO_PROVIDERS",
    "GO_TOOLCHAINS",
    "go_attach_infra",
    "go_attached_infra",
    "go_stdlib_export_data",
    "validate_sdk_export_data",
)
load("//bazel_rules/go/private:paths.bzl", "match_path")
load("//bazel_rules:providers.bzl", "ArccComponentInfo")
load("//bazel_rules/go:providers.bzl", "ArccPackageInfo", "ArccStdlibMapInfo")
load("//bazel_rules/go/private:arcc_metadata.bzl", "arcc_sdk_key_fields")
load("//bazel_rules/go:defs.bzl", "DECLARED", "UNKNOWN", "validate_component_shape")

TestingInfraAttachmentInfo = provider(fields = ["attached_targets"])

TestingStdlibExportDataInfo = provider(
    fields = ["metadata", "export_files", "inputs", "target"],
)

def _fake_export_target(key, mismatch_field):
    values = {
        "toolchain_version": key.toolchain_version,
        "goos": key.goos,
        "goarch": key.goarch,
        "cgo_enabled": key.cgo_enabled,
        "build_tags": tuple(key.build_tags),
        "goexperiment": key.goexperiment,
    }
    if mismatch_field:
        values[mismatch_field] = {
            "toolchain_version": "go0.0.0",
            "goos": "darwin",
            "goarch": "386",
            "cgo_enabled": not key.cgo_enabled,
            "build_tags": ("fake_mismatch",),
            "goexperiment": "fake-mismatch",
        }[mismatch_field]
    return struct(**values)

def _fake_stdlib_export_data_impl(ctx):
    """Publishes sentinel export material for component action-input tests."""
    export_files = depset(ctx.files.export_files)
    metadata = ctx.file.metadata
    extra_inputs = [ctx.file.source_file] if ctx.file.source_file != None else []
    declared_inputs = depset(direct = [metadata] + extra_inputs, transitive = [export_files])
    return [
        DefaultInfo(
            files = declared_inputs,
        ),
        TestingStdlibExportDataInfo(
            metadata = metadata,
            export_files = export_files,
            inputs = declared_inputs,
            target = _fake_export_target(
                arcc_sdk_key_fields(ctx.attr.map[ArccStdlibMapInfo]),
                ctx.attr.mismatch_field,
            ),
        ),
    ]

fake_stdlib_export_data = rule(
    implementation = _fake_stdlib_export_data_impl,
    attrs = {
        "map": attr.label(
            default = "//:arcc_stdlib_map",
            providers = [ArccStdlibMapInfo],
            doc = "Map whose exact target identity the fake descriptor mirrors.",
        ),
        "metadata": attr.label(
            mandatory = True,
            allow_single_file = True,
            doc = "Sentinel package-graph metadata file.",
        ),
        "export_files": attr.label_list(
            allow_files = True,
            doc = "Sentinel compiled export artifacts.",
        ),
        "mismatch_field": attr.string(
            default = "",
            doc = "Test-only target identity field to alter before validation.",
        ),
        "source_file": attr.label(
            allow_single_file = True,
            doc = "Test-only forbidden source input for the export contract.",
        ),
    },
    provides = [TestingStdlibExportDataInfo],
    doc = "Test-only fake SDK export-data descriptor.",
)

def _test_attach_predicate(roots, entry, package_view = None):
    """Implements attachment modes for fixtures without modifying the host seam."""
    attach_mode = entry.attach_mode
    if attach_mode == "ALWAYS":
        return go_attach_infra(roots, entry, package_view)
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
        return go_attach_infra(roots, entry, package_view)
    if attach_mode == "CLOSURE":
        search_patterns = entry.import_path_patterns
        if not search_patterns:
            search_patterns = ["*runtime*", "*injected*", "*member*"]
        if package_view != None:
            candidates = package_view.keys()
        else:
            candidates = [
                pkg.importpath
                for root in roots
                if ArccPackageInfo in root
                for pkg in root[ArccPackageInfo].packages.to_list()
            ]
        for importpath in candidates:
            for pattern in search_patterns:
                if match_path(pattern, importpath):
                    return True
        return False
    return go_attach_infra(roots, entry, package_view)

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

def _testing_attachment_fn(ctx, roots, infra_deps, package_view = None):
    return go_attached_infra(
        ctx,
        roots,
        infra_deps,
        registry = _test_infra_registry(ctx),
        package_view = package_view,
    )

def _testing_infra_attachment_probe_impl(ctx):
    """Exposes the injected attachment seam for identity-only analysis tests."""
    attached = go_attached_infra(
        ctx.attr.consumer,
        [],
        ctx.attr.infra_deps,
        registry = _test_infra_registry(ctx),
    )
    return [
        DefaultInfo(),
        TestingInfraAttachmentInfo(
            attached_targets = tuple(sorted([str(item.target.label) for item in attached])),
        ),
    ]

testing_infra_attachment_probe = rule(
    implementation = _testing_infra_attachment_probe_impl,
    attrs = {
        "consumer": attr.label(
            mandatory = True,
            providers = [ArccComponentInfo],
            doc = "Component whose target label supplies the consuming identity.",
        ),
        "infra_deps": attr.label_list(
            mandatory = True,
            providers = [ArccComponentInfo],
            doc = "Injected infrastructure candidates to evaluate.",
        ),
        "test_infra_patterns": attr.string_list(
            doc = "Test-only import-path patterns used by the attachment fixture harness.",
        ),
        "test_infra_attach": attr.string(
            default = "ALWAYS",
            doc = "Test-only attachment mode: ALWAYS, NEVER, CLOSURE, or ROOTS.",
        ),
    },
    provides = [TestingInfraAttachmentInfo],
    doc = "Test-only probe for the injected infrastructure attachment seam.",
)

def _testing_extra_runtime_packages(ctx, root_packages):
    """Projects test-only runtime targets without changing ordinary roots."""
    _ = root_packages
    packages = []
    for target in ctx.attr.test_runtime_deps:
        if ArccPackageInfo not in target:
            fail("component %s: test runtime target %s has no ArccPackageInfo" % (
                ctx.label.name,
                target.label,
            ))
        packages.extend(target[ArccPackageInfo].packages.to_list())
    for target in ctx.attr.test_runtime_packages:
        packages.extend(target[ArccPackageInfo].packages.to_list())
    return packages

def _testing_stdlib_export_data(ctx, expected_mode = None):
    """Selects a fake descriptor when a seam test requests one."""
    fake = ctx.attr.test_stdlib_export_data
    if fake != None:
        descriptor = fake[TestingStdlibExportDataInfo]
        return validate_sdk_export_data(ctx, descriptor, expected_mode = expected_mode)
    return go_stdlib_export_data(ctx, expected_mode = expected_mode)

def _testing_go_component_impl(ctx):
    return go_component_impl(
        ctx,
        attachment_fn = _testing_attachment_fn,
        runtime_packages_fn = _testing_extra_runtime_packages,
        stdlib_export_data_fn = _testing_stdlib_export_data,
        stdlib_map_target = ctx.attr.test_stdlib_map,
    )

_TEST_COMPONENT_ATTRS = dict(GO_COMPONENT_ATTRS)
_TEST_COMPONENT_ATTRS.update({
    "test_infra_patterns": attr.string_list(
        doc = "Test-only import-path patterns used by the attachment fixture harness.",
    ),
    "test_infra_attach": attr.string(
        doc = "Test-only attachment mode: ALWAYS, NEVER, CLOSURE, or ROOTS.",
    ),
    "test_runtime_deps": attr.label_list(
        providers = GO_PROVIDERS,
        aspects = [arcc_deps_aspect],
        doc = "Test-only hidden Go targets projected as injected runtime packages.",
    ),
    "test_runtime_packages": attr.label_list(
        providers = [ArccPackageInfo],
        doc = "Test-only provider records projected as injected runtime packages.",
    ),
    "test_stdlib_export_data": attr.label(
        providers = [TestingStdlibExportDataInfo],
        doc = "Test-only fake host-neutral SDK export descriptor.",
    ),
    "test_stdlib_map": attr.label(
        providers = [ArccStdlibMapInfo],
        doc = "Test-only selected authority map override.",
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
    target_members = {}
    for m in raw_members:
        m_str = str(m)
        if m_str.startswith("//") or m_str.startswith(":") or m_str.startswith("@"):
            if not any([c in m_str for c in ["*", "?", "[", "]", "\\"]]):
                target_members[m_str] = True
                continue
        fail("component %s: members must be literal target labels: %r" % (name, m_str))

    set_kwargs["members"] = sorted(target_members.keys())

    # Mirror the production macro (defs.bzl): only the explicit authority
    # selector suppresses the generated `.check`; `check_tags` are tags for the
    # generated `.check` only, for fixtures whose check deliberately fails.
    check_tags = list(set_kwargs.pop("check_tags", []))
    component_is_asserted = set_kwargs.get("authority", DECLARED) == UNKNOWN

    testing_go_component_rule(
        name = name,
        visibility = visibility,
        **set_kwargs
    )

    if not component_is_asserted:
        check_kwargs = {key: set_kwargs[key] for key in ("tags", "testonly") if key in set_kwargs}
        if check_tags:
            check_kwargs["tags"] = list(check_kwargs.get("tags", [])) + check_tags
        arcc_check_test(
            name = name + ".check",
            component = ":" + name,
            size = "small",
            visibility = visibility,
            **check_kwargs
        )

# Test-only escape hatch for exercising the private rule implementation without
# the public macro's shape/authority validation. Production callers must use
# `go_component`; this helper pins the direct-rule fail-closed seam.
def testing_go_component_direct(name, visibility = None, **kwargs):
    testing_go_component_rule(
        name = name,
        visibility = visibility,
        **kwargs
    )

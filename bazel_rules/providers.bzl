"""The language-neutral component contract.

`ArccComponentInfo` is what one component target tells another. It carries no
Go-specific data on purpose: `component_deps` edges between a Go component and
a future Rust one only need the manifest/layout files, the coverage closure,
and the identity fields below.
"""

ArccComponentInfo = provider(
    doc = "A checkable arcc component: its manifest, layout, and coverage.",
    fields = {
        "component_name": "string, logical name (= target name)",
        "component_root": "string, workspace-relative root directory (the manifest's logical home)",
        "manifest": "File, generated component.textproto",
        "layout": "File, generated package-layout.json",
        "transitive_manifests": "depset[File], own + all component deps' manifests",
        "transitive_layouts": "depset[File], own + all component deps' layouts",
        "closure": "depset[struct], this component's member + absorbed packages, " +
                   "which dependents subtract from their own closure (design §5.3)",
        "contracts": "depset[File], contract docs (Bazel-only metadata)",
    },
)

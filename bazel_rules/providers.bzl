"""The language-neutral component contract.

`ArccComponentInfo` is what one component target tells another. It carries no
Go-specific data on purpose: `component_deps` edges between a Go component and
a future Rust one only need the manifest/layout files, the coverage closure,
and the identity fields below. Direct surface and report artifacts are
published through the same provider so consumers can bind them into their
package layout without treating the metadata as a verdict or authority claim.
"""

ArccComponentInfo = provider(
    doc = "A checkable arcc component: its manifest, layout, direct artifacts, and coverage.",
    fields = {
        "component_name": "string, logical name (= target name)",
        "component_root": "string, workspace-relative root directory (the manifest's logical home)",
        "manifest": "File, generated component.textproto",
        "layout": "File, generated package-layout.json",
        "transitive_manifests": "depset[File], own + all component deps' manifests",
        "transitive_layouts": "depset[File], own + all component deps' layouts",
        "closure": "depset[struct], this component's member packages, " +
                   "which dependents subtract from their own closure (design §5.3)",
        "contracts": "depset[File], contract docs (Bazel-only metadata)",
        "surface": "File, the component's canonical surface manifest " +
                   "(<name>.surface.json), or None for a producer that emits none",
        "report": "File, the component's canonical check report " +
                  "(<name>.report.json), or None for a producer that emits none",
        "provenance": "string, structural producer kind for the surface/report: " +
                      '"checked" (an ordinary analysis action ran arcc check), ' +
                      '"asserted" (written without analysis), or None for a ' +
                      "producer predating the surface axis; consumers bind this " +
                      "metadata into package-layout without treating it as a verdict or authority claim",
    },
)

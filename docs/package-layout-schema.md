# The package-layout schema (for build-system emitters)

`arcc check <manifest> --package-layout=<layout.json>` runs **hermetically**: no
`go list`, no `go.mod`, no Go toolchain on the host. Everything arcc needs about
the Go package graph comes from the layout file, which the build system emits.
`bazel_rules/` is one emitter (`go_component` writes
`<name>.package-layout.json`); a monorepo with its own Go rules writes its own.

This document is aimed at whoever writes that emitter. Two of its rules —
`is_stdlib` and the platform block — are places where the *natural*
implementation is the wrong one, so they are spelled out at length rather than
left to be inferred from the field names.

## 1. Shape

```json
{
  "go_sdk_root": "rules_go++go_sdk+main___download_0_linux_amd64/src",
  "platform": {
    "goos": "linux",
    "goarch": "amd64",
    "build_tags": [],
    "cgo_enabled": false
  },
  "roots": ["example.com/svc", "example.com/svc/impl"],
  "packages": [
    {
      "ID": "example.com/svc",
      "Name": "svc",
      "PkgPath": "example.com/svc",
      "GoFiles": ["_main/svc/api.go"],
      "CompiledGoFiles": ["_main/svc/api.go"],
      "is_stdlib": false
    }
  ]
}
```

| Field | Meaning |
|---|---|
| `go_sdk_root` | Path to the Go SDK's `src` directory. arcc enumerates and type-checks the standard library from here itself; the emitter does **not** list stdlib packages. |
| `platform` | The target the analysis is for (§3). Optional; absent means `build.Default`. |
| `roots` | The component's member packages. Must equal the manifest's `members`, or the load fails (§2). |
| `packages` | Every package in the closure, in `go/packages`' own driver "flat" encoding, plus the layout-only `is_stdlib` bit (§4). |

Per-package fields are exactly `packages.Package`'s JSON, so `ID`, `Name`,
`PkgPath`, `GoFiles`, `CompiledGoFiles` and `Imports` mean what that type means.
Package IDs are import paths — fold the closure by import path so they are
unique.

**Path frame.** All source paths are resolved against arcc's working directory
(stdlib paths against `go_sdk_root`), so the manifest need not sit next to the
sources. Which directory that is, is the emitter's contract with its own check
runner; the Bazel rules use the runfiles root, the one frame in which both
main-repo and external-repo sources have `..`-free names. A path containing `..`
is rejected.

## 2. `roots` and members

`roots` is the set of packages the component owns and analyzes as roots. arcc
compares it against the manifest's `members` field and fails closed on any
difference — the two artifacts are written by the same emitter from the same
list, so a mismatch means one of them was hand-edited or the emitter is
inconsistent.

A declared member with **no source files** is a load error naming the package: a
component cannot own code the analysis cannot see, and the layout makes that
checkable rather than merely stated.

## 3. `platform` — and the two conforming shapes

The platform block is what the loader builds its `build.Context` from, instead of
using whatever platform the arcc binary was compiled for. Getting this wrong is
silent: type-checking a wrong-platform file makes a package ill-typed, and the
analysis degrades without saying so.

The emitter must not assume it can filter. A host whose Go rules expose the
declared source set — rules_go's `GoInfo.srcs` lists every `_GOOS.go` variant,
because its *compiler*, not its provider, applies build constraints — cannot
produce a per-platform file list at analysis time, and should not have to.
So **two shapes conform**:

1. **Filtered.** The emitter declares files *and* `Imports` for the declared
   platform, consistently.
2. **Unfiltered, `Imports` omitted.** The emitter declares the full source set
   and **no** `Imports` at all; the loader recovers the edges by parsing the
   files that survive filtering. This is what `bazel_rules/` does.

What does **not** conform is the accidental middle: an unfiltered file set with
`Imports` derived from it. That layout claims edges only a
non-compiled file contributes, and it is what a naive emitter produces.

**The loader validates rather than trusts.** It is already parsing sources for
import recovery, so after filtering it compares the declared imports against the
imports of the surviving files and errors on a mismatch **in either direction** —
a declared import no surviving file contributes, or an import a surviving file
contributes that was never declared (an edge FR2 would otherwise never see).

Two details worth knowing before you debug an error from this:

- **Equality is over *resolvable* imports.** An import resolving to a layout
  package or to the standard library participates. One that resolves to
  **nothing** does not: an emitter may legitimately drop an edge with no node
  behind it rather than emit a dangling reference. Such imports are not silently
  dropped either — the loader collects them and `arcc check` reports each as an
  `ANALYSIS_LIMITATION` warning naming the package, file and import path. Post
  filtering these should be rare (the usual cause, a `_windows.go` importing
  something the build never compiled, is exactly what filtering removes), so one
  usually means a wrong platform block or a genuinely incomplete closure.
- **Omitting `Imports` is not the same as `"Imports": {}`.** An absent (or
  `null`) key selects shape 2. An empty object is shape 1 declaring zero imports,
  and any recovered resolvable import will then fail the equality check.

A package left with **no** Go sources after filtering is a load error, not an
empty package.

**Pinning the toolchain.** The platform block may carry two optional
identity fields, `toolchain_version` (`go1.N.M`) and `goexperiment`. When
present they pin the loader's release tags and tool (GOEXPERIMENT) tags to
the named toolchain instead of the host arcc runs under, which is what makes
the emitted surface's SDK key reproducible from the layout alone. Omit a
field when it is empty — a present-but-empty `toolchain_version` is a
validation error, and a present-but-empty `goexperiment` pins
`GOEXPERIMENT=none`. An emitter whose check stamps surfaces against a
declared stdlib-map artifact (the Bazel rules' analysis action) must pin
both fields to exactly the target configuration the map was generated for;
the loader fails closed on any key mismatch between the pinned platform and
the declared map.

## 4. `is_stdlib` — provenance, never a heuristic

Each emitter-listed package carries an `is_stdlib` boolean. It must record
**where the package came from in the build graph** — the SDK or toolchain, versus
a target the emitter enumerated. It must **not** be computed by re-evaluating an
import-path heuristic in the emitter.

This is the load-bearing sentence in the whole schema, because the wrong
implementation is the obvious one and it is silently self-defeating. arcc uses
the declared bit and SDK discovery as structural provenance for resource
resolution; it never reconstructs standard-library membership from the import
path spelling. A dotless host or third-party path therefore remains non-stdlib
when the emitter identifies it as non-SDK. Check-time standard-library membership
comes only from the total authority map, while missing SDK or package data still
fails closed during layout validation.

Practical consequences:

- If every package your emitter lists comes from an enumerated build target — the
  SDK not being in its metadata set at all — then `is_stdlib` is structurally
  `false` for all of them, and `true` never needs computing. That is the
  `bazel_rules/` case.
- Standard-library packages are **not** listed by the emitter. arcc discovers
  them from `go_sdk_root` and treats SDK provenance as structural, in layout mode
  and native mode alike.
- Omitting the field decodes as `false`. That is deliberate compatibility with
  older hand-written layouts; no path-based fallback changes the decoded
  provenance.

## 5. cgo

A cgo package compiles from preprocessed sources that do not exist at analysis
time, so `CompiledGoFiles` cannot be named honestly in a layout emitted from
build-graph metadata. `bazel_rules/` therefore refuses a closure containing one:
the component's declaration fails at analysis time rather than producing a layout
pointing at files that will not be in the sandbox. Native mode is unaffected,
because the go tool preprocesses cgo before `go/packages` sees it. See the
README's limitations for what that means for a component whose closure includes
cgo code.

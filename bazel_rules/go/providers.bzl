"""Go-specific provider: the package closure the layout is emitted from."""

ArccPackageInfo = provider(
    doc = "Go package closure with direct-dependency edges, for arcc layout emission.",
    fields = {
        "packages": "depset of struct(importpath, srcs, deps, cgo, export_file, label); " +
                    "srcs is a tuple of File, deps a tuple of direct-dependency importpaths, " +
                    "export_file is the compiler export File and label identifies the contributor",
    },
)

ArccStdlibMapInfo = provider(
    doc = "The standard-library authority map built for one target SDK configuration.",
    fields = {
        "map": "File — the canonical stdlib-map artifact (JSON).",
        "toolchain_version": "string — the target toolchain version (e.g. go1.26.4).",
        "goos": "string — the target GOOS the map describes.",
        "goarch": "string — the target GOARCH the map describes.",
        "cgo_enabled": "bool — the target cgo state the map describes.",
        "build_tags": "tuple of strings — the target build tags, sorted.",
        "goexperiment": "string — the target GOEXPERIMENT.",
        # The generator-owned identity fields let the
        # asserted surface writer can assemble the complete SDK key in
        # Starlark. Their mirrored values live in go/private:arcc_metadata.bzl
        # and are pinned against the stamped map and the checked emitter's
        # surface by //bazel_rules/go/tests:asserted_surface_sdk_key_test.
        "classifier_hash": "string — the classifier fingerprint the generator stamped into the map.",
        "map_format_version": "int — the stdlib-map format version the key is derived for.",
    },
)

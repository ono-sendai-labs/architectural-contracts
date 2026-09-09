"""The arcc-side identity metadata the Bazel rules must mirror (Step 5 task 05, req 5).

An asserted surface is written at analysis time from data already known to the
rule (design I6), so the identity fields the checked CLI emitter stamps —
producer version, namespace, surface format version, and the generator-owned
SDK-key fields — must exist on the Starlark side too. They live here, once.

`ARCC_PRODUCER_VERSION`, `DEFAULT_NAMESPACE` and `SURFACE_FORMAT_VERSION` mirror
`go/cmd/arcc/app/app.go` (version), `go/internal/hostpolicy` (NamespaceID
default) and `go/internal/artifactio` (SurfaceFormatVersion). `CLASSIFIER_HASH`
and `MAP_FORMAT_VERSION` mirror the values the `arcc stdlibmap generate`
action stamps into the map's SDK key (`stdlibmap.ClassifierHash` over the
generation classifier text and rule version; `artifactio.MapFormatVersion`).

Starlark has no hashing primitive, so the classifier hash cannot be computed
here — that is precisely why these are mirrored constants and not recomputed
values. The mirroring is NOT trusted: `asserted_surface_sdk_key_test` fails
the build the moment any of these constants drifts from the key the checked
emitter derives for the same component or from the key stamped into the
default map artifact (task req 5). When a bump to arcc's classifier, format
or version changes that key, this file is the one place to update.
"""

# Mirrors `version` in go/cmd/arcc/app/app.go: the producer string the checked
# emitter stamps into every surface (`"arcc " + version`).
ARCC_PRODUCER_VERSION = "arcc 0.0.0-dev"

# Mirrors hostpolicy's default NamespaceID: the canonical namespace the
# checked emitter records (DR-08).
DEFAULT_NAMESPACE = "upstream"

# Mirrors artifactio.SurfaceFormatVersion: the only supported major surface
# format version.
SURFACE_FORMAT_VERSION = 1

# Mirrors artifactio.MapFormatVersion: the only supported major stdlib-map
# format version, and the map_format_version component of the SDK key.
MAP_FORMAT_VERSION = 1

# Mirrors stdlibmap.ClassifierHash over the current generation descriptor: the
# classifier_hash component of the SDK key. See the module doc block.
CLASSIFIER_HASH = "adc198ed82f2ae81de8f2d00e26a0c90a22a1317a19579476ab0d7e33cc52eed"

def arcc_sdk_key_fields(info):
    """Assembles the complete SDK-key identity of the target configuration.

    Takes the (extended) `ArccStdlibMapInfo` of the target's stdlib map and
    returns a struct with every SDKKey field the surface schema carries, in
    the checked emitter's semantics. There is deliberately no second
    definition of the key's shape anywhere else in Starlark: the asserted
    writer consumes this function, and the equality test pins it against the
    checked emitter's key (task req 5).
    """
    return struct(
        toolchain_version = info.toolchain_version,
        goos = info.goos,
        goarch = info.goarch,
        cgo_enabled = info.cgo_enabled,
        build_tags = sorted(info.build_tags),
        goexperiment = info.goexperiment,
        classifier_hash = info.classifier_hash,
        map_format_version = info.map_format_version,
    )

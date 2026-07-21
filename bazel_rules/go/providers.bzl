"""Go-specific provider: the package closure the layout is emitted from."""

ArccPackageInfo = provider(
    doc = "Go package closure with direct-dependency edges, for arcc layout emission.",
    fields = {
        "packages": "depset of struct(importpath, srcs, deps, cgo); " +
                    "srcs is a tuple of File, deps a tuple of direct-dependency importpaths",
    },
)

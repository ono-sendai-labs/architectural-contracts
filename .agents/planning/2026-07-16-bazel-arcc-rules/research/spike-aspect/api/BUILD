load("@rules_go//go:def.bzl", "go_library")
load("//rules:defs.bzl", "arcc_deps_probe")

go_library(
    name = "api",
    srcs = ["api.go"],
    importpath = "example.com/aspect/api",
    deps = ["//core", "//shared"],
    visibility = ["//visibility:public"],
)

arcc_deps_probe(
    name = "api_closure",
    interface = ":api",
    visibility = ["//visibility:public"],
)

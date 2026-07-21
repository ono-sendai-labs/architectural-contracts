load("@rules_go//go:def.bzl", "go_library")

go_library(
    name = "core_extra",
    srcs = ["extra_impl.go"],
    importpath = "example.com/aspect/core",
    deps = ["//extradep"],
    visibility = ["//visibility:private"],
)

go_library(
    name = "core",
    srcs = ["core.go", "core_extra.go"],
    embed = [":core_extra"],
    importpath = "example.com/aspect/core",
    deps = ["//lowlevel", "//shared"],
    visibility = ["//visibility:public"],
)

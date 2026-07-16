load("@rules_go//go:def.bzl", "go_library")
load("//rules:defs.bzl", "go_component")

go_library(
    name = "other",
    srcs = ["other.go"],
    importpath = "example.com/spike/other",
    visibility = ["//visibility:public"],
)

go_component(
    name = "other_component",
    interface = [":other"],
    visibility = ["//visibility:public"],
)

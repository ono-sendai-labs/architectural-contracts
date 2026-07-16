load("@rules_go//go:def.bzl", "go_library")
load("//rules:defs.bzl", "go_component")

go_library(
    name = "svc",
    srcs = ["api.go"],
    importpath = "example.com/spike/svc",
    deps = ["//svc/internal"],
    visibility = ["//visibility:public"],
)

go_component(
    name = "svc_component",
    interface = [":svc"],
    component_deps = ["//other:other_component"],
    contract = ["contract.md"],
    declared_authority = ["FILES"],
    visibility = ["//visibility:public"],
)

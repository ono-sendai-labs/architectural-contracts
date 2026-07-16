load("@rules_go//go:def.bzl", "go_library")

go_library(
    name = "internal",
    srcs = ["impl.go"],
    importpath = "example.com/spike/svc/internal",
    deps = ["//other"],
    visibility = ["//svc:__subpackages__"],
)

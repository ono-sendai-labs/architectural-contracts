load("@rules_go//go:def.bzl", "go_test")

go_test(
    name = "consumer_test",
    srcs = ["consumer_test.go"],
    # NOTE: depends on the COMPONENT target, not the go_library.
    deps = ["//svc:svc_component"],
)

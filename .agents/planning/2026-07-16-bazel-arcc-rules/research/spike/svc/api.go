// Package svc is the component's public interface.
package svc

import "example.com/spike/svc/internal"

// Do is svc's public API.
func Do() string { return internal.Impl() }

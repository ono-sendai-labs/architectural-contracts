// Package scope tests the analysis-scope assumptions (design §5.4 / Appendix A):
//   - whole-package scope: Unreached is an exported architecture-private helper
//     that nothing calls and that is NOT part of any declared interface, yet it
//     uses authority. Capslock analyzes every function in the package, so it must
//     still be reported — this is the basis for rejecting the interface-rooted
//     alternative.
//   - _test.go exclusion: the authority-using helpers in scope_test.go and
//     scope_external_test.go must NOT be reported, because go/packages excludes
//     test files from the analyzed build.
package scope

import "os"

// Clean is the "declared interface" surface — authority-free.
func Clean() int { return 42 }

// Unreached is exported but unreachable from Clean; it uses FILES. Whole-package
// scope means it is still reported.
func Unreached() ([]byte, error) { return os.ReadFile("/etc/hostname") }

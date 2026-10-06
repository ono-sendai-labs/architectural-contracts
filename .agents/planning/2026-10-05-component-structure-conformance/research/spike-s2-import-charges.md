# Spike S2 & S3: Import-Time Authority Charges and Injected Capability Passing

**Date:** 2026-10-05  
**Objective:** Resolve whether importing `go/build` (and using `build.Context`) or `runtime` (and using `runtime.Compiler`) incurs authority charges from package initializers (`.init`), and confirm whether operations on an injected `fs.FS` (`fs.WalkDir`, `fs.ReadDir`, `fs.ReadFile`, `fs.Stat`) remain authority-free.

---

## Methodology

A throwaway component `go/internal/zzspike` was constructed with `name: "zzspike"`, explicit members `github.com/ono-sendai-labs/architectural-contracts/go/internal/zzspike`, and no `declared_authority`. It was analyzed under the pinned Go 1.26.4 linux/amd64 stdlib authority map (`go/internal/teststdlibmap/testdata/linux_amd64.stdlib-map.json`) using `arcc check`.

The probe was run jointly and then in isolation for each test case:
1. `a.go`:
   ```go
   package zzspike
   import "go/build"
   var _ build.Context
   ```
2. `b.go`:
   ```go
   package zzspike
   import "runtime"
   var _ = runtime.Compiler
   ```
3. `c.go`:
   ```go
   package zzspike
   import "io/fs"
   func CheckFS(sys fs.FS) error {
       _, err := fs.ReadFile(sys, "foo")
       if err != nil { return err }
       _, err = fs.ReadDir(sys, "bar")
       if err != nil { return err }
       _, err = fs.Stat(sys, "baz")
       if err != nil { return err }
       return fs.WalkDir(sys, ".", func(path string, d fs.DirEntry, err error) error { return err })
   }
   ```

---

## Raw Findings

### 1. Isolated `a.go` (`go/build`)
- **Violations:** 1 `UNDECLARED_AUTHORITY` finding
  - **Capability:** `READ_SYSTEM_STATE`
  - **Stdlib symbol:** `go/build.init`
  - **Source location:** `a.go:3`
  - **Class:** `TrueAuthority`
  - **Evidence:**
    - `go/build.init at :0`
    - `go/build.defaultContext at build.go:289`
    - `os.Getenv at build.go:360`
- **Warnings:** None (null).

### 2. Isolated `b.go` (`runtime`)
- **Violations:** 1 `UNDECLARED_AUTHORITY` finding
  - **Capability:** `RUNTIME`
  - **Stdlib symbol:** `runtime.init`
  - **Source location:** `b.go:3`
  - **Class:** `TrueAuthority`
  - **Evidence:**
    - `(*runtime.addrRanges).init at :0`
- **Warnings:** None (null).

### 3. Isolated `c.go` (`io/fs` capability passing)
- **Violations:** None (`violations: null`).
- **Warnings:** None (`warnings: null`).
- **Verdict:** Conforms (`pass`).

---

## Conclusions

### (i) `import "go/build"` / `build.Context`
**Charged (`READ_SYSTEM_STATE`).** Importing `go/build` triggers package initialization via `go/build.init`, which executes `go/build.defaultContext` calling `os.Getenv` at `build.go:360`. Even when only referencing the type `build.Context` without referencing `build.Default`, the package import itself is charged with `READ_SYSTEM_STATE`. Consequently, per the plan, Step 13 must implement `BuildTags` rather than relying on `go/build`, and Step 11 must retain `READ_SYSTEM_STATE` on `goanalysis` until Step 13 removes it.

### (ii) `import "runtime"` / `runtime.Compiler`
**Charged (`RUNTIME`).** Importing `runtime` triggers package initialization via `runtime.init` (specifically `(*runtime.addrRanges).init`), which incurs the `RUNTIME` capability charge. Merely referencing a constant such as `runtime.Compiler` does not avoid this import-time charge. Therefore, `packagelayout-hostenv` must declare `RUNTIME`, and all `runtime` references must be relocated to `hostenv` in Step 12.

### (iii) `fs.WalkDir`, `fs.ReadDir`, `fs.ReadFile`, `fs.Stat` over injected `fs.FS`
**Free (0 authority charges).** Calling standard library functions in `io/fs` (`fs.WalkDir`, `fs.ReadDir`, `fs.ReadFile`, `fs.Stat`) on an injected `fs.FS` produces zero violations and zero warnings; the report verdict is `pass`. Capability passing is completely free of authority charges under the architectural contract checker. This validates that `packagelayout`'s `stdlibdiscover` and `validate` can remain pure by accepting an `fs.FS`.

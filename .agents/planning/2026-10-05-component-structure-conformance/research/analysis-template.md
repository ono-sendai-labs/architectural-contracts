# Component analysis report template

Produce ONE section per component assigned to you, using exactly these headings.

## Component: <name>  (dir: go/internal/<x>, import path ...)

### 1. Current declaration
- Bazel `go_component` attrs (interface / members / interface_style / declared_authority / component_deps), and the checked-in component.textproto equivalents. Note any divergence between the two (e.g. textproto lists a subset of files as interface_files while the Bazel interface library contains all files).
- Files (non-test) with LOC each; one-line purpose per file.

### 2. Exported API inventory
Table with one row per exported package-level symbol (func, type, var, const; list exported methods under their type):
| symbol | kind | file:line | used by (outside the package, non-test) | used by tests only | internal-only |
"used by" must list the importing packages (e.g. `cmd/arcc/app`, `goanalysis`, `stdlibmap`), found by grepping `go/` for `<pkg>.<Symbol>`. Mark symbols that are used only within their own package as internal-only (candidates for unexporting / moving to a member package).
Finish with: total exported symbols; count used externally (non-test); count test-only; count internal-only.

### 3. Authority inventory (measured)
- Run `timeout 900 bazel build //go/internal/<x>:<x>_component --output_groups=+arcc` and read `bazel-bin/go/internal/<x>/<x>_component.report.json`. (If the component has no Bazel go_component, say so and fall back to grep.)
- Table: | capability | stdlib/third-party symbol referenced | member symbol (func/method) that references it | file:line |
  Derive the file:line by grepping for the referenced stdlib symbol in the member files.
- Then cluster the authority-bearing call sites into functional groups (e.g. "atomic file write", "read source bytes", "platform defaults via go/build", "JSON encode/decode via encoding/json"). For each cluster: LOC estimate, which capabilities it accounts for, whether it is a leaf (does not call back into the rest of the package) and what minimal API would be needed to isolate it in a sub-package.
- Specifically identify the ROOT CAUSE of each declared capability: which import/symbol causes REFLECT, UNSAFE_POINTER, RUNTIME, READ_SYSTEM_STATE, etc. If a declared capability is not actually exercised per the report, say so.

### 4. Internal structure and seams
- Which files/types form cohesive clusters; which unexported identifiers are shared across files (so that splitting files into separate packages would require exporting them). Give a small cross-file dependency table: cluster -> clusters it references.
- Identify "pure computation" clusters vs. "environment interaction" clusters.
- Note any package-level mutable state, init(), or build-tagged files.

### 5. Consumers' view
- For each external consumer package: which symbols of this component it uses (summarize from section 2). This is the candidate interface.

### 6. Tests
- Test files, which are in-package (package x) vs external (package x_test), and which rely on unexported identifiers (these constrain a package split). Testdata dirs.

### 7. Draft split proposal (brief; the architect will decide)
- Proposed interface package contents (symbols), proposed member package(s), proposed authority sub-component(s) with their exact API (function signatures) and declared_authority, and the resulting declared_authority of the parent. Flag risks (import cycles, test breakage, `internal/` visibility, generated code).

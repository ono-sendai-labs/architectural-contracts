package goanalysis

import (
	"bufio"
	"bytes"
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/types"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/goanalysis/internal/hostfs"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/packagelayout"
)

func validateInterfaceFiles(componentRoot string, interfaceFiles []string, loaded facts.PackageFacts) ([]InterfaceFileExclusion, error) {
	var sourceFiles map[string]bool
	if len(loaded.Packages) > 0 {
		ptr := reflect.ValueOf(loaded.Packages).Pointer()
		membershipMu.RLock()
		sourceFiles = loadedFiles[ptr]
		membershipMu.RUnlock()
	}

	root := componentRoot
	if packagelayout.IsLayoutMode() {
		root = packagelayout.GetActiveWorkspaceDir()
	}
	bctx, err := packagelayout.BuildTagsForLayout(nil)
	if err != nil {
		return nil, err
	}
	if packagelayout.IsLayoutMode() {
		bctx, err = packagelayout.BuildTagsForLayout(packagelayout.GetActiveLayout())
		if err != nil {
			return nil, err
		}
	}

	var exclusions []InterfaceFileExclusion
	surviving := 0
	for _, f := range interfaceFiles {
		if filepath.IsAbs(f) {
			return nil, fmt.Errorf("interface file %q is absolute: all interface_files must be relative", f)
		}

		cleaned := filepath.Clean(f)
		if strings.HasPrefix(cleaned, "..") {
			return nil, fmt.Errorf("interface file %q escapes the component root", f)
		}
		cleanedSlash := filepath.ToSlash(cleaned)

		absPath := filepath.Join(root, cleaned)
		info, err := hostfs.Stat(absPath)
		if err != nil {
			if isNotExist(err) {
				return nil, fmt.Errorf("interface file %q does not exist", f)
			}
			return nil, fmt.Errorf("failed to check interface file %q: %w", f, err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("interface file %q is not a regular file", f)
		}

		if sourceFiles == nil || !sourceFiles[cleanedSlash] {
			if strings.HasSuffix(cleanedSlash, ".go") {
				src, readErr := hostfs.ReadFile(absPath)
				if readErr == nil {
					matched, err := packagelayout.FileMatchesBuildConstraints(bctx, absPath, src)
					if err == nil && !matched {
						exclusions = append(exclusions, InterfaceFileExclusion{
							File:       cleanedSlash,
							Constraint: describeInterfaceFileConstraintWithContext(absPath, bctx),
						})
						continue
					}
				}
			}
			return nil, fmt.Errorf("interface file %q does not belong to any loaded Go package under component root", f)
		}
		surviving++
	}

	slices.SortFunc(exclusions, func(a, b InterfaceFileExclusion) int {
		if c := strings.Compare(a.File, b.File); c != 0 {
			return c
		}
		return strings.Compare(a.Constraint, b.Constraint)
	})
	if len(interfaceFiles) > 0 && surviving == 0 {
		var details []string
		for _, exclusion := range exclusions {
			details = append(details, fmt.Sprintf("%q (%s)", exclusion.File, exclusion.Constraint))
		}
		return exclusions, fmt.Errorf("no interface file survives the analysis platform; excluded files: %s", strings.Join(details, "; "))
	}

	return exclusions, nil
}

func describeInterfaceFileConstraintWithContext(path string, bctx packagelayout.BuildTags) string {
	if line, expression, ok := leadingBuildConstraint(path); ok {
		if !expression.Eval(func(tag string) bool {
			return bctx.HasTag(tag)
		}) {
			return line
		}
		// MatchFile already rejected the file. If its directive matches, the
		// remaining possible source of exclusion is its filename suffix.
		if suffix := filenameBuildConstraint(filepath.Base(path)); suffix != "" {
			return suffix
		}
	} else if suffix := filenameBuildConstraint(filepath.Base(path)); suffix != "" {
		return suffix
	}
	return fmt.Sprintf("filename/build constraint in %q", filepath.Base(path))
}

func leadingBuildConstraint(path string) (string, constraint.Expr, bool) {
	src, err := hostfs.ReadFile(path)
	if err != nil {
		return "", nil, false
	}

	var legacy string
	var legacyExpr constraint.Expr
	var goBuild string
	var goBuildExpr constraint.Expr
	scanner := bufio.NewScanner(bytes.NewReader(src))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "//") {
			if constraint.IsGoBuild(line) {
				if expression, err := constraint.Parse(line); err == nil && goBuild == "" {
					goBuild = strings.Join(strings.Fields(line), " ")
					goBuildExpr = expression
				}
			}
			if constraint.IsPlusBuild(line) && legacy == "" {
				if expression, err := constraint.Parse(line); err == nil {
					legacy = strings.Join(strings.Fields(line), " ")
					legacyExpr = expression
				}
			}
			continue
		}
		break
	}
	if scanner.Err() != nil {
		return "", nil, false
	}
	if goBuildExpr != nil {
		return goBuild, goBuildExpr, true
	}
	if legacyExpr != nil {
		return legacy, legacyExpr, true
	}
	return "", nil, false
}

func filenameBuildConstraint(name string) string {
	if !strings.HasSuffix(name, ".go") {
		return ""
	}
	base := strings.TrimSuffix(name, ".go")
	base = strings.TrimSuffix(base, "_test")
	parts := strings.Split(base, "_")
	if len(parts) < 2 {
		return ""
	}
	knownOS := map[string]bool{
		"aix": true, "android": true, "darwin": true, "dragonfly": true,
		"freebsd": true, "hurd": true, "illumos": true, "ios": true,
		"js": true, "linux": true, "nacl": true, "netbsd": true,
		"openbsd": true, "plan9": true, "solaris": true, "wasip1": true,
		"windows": true, "zos": true,
	}
	knownArch := map[string]bool{
		"386": true, "amd64": true, "amd64p32": true, "arm": true,
		"arm64": true, "arm64be": true, "armbe": true, "loong64": true,
		"mips": true, "mips64": true, "mips64le": true, "mips64p32": true,
		"mips64p32le": true, "mipsle": true, "ppc": true, "ppc64": true,
		"ppc64le": true, "riscv": true, "riscv64": true, "s390": true,
		"s390x": true, "sparc": true, "sparc64": true, "wasm": true,
	}
	last := parts[len(parts)-1]
	if len(parts) >= 3 {
		secondLast := parts[len(parts)-2]
		if knownOS[secondLast] && knownArch[last] {
			return fmt.Sprintf("filename suffix %q", "_"+secondLast+"_"+last+".go")
		}
	}
	if knownOS[last] || knownArch[last] {
		return fmt.Sprintf("filename suffix %q", "_"+last+".go")
	}
	return ""
}

// interfaceFileRoot is the directory declared interface files resolve
// against, mirroring ValidateInterfaceFiles: natively that is the component
// root (the manifest's directory); in layout mode the frame is the driver's
// working directory, because Bazel emitters write interface file paths
// relative to the runfiles root, not the manifest's directory.
func interfaceFileRoot(componentRoot string) string {
	if packagelayout.IsLayoutMode() {
		return packagelayout.GetActiveWorkspaceDir()
	}
	return componentRoot
}

// collectInterfaceDeclarations resolves each declared interface file to its
// AST in the loaded member package that owns it, requiring exactly one
// owning package across all interface files so a single types.Info covers
// every declaration (see LoadSurfaceInputs).
func collectInterfaceDeclarations(members []*packages.Package, componentRoot string, interfaceFiles []string, loadRoot string) ([]*ast.File, *types.Info, error) {
	if len(interfaceFiles) == 0 {
		return nil, nil, nil
	}

	// Map each surviving interface file's absolute path to its owning
	// package and AST. Files excluded by build constraints were already
	// reported by ValidateInterfaceFiles and are simply absent here.
	owners := map[*packages.Package]bool{}
	asts := make([]*ast.File, 0, len(interfaceFiles))
	for _, iface := range interfaceFiles {
		abs := filepath.Clean(filepath.Join(componentRoot, iface))
		found := false
		for _, p := range members {
			// Match against the loaded source files, then locate the AST by
			// the fileset position of its package clause.
			matchesFile := false
			for _, f := range p.GoFiles {
				if filepath.Clean(f) == abs {
					matchesFile = true
					break
				}
			}
			if !matchesFile {
				continue
			}
			var matched *ast.File
			for _, syntax := range p.Syntax {
				if syntax == nil {
					continue
				}
				if pos := p.Fset.Position(syntax.Package); filepath.Clean(pos.Filename) == abs {
					matched = syntax
					break
				}
			}
			if matched == nil {
				return nil, nil, fmt.Errorf("interface file %q was loaded but produced no AST", iface)
			}
			if !owners[p] && len(owners) > 0 {
				return nil, nil, fmt.Errorf("interface files span multiple packages; %q is in a different package than the other interface files", iface)
			}
			owners[p] = true
			asts = append(asts, matched)
			found = true
			break
		}
		if !found {
			return nil, nil, fmt.Errorf("interface file %q does not belong to any loaded member package", iface)
		}
	}

	var info *types.Info
	for p := range owners {
		if p.TypesInfo == nil {
			return nil, nil, fmt.Errorf("interface file %q's package carries no type information", interfaceFiles[0])
		}
		info = p.TypesInfo
	}
	return asts, info, nil
}

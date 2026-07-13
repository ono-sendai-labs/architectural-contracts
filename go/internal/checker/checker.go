// Package checker defines the architectural contracts checker. It is a pure core component
// that analyzes package facts, manifests, capabilities, and policies to produce a conformance report.
//
// The core Check function is designed to be completely pure, having no side effects, no I/O, and
// no dependencies on ambient authority.
package checker

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/capanalyzer"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
)

// Inputs contains all injected facts and configurations required for check execution.
type Inputs struct {
	Manifest  manifest.Manifest
	Facts     facts.PackageFacts
	DepIfaces []facts.DependencyInterface     // resolved direct component dependencies
	Caps      []capanalyzer.CapabilityFinding // injected capability findings (pre-pruned at boundaries)
	Policy    capanalyzer.CapabilityPolicy    // capability check policy
}

// Check evaluates the injected inputs against architectural contracts and returns a ConformanceReport.
// This function is completely pure and is a basis for the checker being ambient-authority-free.
//
// Implements:
// - FR3 (dependency allowlist rules)
// - FR4 (well-formedness rules: Method and Explicit Init rules)
// - FR5 (cross-component call-boundary rule and higher-order boundary-call warning)
// - FR6 (policy-aware ambient-authority rule, using StrictPolicy() merged with declared_authority)
// - §5.5 (package-overlap check)
//
// Check is now feature-complete for all pure-core rules (FR3/FR4/FR5/FR6). Findings in Caps are pre-pruned
// at dependency boundaries (Step 9) by the analyzer shell before being passed here.
//
// MVP matching is call-only. Real call edges and resolved interfaces are injected by the shell (Step 9).
func Check(in Inputs) report.ConformanceReport {
	// 1. Build component membership set
	compPkgs := make(map[string]bool)
	for _, p := range in.Facts.Packages {
		compPkgs[p.ImportPath] = true
	}

	// 2. Build allowed component dependency packages map (points to dependency name)
	allowedPkgToCompDep := make(map[string]string)
	for _, di := range in.DepIfaces {
		for _, pkg := range di.Packages {
			allowedPkgToCompDep[pkg] = di.Component
		}
	}

	// 3. Track matches for declared dependencies
	compDepMatched := make(map[string]bool)
	absDepMatched := make(map[string]bool)

	var violations []report.Finding

	// 4. Sweep each component package and check its imports
	for _, pkg := range in.Facts.Packages {
		for _, imp := range pkg.Imports {
			// Skip standard library imports
			if isStdlib(imp) {
				continue
			}

			// Skip intra-component imports
			if compPkgs[imp] {
				continue
			}

			// Check if allowed by resolved component dependencies
			if compDep, exists := allowedPkgToCompDep[imp]; exists {
				compDepMatched[compDep] = true
				continue
			}

			// Check if allowed by absorbed dependencies using glob matching
			allowedByAbsorbed := false
			for _, absDep := range in.Manifest.AbsorbedDependencies {
				matched, err := path.Match(absDep.ImportPath, imp)
				if err == nil && matched {
					absDepMatched[absDep.ImportPath] = true
					allowedByAbsorbed = true
				}
			}

			if allowedByAbsorbed {
				continue
			}

			// Undeclared non-stdlib import -> Violation
			var file string
			if len(pkg.ExportedSymbols) > 0 {
				file = pkg.ExportedSymbols[0].File
			}

			violations = append(violations, report.Finding{
				Kind:    report.UndeclaredDependency,
				Message: fmt.Sprintf("package %q imports undeclared dependency %q", pkg.ImportPath, imp),
				Location: report.Location{
					File: file,
				},
			})
		}
	}

	// 4b. FR4 Well-formedness checks (Method and Explicit Init rules)
	typeDeclFiles := make(map[string]string)
	for _, pkg := range in.Facts.Packages {
		for _, sym := range pkg.ExportedSymbols {
			if sym.Kind == "type" {
				typeDeclFiles[sym.Name] = sym.File
			}
		}
	}

	interfaceFiles := make(map[string]bool)
	for _, f := range in.Manifest.InterfaceFiles {
		interfaceFiles[f] = true
	}

	for _, pkg := range in.Facts.Packages {
		for _, sym := range pkg.ExportedSymbols {
			if sym.Kind == "method" {
				recvType := cleanReceiverType(sym.Receiver)
				declFile, exists := typeDeclFiles[recvType]
				if exists && interfaceFiles[declFile] {
					if !interfaceFiles[sym.File] {
						violations = append(violations, report.Finding{
							Kind:    report.MethodOutsideInterface,
							Message: fmt.Sprintf("exported method %q with receiver %q declared in non-interface file %q", sym.Name, sym.Receiver, sym.File),
							Location: report.Location{
								File: sym.File,
							},
						})
					}
				}
			} else if sym.Kind == "init" {
				if !interfaceFiles[sym.File] {
					violations = append(violations, report.Finding{
						Kind:    report.InitOutsideInterface,
						Message: fmt.Sprintf("explicit init declared in non-interface file %q in package %q", sym.File, pkg.ImportPath),
						Location: report.Location{
							File: sym.File,
						},
					})
				}
			}
		}
	}

	// 4c. Package Overlap Checks (§5.5)
	for _, di := range in.DepIfaces {
		var overlapping []string
		for _, dpkg := range di.Packages {
			if compPkgs[dpkg] {
				overlapping = append(overlapping, dpkg)
			}
		}
		if len(overlapping) > 0 {
			sort.Strings(overlapping)
			violations = append(violations, report.Finding{
				Kind:    report.PackageOverlap,
				Message: fmt.Sprintf("package overlap with dependency %q: overlapping packages: %s", di.Component, strings.Join(overlapping, ", ")),
			})
		}
	}

	// 4d. FR5 Cross-component Call Boundary Checks (design §5.3b)
	// Build package -> DepIface lookup & per-dependency normalized symbols map
	type depInfo struct {
		di          *facts.DependencyInterface
		normSymbols map[string]bool
	}
	pkgToDep := make(map[string]*depInfo)
	for i := range in.DepIfaces {
		di := &in.DepIfaces[i]
		normSyms := make(map[string]bool)
		for _, sym := range di.Symbols {
			normSyms[NormalizeInterfaceSymbol(sym)] = true
		}
		info := &depInfo{
			di:          di,
			normSymbols: normSyms,
		}
		for _, pkg := range di.Packages {
			pkgToDep[pkg] = info
		}
	}

	var warnings []report.Finding

	for _, edge := range in.Facts.CallEdges {
		calleePkg := ExtractPackagePath(string(edge.Callee))
		if info, exists := pkgToDep[calleePkg]; exists {
			// Skip package initializers since they are language-runtime-invoked and cannot be declared as interface symbols
			calleeStr := string(edge.Callee)
			if lastDot := strings.LastIndex(calleeStr, "."); lastDot != -1 {
				funcName := calleeStr[lastDot+1:]
				if funcName == "init" || strings.HasPrefix(funcName, "init#") {
					continue
				}
			}

			normCallee := NormalizeInterfaceSymbol(edge.Callee)
			if !info.normSymbols[normCallee] {
				// Undeclared call -> Violation
				violations = append(violations, report.Finding{
					Kind:    report.CallsUndeclaredInterface,
					Message: fmt.Sprintf("call from %q to undeclared interface symbol %q of dependency %q", edge.Caller, edge.Callee, info.di.Component),
				})
			} else if edge.PassesFuncValue {
				// Declared call passing func value -> Warning
				warnings = append(warnings, report.Finding{
					Kind:    report.HigherOrderBoundaryCall,
					Message: fmt.Sprintf("higher-order boundary call from %q to %q of dependency %q passes function value", edge.Caller, edge.Callee, info.di.Component),
				})
			}
		}
	}

	// 5. Check for unused declared dependencies

	// Check component dependencies
	for _, dep := range in.Manifest.ComponentDependencies {
		if !compDepMatched[dep.Name] {
			warnings = append(warnings, report.Finding{
				Kind:    report.UnusedDependency,
				Message: fmt.Sprintf("declared component dependency %q is unused", dep.Name),
			})
		}
	}

	// Check absorbed dependencies
	for _, absDep := range in.Manifest.AbsorbedDependencies {
		if !absDepMatched[absDep.ImportPath] {
			warnings = append(warnings, report.Finding{
				Kind:    report.UnusedDependency,
				Message: fmt.Sprintf("declared absorbed dependency %q is unused", absDep.ImportPath),
			})
		}
	}

	// 5b. FR6 Policy-aware ambient-authority rule (design §5.4)
	effectivePolicy := deriveEffectivePolicy(in.Policy, in.Manifest.DeclaredAuthority)

	for _, capFinding := range in.Caps {
		decision := capanalyzer.Classify(capFinding.Capability, effectivePolicy)
		switch decision {
		case capanalyzer.DecisionViolation:
			evidence := make([]string, len(capFinding.CallPath))
			for idx, frame := range capFinding.CallPath {
				evidence[idx] = fmt.Sprintf("%s at %s:%d", frame.Func, frame.File, frame.Line)
			}
			violations = append(violations, report.Finding{
				Kind:     report.UndeclaredAuthority,
				Message:  fmt.Sprintf("use of undeclared authority %q in package %q", capFinding.Capability, capFinding.Package),
				Evidence: evidence,
			})
		case capanalyzer.DecisionWarn:
			var kind report.Kind
			if capFinding.Class == capanalyzer.AnalysisDefeating {
				kind = report.AnalysisLimitation
			} else {
				kind = report.AllowedWithWarning
			}
			var msg string
			if kind == report.AnalysisLimitation {
				msg = fmt.Sprintf("capability %q in package %q is an analysis limitation", capFinding.Capability, capFinding.Package)
			} else {
				msg = fmt.Sprintf("capability %q in package %q allowed with warning", capFinding.Capability, capFinding.Package)
			}
			warnings = append(warnings, report.Finding{
				Kind:    kind,
				Message: msg,
			})
		}
	}

	// 6. Ensure deterministic sorting (sorted alphabetically by Message)
	sort.Slice(violations, func(i, j int) bool {
		return violations[i].Message < violations[j].Message
	})
	sort.Slice(warnings, func(i, j int) bool {
		return warnings[i].Message < warnings[j].Message
	})

	return report.ConformanceReport{
		Component:  in.Manifest.Name,
		Violations: violations,
		Warnings:   warnings,
	}
}

// isStdlib checks if a given import path is in the standard library.
// Pure string-based detection based on the convention that standard library
// import paths (except for pseudo-package "C") never contain a dot in their
// first path component.
func isStdlib(importPath string) bool {
	if importPath == "C" {
		return true
	}
	first := importPath
	if idx := strings.Index(importPath, "/"); idx != -1 {
		first = importPath[:idx]
	}
	return !strings.Contains(first, ".")
}

// cleanReceiverType strips parentheses and pointers from receiver type keys
// to resolve the underlying type name (e.g. "(*example.com/store.DB)" -> "example.com/store.DB",
// or "(example.com/store.DB)" -> "example.com/store.DB").
func cleanReceiverType(receiver string) string {
	res := receiver
	if strings.HasPrefix(res, "(*") && strings.HasSuffix(res, ")") {
		res = res[2 : len(res)-1]
	} else if strings.HasPrefix(res, "(") && strings.HasSuffix(res, ")") {
		res = res[1 : len(res)-1]
	}
	return res
}

// StripGenericBrackets removes type-parameter brackets (e.g., "Foo[int]" -> "Foo").
func StripGenericBrackets(s string) string {
	var sb strings.Builder
	depth := 0
	for _, ch := range s {
		if ch == '[' {
			depth++
		} else if ch == ']' {
			if depth > 0 {
				depth--
			}
		} else if depth == 0 {
			sb.WriteRune(ch)
		}
	}
	return sb.String()
}

// NormalizeInterfaceSymbol normalizes a symbol for A4 comparison.
func NormalizeInterfaceSymbol(sym capanalyzer.InterfaceSymbol) string {
	s := string(sym)
	s = StripGenericBrackets(s)
	s = strings.ReplaceAll(s, "(*", "(")
	return s
}

// ExtractPackagePath parses the package path from a symbol.
func ExtractPackagePath(sym string) string {
	s := StripGenericBrackets(sym)
	if strings.HasPrefix(s, "(*") {
		idx := strings.LastIndex(s, ")")
		if idx != -1 {
			receiver := s[2:idx]
			dotIdx := strings.LastIndex(receiver, ".")
			if dotIdx != -1 {
				return receiver[:dotIdx]
			}
		}
	} else if strings.HasPrefix(s, "(") {
		idx := strings.LastIndex(s, ")")
		if idx != -1 {
			receiver := s[1:idx]
			dotIdx := strings.LastIndex(receiver, ".")
			if dotIdx != -1 {
				return receiver[:dotIdx]
			}
		}
	} else {
		dotIdx := strings.LastIndex(s, ".")
		if dotIdx != -1 {
			return s[:dotIdx]
		}
	}
	return ""
}

// deriveEffectivePolicy builds the effective policy by merging the manifest's declared authority
// into Allowed, leaving the original Policy unchanged.
func deriveEffectivePolicy(policy capanalyzer.CapabilityPolicy, declaredAuth []string) capanalyzer.CapabilityPolicy {
	allowed := make(map[string]bool)
	for k, v := range policy.Allowed {
		allowed[k] = v
	}
	for _, auth := range declaredAuth {
		allowed[auth] = true
	}
	return capanalyzer.CapabilityPolicy{
		Allowed: allowed,
		Warn:    policy.Warn,
	}
}

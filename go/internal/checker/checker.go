// Package checker defines the architectural contracts checker. It is a pure core component
// that analyzes package facts, manifests, capabilities, and policies to produce a conformance report.
//
// The core Check function is designed to be completely pure, having no side effects, no I/O, and
// no dependencies on ambient authority.
//
// Component Contract (FR10):
// - What it does: Evaluates Go packages against their declared manifests to verify dependency, interface boundary, and authority conformance.
// - What it requires: Receives fully resolved inputs including parsed manifest, package facts, dependency interface symbols, and capability findings.
// - What it provides: A deterministic ConformanceReport indicating compliance and detailing any architectural violations or warnings.
// - Ambient Authority: This component is guaranteed-pure and holds no ambient authority (no filesystem I/O, no network, no reflection, and no process execution).
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
// - FR4 (well-formedness rule: Method placement)
// - FR5 (cross-component call-boundary rule and higher-order boundary-call warning)
// - FR6 (policy-aware ambient-authority rule, using StrictPolicy() merged with declared_authority)
// - M7 (member-overlap check)
//
// Check is now feature-complete for all pure-core rules (FR3/FR4/FR5/FR6). Findings in Caps are pre-pruned
// at dependency boundaries (Step 9) by the analyzer shell before being passed here.
//
// MVP matching is call-only. Real call edges and resolved interfaces are injected by the shell (Step 9).
func Check(in Inputs) report.ConformanceReport {
	// 1. Build component membership set. Facts may include transitive packages
	// needed for type checking, so only the resulting set is swept as owned code.
	compPkgs := buildMembership(in.Manifest, in.Facts.Packages)

	// 1b. Standard-library imports are skipped using only the loader-provided
	// authoritative fact. Nil and empty slices both represent an empty set.
	stdlibSet := make(map[string]bool, len(in.Facts.StdlibImports))
	for _, imp := range in.Facts.StdlibImports {
		stdlibSet[imp] = true
	}
	isStdlibImport := func(imp string) bool {
		return stdlibSet[imp]
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
		if !compPkgs[pkg.ImportPath] {
			continue
		}
		for _, imp := range pkg.Imports {
			// Skip standard library imports
			if isStdlibImport(imp) {
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

	// 4b. FR4 Method placement check
	typeDeclFiles := make(map[string]string)
	for _, pkg := range in.Facts.Packages {
		if !compPkgs[pkg.ImportPath] {
			continue
		}
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
		if !compPkgs[pkg.ImportPath] {
			continue
		}
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
			}
		}
	}

	// 4c. Member overlap checks (M7). Effective members must not also be
	// covered by a resolved component dependency or an absorbed declaration.
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
				Kind:    report.MemberOverlap,
				Message: fmt.Sprintf("member overlap with dependency %q: overlapping packages: %s", di.Component, strings.Join(overlapping, ", ")),
			})
		}
	}

	for _, absDep := range in.Manifest.AbsorbedDependencies {
		var overlapping []string
		for pkg := range compPkgs {
			matched, err := path.Match(absDep.ImportPath, pkg)
			if err == nil && matched {
				overlapping = append(overlapping, pkg)
			}
		}
		if len(overlapping) == 0 {
			continue
		}
		sort.Strings(overlapping)
		violations = append(violations, report.Finding{
			Kind:    report.MemberOverlap,
			Message: fmt.Sprintf("member overlap with absorbed dependency %q: overlapping packages: %s", absDep.ImportPath, strings.Join(overlapping, ", ")),
		})
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
	for _, unresolved := range in.Facts.UnresolvedImports {
		warnings = append(warnings, report.Finding{
			Kind:    report.AnalysisLimitation,
			Message: fmt.Sprintf("unresolved import %q in source file %q of package %q is an analysis limitation", unresolved.ImportPath, unresolved.File, unresolved.Package),
			Location: report.Location{
				File: unresolved.File,
			},
		})
	}

	for _, pkg := range in.Facts.BodilessAbsorbedPackages {
		warnings = append(warnings, report.Finding{
			Kind:    report.AnalysisLimitation,
			Message: fmt.Sprintf("absorbed package %q has no source bodies; its authority could not be analyzed", pkg),
		})
	}

	for _, esc := range in.Facts.FuncValueEscapes {
		warnings = append(warnings, report.Finding{
			Kind:    report.AbsorbedFuncValueEscape,
			Message: fmt.Sprintf("member package %q takes function value %q from absorbed package without calling it; body is unanalyzed", esc.Package, esc.Symbol),
			Location: report.Location{
				File: esc.File,
				Line: esc.Line,
			},
		})
	}

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

			if info.di.InterfaceStyle == manifest.InterfaceStylePackageSurface {
				continue
			}

			normCallee := NormalizeInterfaceSymbol(edge.Callee)
			if !info.normSymbols[normCallee] {
				// Undeclared call -> Violation
				violations = append(violations, report.Finding{
					Kind:    report.CallsUndeclaredInterface,
					Message: fmt.Sprintf("call from %q to undeclared interface symbol %q of dependency %q", edge.Caller, edge.Callee, info.di.Component),
				})
			}
		}
	}

	// 5. Check for unused declared dependencies

	// Check component dependencies
	for _, dep := range in.Manifest.ComponentDependencies {
		if dep.AutoAttached {
			continue
		}
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

	// 6. Ensure deterministic sorting (sorted alphabetically by Message, then Location)
	sort.Slice(violations, func(i, j int) bool {
		if violations[i].Message != violations[j].Message {
			return violations[i].Message < violations[j].Message
		}
		if violations[i].Location.File != violations[j].Location.File {
			return violations[i].Location.File < violations[j].Location.File
		}
		return violations[i].Location.Line < violations[j].Location.Line
	})
	sort.Slice(warnings, func(i, j int) bool {
		if warnings[i].Message != warnings[j].Message {
			return warnings[i].Message < warnings[j].Message
		}
		if warnings[i].Location.File != warnings[j].Location.File {
			return warnings[i].Location.File < warnings[j].Location.File
		}
		return warnings[i].Location.Line < warnings[j].Location.Line
	})

	var depBoundaries []report.DependencyBoundary
	for _, di := range in.DepIfaces {
		depBoundaries = append(depBoundaries, report.DependencyBoundary{
			Component:              di.Component,
			OwnCheckRuns:           di.OwnCheckRuns,
			CertificationReference: di.CertificationReference,
		})
	}
	sort.Slice(depBoundaries, func(i, j int) bool {
		return depBoundaries[i].Component < depBoundaries[j].Component
	})

	return report.ConformanceReport{
		Component:    in.Manifest.Name,
		Dependencies: depBoundaries,
		Violations:   violations,
		Warnings:     warnings,
	}
}

// buildMembership returns the package paths owned by a component. An empty
// members declaration preserves FR1 by owning every supplied package. When
// members are declared, entries are matched against package import paths; the
// interface package is then added from the package fact containing each
// declared interface file.
func buildMembership(m manifest.Manifest, packages []facts.PackageFact) map[string]bool {
	members := make(map[string]bool)
	if len(m.Members) == 0 {
		for _, pkg := range packages {
			members[pkg.ImportPath] = true
		}
		return members
	}

	for _, pkg := range packages {
		for _, pattern := range m.Members {
			matched, err := path.Match(pattern, pkg.ImportPath)
			if err == nil && matched {
				members[pkg.ImportPath] = true
				break
			}
		}
	}

	interfaceFiles := make(map[string]bool, len(m.InterfaceFiles))
	for _, file := range m.InterfaceFiles {
		interfaceFiles[file] = true
	}
	if len(interfaceFiles) == 0 {
		return members
	}
	for _, pkg := range packages {
		for _, sym := range pkg.ExportedSymbols {
			if interfaceFiles[sym.File] {
				members[pkg.ImportPath] = true
				break
			}
		}
	}

	return members
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

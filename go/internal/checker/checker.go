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
	Caps      []capanalyzer.CapabilityFinding // unused in this step, used in Step 5
	Policy    capanalyzer.CapabilityPolicy    // unused in this step, used in Step 5
}

// Check evaluates the injected inputs against architectural contracts and returns a ConformanceReport.
// This function is completely pure and is a basis for the checker being ambient-authority-free.
//
// Currently implements FR3 (dependency allowlist rules). FR4/FR5/FR6 will extend this same function.
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

	// 5. Check for unused declared dependencies
	var warnings []report.Finding

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
// to resolve the underlying type name (e.g. "(*example.com/store.DB)" -> "example.com/store.DB").
func cleanReceiverType(receiver string) string {
	res := receiver
	if strings.HasPrefix(res, "(*") && strings.HasSuffix(res, ")") {
		res = res[2 : len(res)-1]
	}
	return res
}

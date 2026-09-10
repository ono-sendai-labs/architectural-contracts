// Package checker defines the architectural contracts checker. It is a pure core component
// that analyzes package facts, manifests, capabilities, and policies to produce a conformance report.
//
// The core Check function is designed to be completely pure, having no side effects, no I/O, and
// no dependencies on ambient authority.
//
// Component Contract (FR10):
// - What it does: Evaluates Go packages against their declared manifests to verify dependency, interface boundary, and authority conformance. Every boundary and authority decision comes from the typed reference/import facts and the resolved stdlib authority map: boundary edges through ClassifyEdges, capability findings through AggregateAuthority and the policy.
// - What it requires: Fully resolved inputs — parsed manifest, package facts carrying the typed reference/import/bypass edges, dependency interfaces, the StdlibAuthority port, the target SDK key, and the capability policy. Fail-closed tool errors (dependency overlap, stdlib inventory gaps, missing type data) are returned as errors, not findings.
// - What it provides: A deterministic ConformanceReport indicating compliance and detailing any architectural violations or warnings.
// - Ambient Authority: This component is guaranteed-pure and holds no ambient authority (no filesystem I/O, no network, no reflection, and no process execution).
package checker

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/capanalyzer"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibauthority"
)

// Inputs contains all injected facts and configurations required for check execution.
type Inputs struct {
	Manifest  manifest.Manifest
	Facts     facts.PackageFacts
	DepIfaces []facts.DependencyInterface // resolved direct component dependencies
	// Authority is the resolved stdlib authority port (the Step 4 map);
	// SDKKey is the target configuration the classification is expected to
	// have been decided under (N3). A nil Authority is a tool error.
	Authority stdlibauthority.StdlibAuthority
	SDKKey    stdlibauthority.SDKKey
	Policy    capanalyzer.CapabilityPolicy // capability check policy
}

// Check evaluates the injected inputs against architectural contracts and
// returns a ConformanceReport. A non-nil error is a fail-closed tool error
// (dependency overlap, stdlib inventory gap, missing type data): no verdict
// is produced and the caller must not publish artifacts.
//
// Implements:
// - FR3 (dependency allowlist rules, via the typed import classification)
// - FR4 (well-formedness rule: Method placement)
// - FR5 (cross-component call-boundary rule, via the declaring-object rule)
// - FR6 (policy-aware ambient-authority rule, using StrictPolicy() merged with declared_authority)
// - DR-11 (analysis-defeating constructs, via the bypass aggregation)
// - M7 (member-overlap check)
func Check(in Inputs) (report.ConformanceReport, error) {
	// 1. Build component membership set. Facts may include transitive packages
	// needed for type checking, so only the resulting set is swept as owned code.
	compPkgs := buildMembership(in.Manifest, in.Facts.Packages)
	members, err := facts.NewMemberSet(sortedKeys(compPkgs)...)
	if err != nil {
		return report.ConformanceReport{}, fmt.Errorf("building the component membership: %w", err)
	}

	// 2. Classify every typed reference and import edge against the exact
	// declaring-object boundary and the stdlib authority map (FR3/FR5,
	// DR-10). Fails closed on tool errors before any verdict forms.
	classified, err := ClassifyEdges(members, in.DepIfaces, in.Facts.References, in.Facts.Imports, in.Authority, in.SDKKey)
	if err != nil {
		return report.ConformanceReport{}, err
	}

	var violations []report.Finding
	for _, b := range classified.Boundary {
		violations = append(violations, boundaryReportFinding(b))
	}

	// 3. FR4 Method placement check
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

	// 4. Member overlap checks (M7). Effective members must not also be
	// covered by a resolved component dependency.
	var warnings []report.Finding
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

	// 5. Check for unused declared dependencies: a dependency is used when an
	// import edge resolved behind its boundary (declared or auto-attached).
	usedDeps := make(map[string]bool, len(classified.UsedDependencies))
	for _, dep := range classified.UsedDependencies {
		usedDeps[dep] = true
	}
	for _, dep := range in.Manifest.ComponentDependencies {
		if dep.AutoAttached {
			continue
		}
		if !usedDeps[dep.Name] {
			warnings = append(warnings, report.Finding{
				Kind:    report.UnusedDependency,
				Message: fmt.Sprintf("declared component dependency %q is unused", dep.Name),
			})
		}
	}

	// A checked-fail dependency remains a usable architectural boundary: its
	// surface still owns the packages and symbols, while its own violations
	// stay in its report. Record one downstream warning per failed report rather
	// than copying findings or failing the dependent a second time. Native stale
	// surfaces are likewise usable, but their best-effort byte audit is visible.
	for _, di := range in.DepIfaces {
		if di.Provenance == facts.DependencyProvenanceCheckedFail {
			warnings = append(warnings, report.Finding{
				Kind:    report.DependencyCheckFailed,
				Message: fmt.Sprintf("dependency %q failed its conformance check", di.Component),
			})
		}
		if di.Freshness == facts.DependencyFreshnessStale {
			warnings = append(warnings, report.Finding{
				Kind:    report.DependencySurfaceStale,
				Message: fmt.Sprintf("dependency %q surface is stale", di.Component),
			})
		}
	}

	// 6. FR6 policy-aware ambient-authority rule (DR-11, DR-17): aggregate
	// the stdlib classifications and bypass observations, then apply the
	// effective policy.
	effectivePolicy := deriveEffectivePolicy(in.Policy, in.Manifest.DeclaredAuthority)
	authorityFindings, err := AggregateAuthority(classified.Authority, in.Facts.Bypasses, in.SDKKey)
	if err != nil {
		return report.ConformanceReport{}, err
	}
	authorityViolations, authorityWarnings := ApplyAuthorityPolicy(authorityFindings, effectivePolicy)
	violations = append(violations, authorityViolations...)
	warnings = append(warnings, authorityWarnings...)

	// 7. Ensure deterministic sorting (sorted alphabetically by Message, then Location)
	slices.SortFunc(violations, func(a, b report.Finding) int {
		if c := strings.Compare(a.Message, b.Message); c != 0 {
			return c
		}
		if c := strings.Compare(a.Location.File, b.Location.File); c != 0 {
			return c
		}
		return a.Location.Line - b.Location.Line
	})
	slices.SortFunc(warnings, func(a, b report.Finding) int {
		if c := strings.Compare(a.Message, b.Message); c != 0 {
			return c
		}
		if c := strings.Compare(a.Location.File, b.Location.File); c != 0 {
			return c
		}
		return a.Location.Line - b.Location.Line
	})

	var depBoundaries []report.DependencyBoundary
	for _, di := range in.DepIfaces {
		boundary := report.DependencyBoundary{Component: di.Component}
		// Zero-value interfaces are retained for pure checker fixtures written
		// before the surface status axes existed. Every production surface
		// resolver supplies provenance, freshness, and authority, so published
		// reports always carry all three axes.
		if di.Provenance != "" || di.Freshness != "" || di.Authority.Known || len(di.Authority.Set) > 0 {
			boundary.Provenance = report.DependencyProvenance(di.Provenance)
			boundary.Freshness = report.DependencyFreshness(di.Freshness)
			if di.Authority.Known {
				boundary.Authority = report.DependencyAuthorityDeclared
				boundary.DeclaredAuthority = slices.Clone(di.Authority.Set)
				slices.Sort(boundary.DeclaredAuthority)
			} else {
				boundary.Authority = report.DependencyAuthorityUnknown
			}
		}
		depBoundaries = append(depBoundaries, boundary)
	}
	slices.SortFunc(depBoundaries, func(a, b report.DependencyBoundary) int {
		return strings.Compare(a.Component, b.Component)
	})

	return report.ConformanceReport{
		Component:    in.Manifest.Name,
		Dependencies: depBoundaries,
		Violations:   violations,
		Warnings:     warnings,
	}, nil
}

// boundaryReportFinding converts one boundary observation to its report
// finding: the exact site is the location, and the message names the
// referent, the owning dependency (when the edge reached one), and the site.
func boundaryReportFinding(b BoundaryObservation) report.Finding {
	switch b.Kind {
	case report.CallsUndeclaredInterface:
		return report.Finding{
			Kind:     b.Kind,
			Message:  fmt.Sprintf("package %q references undeclared interface symbol %q of dependency %q at %s:%d", b.FromPackage, b.Referent, b.Dependency, b.Site.File, b.Site.Line),
			Location: report.Location{File: b.Site.File, Line: b.Site.Line},
		}
	case report.UndeclaredDependency:
		if b.Referent == "" {
			return report.Finding{
				Kind:     b.Kind,
				Message:  fmt.Sprintf("package %q imports undeclared dependency %q at %s:%d", b.FromPackage, b.ReferentPackage, b.Site.File, b.Site.Line),
				Location: report.Location{File: b.Site.File, Line: b.Site.Line},
			}
		}
		return report.Finding{
			Kind:     b.Kind,
			Message:  fmt.Sprintf("package %q references undeclared dependency symbol %q at %s:%d", b.FromPackage, b.Referent, b.Site.File, b.Site.Line),
			Location: report.Location{File: b.Site.File, Line: b.Site.Line},
		}
	default:
		return report.Finding{
			Kind:     b.Kind,
			Message:  fmt.Sprintf("boundary violation %s at %s:%d", b.Kind, b.Site.File, b.Site.Line),
			Location: report.Location{File: b.Site.File, Line: b.Site.Line},
		}
	}
}

// buildMembership returns the package paths owned by a component. An empty
// members declaration preserves FR1 by owning every supplied package. When
// members are declared, membership is exact canonical identity; the interface
// package is then added from the package fact containing each declared
// interface file.
func buildMembership(m manifest.Manifest, packages []facts.PackageFact) map[string]bool {
	members := make(map[string]bool)
	if len(m.Members) == 0 {
		for _, pkg := range packages {
			members[pkg.ImportPath] = true
		}
		return members
	}

	declared := make(map[string]bool, len(m.Members))
	for _, member := range m.Members {
		declared[member] = true
	}
	for _, pkg := range packages {
		if declared[pkg.ImportPath] {
			members[pkg.ImportPath] = true
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

// sortedKeys returns the map's keys in sorted order.
func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

package goanalysis

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"strconv"

	"golang.org/x/tools/go/packages"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/facts"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/hostpolicy"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/symbol"
)

// ScanReferences scans member sources and emits the typed reference and
// import edges of the Step 6 reference vocabulary (design R1–R4, DR-04).
//
// Only member packages are inspected, even though the loaded package set
// still contains the transitive closure: a reference whose declaring object
// lives in a non-member package is recorded as an edge from the member, and
// every edge's FromPackage is a member. The AST + types.Info walk names the
// declaring object the source writes (types.Info.Uses and
// types.Info.Selections under the declaring-object rule, DR-04), so no
// dynamically resolved implementation ever becomes an edge.
//
// Overlapping observations — the selector ident of one selection appears in
// both Uses and Selections — collapse to one edge per semantic site; the same
// referent at another site stays a distinct edge. The result is sorted and
// duplicate-free, so identical inputs produce identical output in any map
// enumeration order (N4).
//
// Builtins, labels and other universe objects (nil package) are not declared
// symbols and are skipped. Malformed input — an import path that cannot be
// read, or a position outside the component root — is an error: the scan
// returns no partial facts.
//
// root is the component root (native mode) or active workspace directory
// (layout mode) used to make SourceSite.File component-relative, mirroring
// extractSymbols.
//
// Import resolution states: ImportResolved when the written path resolved to
// a loaded package, ImportUnresolved otherwise. The layout-aware
// ImportMissingTypeData refinement is applied by the Step 6 classifier, which
// owns the layout/SDK knowledge this scanner deliberately does not consult.
func ScanReferences(
	pkgs []*packages.Package,
	members facts.MemberSet,
	root string,
) ([]facts.ReferenceEdge, []facts.ImportEdge, error) {
	refs := make(map[facts.ReferenceKey]struct{})
	imports := make(map[facts.ImportKey]struct{})

	for _, p := range pkgs {
		if p == nil {
			continue
		}
		if !members.Contains(hostpolicy.CanonicalizePath(p.PkgPath)) {
			continue
		}
		// Incomplete member type information is a fail-closed tool error, not
		// a silent omission: a member package whose Uses/Selections/imports
		// cannot be fully walked must never yield a partial-facts pass.
		if p.TypesInfo == nil || p.Fset == nil || p.Syntax == nil {
			return nil, nil, fmt.Errorf("scanning references: member package %q has incomplete type or syntax data", p.PkgPath)
		}
		if err := scanPackage(p, members, root, refs, imports); err != nil {
			return nil, nil, err
		}
	}

	refEdges := make([]facts.ReferenceEdge, 0, len(refs))
	for key := range refs {
		refEdges = append(refEdges, facts.ReferenceEdge{
			Kind:            key.Kind,
			FromPackage:     key.FromPackage,
			ReferentPackage: key.ReferentPackage,
			Referent:        key.Referent,
			Site:            key.Site,
		})
	}
	refEdges = facts.DedupReferenceEdges(facts.SortReferenceEdges(refEdges))

	importEdges := make([]facts.ImportEdge, 0, len(imports))
	for key := range imports {
		importEdges = append(importEdges, facts.ImportEdge{
			ImportingPackage: key.ImportingPackage,
			ImportPath:       key.ImportPath,
			Resolution:       key.Resolution,
			Site:             key.Site,
		})
	}
	importEdges = facts.DedupImportEdges(facts.SortImportEdges(importEdges))

	return refEdges, importEdges, nil
}

// scanPackage walks one member package's syntax and type info, adding
// reference and import edges to the accumulation sets.
func scanPackage(
	p *packages.Package,
	members facts.MemberSet,
	root string,
	refs map[facts.ReferenceKey]struct{},
	imports map[facts.ImportKey]struct{},
) error {
	fromPkg := hostpolicy.CanonicalizePath(p.PkgPath)

	for ident, obj := range p.TypesInfo.Uses {
		if !externalObject(obj, members) {
			continue
		}
		kind, id, err := classifyingReference(obj)
		if err != nil {
			return fmt.Errorf("scanning references of %s: %w", fromPkg, err)
		}
		site, err := sourceSite(p.Fset, root, ident.Pos())
		if err != nil {
			return fmt.Errorf("scanning references of %s: %w", fromPkg, err)
		}
		refs[siteKey(kind, fromPkg, id, site)] = struct{}{}
	}

	for sel, selection := range p.TypesInfo.Selections {
		obj := selection.Obj()
		if !externalObject(obj, members) {
			continue
		}
		kind, id, err := classifyingReference(obj)
		if err != nil {
			return fmt.Errorf("scanning references of %s: %w", fromPkg, err)
		}
		site, err := sourceSite(p.Fset, root, sel.Sel.Pos())
		if err != nil {
			return fmt.Errorf("scanning references of %s: %w", fromPkg, err)
		}
		refs[siteKey(kind, fromPkg, id, site)] = struct{}{}
	}

	for _, file := range p.Syntax {
		if file == nil {
			continue
		}
		for _, decl := range file.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.IMPORT {
				continue
			}
			for _, spec := range genDecl.Specs {
				importSpec, ok := spec.(*ast.ImportSpec)
				if !ok || importSpec.Path == nil {
					return fmt.Errorf("scanning imports of %s: malformed import declaration", fromPkg)
				}
				path, err := strconv.Unquote(importSpec.Path.Value)
				if err != nil || path == "" {
					return fmt.Errorf("scanning imports of %s: malformed import path %q", fromPkg, importSpec.Path.Value)
				}
				site, err := sourceSite(p.Fset, root, importSpec.Path.Pos())
				if err != nil {
					return fmt.Errorf("scanning imports of %s: %w", fromPkg, err)
				}
				resolution := facts.ImportUnresolved
				if imp, resolved := p.Imports[path]; resolved {
					// A nil import-package entry means the loader knows the
					// path but presents no type data for it: the deliberate
					// missing-type-data state, never a resolved pass.
					if imp == nil {
						resolution = facts.ImportMissingTypeData
					} else {
						resolution = facts.ImportResolved
					}
				}
				imports[facts.ImportKey{
					ImportingPackage: fromPkg,
					ImportPath:       hostpolicy.CanonicalizePath(path),
					Resolution:       resolution,
					Site:             site,
				}] = struct{}{}
			}
		}
	}
	return nil
}

// externalObject reports whether obj denotes a declared symbol outside the
// member set: non-nil, package-scoped (not a universe object such as builtins
// or the predeclared error type), and not a member package's object. Package
// names and labels are not declared symbols and are rejected here.
func externalObject(obj types.Object, members facts.MemberSet) bool {
	switch obj.(type) {
	case nil, *types.PkgName, *types.Label, *types.Builtin:
		return false
	}
	if obj.Pkg() == nil {
		return false
	}
	return !members.Contains(hostpolicy.CanonicalizePath(obj.Pkg().Path()))
}

// classifyingReference converts one resolved object to its declaring-object
// SymbolID (DR-04) and the reference kind of the edge that names it.
func classifyingReference(obj types.Object) (facts.ReferenceKind, facts.SymbolID, error) {
	id, err := symbol.FromObject(obj)
	if err != nil {
		return "", "", err
	}
	var kind facts.ReferenceKind
	switch o := obj.(type) {
	case *types.Func:
		if sig, ok := o.Type().(*types.Signature); ok && sig.Recv() != nil {
			kind = facts.RefMethod
		} else {
			kind = facts.RefFunc
		}
	case *types.Var:
		if o.Parent() != nil && o.Pkg() != nil && o.Parent() == o.Pkg().Scope() {
			kind = facts.RefVar
		} else {
			kind = facts.RefField
		}
	case *types.Const:
		kind = facts.RefConst
	case *types.TypeName:
		kind = facts.RefType
	default:
		return "", "", fmt.Errorf("object %q (%T) is not a referenceable declared symbol", obj.Name(), obj)
	}
	return kind, id, nil
}

// sourceSite converts an AST position to a validated component-relative site.
func sourceSite(fset *token.FileSet, root string, pos token.Pos) (facts.SourceSite, error) {
	position := fset.Position(pos)
	if !position.IsValid() {
		return facts.SourceSite{}, fmt.Errorf("object position is not valid")
	}
	rel, err := filepath.Rel(root, position.Filename)
	if err != nil {
		return facts.SourceSite{}, fmt.Errorf("resolving %q relative to component root: %w", position.Filename, err)
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	site := facts.SourceSite{File: rel, Line: position.Line}
	if err := site.Validate(); err != nil {
		return facts.SourceSite{}, err
	}
	return site, nil
}

func siteKey(kind facts.ReferenceKind, fromPkg string, id facts.SymbolID, site facts.SourceSite) facts.ReferenceKey {
	return facts.ReferenceKey{
		Kind:            kind,
		FromPackage:     fromPkg,
		ReferentPackage: facts.SymbolIDPackage(id),
		Referent:        id,
		Site:            site,
	}
}

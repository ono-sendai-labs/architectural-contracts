package symbol

import (
	"go/ast"
	"go/token"
	"go/types"
	"slices"
)

// ExtractSurface emits the exact declared surface of an already type-checked
// package (task req 6): every exported package-level function, variable,
// constant and named/alias type, every exported declared method, and the
// aggregate init — nothing else. This is both the emitted surface's symbol
// set and the set the check classifies references against.
//
// Under the declaring-object rule (DR-04) interface method specs and struct
// fields have no ID of their own; references to them are authorised by their
// declaring type, which the extractor emits directly.
//
// Alias expansion (task req 5): an exported alias always emits its own
// top-level ID; when its target is a same-package named type the target's key
// is additionally emitted so the alias's members resolve. A cross-package
// target is never claimed: the alias key alone is emitted.
//
// Package paths are canonicalized through the host-policy hook (inside the
// object conversion) so surfaces are keyed in the canonical namespace. The
// result is sorted and duplicate-free: repeated or reordered observations of
// the same declaration collapse to one entry, so identical inputs produce
// byte-identical output after schema encoding (N4).
func ExtractSurface(files []*ast.File, info *types.Info) []SymbolID {
	seen := map[SymbolID]bool{}
	add := func(id SymbolID) {
		if id != "" {
			seen[id] = true
		}
	}

	for _, file := range files {
		if file == nil {
			continue
		}
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				obj := info.Defs[d.Name]
				if obj == nil {
					continue
				}
				id, err := FromObject(obj)
				if err != nil || !surfaceExportable(obj) {
					continue
				}
				add(id)

			case *ast.GenDecl:
				if d.Tok == token.IMPORT {
					continue
				}
				for _, spec := range d.Specs {
					var names []*ast.Ident
					switch s := spec.(type) {
					case *ast.ValueSpec:
						names = s.Names
					case *ast.TypeSpec:
						names = []*ast.Ident{s.Name}
					default:
						continue
					}
					for _, name := range names {
						obj := info.Defs[name]
						if obj == nil {
							continue
						}
						id, err := FromObject(obj)
						if err != nil || !surfaceExportable(obj) {
							continue
						}
						add(id)
						// Alias expansion: a same-package named-type target
						// is additionally emitted so the alias's members
						// resolve; a cross-package target is not claimed.
						if tn, ok := obj.(*types.TypeName); ok && tn.IsAlias() {
							add(aliasTargetID(tn))
						}
					}
				}
			}
		}
	}

	result := make([]SymbolID, 0, len(seen))
	for id := range seen {
		result = append(result, id)
	}
	slices.SortFunc(result, Compare)
	return result
}

// surfaceExportable reports whether obj belongs on a component's declared
// surface: package-level or declared-method objects with an exported name,
// plus the aggregate init.
func surfaceExportable(obj types.Object) bool {
	switch o := obj.(type) {
	case *types.Func:
		if o.Name() == "init" {
			return true
		}
		return ast.IsExported(o.Name())
	default:
		return ast.IsExported(obj.Name())
	}
}

// aliasTargetID returns the v1 ID of an alias's target type when the target
// is a named type declared in the same package; otherwise it returns "" so
// no foreign declaration is claimed.
func aliasTargetID(alias *types.TypeName) SymbolID {
	target := types.Unalias(alias.Type())
	named, ok := target.(*types.Named)
	if !ok {
		return ""
	}
	targetObj := named.Obj()
	if targetObj.Pkg() == nil || targetObj.Pkg().Path() != alias.Pkg().Path() {
		return ""
	}
	id, err := canonicalID(targetObj.Pkg().Path(), targetObj.Name())
	if err != nil {
		return ""
	}
	return id
}

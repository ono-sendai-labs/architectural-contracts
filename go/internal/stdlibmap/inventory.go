package stdlibmap

import (
	"fmt"
	"go/types"
	"sort"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/symbol"
)

// Loader is the injectable batch package-loading seam (task req 9): it loads
// every requested importable package for the target configuration described by
// the generation environment. Implementations must not consult global
// process state beyond the configuration they were constructed with.
type Loader interface {
	// Load type-loads every path and returns the loaded *types.Package per
	// path. A load or type error for any path fails the whole batch with an
	// actionable error; a path absent from the result is likewise an error,
	// so a partial result can never masquerade as a complete one.
	Load(paths []string) (map[string]*types.Package, error)
}

// LoaderFunc adapts a function to the Loader seam.
type LoaderFunc func(paths []string) (map[string]*types.Package, error)

// Load implements Loader.
func (f LoaderFunc) Load(paths []string) (map[string]*types.Package, error) { return f(paths) }

// Inventory is the independent declaration oracle for one SDK configuration
// (design I3, DR-05): the total package list and, for every importable
// package, its complete externally referencable exported symbol set plus
// exactly one aggregate init. Every collection is sorted and duplicate-free.
type Inventory struct {
	// Packages is the total normalized package enumeration, sorted by path.
	Packages []PackageEntry
	// Symbols is the total symbol inventory over all importable packages:
	// every exported package-scope object and every exported method of every
	// exported named or alias type, keyed under the declaring-object rule.
	// The aggregate init is not here; it lives in Inits. Interface method
	// specs and struct fields have no IDs (DR-04) and are never emitted.
	Symbols []symbol.SymbolID
	// Inits holds exactly one aggregate pkg.init per importable package,
	// sorted.
	Inits []symbol.SymbolID
}

// observed pairs one inventoried ID with the object identity that produced
// it, so a genuine canonical-ID collision — two distinct declarations
// canonicalizing to one ID — is detected and rejected rather than silently
// deduplicated (task req 6). Repeated observations of the same object (for
// example an alias expansion and its target's own declaration) collapse.
type observed struct {
	id  symbol.SymbolID
	obj types.Object
}

// BuildInventory inventories packages through loader (task reqs 3–6): every
// importable package is loaded and its exported surface walked; every
// non-importable (internal) package is retained in the package list but
// contributes no symbols or init. Any load error, type error, malformed
// object conversion, or canonical-ID collision fails the whole generation
// with an actionable error and a nil Inventory — a partial inventory is never
// returned.
func BuildInventory(packages []PackageEntry, loader Loader) (*Inventory, error) {
	if loader == nil {
		return nil, fmt.Errorf("building the stdlib inventory: the package loader is required")
	}
	var importable []string
	for _, p := range packages {
		if p.Importable {
			importable = append(importable, p.Path)
		}
	}
	loaded, err := loader.Load(importable)
	if err != nil {
		return nil, fmt.Errorf("building the stdlib inventory: %w", err)
	}

	all := make([]observed, 0, len(importable)*64)
	inits := make([]symbol.SymbolID, 0, len(importable))
	for _, path := range importable {
		pkg, ok := loaded[path]
		if pkg == nil || !ok {
			return nil, fmt.Errorf("building the stdlib inventory: package %q was not loaded; the loader result is incomplete", path)
		}
		if pkg.Path() != path {
			return nil, fmt.Errorf("building the stdlib inventory: loader returned package %q for requested path %q", pkg.Path(), path)
		}
		pkgObserved, err := packageSymbols(pkg)
		if err != nil {
			return nil, fmt.Errorf("building the stdlib inventory: %w", err)
		}
		init, err := initID(pkg.Path())
		if err != nil {
			return nil, fmt.Errorf("building the stdlib inventory: %w", err)
		}
		all = append(all, pkgObserved...)
		inits = append(inits, init)
	}

	symbols, err := mergeObserved(all)
	if err != nil {
		return nil, fmt.Errorf("building the stdlib inventory: %w", err)
	}
	sortIDs(symbols)
	sortIDs(inits)
	return &Inventory{
		Packages: sortedEntries(packages),
		Symbols:  symbols,
		Inits:    inits,
	}, nil
}

// objectID keys one exported package-scope object under the declaring-object
// rule. unsafe's exported builtins (*types.Builtin) are compiler intrinsics
// with no *types.Func identity; they are externally referencable exported
// objects and belong in the total inventory (design DR-05, spike finding 5),
// keyed by the canonical top-level ID.
func objectID(pkg *types.Package, obj types.Object) (symbol.SymbolID, error) {
	if _, ok := obj.(*types.Builtin); ok {
		id, err := symbol.Parse(pkg.Path() + "." + obj.Name())
		if err != nil {
			return "", fmt.Errorf("inventorying builtin %q in package %q: %w", obj.Name(), pkg.Path(), err)
		}
		return id, nil
	}
	return symbol.FromObject(obj)
}

// packageSymbols walks one loaded package's scope and returns its observed
// declarations (task reqs 4–5): every exported package-scope object, the
// same-package named-type targets of exported aliases, and every exported
// method declared on those types. Interface method specs and struct fields
// have no ID of their own and are skipped (DR-04).
func packageSymbols(pkg *types.Package) ([]observed, error) {
	scope := pkg.Scope()
	obs := make([]observed, 0, scope.Len())
	var methodTargets []*types.Named
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		if !obj.Exported() {
			continue
		}
		id, err := objectID(pkg, obj)
		if err != nil {
			return nil, fmt.Errorf("inventorying package %q: %w", pkg.Path(), err)
		}
		obs = append(obs, observed{id: id, obj: obj})
		if tn, ok := obj.(*types.TypeName); ok {
			named := methodScanTarget(tn)
			if named != nil {
				methodTargets = append(methodTargets, named)
			}
			// Alias expansion (design DR-04, task req 4): a same-package
			// named-type target's key is additionally emitted so the alias's
			// members resolve; a cross-package target is not claimed.
			if tn.IsAlias() {
				if target := aliasSamePackageTarget(tn); target != nil {
					targetID, err := symbol.FromObject(target)
					if err != nil {
						return nil, fmt.Errorf("inventorying package %q: %w", pkg.Path(), err)
					}
					obs = append(obs, observed{id: targetID, obj: target})
					methodTargets = append(methodTargets, namedTypeOf(target))
				}
			}
		}
	}
	for _, named := range methodTargets {
		for i := 0; i < named.NumMethods(); i++ {
			m := named.Method(i)
			if !m.Exported() {
				continue
			}
			id, err := symbol.FromObject(m)
			if err != nil {
				return nil, fmt.Errorf("inventorying package %q: %w", pkg.Path(), err)
			}
			obs = append(obs, observed{id: id, obj: m})
		}
	}
	return obs, nil
}

// methodScanTarget returns the named type whose methods an exported type
// declaration contributes to the inventory: the named type itself, with
// aliases resolved (task req 4 — methods of exported named or alias types).
// Instantiations never appear here because the package scope only holds
// declarations.
func methodScanTarget(tn *types.TypeName) *types.Named {
	t := types.Unalias(tn.Type())
	named, ok := t.(*types.Named)
	if !ok {
		return nil
	}
	return named
}

// aliasSamePackageTarget returns the alias's target TypeName when it is a
// named type declared in the same package; otherwise nil (no foreign
// declaration is claimed).
func aliasSamePackageTarget(alias *types.TypeName) *types.TypeName {
	target := types.Unalias(alias.Type())
	named, ok := target.(*types.Named)
	if !ok {
		return nil
	}
	obj := named.Obj()
	if obj.Pkg() == nil || obj.Pkg().Path() != alias.Pkg().Path() {
		return nil
	}
	return obj
}

// namedTypeOf returns the named type a TypeName object declares.
func namedTypeOf(tn *types.TypeName) *types.Named {
	named, _ := types.Unalias(tn.Type()).(*types.Named)
	return named
}

// initID assembles the aggregate init's canonical ID for pkgPath (task req
// 5). go/types does not scope an init object, so the ID is built with the
// canonical SymbolID constructor directly.
func initID(pkgPath string) (symbol.SymbolID, error) {
	id, err := symbol.Parse(pkgPath + ".init")
	if err != nil {
		return "", fmt.Errorf("assembling the aggregate init ID for %q: %w", pkgPath, err)
	}
	return id, nil
}

// mergeObserved collapses observations and rejects genuine canonical-ID
// collisions: the same ID produced by two distinct declaration objects is an
// inventory corruption and fails generation (task req 6).
func mergeObserved(all []observed) ([]symbol.SymbolID, error) {
	byID := make(map[symbol.SymbolID]types.Object, len(all))
	out := make([]symbol.SymbolID, 0, len(all))
	for _, ob := range all {
		if prev, ok := byID[ob.id]; ok {
			if prev != ob.obj {
				return nil, fmt.Errorf("canonical symbol ID collision: %q is produced by two distinct declarations", ob.id)
			}
			continue
		}
		byID[ob.id] = ob.obj
		out = append(out, ob.id)
	}
	return out, nil
}

func sortIDs(ids []symbol.SymbolID) {
	sort.Slice(ids, func(i, j int) bool { return symbol.Compare(ids[i], ids[j]) < 0 })
}

func sortedEntries(entries []PackageEntry) []PackageEntry {
	out := append([]PackageEntry(nil), entries...)
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

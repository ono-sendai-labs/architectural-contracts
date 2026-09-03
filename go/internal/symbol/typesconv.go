package symbol

import (
	"fmt"
	"go/types"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/hostpolicy"
)

// FromObject converts a go/types object to the SymbolID of the top-level
// declaration that owns it — the declaring-object rule (DR-04, task req 4).
//
// Package-level funcs, vars, consts and named/alias types key themselves;
// declared methods use Func.Origin() and their receiver's base named type
// (no pointer marker, no type arguments); interface method specs and struct
// fields key their declaring interface or struct type; the aggregate
// init is pkg.init. Objects that are not declared symbols — local
// variables, parameters, type parameters, builtins, package names — are
// rejected with an actionable error rather than guessed.
//
// The package path is canonicalized through the host-policy hook, so the
// emitted IDs are in the canonical namespace, persisted only that way.
func FromObject(obj types.Object) (SymbolID, error) {
	switch o := obj.(type) {
	case *types.Func:
		return fromFunc(o)
	case *types.Var:
		return fromVar(o)
	case *types.Const:
		return fromConst(o)
	case *types.TypeName:
		return fromTypeName(o)
	default:
		return "", fmt.Errorf("object %q (%T) is not a declared symbol", obj.Name(), obj)
	}
}

// FromSelection converts a field or method selection to the SymbolID of the
// top-level declaration that owns the selected member (DR-04): fields and
// interface method specs key their declaring type; promoted selections key
// the object that actually declares the member (via Selection.Obj and the
// selection index path); generic methods resolve to their origin.
func FromSelection(sel *types.Selection) (SymbolID, error) {
	if sel == nil {
		return "", fmt.Errorf("selection is nil")
	}
	switch o := sel.Obj().(type) {
	case *types.Func:
		return fromFunc(o)
	case *types.Var:
		return fromFieldSelection(sel, o)
	default:
		return "", fmt.Errorf("selection member %q (%T) is not a field or method", o.Name(), o)
	}
}

// fromFunc implements the Func rows of the declaring-object table.
func fromFunc(fn *types.Func) (SymbolID, error) {
	sig, ok := fn.Type().(*types.Signature)
	if !ok {
		return "", fmt.Errorf("func %q does not have a signature", fn.Name())
	}
	// The declaring object: instantiated methods resolve to their origin.
	origin := fn.Origin()
	if sig.Recv() == nil {
		if origin.Parent() != nil && origin.Pkg() != nil && origin.Parent() == origin.Pkg().Scope() {
			// Package-level function, or the aggregate init.
			if origin.Name() == "init" {
				return canonicalID(pkgPathOf(origin), "init")
			}
			return canonicalID(pkgPathOf(origin), origin.Name())
		}
		// A Func object with no receiver that is not package-level: an
		// interface method spec whose receiver is not recorded. Key the
		// declaring interface type; the object has no back pointer to it,
		// so resolve it by searching the package scope (deterministic:
		// scope names are sorted, and the object identifies exactly one
		// declaring interface).
		iface, err := declaringInterface(origin)
		if err != nil {
			return "", err
		}
		return canonicalID(pkgPathOf(iface.Obj()), iface.Obj().Name())
	}
	base, err := receiverBaseNamed(origin)
	if err != nil {
		return "", err
	}
	if _, isIface := base.Underlying().(*types.Interface); isIface {
		// An interface method spec keys its declaring interface type; there
		// is no method ID of its own (DR-04).
		return canonicalID(pkgPathOf(base.Obj()), base.Obj().Name())
	}
	return methodID(pkgPathOf(base.Obj()), base.Obj().Name(), origin.Name())
}

// fromVar implements the var rows: package-level vars key themselves; struct
// fields key their declaring struct type.
func fromVar(v *types.Var) (SymbolID, error) {
	if isPackageLevel(v) {
		return canonicalID(pkgPathOf(v), v.Name())
	}
	// A field (possibly embedded). Struct fields have no back pointer to
	// their declaring type either; resolve it by searching the package scope.
	named, err := declaringStruct(v)
	if err != nil {
		return "", err
	}
	return canonicalID(pkgPathOf(named.Obj()), named.Obj().Name())
}

// fromFieldSelection resolves a field selection to its declaring struct type
// by walking the selection index path from the receiver to the embedded
// struct that actually declares the field.
func fromFieldSelection(sel *types.Selection, field *types.Var) (SymbolID, error) {
	named, err := baseNamedOf(sel.Recv())
	if err != nil {
		return "", fmt.Errorf("selection of field %q: %w", field.Name(), err)
	}
	indices := sel.Index()
	for _, idx := range indices[:len(indices)-1] {
		st, ok := named.Underlying().(*types.Struct)
		if !ok {
			return "", fmt.Errorf("selection of field %q: %s is not a struct", field.Name(), named)
		}
		if idx >= st.NumFields() {
			return "", fmt.Errorf("selection of field %q: index %d out of range for %s", field.Name(), idx, named)
		}
		next, err := baseNamedOf(st.Field(idx).Type())
		if err != nil {
			return "", fmt.Errorf("selection of field %q: %w", field.Name(), err)
		}
		named = next
	}
	return canonicalID(pkgPathOf(named.Obj()), named.Obj().Name())
}

// fromConst implements the const row: package-level constants key themselves.
func fromConst(c *types.Const) (SymbolID, error) {
	if !isPackageLevel(c) {
		return "", fmt.Errorf("const %q is not a package-level declaration", c.Name())
	}
	return canonicalID(pkgPathOf(c), c.Name())
}

// fromTypeName implements the type rows: named and alias types key
// themselves. Aliases keep their own top-level ID; expansion to the target's
// key is the extractor's job (task req 5), not the conversion's.
func fromTypeName(tn *types.TypeName) (SymbolID, error) {
	if !isPackageLevel(tn) {
		return "", fmt.Errorf("type %q is not a package-level declaration (type parameters are not declared symbols)", tn.Name())
	}
	return canonicalID(pkgPathOf(tn), tn.Name())
}

// pkgPathOf returns obj's package import path, or "" for universe and
// otherwise unnamed objects (go/types reports Pkg() == nil for predeclared
// identifiers such as error). Callers turn "" into an actionable error.
func pkgPathOf(obj types.Object) string {
	if obj.Pkg() == nil {
		return ""
	}
	return obj.Pkg().Path()
}

// canonicalID assembles a v1 top-level ID for pkgPath (canonicalized through
// the host-policy hook) and name. Parse cannot fail for names taken from
// real Go objects; the error is surfaced with context if it ever does.
func canonicalID(pkgPath, name string) (SymbolID, error) {
	if pkgPath == "" {
		return "", fmt.Errorf("cannot form a symbol ID for %q: the declaring package is unknown (universe or unnamed object)", name)
	}
	id, err := Parse(hostpolicy.CanonicalizePath(pkgPath) + "." + name)
	if err != nil {
		return "", fmt.Errorf("converting object to symbol ID: %w", err)
	}
	return id, nil
}

// methodID assembles a v1 method ID "(pkg.Type).Method" for the canonical
// pkgPath.
func methodID(pkgPath, typeName, method string) (SymbolID, error) {
	if pkgPath == "" {
		return "", fmt.Errorf("cannot form a symbol ID for (%s).%s: the declaring package is unknown (universe or unnamed object)", typeName, method)
	}
	id, err := Parse("(" + hostpolicy.CanonicalizePath(pkgPath) + "." + typeName + ")." + method)
	if err != nil {
		return "", fmt.Errorf("converting method to symbol ID: %w", err)
	}
	return id, nil
}

// isPackageLevel reports whether obj is declared in its package scope.
func isPackageLevel(obj types.Object) bool {
	return obj.Parent() != nil && obj.Pkg() != nil && obj.Parent() == obj.Pkg().Scope()
}

// receiverBaseNamed returns the named type that declares the method, given
// the origin Func: the receiver type, pointer-dereferenced, aliases resolved.
func receiverBaseNamed(origin *types.Func) (*types.Named, error) {
	sig, ok := origin.Type().(*types.Signature)
	if !ok || sig.Recv() == nil {
		return nil, fmt.Errorf("method %q has no receiver", origin.Name())
	}
	return baseNamedOf(sig.Recv().Type())
}

// baseNamedOf resolves a receiver or field type to the named type that
// declares its members: pointer-dereferenced, aliases resolved, and generic
// instantiations reduced to their origin.
func baseNamedOf(t types.Type) (*types.Named, error) {
	t = types.Unalias(t)
	if p, ok := t.(*types.Pointer); ok {
		t = types.Unalias(p.Elem())
	}
	named, ok := t.(*types.Named)
	if !ok {
		return nil, fmt.Errorf("type %s is not a named type", t)
	}
	return named, nil
}

// declaringInterface finds the package-level interface type whose method set
// explicitly declares method.
func declaringInterface(method *types.Func) (*types.Named, error) {
	scope := method.Pkg().Scope()
	for _, name := range scope.Names() {
		tn, ok := scope.Lookup(name).(*types.TypeName)
		if !ok {
			continue
		}
		iface, ok := types.Unalias(tn.Type()).Underlying().(*types.Interface)
		if !ok {
			continue
		}
		for i := 0; i < iface.NumExplicitMethods(); i++ {
			if iface.ExplicitMethod(i) == method {
				return types.Unalias(tn.Type()).(*types.Named), nil
			}
		}
	}
	return nil, fmt.Errorf("interface method %q: no declaring interface found in package %q", method.Name(), method.Pkg().Path())
}

// declaringStruct finds the package-level struct type whose field set
// declares field.
func declaringStruct(field *types.Var) (*types.Named, error) {
	scope := field.Pkg().Scope()
	for _, name := range scope.Names() {
		tn, ok := scope.Lookup(name).(*types.TypeName)
		if !ok {
			continue
		}
		st, ok := types.Unalias(tn.Type()).Underlying().(*types.Struct)
		if !ok {
			continue
		}
		for i := 0; i < st.NumFields(); i++ {
			if st.Field(i) == field {
				return types.Unalias(tn.Type()).(*types.Named), nil
			}
		}
	}
	return nil, fmt.Errorf("field %q: no declaring struct found in package %q", field.Name(), field.Pkg().Path())
}

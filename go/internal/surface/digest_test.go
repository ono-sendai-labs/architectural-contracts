package surface_test

import (
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/surface"
)

// digestInput derives a declared-interface surface with fixed digest inputs
// and returns the input and its digest.
func digestInput(t *testing.T) (surface.Input, string) {
	t.Helper()
	auth, err := manifest.NewDeclared("FILES")
	if err != nil {
		t.Fatalf("authority: %v", err)
	}
	in := surface.Input{
		Component:       "dep",
		Style:           manifest.InterfaceStylePackageSurface,
		Authority:       auth,
		Namespace:       "upstream",
		Key:             fullKey(),
		ProducerVersion: "test-1",
		MemberPackages:  []string{"example.com/dep"},
		Manifest:        []byte("name: \"dep\"\n"),
		Sources: []surface.SourceFile{
			{Path: "b.go", Bytes: []byte("package dep\n")},
			{Path: "a.go", Bytes: []byte("package dep\n")},
		},
	}
	m, err := surface.Derive(in)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	return in, m.Digest
}

// TestDigestSensitiveToProducerInputs pins the sensitivity half of the
// digest contract (task req 5): changing the manifest, a member source, the
// namespace, the producer version, or the SDK key changes the digest.
func TestDigestSensitiveToProducerInputs(t *testing.T) {
	in, want := digestInput(t)
	cases := map[string]func(*surface.Input){
		"member source bytes": func(i *surface.Input) { i.Sources[0].Bytes = []byte("package dep // changed\n") },
		"new member source": func(i *surface.Input) {
			i.Sources = append(i.Sources, surface.SourceFile{Path: "c.go", Bytes: []byte("package dep\n")})
		},
		"manifest bytes":   func(i *surface.Input) { i.Manifest = []byte("name: \"dep2\"\n") },
		"namespace":        func(i *surface.Input) { i.Namespace = "vendor-x" },
		"producer version": func(i *surface.Input) { i.ProducerVersion = "test-2" },
		"sdk key":          func(i *surface.Input) { k := *i.Key; k.ToolchainVersion = "go1.25.0"; i.Key = &k },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			mutated := in
			mutate(&mutated)
			m, err := surface.Derive(mutated)
			if err != nil {
				t.Fatalf("Derive: %v", err)
			}
			if m.Digest == want {
				t.Errorf("digest unchanged after changing %s", name)
			}
		})
	}
}

// TestDigestInsensitiveToEnumerationOrder pins the exclusion/ordering half of
// the digest contract (task req 5, AC4): reordering the same member sources
// yields an identical digest.
func TestDigestInsensitiveToEnumerationOrder(t *testing.T) {
	_, want := digestInput(t)
	in, _ := digestInput(t)
	in.Sources = []surface.SourceFile{
		{Path: "a.go", Bytes: []byte("package dep\n")},
		{Path: "b.go", Bytes: []byte("package dep\n")},
	}
	m, err := surface.Derive(in)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if m.Digest != want {
		t.Errorf("digest changed with enumeration order: %s != %s", m.Digest, want)
	}
}

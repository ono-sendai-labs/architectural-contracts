package stdlibmap

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibauthority"
)

func baseDescriptor() GenerationDescriptor {
	text, err := CanonicalClassifierText([]ClassifierRule{
		{Name: "unsafe.builtins", Effect: "ARBITRARY_EXECUTION/UNSAFE_POINTER"},
		{Name: "minting-site", Effect: "handle use methods are SAFE at the minting site"},
		{Name: "var-handle", Effect: "vars of handle types inherit the minting capability"},
	})
	if err != nil {
		panic(err)
	}
	return GenerationDescriptor{
		Target: TargetConfig{
			ToolchainVersion: "go1.26.4",
			GOOS:             "linux",
			GOARCH:           "amd64",
			CgoEnabled:       true,
			BuildTags:        []string{"b", "a"},
			GOEXPERIMENT:     "greenteagc",
		},
		ClassifierText:   text,
		RuleVersion:      "rules/1",
		MapFormatVersion: 1,
	}
}

// --- AC4: SDK keys describe the target configuration --------------------------

func TestDeriveSDKKeyDistinctPerConfiguration(t *testing.T) {
	base := DeriveSDKKey(baseDescriptor())

	noCgo := baseDescriptor()
	noCgo.Target.CgoEnabled = false
	cross := baseDescriptor()
	cross.Target.GOOS = "darwin"
	cross.Target.GOARCH = "arm64"

	if len(stdlibauthority.EqualKeys(base, DeriveSDKKey(noCgo))) == 0 {
		t.Fatalf("toggling cgo did not change the key")
	}
	if len(stdlibauthority.EqualKeys(base, DeriveSDKKey(cross))) == 0 {
		t.Fatalf("changing the target platform did not change the key")
	}
}

func TestDeriveSDKKeySortsBuildTags(t *testing.T) {
	sortedInput := baseDescriptor()
	sortedInput.Target.BuildTags = []string{"a", "b"}
	if diff := stdlibauthority.EqualKeys(DeriveSDKKey(baseDescriptor()), DeriveSDKKey(sortedInput)); len(diff) != 0 {
		t.Fatalf("unsorted tags changed the key: %v", diff)
	}
	if got := DeriveSDKKey(baseDescriptor()).BuildTags; !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("key build tags = %v, want canonically sorted", got)
	}
}

func TestDeriveSDKKeyByteStable(t *testing.T) {
	first := DeriveSDKKey(baseDescriptor()).String()
	second := DeriveSDKKey(baseDescriptor()).String()
	if first != second {
		t.Fatalf("repeated derivation is not byte-stable:\n%s\n%s", first, second)
	}
}

// --- AC5: classifier drift changes the key ------------------------------------

func TestClassifierHashDrift(t *testing.T) {
	base := baseDescriptor()

	changedText := baseDescriptor()
	changedText.ClassifierText += "minting-site\x1fchanged\x1e"
	if ClassifierHash(changedText) == ClassifierHash(base) {
		t.Fatalf("changing the classifier text did not change the hash")
	}

	changedRules := baseDescriptor()
	changedRules.RuleVersion = "rules/2"
	if ClassifierHash(changedRules) == ClassifierHash(base) {
		t.Fatalf("changing the rule version did not change the hash")
	}

	key := DeriveSDKKey(base)
	keyDrift := DeriveSDKKey(changedRules)
	if len(stdlibauthority.EqualKeys(key, keyDrift)) == 0 {
		t.Fatalf("classifier drift did not change the key")
	}

	changedFormat := baseDescriptor()
	changedFormat.MapFormatVersion = 2
	if len(stdlibauthority.EqualKeys(DeriveSDKKey(changedFormat), key)) == 0 {
		t.Fatalf("changing the map format version did not change the key")
	}
}

func TestCanonicalClassifierTextOrderInvariant(t *testing.T) {
	a := []ClassifierRule{{Name: "x", Effect: "1"}, {Name: "a", Effect: "2"}, {Name: "m", Effect: "3"}}
	b := []ClassifierRule{{Name: "m", Effect: "3"}, {Name: "a", Effect: "2"}, {Name: "x", Effect: "1"}}
	ta, err := CanonicalClassifierText(a)
	if err != nil {
		t.Fatalf("CanonicalClassifierText: %v", err)
	}
	tb, err := CanonicalClassifierText(b)
	if err != nil {
		t.Fatalf("CanonicalClassifierText: %v", err)
	}
	if ta != tb {
		t.Fatalf("iteration order changed the classifier text:\n%q\n%q", ta, tb)
	}
}

func TestCanonicalClassifierTextRejectsDuplicateNames(t *testing.T) {
	_, err := CanonicalClassifierText([]ClassifierRule{
		{Name: "x", Effect: "1"}, {Name: "x", Effect: "2"},
	})
	if err == nil || !strings.Contains(err.Error(), "twice") {
		t.Fatalf("CanonicalClassifierText = %v; want duplicate-rule error", err)
	}
}

func TestDeriveSDKKeyDescribesTargetNotHost(t *testing.T) {
	cross := baseDescriptor()
	cross.Target.GOOS = "darwin"
	cross.Target.GOARCH = "arm64"
	key := DeriveSDKKey(cross)
	if key.GOOS != "darwin" || key.GOARCH != "arm64" {
		t.Fatalf("key = %s; want the cross-compiled target's platform", key)
	}
	if key.ToolchainVersion != "go1.26.4" {
		t.Fatalf("key toolchain = %q, want the target toolchain version", key.ToolchainVersion)
	}
	if key.ClassifierHash == "" || key.MapFormatVersion != 1 {
		t.Fatalf("key = %s; want classifier hash and format version stamped", key)
	}
}

// --- target environment --------------------------------------------------------

func TestTargetEnv(t *testing.T) {
	env := TargetEnv(TargetConfig{
		GOOS: "darwin", GOARCH: "arm64", CgoEnabled: false, GOEXPERIMENT: "greenteagc", BuildTags: []string{"x"},
	})
	joined := strings.Join(env, " ")
	for _, want := range []string{"GOOS=darwin", "GOARCH=arm64", "CGO_ENABLED=0", "GOEXPERIMENT=greenteagc", "GOFLAGS=-tags=x"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("TargetEnv = %q; missing %q", joined, want)
		}
	}
}

// --- NativeLoader (integration-flavored but hermetic: host toolchain, no network)

func TestNativeLoaderLoadsHostPackages(t *testing.T) {
	loader := &NativeLoader{}
	loaded, err := loader.Load([]string{"os", "strings"})
	if err != nil {
		t.Fatalf("NativeLoader.Load: %v", err)
	}
	if loaded["os"] == nil || loaded["os"].Scope().Lookup("Open") == nil {
		t.Fatalf("NativeLoader did not load package os with its declarations")
	}
	if _, err := loader.Load([]string{"os/nonexistent"}); err == nil {
		t.Fatalf("NativeLoader.Load(os/nonexistent): want error, got nil")
	}
}

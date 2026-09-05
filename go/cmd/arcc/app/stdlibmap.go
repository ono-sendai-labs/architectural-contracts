package app

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/schema/gen"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibauthority"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibmap"
)

// runStdlibmap dispatches the `arcc stdlibmap` subcommands (task reqs 5–6):
// generate produces a canonical map artifact for a target configuration;
// inspect validates an artifact and answers deterministic queries. A
// dedicated parser keeps this command's argument grammar decoupled from
// `check` argument parsing (task approach step 3).
func (r *Runner) runStdlibmap(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "error: stdlibmap requires a subcommand (generate or inspect)")
		printUsage(stderr)
		return 2
	}
	switch args[0] {
	case "generate":
		return r.runStdlibmapGenerate(args[1:], stdout, stderr)
	case "inspect":
		return r.runStdlibmapInspect(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "error: unknown stdlibmap command: %s\n", args[0])
		printUsage(stderr)
		return 2
	}
}

// stdlibmapGenerateOptions carries the parsed `stdlibmap generate` inputs
// (task req 5): the explicit output path and the target-configuration
// overrides over the native default discovery.
type stdlibmapGenerateOptions struct {
	output       string
	toolchain    string
	goos         string
	goarch       string
	cgo          bool
	buildTags    []string
	goexperiment string
}

// parseStdlibmapGenerate parses the generate subcommand's arguments. Every
// usage error exits 2 (task req 7).
func parseStdlibmapGenerate(args []string) (*stdlibmapGenerateOptions, error) {
	opts := &stdlibmapGenerateOptions{}
	var hasOutput bool
	for _, arg := range args {
		switch {
		case strings.HasPrefix(arg, "--output="):
			if hasOutput {
				return nil, fmt.Errorf("duplicate option: --output")
			}
			opts.output = strings.TrimPrefix(arg, "--output=")
			if opts.output == "" {
				return nil, fmt.Errorf("empty output value")
			}
			hasOutput = true
		case strings.HasPrefix(arg, "--toolchain="):
			opts.toolchain = strings.TrimPrefix(arg, "--toolchain=")
		case strings.HasPrefix(arg, "--goos="):
			opts.goos = strings.TrimPrefix(arg, "--goos=")
		case strings.HasPrefix(arg, "--goarch="):
			opts.goarch = strings.TrimPrefix(arg, "--goarch=")
		case arg == "--cgo":
			opts.cgo = true
		case strings.HasPrefix(arg, "--tags="):
			spec := strings.TrimPrefix(arg, "--tags=")
			if spec != "" {
				opts.buildTags = strings.Split(spec, ",")
			}
		case strings.HasPrefix(arg, "--goexperiment="):
			opts.goexperiment = strings.TrimPrefix(arg, "--goexperiment=")
		default:
			return nil, fmt.Errorf("unknown option: %s", arg)
		}
	}
	if !hasOutput {
		return nil, fmt.Errorf("generate requires --output=<path>")
	}
	return opts, nil
}

// runStdlibmapGenerate implements `arcc stdlibmap generate` (task req 5,
// AC 4): native discovery (current toolchain, `go list std`), one
// deterministic generation, and an atomic canonical-bytes write to the
// explicit output path. Discovery, generation, and write errors exit 2;
// success prints the artifact summary to stdout and exits 0.
func (r *Runner) runStdlibmapGenerate(args []string, stdout, stderr io.Writer) int {
	opts, err := parseStdlibmapGenerate(args)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		printUsage(stderr)
		return 2
	}

	// Target configuration: the native default is the current toolchain
	// (`go env` GOVERSION/GOOS/GOARCH/CGO_ENABLED/GOEXPERIMENT); explicit
	// options override individual fields, which is also how the later Bazel
	// rule passes its pinned target (task req 5).
	target, err := stdlibmap.NativeTargetConfig()
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	if opts.toolchain != "" {
		target.ToolchainVersion = opts.toolchain
	}
	if opts.goos != "" {
		target.GOOS = opts.goos
	}
	if opts.goarch != "" {
		target.GOARCH = opts.goarch
	}
	if opts.cgo {
		target.CgoEnabled = true
	}
	if len(opts.buildTags) > 0 {
		target.BuildTags = opts.buildTags
	}
	if opts.goexperiment != "" {
		target.GOEXPERIMENT = opts.goexperiment
	}

	env := stdlibmap.TargetEnv(target)
	out, err := stdlibmap.Generate(stdlibmap.GenerationInput{
		Target:      target,
		RuleVersion: stdlibmap.RuleVersion,
		Oracle: stdlibmap.PackageOracleFunc(func() ([]stdlibmap.PackageEntry, error) {
			return stdlibmap.NativeStdPackageList(context.Background(), env)
		}),
		Loader: &stdlibmap.NativeLoader{Env: env},
	})
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	if err := stdlibmap.WriteArtifactAtomic(opts.output, out.Bytes, 0o644); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}

	// Summary on stdout: the written path, the canonical digest, the key the
	// map is keyed by, and its inventory shape (deterministic rendering).
	var importable int
	for _, p := range out.Map.Packages {
		if p.Importable {
			importable++
		}
	}
	key, err := mapSDKKey(out.Map)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	fmt.Fprintf(stdout, "wrote %s\n", opts.output)
	fmt.Fprintf(stdout, "digest: %s\n", out.Digest)
	fmt.Fprintf(stdout, "sdk key: %s\n", key)
	fmt.Fprintf(stdout, "packages: %d (importable %d)\n", len(out.Map.Packages), importable)
	fmt.Fprintf(stdout, "symbols: %d\n", len(out.Map.Symbols))
	fmt.Fprintf(stdout, "inits: %d\n", len(out.Map.Inits))
	return 0
}

// runStdlibmapInspect implements `arcc stdlibmap inspect` (task req 6,
// AC 5): it validates the artifact, applies any expected-key checks, and
// answers the summary, symbol, and init queries with deterministic output.
// Unknown packages/symbols and mismatched expected keys are tool errors
// (exit 2).
func (r *Runner) runStdlibmapInspect(args []string, stdout, stderr io.Writer) int {
	var artifactPath string
	var expectSpecs []string
	var rawQueries []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case strings.HasPrefix(arg, "--expect-key="):
			expectSpecs = append(expectSpecs, strings.TrimPrefix(arg, "--expect-key="))
		case strings.HasPrefix(arg, "-"):
			fmt.Fprintf(stderr, "error: unknown option: %s\n", arg)
			printUsage(stderr)
			return 2
		case artifactPath == "":
			artifactPath = arg
		default:
			rawQueries = append(rawQueries, arg)
		}
	}
	if artifactPath == "" {
		fmt.Fprintln(stderr, "error: inspect requires exactly one artifact path")
		printUsage(stderr)
		return 2
	}
	expected, err := parseExpectKey(expectSpecs)
	if err != nil {
		fmt.Fprintf(stderr, "error: --expect-key: %v\n", err)
		return 2
	}
	// Parse the query grammar before touching the artifact, so a usage
	// error never depends on the file's contents.
	queries, err := parseInspectQueries(rawQueries)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		printUsage(stderr)
		return 2
	}

	// Read, decode and validate the artifact (fail closed on any decode or
	// validation fault), then check the expected key, then run the queries.
	data, err := os.ReadFile(artifactPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: opening the stdlib map artifact: %v\n", err)
		return 2
	}
	m, err := stdlibmap.DecodeMapArtifact(bytes.NewReader(data))
	if err != nil {
		fmt.Fprintf(stderr, "error: decoding the stdlib map artifact %s: %v\n", artifactPath, err)
		return 2
	}
	key, err := mapSDKKey(m)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	if expected != nil {
		// Only the fields the user named are checked: a partial expectation
		// verifies those fields exactly, not the whole key.
		mismatched := map[string]bool{}
		for _, f := range stdlibauthority.EqualKeys(key, *expected.key) {
			mismatched[f] = true
		}
		var fields []string
		for _, f := range expected.checked {
			if mismatched[f] {
				fields = append(fields, f)
			}
		}
		if len(fields) > 0 {
			fmt.Fprintf(stderr, "error: the artifact's SDK key does not match the expected key; mismatched fields: %v\n", fields)
			return 2
		}
	}

	// No queries: the deterministic summary.
	if len(queries) == 0 {
		queries = []inspectQuery{{kind: "summary"}}
	}
	for _, q := range queries {
		switch q.kind {
		case "summary":
			var importable int
			for _, p := range m.Packages {
				if p.Importable {
					importable++
				}
			}
			fmt.Fprintf(stdout, "format version: %d\n", key.MapFormatVersion)
			fmt.Fprintf(stdout, "sdk key: %s\n", key)
			fmt.Fprintf(stdout, "packages: %d (importable %d)\n", len(m.Packages), importable)
			fmt.Fprintf(stdout, "symbols: %d\n", len(m.Symbols))
			fmt.Fprintf(stdout, "inits: %d\n", len(m.Inits))
		case "symbol":
			if code := inspectSymbol(m, q.argument, stdout, stderr); code != 0 {
				return code
			}
		case "init":
			if code := inspectInit(m, q.argument, stdout, stderr); code != 0 {
				return code
			}
		default:
			fmt.Fprintf(stderr, "error: unknown query %q (want summary, symbol <id>, or init <pkg>)\n", q.kind)
			printUsage(stderr)
			return 2
		}
	}
	return 0
}

// inspectQuery is one parsed inspect query: its kind and, for symbol and
// init queries, the queried identifier.
type inspectQuery struct {
	kind     string
	argument string
}

// parseInspectQueries validates the query grammar up front: summary takes
// no argument; symbol and init take exactly one; anything else is a usage
// error.
func parseInspectQueries(raw []string) ([]inspectQuery, error) {
	queries := make([]inspectQuery, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		switch raw[i] {
		case "summary":
			queries = append(queries, inspectQuery{kind: "summary"})
		case "symbol", "init":
			if i+1 >= len(raw) {
				return nil, fmt.Errorf("a %s query requires %s", raw[i], map[string]string{"symbol": "a symbol ID", "init": "a package path"}[raw[i]])
			}
			queries = append(queries, inspectQuery{kind: raw[i], argument: raw[i+1]})
			i++
		default:
			return nil, fmt.Errorf("unknown query %q (want summary, symbol <id>, or init <pkg>)", raw[i])
		}
	}
	return queries, nil
}

// parseExpectKey builds the expected SDKKey from `--expect-key` specs: each
// spec is a comma-separated field=value list naming proto field names
// (toolchain_version, goos, goarch, cgo_enabled, goexperiment,
// classifier_hash, map_format_version) plus tag=<value> entries for build
// tags. A field given twice, or an unknown field, is a usage error. The
// returned checked list names exactly the fields the user provided, so a
// partial expectation verifies only those fields.
func parseExpectKey(specs []string) (*expectedKey, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	key := &stdlibauthority.SDKKey{}
	checkedMap := map[string]bool{}
	var checked []string
	note := func(field string) {
		if !checkedMap[field] {
			checkedMap[field] = true
			checked = append(checked, field)
		}
	}
	for _, spec := range specs {
		for _, kv := range strings.Split(spec, ",") {
			field, value, ok := strings.Cut(kv, "=")
			if !ok || field == "" {
				return nil, fmt.Errorf("expected %q is not field=value", kv)
			}
			switch field {
			case "toolchain_version":
				key.ToolchainVersion = value
			case "goos":
				key.GOOS = value
			case "goarch":
				key.GOARCH = value
			case "cgo_enabled":
				key.CgoEnabled = value == "true"
			case "goexperiment":
				key.GOEXPERIMENT = value
			case "classifier_hash":
				key.ClassifierHash = value
			case "map_format_version":
				var v int64
				if _, err := fmt.Sscanf(value, "%d", &v); err != nil {
					return nil, fmt.Errorf("expected map_format_version %q is not a number", value)
				}
				key.MapFormatVersion = int32(v)
			case "tag":
				key.BuildTags = append(key.BuildTags, value)
			default:
				return nil, fmt.Errorf("unknown expected-key field %q", field)
			}
			note(field)
		}
	}
	return &expectedKey{key: key, checked: checked}, nil
}

// expectedKey is a parsed `--expect-key`: the key to compare against and the
// subset of fields that comparison covers.
type expectedKey struct {
	key     *stdlibauthority.SDKKey
	checked []string
}

// mapSDKKey converts a validated map's persisted key to the core SDKKey.
func mapSDKKey(m *gen.StdlibMap) (stdlibauthority.SDKKey, error) {
	if m.Key == nil {
		return stdlibauthority.SDKKey{}, fmt.Errorf("the stdlib map artifact carries no SDK key")
	}
	return stdlibauthority.SDKKey{
		ToolchainVersion: m.Key.ToolchainVersion,
		GOOS:             m.Key.Goos,
		GOARCH:           m.Key.Goarch,
		CgoEnabled:       m.Key.CgoEnabled,
		BuildTags:        m.Key.BuildTags,
		GOEXPERIMENT:     m.Key.Goexperiment,
		ClassifierHash:   m.Key.ClassifierHash,
		MapFormatVersion: m.Key.MapFormatVersion,
	}, nil
}

// inspectSymbol renders one symbol's terminal classification and its
// ordered evidence paths (AC 5).
func inspectSymbol(m *gen.StdlibMap, idText string, stdout, stderr io.Writer) int {
	var record *gen.SymbolRecord
	for _, s := range m.Symbols {
		if s.Id == idText {
			record = s
			break
		}
	}
	if record == nil {
		fmt.Fprintf(stderr, "error: the stdlib map has no record for symbol %q; inspect the summary for the total inventory\n", idText)
		return 2
	}
	class := recordClassification(record.Classification, record.Capabilities)
	fmt.Fprintf(stdout, "%s: %s\n", idText, renderClass(class))
	for _, cap := range class.Capabilities {
		fmt.Fprintf(stdout, "evidence %s:\n", cap)
		renderEvidence(m.Evidence, idText, cap, stdout)
	}
	return 0
}

// inspectInit renders one package's aggregate init classification and
// evidence (AC 5).
func inspectInit(m *gen.StdlibMap, pkg string, stdout, stderr io.Writer) int {
	var record *gen.InitRecord
	for _, i := range m.Inits {
		if i.Package == pkg {
			record = i
			break
		}
	}
	if record == nil {
		fmt.Fprintf(stderr, "error: the stdlib map has no record for package %q; inspect the summary for the total inventory\n", pkg)
		return 2
	}
	class := recordClassification(record.Classification, record.Capabilities)
	fmt.Fprintf(stdout, "%s.init: %s\n", pkg, renderClass(class))
	for _, cap := range class.Capabilities {
		fmt.Fprintf(stdout, "evidence %s:\n", cap)
		renderEvidence(m.Evidence, pkg+".init", cap, stdout)
	}
	return 0
}

// recordClassification converts a validated persisted record's fields to the
// core terminal classification.
func recordClassification(class gen.Classification, capabilities []string) stdlibauthority.Classification {
	switch class {
	case gen.Classification_SAFE:
		return stdlibauthority.Classification{Safe: true}
	case gen.Classification_CAPABILITIES:
		return stdlibauthority.Classification{Capabilities: capabilities}
	case gen.Classification_UNANALYZED:
		return stdlibauthority.Classification{Unanalyzed: true}
	default:
		// Unreachable: DecodeMap rejects non-terminal records.
		return stdlibauthority.Classification{}
	}
}

// renderEvidence prints the persisted evidence frames for one (symbol,
// capability) pair in their persisted caller-to-capability order (DR-17).
func renderEvidence(evidence []*gen.Evidence, idText, cap string, w io.Writer) {
	for _, e := range evidence {
		if e.SymbolId != idText || e.Capability != cap {
			continue
		}
		for _, f := range e.Frames {
			if f.File != "" {
				fmt.Fprintf(w, "  %s %s:%d\n", f.Function, f.File, f.Line)
			} else {
				fmt.Fprintf(w, "  %s\n", f.Function)
			}
		}
		return
	}
}

// renderClass renders a terminal classification deterministically:
// SAFE, UNANALYZED, or CAPABILITIES [c1,c2] with capabilities sorted.
func renderClass(class stdlibauthority.Classification) string {
	switch {
	case class.Safe:
		return "SAFE"
	case class.Unanalyzed:
		return "UNANALYZED"
	default:
		return "CAPABILITIES [" + strings.Join(class.Capabilities, ",") + "]"
	}
}

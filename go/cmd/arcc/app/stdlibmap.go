package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
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
		// Generation is shared with the thin Bazel generator. Keeping the
		// command implementation in stdlibmap prevents the hermetic tool from
		// depending on the ordinary check path.
		return stdlibmap.RunGenerateCommand(args[1:], stdout, stderr)
	case "inspect":
		return r.runStdlibmapInspect(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "error: unknown stdlibmap command: %s\n", args[0])
		printUsage(stderr)
		return 2
	}
}

// stdlibmapGenerateOptions carries the parsed `stdlibmap generate` inputs
// (task req 5): the explicit output path, the target-configuration overrides
// over the native default discovery, and — in explicit-input mode (task req
// 7) — the three declared inputs: a toolchain package-list file, a
// deterministic key=value target-configuration file, and the SDK root.
type stdlibmapGenerateOptions struct {
	output       string
	toolchain    string
	goos         string
	goarch       string
	cgo          bool
	buildTags    []string
	goexperiment string

	packageList string
	configFile  string
	sdkRoot     string
	explicit    bool
}

// parseStdlibmapGenerate parses the generate subcommand's arguments. Every
// usage error exits 2 (task req 7). The three explicit-input flags are
// all-or-nothing and single-occurrence; a partial set, a duplicate, or any
// native-discovery flag alongside them is a usage error.
func parseStdlibmapGenerate(args []string) (*stdlibmapGenerateOptions, error) {
	opts := &stdlibmapGenerateOptions{}
	var hasOutput bool
	explicit := map[string]int{"--package-list": 0, "--config-file": 0, "--sdk-root": 0}
	isNativeFlag := func(arg string) bool {
		for _, prefix := range []string{"--toolchain=", "--goos=", "--goarch=", "--tags=", "--goexperiment="} {
			if strings.HasPrefix(arg, prefix) {
				return true
			}
		}
		return arg == "--cgo"
	}
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
		case strings.HasPrefix(arg, "--package-list="):
			if explicit["--package-list"] > 0 {
				return nil, fmt.Errorf("duplicate option: --package-list")
			}
			explicit["--package-list"]++
			opts.packageList = strings.TrimPrefix(arg, "--package-list=")
		case strings.HasPrefix(arg, "--config-file="):
			if explicit["--config-file"] > 0 {
				return nil, fmt.Errorf("duplicate option: --config-file")
			}
			explicit["--config-file"]++
			opts.configFile = strings.TrimPrefix(arg, "--config-file=")
		case strings.HasPrefix(arg, "--sdk-root="):
			if explicit["--sdk-root"] > 0 {
				return nil, fmt.Errorf("duplicate option: --sdk-root")
			}
			explicit["--sdk-root"]++
			opts.sdkRoot = strings.TrimPrefix(arg, "--sdk-root=")
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
	given := 0
	for _, flag := range []string{"--package-list", "--config-file", "--sdk-root"} {
		given += explicit[flag]
	}
	switch {
	case given == 0:
	case given == len(explicit):
		opts.explicit = true
		for _, arg := range args {
			if isNativeFlag(arg) {
				return nil, fmt.Errorf("explicit-input mode (--package-list, --config-file, --sdk-root) declares its whole target; the native-discovery flag %s is not allowed alongside it", arg)
			}
		}
	default:
		return nil, fmt.Errorf("explicit-input flags are all-or-nothing: --package-list, --config-file and --sdk-root must be given together (got %d of 3)", given)
	}
	if !hasOutput {
		return nil, fmt.Errorf("generate requires --output=<path>")
	}
	return opts, nil
}

// explicitSet reports whether any explicit-input flag has been seen so far.
func (o *stdlibmapGenerateOptions) explicitSet() bool {
	return o.packageList != "" || o.configFile != "" || o.sdkRoot != ""
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

	// Explicit-input mode (task reqs 7–8, design I5): every target input is
	// declared — no NativeTargetConfig, no `go env`, no `go list std`, no
	// host discovery — and the standard library is loaded from the SDK root
	// through the layout driver. Usage errors above already exited 2; the
	// explicit path re-parses nothing from the host.
	if opts.explicit {
		f, err := os.Open(opts.configFile)
		if err != nil {
			fmt.Fprintf(stderr, "error: opening the target config file: %v\n", err)
			return 2
		}
		target, err := stdlibmap.ParseTargetConfig(f)
		f.Close()
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 2
		}
		lf, err := os.Open(opts.packageList)
		if err != nil {
			fmt.Fprintf(stderr, "error: opening the toolchain package list file: %v\n", err)
			return 2
		}
		paths, err := stdlibmap.ReadToolchainPackageList(lf)
		lf.Close()
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 2
		}
		out, err := stdlibmap.GenerateExplicit(*target, paths, opts.sdkRoot)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 2
		}
		return writeGeneratedMap(opts.output, out, stdout, stderr)
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
	// Toolchain identity: the native path runs the current `go` toolchain, so
	// an explicit --toolchain must agree with the discovered version — an
	// override that only re-stamps the key would label host-SDK bytes with a
	// foreign SDK version, an artifact a later fail-closed key check would
	// silently trust (review finding). A pinned-toolchain producer (the later
	// Bazel rule) constructs the target configuration directly instead of
	// through this command.
	if opts.toolchain != "" && opts.toolchain != target.ToolchainVersion {
		fmt.Fprintf(stderr, "error: --toolchain=%s does not match the current toolchain %s; native generation runs the host toolchain, so the map would be stamped with a version it was not generated against\n", opts.toolchain, target.ToolchainVersion)
		return 2
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

	out, err := stdlibmap.Generate(stdlibmap.GenerationInput{
		Target:      target,
		RuleVersion: stdlibmap.RuleVersion,
		Oracle: stdlibmap.PackageOracleFunc(func() ([]stdlibmap.PackageEntry, error) {
			// The oracle enumerates in the loader's complete merged
			// environment, so host GOFLAGS/GOTOOLCHAIN/GOROOT state cannot
			// skew discovery against loading (review finding).
			return stdlibmap.NativeStdPackageList(context.Background(), stdlibmap.NativeEnvironment(target))
		}),
		Loader: &stdlibmap.NativeLoader{Env: stdlibmap.TargetEnv(target)},
	})
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	return writeGeneratedMap(opts.output, out, stdout, stderr)
}

// writeGeneratedMap writes the generation's canonical bytes atomically to
// outputPath and prints the deterministic summary (written path, digest, SDK
// key, inventory shape). Errors exit 2.
func writeGeneratedMap(outputPath string, out *stdlibmap.GeneratedMap, stdout, stderr io.Writer) int {
	if err := stdlibmap.WriteArtifactAtomic(outputPath, out.Bytes, 0o644); err != nil {
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
	fmt.Fprintf(stdout, "wrote %s\n", outputPath)
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

	// Stream the artifact straight into the bounded decoder: the decoder
	// reads at most MaxMapBytes, so a user-supplied path can never allocate
	// unbounded memory before validation rejects the content (review
	// round-3 finding).
	f, err := os.Open(artifactPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: opening the stdlib map artifact: %v\n", err)
		return 2
	}
	defer f.Close()
	m, err := stdlibmap.DecodeMapArtifact(f)
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
// tags. Values are parsed strictly: cgo_enabled must be exactly true or
// false and map_format_version must be a decimal integer; a non-repeatable
// field named twice, an unknown field, or a malformed entry is a usage
// error. The returned checked list names exactly the fields the user
// provided, so a partial expectation verifies only those fields.
func parseExpectKey(specs []string) (*expectedKey, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	key := &stdlibauthority.SDKKey{}
	seenRaw := map[string]bool{}
	checkedMap := map[string]bool{}
	var checked []string
	note := func(field string) error {
		// `tag=<value>` entries supply the build_tags field: repeats of the
		// tag alias are legal (one per tag), but the checked-field list must
		// record the field's canonical name so a mismatch is reported — and
		// matched — as build_tags, not the tag alias.
		if field != "tag" && seenRaw[field] {
			return fmt.Errorf("expected-key field %q is given twice", field)
		}
		seenRaw[field] = true
		canonical := field
		if canonical == "tag" {
			canonical = "build_tags"
		}
		if !checkedMap[canonical] {
			checkedMap[canonical] = true
			checked = append(checked, canonical)
		}
		return nil
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
				switch value {
				case "true":
					key.CgoEnabled = true
				case "false":
					key.CgoEnabled = false
				default:
					return nil, fmt.Errorf("expected cgo_enabled %q is not true or false", value)
				}
			case "goexperiment":
				key.GOEXPERIMENT = value
			case "classifier_hash":
				key.ClassifierHash = value
			case "map_format_version":
				v, err := strconv.ParseInt(value, 10, 32)
				if err != nil {
					return nil, fmt.Errorf("expected map_format_version %q is not a decimal integer", value)
				}
				key.MapFormatVersion = int32(v)
			case "tag":
				key.BuildTags = append(key.BuildTags, value)
			default:
				return nil, fmt.Errorf("unknown expected-key field %q", field)
			}
			if err := note(field); err != nil {
				return nil, err
			}
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
	// The generation-time trust annotation is part of the answer (AC 5): a
	// SAFE record's provenance names who decided it is pure.
	if record.Provenance != "" {
		fmt.Fprintf(stdout, "provenance: %s\n", record.Provenance)
	}
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
	if record.Provenance != "" {
		fmt.Fprintf(stdout, "provenance: %s\n", record.Provenance)
	}
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

package stdlibmap

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/schema/gen"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/stdlibauthority"
)

// RunGenerateCommand implements the shared `stdlibmap generate` command used
// by the full CLI and the thin Bazel generator. Keeping parsing and generation
// here ensures both binaries have exactly the same explicit-input semantics.
func RunGenerateCommand(args []string, stdout, stderr io.Writer) int {
	opts, err := parseGenerateOptions(args)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}

	if opts.explicit {
		configFile, err := os.Open(opts.configFile)
		if err != nil {
			fmt.Fprintf(stderr, "error: opening the target config file: %v\n", err)
			return 2
		}
		target, err := ParseTargetConfig(configFile)
		configFile.Close()
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 2
		}
		listFile, err := os.Open(opts.packageList)
		if err != nil {
			fmt.Fprintf(stderr, "error: opening the toolchain package list file: %v\n", err)
			return 2
		}
		paths, err := ReadToolchainPackageList(listFile)
		listFile.Close()
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 2
		}
		out, err := GenerateExplicit(*target, paths, opts.sdkRoot)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 2
		}
		return writeGeneratedMap(opts.output, out, stdout, stderr)
	}

	target, err := NativeTargetConfig()
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
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

	out, err := Generate(GenerationInput{
		Target:      target,
		RuleVersion: RuleVersion,
		Oracle: PackageOracleFunc(func() ([]PackageEntry, error) {
			return NativeStdPackageList(context.Background(), NativeEnvironment(target))
		}),
		Loader: &NativeLoader{Env: TargetEnv(target)},
	})
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	return writeGeneratedMap(opts.output, out, stdout, stderr)
}

type generateOptions struct {
	output, toolchain, goos, goarch, goexperiment string
	cgo, explicit                                 bool
	buildTags                                     []string
	packageList, configFile, sdkRoot              string
}

func parseGenerateOptions(args []string) (*generateOptions, error) {
	opts := &generateOptions{}
	hasOutput := false
	seen := map[string]int{"--package-list": 0, "--config-file": 0, "--sdk-root": 0}
	isNative := func(arg string) bool {
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
			hasOutput = true
			opts.output = strings.TrimPrefix(arg, "--output=")
			if opts.output == "" {
				return nil, fmt.Errorf("empty output value")
			}
		case strings.HasPrefix(arg, "--package-list="):
			if seen["--package-list"] > 0 {
				return nil, fmt.Errorf("duplicate option: --package-list")
			}
			seen["--package-list"]++
			opts.packageList = strings.TrimPrefix(arg, "--package-list=")
		case strings.HasPrefix(arg, "--config-file="):
			if seen["--config-file"] > 0 {
				return nil, fmt.Errorf("duplicate option: --config-file")
			}
			seen["--config-file"]++
			opts.configFile = strings.TrimPrefix(arg, "--config-file=")
		case strings.HasPrefix(arg, "--sdk-root="):
			if seen["--sdk-root"] > 0 {
				return nil, fmt.Errorf("duplicate option: --sdk-root")
			}
			seen["--sdk-root"]++
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

	given := seen["--package-list"] + seen["--config-file"] + seen["--sdk-root"]
	switch {
	case given == 0:
	case given == len(seen):
		opts.explicit = true
		for _, arg := range args {
			if isNative(arg) {
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

func writeGeneratedMap(outputPath string, out *GeneratedMap, stdout, stderr io.Writer) int {
	if err := WriteArtifactAtomic(outputPath, out.Bytes, 0o644); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	key, err := generatedMapKey(out.Map)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	var importable int
	for _, p := range out.Map.Packages {
		if p.Importable {
			importable++
		}
	}
	fmt.Fprintf(stdout, "wrote %s\ndigest: %s\nsdk key: %s\npackages: %d (importable %d)\nsymbols: %d\ninits: %d\n", outputPath, out.Digest, key, len(out.Map.Packages), importable, len(out.Map.Symbols), len(out.Map.Inits))
	return 0
}

func generatedMapKey(m *gen.StdlibMap) (stdlibauthority.SDKKey, error) {
	if m == nil || m.Key == nil {
		return stdlibauthority.SDKKey{}, fmt.Errorf("the generated stdlib map carries no SDK key")
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

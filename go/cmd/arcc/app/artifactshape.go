package app

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/artifactio"
)

// runArtifactShape validates one persisted artifact through artifactio and
// compares its typed shape snapshot with a checked-in golden. The command is
// intentionally assertion-only: it never regenerates an artifact or invokes a
// producer action.
func (r *Runner) runArtifactShape(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		fmt.Fprintln(stderr, "error: artifact-shape requires a kind, artifact path, and --golden=<path>")
		return 2
	}
	kindName := args[0]
	kind := artifactio.ShapeKind(kindName)
	artifactPath := args[1]
	goldenPath := ""
	printSnapshot := false
	for _, arg := range args[2:] {
		switch {
		case strings.HasPrefix(arg, "--golden="):
			if goldenPath != "" {
				fmt.Fprintln(stderr, "error: duplicate option: --golden")
				return 2
			}
			goldenPath = strings.TrimPrefix(arg, "--golden=")
			if goldenPath == "" {
				fmt.Fprintln(stderr, "error: empty artifact-shape golden value")
				return 2
			}
		case arg == "--print":
			if printSnapshot {
				fmt.Fprintln(stderr, "error: duplicate option: --print")
				return 2
			}
			printSnapshot = true
		case strings.HasPrefix(arg, "-"):
			fmt.Fprintf(stderr, "error: unknown artifact-shape option: %s\n", arg)
			return 2
		default:
			fmt.Fprintf(stderr, "error: unexpected artifact-shape argument: %s\n", arg)
			return 2
		}
	}
	if goldenPath != "" && printSnapshot {
		fmt.Fprintln(stderr, "error: artifact-shape cannot combine --print and --golden")
		return 2
	}
	if goldenPath == "" && !printSnapshot {
		fmt.Fprintln(stderr, "error: artifact-shape requires --golden=<path>")
		return 2
	}
	if kindName == "layout" {
		return r.runLayoutArtifactShape(artifactPath, goldenPath, printSnapshot, stdout, stderr)
	}

	artifact, err := os.Open(artifactPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: opening artifact %s: %v\n", artifactPath, err)
		return 2
	}
	defer artifact.Close()
	if printSnapshot {
		snapshot, err := snapshotArtifactShape(kind, artifact)
		if err != nil {
			fmt.Fprintf(stderr, "error: snapshot %s artifact: %v\n", kind, err)
			return 2
		}
		_, err = stdout.Write(snapshot)
		if err != nil {
			fmt.Fprintf(stderr, "error: writing artifact shape snapshot: %v\n", err)
			return 2
		}
		return 0
	}
	golden, err := os.Open(goldenPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: opening shape golden %s: %v\n", goldenPath, err)
		return 2
	}
	defer golden.Close()

	err = compareArtifactShape(kind, artifact, golden)
	if err == nil {
		return 0
	}
	if artifactio.IsShapeMismatch(err) || isLayoutShapeMismatch(err) {
		fmt.Fprintf(stderr, "shape mismatch: %v\n", err)
		return 1
	}
	fmt.Fprintf(stderr, "error: %v\n", err)
	return 2
}

func (r *Runner) runLayoutArtifactShape(artifactPath, goldenPath string, printSnapshot bool, stdout, stderr io.Writer) int {
	artifact, err := os.ReadFile(artifactPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: opening artifact %s: %v\n", artifactPath, err)
		return 2
	}
	if printSnapshot {
		snapshot, err := snapshotLayoutShape(artifact)
		if err != nil {
			fmt.Fprintf(stderr, "error: snapshot layout artifact: %v\n", err)
			return 2
		}
		if _, err := stdout.Write(snapshot); err != nil {
			fmt.Fprintf(stderr, "error: writing layout shape snapshot: %v\n", err)
			return 2
		}
		return 0
	}

	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		fmt.Fprintf(stderr, "error: opening shape golden %s: %v\n", goldenPath, err)
		return 2
	}
	if err := compareLayoutShape(artifact, golden); err == nil {
		return 0
	} else if isLayoutShapeMismatch(err) {
		fmt.Fprintf(stderr, "shape mismatch: %v\n", err)
		return 1
	} else {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
}

func snapshotArtifactShape(kind artifactio.ShapeKind, artifact io.Reader) ([]byte, error) {
	return artifactio.Snapshot(kind, artifact)
}

func compareArtifactShape(kind artifactio.ShapeKind, artifact, golden io.Reader) error {
	return artifactio.CompareShape(kind, artifact, golden)
}

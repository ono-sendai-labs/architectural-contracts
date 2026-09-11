package artifactio

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/report"
	"github.com/ono-sendai-labs/architectural-contracts/go/internal/schema/gen"
)

// ShapeKind selects one of the persisted artifact schemas supported by the
// shape-golden seam. The snapshot is deliberately a test-facing view: it
// retains contract fields and collection order while replacing only values
// whose exact spelling is target-, host-, or content-derived.
type ShapeKind string

const (
	ShapeReport  ShapeKind = "report"
	ShapeSurface ShapeKind = "surface"
	ShapeMap     ShapeKind = "stdlib-map"
)

// ShapeMismatchError identifies a valid artifact/golden pair whose snapshots
// differ. Callers can map this assertion result to a test failure while still
// treating decoder, filesystem, and canonicality errors as tool errors.
type ShapeMismatchError struct {
	Detail string
}

func (e *ShapeMismatchError) Error() string {
	return "artifact shape mismatch: " + e.Detail
}

// IsShapeMismatch reports whether err is the assertion result from
// CompareShape. Keeping the result classification inside artifactio lets CLI
// hosts depend on this small semantic port without importing another
// standard-library error traversal edge into their component boundary.
func IsShapeMismatch(err error) bool {
	_, ok := err.(*ShapeMismatchError)
	return ok
}

const (
	shapeSourcePath                = "<SOURCE_PATH>"
	shapeExportArtifactCount       = "<EXPORT_ARTIFACT_COUNT>"
	shapeExportBytes               = "<EXPORT_BYTES>"
	shapeFindingMessagePlaceholder = "<FINDING_MESSAGE>"
	shapeEvidence                  = "<EVIDENCE>"
	shapeSDKKey                    = "<SDK_KEY>"
	shapeSDKToolchain              = "<SDK_TOOLCHAIN_VERSION>"
	shapeSDKGOOS                   = "<SDK_GOOS>"
	shapeSDKGOARCH                 = "<SDK_GOARCH>"
	shapeSDKClassifier             = "<SDK_CLASSIFIER_HASH>"
	shapeSDKExperiment             = "<SDK_GOEXPERIMENT>"
	shapeContentDigest             = "<CONTENT_DIGEST>"
)

// Snapshot validates one bounded persisted artifact with its production
// decoder, rejects bytes that are not the production canonical encoding, and
// returns a deterministic schema-aware shape snapshot. A caller must not
// normalize arbitrary JSON before calling this function: doing so would let a
// malformed or stale producer artifact pass as a golden.
func Snapshot(kind ShapeKind, r io.Reader) ([]byte, error) {
	switch kind {
	case ShapeReport:
		persisted, err := readCanonicalReport(r)
		if err != nil {
			return nil, err
		}
		return marshalShape(reportShape(persisted)), nil
	case ShapeSurface:
		surface, err := readCanonicalSurface(r)
		if err != nil {
			return nil, err
		}
		return marshalSurfaceShape(surface)
	case ShapeMap:
		stdlibMap, err := readCanonicalMap(r)
		if err != nil {
			return nil, err
		}
		return marshalMapShape(stdlibMap)
	default:
		return nil, fmt.Errorf("unknown artifact shape kind %q", kind)
	}
}

// CompareShape validates and snapshots artifact, validates that golden is
// canonical JSON, and compares the two snapshots. The mismatch includes the
// first focused line/value difference so an intentional schema or semantic
// change points at the changed shape field instead of dumping the artifact.
func CompareShape(kind ShapeKind, artifact io.Reader, golden io.Reader) error {
	actual, err := Snapshot(kind, artifact)
	if err != nil {
		return fmt.Errorf("snapshot %s artifact: %w", kind, err)
	}
	wantBytes, err := boundedRead(golden, MaxMapBytes)
	if err != nil {
		return fmt.Errorf("read %s shape golden: %w", kind, err)
	}
	want, err := canonicalShapeJSON(wantBytes)
	if err != nil {
		return fmt.Errorf("validate %s shape golden: %w", kind, err)
	}
	if bytes.Equal(actual, want) {
		return nil
	}
	return &ShapeMismatchError{Detail: fmt.Sprintf("%s shape snapshot differs: %s", kind, shapeDiff(want, actual))}
}

func readCanonicalReport(r io.Reader) (PersistedReport, error) {
	data, err := boundedRead(r, MaxReportBytes)
	if err != nil {
		return PersistedReport{}, err
	}
	persisted, err := DecodeReport(data)
	if err != nil {
		return PersistedReport{}, err
	}
	canonical, err := MarshalReport(persisted.Report)
	if err != nil {
		return PersistedReport{}, fmt.Errorf("re-encode report artifact: %w", err)
	}
	if !bytes.Equal(data, canonical) {
		return PersistedReport{}, fmt.Errorf("report artifact is not canonical")
	}
	return persisted, nil
}

func readCanonicalSurface(r io.Reader) (*gen.SurfaceManifest, error) {
	data, err := boundedRead(r, MaxSurfaceBytes)
	if err != nil {
		return nil, err
	}
	surface, err := DecodeSurface(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	canonical, err := MarshalSurface(surface)
	if err != nil {
		return nil, fmt.Errorf("re-encode surface artifact: %w", err)
	}
	if !bytes.Equal(data, canonical) {
		return nil, fmt.Errorf("surface artifact is not canonical")
	}
	return surface, nil
}

func readCanonicalMap(r io.Reader) (*gen.StdlibMap, error) {
	data, err := boundedRead(r, MaxMapBytes)
	if err != nil {
		return nil, err
	}
	stdlibMap, err := DecodeMap(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	canonical, err := MarshalMap(stdlibMap)
	if err != nil {
		return nil, fmt.Errorf("re-encode stdlib map artifact: %w", err)
	}
	if !bytes.Equal(data, canonical) {
		return nil, fmt.Errorf("stdlib-map artifact is not canonical")
	}
	return stdlibMap, nil
}

type reportShapeSnapshot struct {
	FormatVersion int                `json:"format_version"`
	Verdict       string             `json:"verdict"`
	Report        reportShapeDetails `json:"report"`
}

type reportShapeDetails struct {
	Component    string                  `json:"component"`
	Dependencies []reportDependencyShape `json:"dependencies"`
	Diagnostics  reportDiagnosticsShape  `json:"diagnostics"`
	Violations   []reportFindingShape    `json:"violations"`
	Warnings     []reportFindingShape    `json:"warnings"`
}

type reportDependencyShape struct {
	Component         string   `json:"component"`
	Provenance        string   `json:"provenance"`
	Freshness         string   `json:"freshness"`
	Authority         string   `json:"authority"`
	DeclaredAuthority []string `json:"declared_authority"`
}

type reportDiagnosticsShape struct {
	NonMemberExportArtifactCount string `json:"non_member_export_artifact_count"`
	NonMemberExportBytes         string `json:"non_member_export_bytes"`
}

type reportFindingShape struct {
	Kind     string                `json:"kind"`
	Message  string                `json:"message"`
	Location reportLocationShape   `json:"location"`
	Evidence []string              `json:"evidence"`
	Class    string                `json:"class"`
	SDKKey   string                `json:"sdk_key"`
	Sites    []reportAuthoritySite `json:"sites"`
}

type reportLocationShape struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

type reportAuthoritySite struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Symbol string `json:"symbol"`
}

func reportShape(p PersistedReport) reportShapeSnapshot {
	return reportShapeSnapshot{
		FormatVersion: p.FormatVersion,
		Verdict:       string(p.Verdict),
		Report: reportShapeDetails{
			Component:    p.Report.Component,
			Dependencies: reportDependenciesShape(p.Report.Dependencies),
			Diagnostics: reportDiagnosticsShape{
				NonMemberExportArtifactCount: shapeExportArtifactCount,
				NonMemberExportBytes:         shapeExportBytes,
			},
			Violations: reportFindingsShape(p.Report.Violations),
			Warnings:   reportFindingsShape(p.Report.Warnings),
		},
	}
}

func reportDependenciesShape(dependencies []report.DependencyBoundary) []reportDependencyShape {
	if dependencies == nil {
		return nil
	}
	shaped := make([]reportDependencyShape, len(dependencies))
	for i, dependency := range dependencies {
		shaped[i] = reportDependencyShape{
			Component:         dependency.Component,
			Provenance:        string(dependency.Provenance),
			Freshness:         string(dependency.Freshness),
			Authority:         string(dependency.Authority),
			DeclaredAuthority: nonNilStrings(dependency.DeclaredAuthority),
		}
	}
	return shaped
}

func reportFindingsShape(findings []report.Finding) []reportFindingShape {
	if findings == nil {
		return nil
	}
	shaped := make([]reportFindingShape, len(findings))
	for i, finding := range findings {
		shaped[i] = reportFindingShape{
			Kind:    string(finding.Kind),
			Message: shapeFindingMessage(finding.Message),
			Location: reportLocationShape{
				File: shapeSourceFile(finding.Location.File),
				Line: finding.Location.Line,
			},
			Evidence: shapeEvidenceValues(finding.Evidence),
			Class:    finding.Class,
			SDKKey:   shapeSDKKeyValue(finding.SDKKey),
			Sites:    reportSitesShape(finding.Sites),
		}
	}
	return shaped
}

func reportSitesShape(sites []report.AuthoritySite) []reportAuthoritySite {
	if sites == nil {
		return nil
	}
	shaped := make([]reportAuthoritySite, len(sites))
	for i, site := range sites {
		shaped[i] = reportAuthoritySite{
			File:   shapeSourceFile(site.File),
			Line:   site.Line,
			Symbol: site.Symbol,
		}
	}
	return shaped
}

func shapeFindingMessage(message string) string {
	if message == "" {
		return ""
	}
	return shapeFindingMessagePlaceholder
}

func shapeEvidenceValues(evidence []string) []string {
	if evidence == nil {
		return nil
	}
	shaped := make([]string, len(evidence))
	for i, value := range evidence {
		if value != "" {
			shaped[i] = shapeEvidence
		}
	}
	return shaped
}

func shapeSourceFile(file string) string {
	if file == "" {
		return ""
	}
	return shapeSourcePath
}

func shapeSDKKeyValue(key string) string {
	if key == "" {
		return ""
	}
	return shapeSDKKey
}

type surfaceShapeSnapshot struct {
	FormatVersion   int              `json:"formatVersion"`
	Component       string           `json:"component"`
	InterfaceStyle  string           `json:"interfaceStyle"`
	Authority       surfaceAuthority `json:"authority"`
	Packages        []string         `json:"packages"`
	Symbols         []string         `json:"symbols"`
	Namespace       string           `json:"namespace"`
	SDKKey          sdkKeyShape      `json:"sdkKey"`
	ProducerVersion string           `json:"producerVersion"`
	Digest          string           `json:"digest"`
}

type surfaceAuthority struct {
	Authority         string   `json:"authority"`
	DeclaredAuthority []string `json:"declaredAuthority"`
}

type sdkKeyShape struct {
	ToolchainVersion string   `json:"toolchainVersion"`
	GOOS             string   `json:"goos"`
	GOARCH           string   `json:"goarch"`
	CgoEnabled       bool     `json:"cgoEnabled"`
	BuildTags        []string `json:"buildTags"`
	GOExperiment     string   `json:"goexperiment"`
	ClassifierHash   string   `json:"classifierHash"`
	MapFormatVersion int32    `json:"mapFormatVersion"`
}

func marshalSurfaceShape(surface *gen.SurfaceManifest) ([]byte, error) {
	shape, err := surfaceShape(surface)
	if err != nil {
		return nil, err
	}
	return marshalShape(shape), nil
}

func surfaceShape(surface *gen.SurfaceManifest) (surfaceShapeSnapshot, error) {
	if surface == nil || surface.SdkKey == nil {
		return surfaceShapeSnapshot{}, fmt.Errorf("surface shape requires a complete SDK key")
	}
	style := ""
	switch surface.InterfaceStyle {
	case gen.InterfaceStyle_INTERFACE_STYLE_UNSPECIFIED:
		style = "INTERFACE_STYLE_UNSPECIFIED"
	case gen.InterfaceStyle_INTERFACE_STYLE_PACKAGE_SURFACE:
		style = "INTERFACE_STYLE_PACKAGE_SURFACE"
	default:
		return surfaceShapeSnapshot{}, fmt.Errorf("surface shape has unknown interface style %d", surface.InterfaceStyle)
	}
	authority := surfaceAuthority{Authority: "DECLARED", DeclaredAuthority: []string{}}
	if surface.Authority != nil && surface.Authority.Authority == gen.Authority_UNKNOWN {
		authority.Authority = "UNKNOWN"
	}
	if surface.Authority != nil && surface.Authority.Authority == gen.Authority_DECLARED {
		authority.DeclaredAuthority = nonNilStrings(surface.Authority.DeclaredAuthority)
	}
	key, err := sdkKeyShapeOf(surface.SdkKey)
	if err != nil {
		return surfaceShapeSnapshot{}, err
	}
	digest := ""
	if surface.Digest != "" {
		digest = shapeContentDigest
	}
	return surfaceShapeSnapshot{
		FormatVersion:   int(surface.FormatVersion),
		Component:       surface.Component,
		InterfaceStyle:  style,
		Authority:       authority,
		Packages:        nonNilStrings(surface.Packages),
		Symbols:         nonNilStrings(surface.Symbols),
		Namespace:       surface.Namespace,
		SDKKey:          key,
		ProducerVersion: surface.ProducerVersion,
		Digest:          digest,
	}, nil
}

func sdkKeyShapeOf(key *gen.SDKKey) (sdkKeyShape, error) {
	if key == nil {
		return sdkKeyShape{}, fmt.Errorf("artifact shape requires a complete SDK key")
	}
	goexperiment := key.Goexperiment
	if goexperiment != "" {
		goexperiment = shapeSDKExperiment
	}
	return sdkKeyShape{
		ToolchainVersion: shapeSDKToolchain,
		GOOS:             shapeSDKGOOS,
		GOARCH:           shapeSDKGOARCH,
		CgoEnabled:       key.CgoEnabled,
		BuildTags:        nonNilStrings(key.BuildTags),
		GOExperiment:     goexperiment,
		ClassifierHash:   shapeSDKClassifier,
		MapFormatVersion: key.MapFormatVersion,
	}, nil
}

type mapShapeSnapshot struct {
	FormatVersion int                `json:"formatVersion"`
	Key           sdkKeyShape        `json:"key"`
	Packages      []mapPackageShape  `json:"packages"`
	Symbols       []mapSymbolShape   `json:"symbols"`
	Inits         []mapInitShape     `json:"inits"`
	Evidence      []mapEvidenceShape `json:"evidence"`
}

type mapPackageShape struct {
	Path       string `json:"path"`
	Importable bool   `json:"importable"`
}

type mapSymbolShape struct {
	Package        string   `json:"package"`
	ID             string   `json:"id"`
	Classification string   `json:"classification"`
	Capabilities   []string `json:"capabilities"`
	Provenance     string   `json:"provenance"`
}

type mapInitShape struct {
	Package        string   `json:"package"`
	Classification string   `json:"classification"`
	Capabilities   []string `json:"capabilities"`
	Provenance     string   `json:"provenance"`
}

type mapEvidenceShape struct {
	SymbolID   string          `json:"symbolId"`
	Capability string          `json:"capability"`
	Frames     []mapFrameShape `json:"frames"`
}

type mapFrameShape struct {
	Function string `json:"function"`
	File     string `json:"file"`
	Line     int32  `json:"line"`
}

func marshalMapShape(stdlibMap *gen.StdlibMap) ([]byte, error) {
	shape, err := mapShape(stdlibMap)
	if err != nil {
		return nil, err
	}
	return marshalShape(shape), nil
}

func mapShape(stdlibMap *gen.StdlibMap) (mapShapeSnapshot, error) {
	if stdlibMap == nil || stdlibMap.Key == nil {
		return mapShapeSnapshot{}, fmt.Errorf("stdlib-map shape requires a complete SDK key")
	}
	key, err := sdkKeyShapeOf(stdlibMap.Key)
	if err != nil {
		return mapShapeSnapshot{}, err
	}
	packages := make([]mapPackageShape, len(stdlibMap.Packages))
	for i, packageInventory := range stdlibMap.Packages {
		packages[i] = mapPackageShape{Path: packageInventory.Path, Importable: packageInventory.Importable}
	}
	symbols := make([]mapSymbolShape, len(stdlibMap.Symbols))
	for i, symbol := range stdlibMap.Symbols {
		symbols[i] = mapSymbolShape{
			Package:        symbol.Package,
			ID:             symbol.Id,
			Classification: symbol.Classification.String(),
			Capabilities:   nonNilStrings(symbol.Capabilities),
			Provenance:     symbol.Provenance,
		}
	}
	inits := make([]mapInitShape, len(stdlibMap.Inits))
	for i, init := range stdlibMap.Inits {
		inits[i] = mapInitShape{
			Package:        init.Package,
			Classification: init.Classification.String(),
			Capabilities:   nonNilStrings(init.Capabilities),
			Provenance:     init.Provenance,
		}
	}
	evidence := make([]mapEvidenceShape, len(stdlibMap.Evidence))
	for i, entry := range stdlibMap.Evidence {
		frames := make([]mapFrameShape, len(entry.Frames))
		for j, frame := range entry.Frames {
			frames[j] = mapFrameShape{
				Function: frame.Function,
				File:     shapeSourceFile(frame.File),
				Line:     frame.Line,
			}
		}
		evidence[i] = mapEvidenceShape{
			SymbolID:   entry.SymbolId,
			Capability: entry.Capability,
			Frames:     frames,
		}
	}
	return mapShapeSnapshot{
		FormatVersion: int(stdlibMap.FormatVersion),
		Key:           key,
		Packages:      packages,
		Symbols:       symbols,
		Inits:         inits,
		Evidence:      evidence,
	}, nil
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return slices.Clone(values)
}

func marshalShape(value any) []byte {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	err := encoder.Encode(value)
	if err != nil {
		// All snapshot values are concrete strings, bools, numbers, and slices;
		// this is unreachable unless a new shape field adds an unsupported type.
		panic(fmt.Sprintf("marshal artifact shape: %v", err))
	}
	return buffer.Bytes()
}

func canonicalShapeJSON(data []byte) ([]byte, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}
	if object == nil {
		return nil, fmt.Errorf("shape golden must be a JSON object")
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, data); err != nil {
		return nil, fmt.Errorf("compact JSON: %w", err)
	}
	var indented bytes.Buffer
	if err := json.Indent(&indented, compact.Bytes(), "", "  "); err != nil {
		return nil, fmt.Errorf("indent JSON: %w", err)
	}
	indented.WriteByte('\n')
	canonical := indented.Bytes()
	if !bytes.Equal(data, canonical) {
		return nil, fmt.Errorf("shape golden is not canonical")
	}
	return canonical, nil
}

func shapeDiff(want, got []byte) string {
	wantLines := strings.Split(strings.TrimSuffix(string(want), "\n"), "\n")
	gotLines := strings.Split(strings.TrimSuffix(string(got), "\n"), "\n")
	n := len(wantLines)
	if len(gotLines) < n {
		n = len(gotLines)
	}
	for i := 0; i < n; i++ {
		if wantLines[i] != gotLines[i] {
			return fmt.Sprintf("at line %d:\n- %s\n+ %s", i+1, wantLines[i], gotLines[i])
		}
	}
	return fmt.Sprintf("line count differs (golden %d, actual %d)", len(wantLines), len(gotLines))
}

package manifestparity

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/ono-sendai-labs/architectural-contracts/go/internal/manifest"
)

const (
	productionManifestFilename = "component.textproto"
	productionBuildFilename    = "BUILD.bazel"
	productionFixtureDirectory = "testdata"
)

// productionInventoryRoots is the complete set of source-tree roots in which
// checked-in components may be declared. Keep this list explicit: a recursive
// walk below each root catches examples and future nesting without accidentally
// treating unrelated repository fixtures as production components.
var productionInventoryRoots = []string{
	"go/internal",
	"go/cmd",
	"go/examples",
}

// ManifestRecord keeps the source path with the parsed manifest so ownership
// diagnostics can identify both sides of a duplicate rather than only a
// component name.
type ManifestRecord struct {
	Path     string
	Manifest manifest.Manifest
}

// ManifestInventory is the deterministic set of checked-in production
// manifests. Components are sorted by repository-relative path.
type ManifestInventory struct {
	Components []ManifestRecord
}

// ByName returns the inventory keyed by the validated component name.
func (i ManifestInventory) ByName() map[string]manifest.Manifest {
	byName := make(map[string]manifest.Manifest, len(i.Components))
	for _, component := range i.Components {
		byName[component.Manifest.Name] = component.Manifest
	}
	return byName
}

// DuplicateComponentNameError reports both production manifests that claim the
// same component identity. Returning this error before producing an inventory
// prevents a name-keyed map from silently replacing one declaration.
type DuplicateComponentNameError struct {
	Name  string
	Paths []string
}

func (e *DuplicateComponentNameError) Error() string {
	return fmt.Sprintf("duplicate component name %q in manifests %q and %q", e.Name, e.Paths[0], e.Paths[1])
}

// DiscoverProductionManifests recursively parses checked-in component manifests
// beneath the explicit production roots. A directory named testdata is a
// documented fixture boundary at any depth; files below it are not production
// declarations and are never parsed.
func DiscoverProductionManifests(repoRoot string) (ManifestInventory, error) {
	root, err := absoluteRoot(repoRoot)
	if err != nil {
		return ManifestInventory{}, err
	}
	paths, err := discoverProductionFiles(root, productionManifestFilename)
	if err != nil {
		return ManifestInventory{}, err
	}

	components := make([]ManifestRecord, 0, len(paths))
	seenNames := make(map[string]string, len(paths))
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			return ManifestInventory{}, fmt.Errorf("reading production manifest %q: %w", repositoryPath(root, path), err)
		}
		parsed, err := manifest.Parse(bytes.NewReader(content))
		if err != nil {
			return ManifestInventory{}, fmt.Errorf("parsing production manifest %q: %w", repositoryPath(root, path), err)
		}
		relativePath := repositoryPath(root, path)
		if previousPath, exists := seenNames[parsed.Name]; exists {
			paths := []string{previousPath, relativePath}
			sort.Strings(paths)
			return ManifestInventory{}, &DuplicateComponentNameError{
				Name:  parsed.Name,
				Paths: paths,
			}
		}
		seenNames[parsed.Name] = relativePath
		components = append(components, ManifestRecord{
			Path:     relativePath,
			Manifest: parsed,
		})
	}

	return ManifestInventory{Components: components}, nil
}

// BazelMember is one literal target named by a go_component members attribute.
// ImportPath is resolved when the target is a local go_library or a recognized
// external Go repository; unresolved non-foreign labels remain represented by
// their raw label and do not become ownership merely because they are compile
// dependencies elsewhere.
type BazelMember struct {
	Label      string
	ImportPath string
}

// BazelComponent is one production go_component declaration, including every
// literal member target it declares and the BUILD path containing it.
type BazelComponent struct {
	Name                string
	BuildPath           string
	InterfaceStyle      string
	InterfaceLabel      string
	InterfaceImportPath string
	Members             []BazelMember
}

// BazelInventory is the deterministic set of production go_component
// declarations, sorted by BUILD path and component name.
type BazelInventory struct {
	Components []BazelComponent
}

// DiscoverProductionBazelComponents recursively inventories every production
// BUILD.bazel file under the same explicit roots as the manifest inventory. It
// parses go_component members as structural declarations, while separately
// reading go_library importpaths only to resolve a member target's identity.
// go_library deps are never inspected as component ownership.
func DiscoverProductionBazelComponents(repoRoot string) (BazelInventory, error) {
	root, err := absoluteRoot(repoRoot)
	if err != nil {
		return BazelInventory{}, err
	}
	paths, err := discoverProductionFiles(root, productionBuildFilename)
	if err != nil {
		return BazelInventory{}, err
	}

	targetImportPaths := make(map[string]string)
	type parsedBuild struct {
		path string
		text string
	}
	builds := make([]parsedBuild, 0, len(paths))
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			return BazelInventory{}, fmt.Errorf("reading production BUILD file %q: %w", repositoryPath(root, path), err)
		}
		text := string(content)
		builds = append(builds, parsedBuild{path: path, text: text})

		calls, err := findBazelCalls(text, "go_library")
		if err != nil {
			return BazelInventory{}, fmt.Errorf("parsing go_library declarations in %q: %w", repositoryPath(root, path), err)
		}
		packagePath := bazelPackagePath(root, path)
		for _, call := range calls {
			attrs, err := parseBazelCall(call, repositoryPath(root, path))
			if err != nil {
				return BazelInventory{}, err
			}
			if attrs.name == "" || attrs.importPath == "" {
				continue
			}
			targetImportPaths[localBazelLabel(packagePath, attrs.name)] = attrs.importPath
		}
	}

	components := make([]BazelComponent, 0)
	for _, build := range builds {
		calls, err := findBazelCalls(build.text, "go_component")
		if err != nil {
			return BazelInventory{}, fmt.Errorf("parsing go_component declarations in %q: %w", repositoryPath(root, build.path), err)
		}
		packagePath := bazelPackagePath(root, build.path)
		buildPath := repositoryPath(root, build.path)
		for _, call := range calls {
			attrs, err := parseBazelCall(call, buildPath)
			if err != nil {
				return BazelInventory{}, err
			}
			members := make([]BazelMember, len(attrs.members))
			for i, label := range attrs.members {
				importPath, err := resolveBazelImportPath(label, packagePath, targetImportPaths)
				if err != nil {
					return BazelInventory{}, fmt.Errorf("%s: %w", buildPath, err)
				}
				members[i] = BazelMember{
					Label:      label,
					ImportPath: importPath,
				}
			}
			interfaceImportPath, err := resolveBazelImportPath(attrs.interfaceLabel, packagePath, targetImportPaths)
			if err != nil {
				return BazelInventory{}, fmt.Errorf("%s: %w", buildPath, err)
			}
			components = append(components, BazelComponent{
				Name:                attrs.name,
				BuildPath:           buildPath,
				InterfaceStyle:      attrs.interfaceStyle,
				InterfaceLabel:      attrs.interfaceLabel,
				InterfaceImportPath: interfaceImportPath,
				Members:             members,
			})
		}
	}

	sort.SliceStable(components, func(i, j int) bool {
		if components[i].BuildPath != components[j].BuildPath {
			return components[i].BuildPath < components[j].BuildPath
		}
		return components[i].Name < components[j].Name
	})
	return BazelInventory{Components: components}, nil
}

type ownershipRule struct {
	prefix string
	owner  string
}

var foreignOwnershipRules = []ownershipRule{
	{prefix: "google.golang.org/protobuf/", owner: "protobuf-runtime"},
	{prefix: "golang.org/x/mod/", owner: "x-tools"},
	{prefix: "golang.org/x/sync/", owner: "x-tools"},
	{prefix: "golang.org/x/tools/", owner: "x-tools"},
}

type foreignOwner struct {
	component string
	path      string
}

// AuditForeignManifestOwnership verifies that every foreign package named by a
// production manifest is owned by its canonical wrapper and is not claimed by
// more than one component. Exact wrapper sets remain pinned by the protobuf and
// x-tools ownership tests; this audit supplies the repository-wide guard.
func AuditForeignManifestOwnership(inventory ManifestInventory) error {
	owners := make(map[string][]foreignOwner)
	var violations []string
	for _, component := range inventory.Components {
		for _, member := range component.Manifest.Members {
			rule, foreign := foreignRuleFor(member)
			if !foreign {
				continue
			}
			owners[member] = append(owners[member], foreignOwner{
				component: component.Manifest.Name,
				path:      component.Path,
			})
			if component.Manifest.Name != rule.owner {
				violations = append(violations, fmt.Sprintf(
					"foreign package %q is owned by component %q at %q; expected wrapper %q",
					member, component.Manifest.Name, component.Path, rule.owner,
				))
			}
		}
	}
	violations = append(violations, multipleOwnerViolations(owners, "component")...)

	return ownershipAuditError("manifest", violations)
}

// AuditForeignBazelOwnership verifies every literal member of every
// production go_component declaration. A declaration is allowed to own a
// protobuf package only when its name is protobuf-runtime, and an x/tools,
// x/mod, or x/sync package only when its name is x-tools. It intentionally does
// not inspect go_library deps, which are ordinary compile dependencies rather
// than architectural membership.
func AuditForeignBazelOwnership(inventory BazelInventory) error {
	owners := make(map[string][]foreignOwner)
	var violations []string
	for _, component := range inventory.Components {
		if component.InterfaceImportPath != "" {
			appendBazelForeignOwner(&violations, owners, component.Name, component.BuildPath, component.InterfaceImportPath)
		} else if component.InterfaceLabel != "" {
			violations = append(violations, fmt.Sprintf(
				"go_component %q in %q has an unresolved interface label %q",
				component.Name, component.BuildPath, component.InterfaceLabel,
			))
		}
		for _, member := range component.Members {
			if member.ImportPath == "" {
				violations = append(violations, fmt.Sprintf(
					"go_component %q in %q has an unresolved member label %q",
					component.Name, component.BuildPath, member.Label,
				))
				continue
			}
			appendBazelForeignOwner(&violations, owners, component.Name, component.BuildPath, member.ImportPath)
		}
	}
	violations = append(violations, multipleOwnerViolations(owners, "Bazel component")...)

	return ownershipAuditError("Bazel", violations)
}

func appendBazelForeignOwner(violations *[]string, owners map[string][]foreignOwner, component, path, importPath string) {
	rule, foreign := foreignRuleFor(importPath)
	if !foreign {
		return
	}
	owners[importPath] = append(owners[importPath], foreignOwner{
		component: component,
		path:      path,
	})
	if component != rule.owner {
		*violations = append(*violations, fmt.Sprintf(
			"foreign package %q is owned by non-wrapper component %q in %q; expected wrapper %q",
			importPath, component, path, rule.owner,
		))
	}
}

func multipleOwnerViolations(owners map[string][]foreignOwner, ownerKind string) []string {
	var violations []string
	for _, packagePath := range sortedKeys(owners) {
		packageOwners := append([]foreignOwner(nil), owners[packagePath]...)
		sort.Slice(packageOwners, func(i, j int) bool {
			if packageOwners[i].component != packageOwners[j].component {
				return packageOwners[i].component < packageOwners[j].component
			}
			return packageOwners[i].path < packageOwners[j].path
		})
		uniqueOwners := make([]foreignOwner, 0, len(packageOwners))
		for _, packageOwner := range packageOwners {
			if len(uniqueOwners) == 0 || uniqueOwners[len(uniqueOwners)-1] != packageOwner {
				uniqueOwners = append(uniqueOwners, packageOwner)
			}
		}
		if len(uniqueOwners) < 2 {
			continue
		}
		descriptions := make([]string, len(uniqueOwners))
		for i, packageOwner := range uniqueOwners {
			descriptions[i] = fmt.Sprintf("%s at %s", packageOwner.component, packageOwner.path)
		}
		violations = append(violations, fmt.Sprintf(
			"foreign package %q has multiple %s owners: %s",
			packagePath, ownerKind, strings.Join(descriptions, ", "),
		))
	}
	return violations
}

func ownershipAuditError(kind string, violations []string) error {
	if len(violations) == 0 {
		return nil
	}
	sort.Strings(violations)
	return fmt.Errorf("%s foreign ownership audit failed: %s", kind, strings.Join(violations, "; "))
}

func foreignRuleFor(importPath string) (ownershipRule, bool) {
	for _, rule := range foreignOwnershipRules {
		if strings.HasPrefix(importPath, rule.prefix) {
			return rule, true
		}
	}
	return ownershipRule{}, false
}

func sortedKeys[T any](values map[string][]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func absoluteRoot(repoRoot string) (string, error) {
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", fmt.Errorf("resolving repository root %q: %w", repoRoot, err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("reading repository root %q: %w", repoRoot, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("repository root %q is not a directory", repoRoot)
	}
	return root, nil
}

func discoverProductionFiles(root, filename string) ([]string, error) {
	var paths []string
	for _, relativeRoot := range productionInventoryRoots {
		walkRoot := filepath.Join(root, filepath.FromSlash(relativeRoot))
		info, err := os.Stat(walkRoot)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("reading production root %q: %w", relativeRoot, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("production root %q is not a directory", relativeRoot)
		}

		err = filepath.WalkDir(walkRoot, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return fmt.Errorf("walking %q: %w", repositoryPath(root, path), walkErr)
			}
			if entry.IsDir() && entry.Name() == productionFixtureDirectory {
				// `testdata` is a named fixture boundary, not a depth-based
				// exception. It applies uniformly to manifests and BUILD files.
				return filepath.SkipDir
			}
			if !entry.IsDir() && entry.Name() == filename {
				paths = append(paths, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func repositoryPath(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(relative)
}

func bazelPackagePath(root, buildPath string) string {
	return repositoryPath(root, filepath.Dir(buildPath))
}

func localBazelLabel(packagePath, target string) string {
	if packagePath == "." || packagePath == "" {
		return "//:" + target
	}
	return "//" + packagePath + ":" + target
}

type parsedBazelCall struct {
	name           string
	interfaceStyle string
	interfaceLabel string
	importPath     string
	members        []string
}

func parseBazelCall(body, sourcePath string) (parsedBazelCall, error) {
	var result parsedBazelCall
	seen := make(map[string]bool)
	for index := 0; index < len(body); {
		index = skipBazelTrivia(body, index)
		if index >= len(body) {
			break
		}
		if body[index] == ',' {
			index++
			continue
		}
		key, next, ok := scanBazelIdentifier(body, index)
		if !ok {
			return parsedBazelCall{}, fmt.Errorf("%s: expected keyword argument at byte %d", sourcePath, index)
		}
		index = skipBazelTrivia(body, next)
		if index >= len(body) || body[index] != '=' {
			return parsedBazelCall{}, fmt.Errorf("%s: keyword %q is missing an '='", sourcePath, key)
		}
		if seen[key] {
			return parsedBazelCall{}, fmt.Errorf("%s: duplicate go declaration attribute %q", sourcePath, key)
		}
		seen[key] = true
		index = skipBazelTrivia(body, index+1)

		switch key {
		case "name", "interface", "importpath":
			value, end, err := parseBazelStringValue(body, index, sourcePath, key)
			if err != nil {
				return parsedBazelCall{}, err
			}
			switch key {
			case "name":
				result.name = value
			case "interface":
				result.interfaceLabel = value
			case "importpath":
				result.importPath = value
			}
			index = end
		case "interface_style":
			value, end, err := parseBazelStringOrIdentifierValue(body, index, sourcePath, key)
			if err != nil {
				return parsedBazelCall{}, err
			}
			result.interfaceStyle = value
			index = end
		case "members":
			values, end, err := parseBazelStringList(body, index, sourcePath, key)
			if err != nil {
				return parsedBazelCall{}, err
			}
			result.members = values
			index = end
		default:
			end, err := scanBazelValueEnd(body, index)
			if err != nil {
				return parsedBazelCall{}, fmt.Errorf("%s: parsing attribute %q: %w", sourcePath, key, err)
			}
			index = end
		}
		index = skipBazelTrivia(body, index)
		if index < len(body) && body[index] == ',' {
			index++
		}
	}
	if result.name == "" {
		return parsedBazelCall{}, fmt.Errorf("%s: go declaration has no name", sourcePath)
	}
	return result, nil
}

func resolveBazelImportPath(label, packagePath string, targetImportPaths map[string]string) (string, error) {
	if importPath, ok, err := externalBazelImportPath(label); ok || err != nil {
		return importPath, err
	}
	canonical := canonicalLocalBazelLabel(label, packagePath)
	return targetImportPaths[canonical], nil
}

var externalBazelRepositories = []struct {
	repository string
	prefix     string
}{
	{repository: "org_golang_google_protobuf", prefix: "google.golang.org/protobuf/"},
	{repository: "org_golang_x_mod", prefix: "golang.org/x/mod/"},
	{repository: "org_golang_x_sync", prefix: "golang.org/x/sync/"},
	{repository: "org_golang_x_tools", prefix: "golang.org/x/tools/"},
}

func externalBazelImportPath(label string) (string, bool, error) {
	for _, repository := range externalBazelRepositories {
		prefix := "@" + repository.repository + "//"
		if !strings.HasPrefix(label, prefix) {
			continue
		}
		packageAndTarget := strings.TrimPrefix(label, prefix)
		packagePath := packageAndTarget
		target := ""
		if colon := strings.IndexByte(packagePath, ':'); colon >= 0 {
			target = packagePath[colon+1:]
			packagePath = packagePath[:colon]
		}
		if packagePath == "" {
			return "", false, nil
		}
		if target == "" {
			return "", true, fmt.Errorf("foreign member label %q has no target", label)
		}
		if target != filepath.Base(packagePath) {
			return "", true, fmt.Errorf("foreign member label %q does not use its package basename as target", label)
		}
		return repository.prefix + packagePath, true, nil
	}
	return "", false, nil
}

func canonicalLocalBazelLabel(label, packagePath string) string {
	switch {
	case strings.HasPrefix(label, ":"):
		return localBazelLabel(packagePath, strings.TrimPrefix(label, ":"))
	case strings.HasPrefix(label, "//"):
		body := strings.TrimPrefix(label, "//")
		if colon := strings.IndexByte(body, ':'); colon >= 0 {
			return "//" + body
		}
		return localBazelLabel(body, filepath.Base(body))
	default:
		return ""
	}
}

func findBazelCalls(source, functionName string) ([]string, error) {
	var calls []string
	for index := 0; index < len(source); {
		switch source[index] {
		case '#':
			index = skipBazelComment(source, index)
		case '\'', '"':
			_, next, err := scanBazelString(source, index)
			if err != nil {
				return nil, err
			}
			index = next
		default:
			identifier, next, ok := scanBazelIdentifier(source, index)
			if !ok {
				index++
				continue
			}
			index = next
			if identifier != functionName {
				continue
			}
			index = skipBazelTrivia(source, index)
			if index >= len(source) || source[index] != '(' {
				continue
			}
			end, err := scanBazelDelimited(source, index, '(', ')')
			if err != nil {
				return nil, err
			}
			calls = append(calls, source[index+1:end])
			index = end + 1
		}
	}
	return calls, nil
}

func skipBazelTrivia(source string, index int) int {
	for index < len(source) {
		switch source[index] {
		case ' ', '\t', '\r', '\n':
			index++
		case '#':
			index = skipBazelComment(source, index)
		default:
			return index
		}
	}
	return index
}

func skipBazelComment(source string, index int) int {
	for index < len(source) && source[index] != '\n' {
		index++
	}
	return index
}

func scanBazelIdentifier(source string, index int) (string, int, bool) {
	if index >= len(source) || !isBazelIdentifierStart(source[index]) {
		return "", index, false
	}
	start := index
	index++
	for index < len(source) && isBazelIdentifierPart(source[index]) {
		index++
	}
	return source[start:index], index, true
}

func isBazelIdentifierStart(char byte) bool {
	return char == '_' || char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z'
}

func isBazelIdentifierPart(char byte) bool {
	return isBazelIdentifierStart(char) || char >= '0' && char <= '9'
}

func scanBazelString(source string, index int) (string, int, error) {
	if index >= len(source) || (source[index] != '\'' && source[index] != '"') {
		return "", index, fmt.Errorf("expected string at byte %d", index)
	}
	quote := source[index]
	start := index
	index++
	for index < len(source) {
		switch source[index] {
		case '\\':
			index += 2
		case byte(quote):
			index++
			value, err := strconv.Unquote(source[start:index])
			if err != nil {
				return "", index, fmt.Errorf("invalid string at byte %d: %w", start, err)
			}
			return value, index, nil
		default:
			index++
		}
	}
	return "", index, fmt.Errorf("unterminated string at byte %d", start)
}

func scanBazelDelimited(source string, start int, open, close byte) (int, error) {
	if start >= len(source) || source[start] != open {
		return start, fmt.Errorf("expected %q at byte %d", open, start)
	}
	stack := []byte{open}
	for index := start + 1; index < len(source); {
		switch source[index] {
		case '#':
			index = skipBazelComment(source, index)
		case '\'', '"':
			_, next, err := scanBazelString(source, index)
			if err != nil {
				return next, err
			}
			index = next
		case '(', '[', '{':
			stack = append(stack, source[index])
			index++
		case ')', ']', '}':
			if !matchingBazelDelimiter(stack[len(stack)-1], source[index]) {
				return index, fmt.Errorf("mismatched delimiter %q at byte %d", source[index], index)
			}
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				if source[index] != close {
					return index, fmt.Errorf("unexpected delimiter %q at byte %d", source[index], index)
				}
				return index, nil
			}
			index++
		default:
			index++
		}
	}
	return len(source), fmt.Errorf("unterminated %q declaration", open)
}

func matchingBazelDelimiter(open, close byte) bool {
	return open == '(' && close == ')' || open == '[' && close == ']' || open == '{' && close == '}'
}

func parseBazelStringValue(source string, index int, sourcePath, key string) (string, int, error) {
	value, end, err := scanBazelString(source, index)
	if err != nil {
		return "", end, fmt.Errorf("%s: attribute %q: %w", sourcePath, key, err)
	}
	end = skipBazelTrivia(source, end)
	if end < len(source) && source[end] != ',' {
		return "", end, fmt.Errorf("%s: attribute %q must be a string literal", sourcePath, key)
	}
	return value, end, nil
}

func parseBazelStringOrIdentifierValue(source string, index int, sourcePath, key string) (string, int, error) {
	if index < len(source) && (source[index] == '\'' || source[index] == '"') {
		return parseBazelStringValue(source, index, sourcePath, key)
	}
	value, end, ok := scanBazelIdentifier(source, index)
	if !ok {
		return "", end, fmt.Errorf("%s: attribute %q must be a string or identifier", sourcePath, key)
	}
	end = skipBazelTrivia(source, end)
	if end < len(source) && source[end] != ',' {
		return "", end, fmt.Errorf("%s: attribute %q must be a string or identifier", sourcePath, key)
	}
	return value, end, nil
}

func parseBazelStringList(source string, index int, sourcePath, key string) ([]string, int, error) {
	if index >= len(source) || source[index] != '[' {
		return nil, index, fmt.Errorf("%s: attribute %q must be a literal string list", sourcePath, key)
	}
	end, err := scanBazelDelimited(source, index, '[', ']')
	if err != nil {
		return nil, end, fmt.Errorf("%s: attribute %q: %w", sourcePath, key, err)
	}
	listBody := source[index+1 : end]
	var values []string
	for position := 0; position < len(listBody); {
		position = skipBazelTrivia(listBody, position)
		if position >= len(listBody) {
			break
		}
		if listBody[position] == ',' {
			position++
			continue
		}
		value, next, err := scanBazelString(listBody, position)
		if err != nil {
			return nil, end, fmt.Errorf("%s: attribute %q contains a non-literal member: %w", sourcePath, key, err)
		}
		values = append(values, value)
		position = next
		position = skipBazelTrivia(listBody, position)
		if position < len(listBody) && listBody[position] != ',' {
			return nil, end, fmt.Errorf("%s: attribute %q contains adjacent values without a comma", sourcePath, key)
		}
	}
	return values, skipBazelTrivia(source, end+1), nil
}

func scanBazelValueEnd(source string, index int) (int, error) {
	stack := []byte{}
	for index < len(source) {
		switch source[index] {
		case '#':
			index = skipBazelComment(source, index)
		case '\'', '"':
			_, next, err := scanBazelString(source, index)
			if err != nil {
				return next, err
			}
			index = next
		case '(', '[', '{':
			stack = append(stack, source[index])
			index++
		case ')', ']', '}':
			if len(stack) == 0 {
				return index, nil
			}
			if !matchingBazelDelimiter(stack[len(stack)-1], source[index]) {
				return index, fmt.Errorf("mismatched delimiter %q at byte %d", source[index], index)
			}
			stack = stack[:len(stack)-1]
			index++
		default:
			if len(stack) == 0 && source[index] == ',' {
				return index, nil
			}
			index++
		}
	}
	return index, nil
}

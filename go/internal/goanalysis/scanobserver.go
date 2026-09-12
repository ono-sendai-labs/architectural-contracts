package goanalysis

// scanObserverScanner identifies the production scanner that emitted an
// observation. It is intentionally package-private: the seam exists to make
// integration evidence about the member-only boundary without adding a
// persisted or public representation of scan work.
type scanObserverScanner string

const (
	scanObserverReferences      scanObserverScanner = "ScanReferences"
	scanObserverAnalysisDefeats scanObserverScanner = "ScanAnalysisDefeats"
)

// scanObserverOperation identifies one kind of work performed at a scan
// boundary. Counts are emitted in aggregate at the loop that performs the
// work, rather than once per map entry, so an observer can canonicalize its
// package identities and remain deterministic despite map iteration order.
type scanObserverOperation string

const (
	scanObserverPackageConsidered  scanObserverOperation = "package-considered"
	scanObserverPackageEntered     scanObserverOperation = "package-entered"
	scanObserverSyntaxFiles        scanObserverOperation = "syntax-files"
	scanObserverTypeInfoPackages   scanObserverOperation = "type-info-packages"
	scanObserverUses               scanObserverOperation = "uses"
	scanObserverSelections         scanObserverOperation = "selections"
	scanObserverImportPackages     scanObserverOperation = "import-packages"
	scanObserverASTFiles           scanObserverOperation = "ast-files"
	scanObserverASTCommentGroups   scanObserverOperation = "ast-comment-groups"
	scanObserverASTComments        scanObserverOperation = "ast-comments"
	scanObserverASTDeclarations    scanObserverOperation = "ast-declarations"
	scanObserverImportDeclarations scanObserverOperation = "import-declarations"
	scanObserverImportSpecs        scanObserverOperation = "import-specs"
	scanObserverDeclaredFiles      scanObserverOperation = "declared-files"
	scanObserverAssemblyFiles      scanObserverOperation = "assembly-files"
	scanObserverGoSourceFiles      scanObserverOperation = "go-source-files"
	scanObserverParsedSyntaxFiles  scanObserverOperation = "parsed-syntax-files"
	scanObserverLexedSourceFiles   scanObserverOperation = "lexed-source-files"
	scanObserverLexedSourceBytes   scanObserverOperation = "lexed-source-bytes"
	scanObserverSourceTokens       scanObserverOperation = "source-tokens"
)

// scanObserverEvent is an immutable observation value. Member is the
// membership decision at the scanner boundary, not a property inferred from
// the result. Package is canonicalized before emission. A nil observer is the
// production default and leaves no scan-work state behind.
type scanObserverEvent struct {
	Scanner   scanObserverScanner
	Operation scanObserverOperation
	Package   string
	Member    bool
	Count     int
}

type scanObservationObserver func(scanObserverEvent)

// scanObserver is deliberately nil in production. Same-package integration
// harnesses install it for a bounded invocation and restore the prior value
// with defer, following the phaseObserver seam.
var scanObserver scanObservationObserver

func notifyScanObserver(observer scanObservationObserver, event scanObserverEvent) {
	if observer == nil || event.Count <= 0 {
		return
	}
	observer(event)
}

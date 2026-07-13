# Task: Add signed release workflow

## Description
Add a minimal GitHub Actions workflow that builds `arcc` for supported Linux and macOS architectures, publishes tagged releases, and attaches Sigstore-backed SLSA provenance to final binaries while supporting unattested private-repository prereleases.

## Background
The MVP has no distribution path. Final tags need verifiable provenance tying each binary to this workflow and source commit. Hyphenated tags must remain usable as private-repository release candidates, where provenance attestation is unsupported, and must be marked prereleases.

## Reference Documentation
**Required:**
- Design: `.agents/planning/2026-07-06-arch-contracts-go-mvp/design/detailed-design.md` (Appendix D release decision)
- Plan: `.agents/planning/2026-07-06-arch-contracts-go-mvp/implementation/plan.md` (Step 11)

**Note:** Read the detailed design document before beginning implementation.

## Technical Requirements
1. Add `.github/workflows/release.yml` triggered by final and prerelease tags using `v[0-9]+.[0-9]+.[0-9]+-?*`; include `workflow_dispatch` only if its release tag is unambiguous.
2. Grant `contents: write`, `id-token: write`, and `attestations: write` and no broader permissions.
3. Use plain `go build`, not GoReleaser, from `go/` for `linux/amd64`, `linux/arm64`, `darwin/amd64`, and `darwin/arm64`.
4. Produce unique OS/architecture artifact names and gather every matrix output into one release without overwrites.
5. For final tags (no `-`), run `actions/attest-build-provenance@v2` for every exact binary published, using GitHub OIDC/Sigstore.
6. For prerelease tags (containing `-`), skip attestation and pass `--prerelease` to `gh release create`; final releases must not receive the flag.
7. Publish binaries with generated notes using `gh release create`, with safe shell quoting and conditional handling.
8. Use stable action versions consistent with existing workflow conventions.

## Dependencies
- Task 1, so releases have complete usage and verification guidance.
- Existing Go module, CLI entry point, and CI workflow conventions.

## Implementation Approach
1. Use matrix build jobs plus artifact collection/release while preserving the exact attestation subjects.
2. Derive prerelease status from whether `github.ref_name` contains `-`.
3. Validate YAML and Actions expressions with `actionlint` if available and a YAML structural check.
4. Audit matrix expansion, artifact names, permissions, attestation guard, prerelease skip, and release arguments against both tag forms.
5. Run `just ci`.

## Acceptance Criteria

1. **Version tags trigger releases**
   - Given `v0.1.0` or `v0.1.0-rc.1`
   - When the tag is pushed
   - Then the workflow starts and targets that GitHub Release.

2. **All binaries are published**
   - Given a release run
   - When the matrix completes
   - Then unique `arcc` binaries for Linux and macOS on amd64 and arm64 attach to one release.

3. **Final releases carry provenance**
   - Given a final tag
   - When release jobs run
   - Then every binary has an `attest-build-provenance@v2` attestation verifiable against this repository.

4. **Prereleases work while private**
   - Given a hyphenated tag
   - When release jobs run
   - Then attestation is skipped and GitHub marks the release as a prerelease.

5. **Privileges and tooling are constrained**
   - Given the workflow
   - When inspected
   - Then it uses plain Go builds and only the specified release/OIDC/attestation permissions.

6. **Checks pass**
   - Given the completed workflow
   - When Actions validation and `just ci` run
   - Then the YAML is valid and repository gates remain green.

## Metadata
- **Complexity**: Medium
- **Labels**: release, GitHub-Actions, SLSA, Sigstore, supply-chain
- **Required Skills**: GitHub Actions, Go cross-compilation, release automation, artifact attestations

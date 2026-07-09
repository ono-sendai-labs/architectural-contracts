# Agent Instructions

## Project Overview

Architectural Contracts is a Go MVP for checking component manifests against Go package structure, dependency boundaries, and ambient authority findings.

## Workflow

This repo follows the `structured-spec-to-code` skills framework. Planning artifacts, task files, and scratchpad notes live under `.agents/`.

## Sandboxing

Agents working on this repository typically run in a sandbox which permits read/write access to the working repo and may provide read-only access to reference material via read-only bind mounts.

## Version Control

### Local repo: Jujutsu

Use Jujutsu (`jj`) for local version-control workflow. Prefer `jj new` + `jj squash` over `jj edit`.  Do NOT use raw `git` operations to mutate repo state.

### GitHub

Agents access the GH origin repository using a GH token. To refresh the token, use:

```sh
gh-bot-token | gh auth login --with-token
```

GH Permissions:

The GH token has limited permissions. If GH operations still fail after refreshing the token, STOP!


## Development

Run `just ci` before every commit. The Go implementation lives in the `go/` module; shared protobuf schemas live in top-level `proto/`.

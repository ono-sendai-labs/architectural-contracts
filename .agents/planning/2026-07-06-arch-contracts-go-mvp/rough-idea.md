# Rough Idea — Architectural Contracts MVP (Go)

I want to implement a proof of concept/MVP for the concepts described in
`../ideas-writing/architecture-as-code--contract-driven-design/rationale-and-concepts.md`.

My initial plan is to target Go, and focus on a subset of constraints.
Specifically, I want to address pillars one and three.

**Pillar 1 — Architecture as code**, which involves a manifest describing the
module, its dependencies, and its declared interfaces.

**Pillar 3 — Ambient authority**: ensuring the module only uses declared ambient
authority. For the MVP, we can initially require the ambient authority to be
empty, making the module ambient authority-free. This means the module can only
interact with the system through code it calls in its declared dependencies. We
want to enforce that the module as a whole does not call anything outside of its
declared dependencies and adheres to the ambient authority constraint.

## Implementation sketch

- A manifest that describes the module and specifies which files constitute its
  external interface.
- Contracts, as mentioned in the document, would be informal, just included in
  comments — we won't use contract-driven design with verified contracts yet.
- The manifest would also include a specification of the ambient authority, which
  would be empty, perhaps by default, so we might not even need a proto field.
- We are using Protocol Buffers because they are widely used in the environment
  I'll want to try this out in.

## Repository structure

- I've created an empty repository where we are now.
- I want to keep the option open to support other languages, with Rust likely
  being the next one. Therefore, we should put Go-specific tooling into a
  subdirectory, like `./go`, so we can later add Rust tooling in the same
  repository.
- While entirely separate Git repositories for different languages could be
  easier, there will likely be shared tooling, such as the protocol buffers for
  the manifest files and tools to manipulate and parse them.

## Surface

- The expected surface is a CLI that you invoke on the manifest file. It would
  verify that the code adheres to the constraints specified in the manifest.
- We should also consider — as mentioned in the background document — having the
  manifest file generated from a custom Bazel build rule. This could be a wrapper
  rule for `go_library`, something like `go_architectural_component`, which would
  specify the files that make up the external-facing interface and the allowed
  outbound dependencies from anywhere in the module. This would prevent a
  submodule or implementation detail from adding an external dependency to a
  different component or a third-party dependency.
- This Bazel integration can be a second step. The initial MVP should just be the
  CLI that you run as a conformance check on the manifest file to verify if the
  code adheres to it.

## Self-hosting

- I would also like to structure the tooling code itself — the prototype we
  implement here — in the same architectural form. It should be decomposed into
  components, and we should add manifest files that specify the interface files
  for each module.
- It might be difficult to demonstrate this concept on the CLI itself initially.
  When we call Capslock from the tool, Capslock currently requires ambient
  authority. A future goal would be to refactor Capslock using a
  capability-oriented API. For now, we will probably have to proceed this way,
  unless Capslock has a lower-level interface that takes streams, allowing the
  CLI to open the files and hand the capability as a stream to Capslock for
  analysis. However, this probably won't work because Capslock likely needs to
  crawl the whole tree.
- For reference, I've checked out capslock in `../../external/capslock/`.

## Examples

- Since demonstrating the concept on the CLI will be difficult, we should create
  a set of examples. These would be simple Go projects with a couple of
  components (Go packages or modules) that illustrate what this looks like.
- We could have a simple tool that parses a file and does something, perhaps
  parsing a CSV file and returning the top row sorted by a specific column — just
  a very simple example to try it out.

## Absorption of third-party dependencies

- One interesting question we need to resolve is how third-party dependencies are
  "absorbed." If a component uses a third-party dependency, that use must be
  declared as allowed in the manifest. But regarding the ambient authority used,
  if the third-party dependency (like a CSV parser) uses ambient authority, the
  component's declared ambient authority should absorb that dependency's use
  rather than declaring it as a separate dependency. The third-party dependency's
  use of ambient authority becomes an implementation detail of the component, and
  whatever it does becomes part of the component's contract.
- There are two ways a dependency can be absorbed: either it is an explicitly
  discussed architectural component, or it is an implementation detail, in which
  case it is absorbed into the component, and its use of ambient authority needs
  to be surfaced by that component.
- IOW, we might have two kinds of dependencies: those that are other components
  (with a manifest, declared interface etc.), and those that are implementation
  details — the latter ones are "absorbed".

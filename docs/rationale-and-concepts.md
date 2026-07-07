# Architecture as Code and Contract-Driven Design

*Rationale and core concepts — background for later implementation plans.*

Status: draft for discussion. This document explains **what** we are trying to
do and **why**. It deliberately presents the design space (including
alternatives) rather than committing to final mechanisms; those decisions belong
to the implementation plans that will build on this. Concrete alternatives and
language-specific details drawn from the working notes are collected in the
appendix.

---

## 1. Rationale

Modern software systems are too large for any one person to hold in mind at the
level of their source code. Yet the properties we most care about — how the
system is decomposed, how the pieces depend on one another, what guarantees they
make to each other, and what damage a defect in any one piece can do — are
*architectural* properties. They live above the code. Today they are typically
recorded, if at all, in prose documents and diagrams that drift out of sync with
the code the moment either changes.

The goal of this work is to **give a developer durable intellectual control over
a system's high-level architecture without requiring them to read the
implementation details.** The meta-goal behind every concrete idea below is
this: a reviewer should be able to understand and maintain the *shape* of the
system by reading a small, declarative description that is *mechanically
guaranteed* to match the code.

Three consequences follow directly, and they are the reason the rest of this
document exists:

1. **Separate architectural change from implementation change.** If the
   structure of the system changes — a new dependency, a widened interface, a
   component reaching for a capability it previously did not have — that change
   should be *impossible to make silently*. It must show up as an edit to a
   declarative description in a separate file, which is exactly where focused
   review should be spent. Conversely, if only an implementation changes, the
   conformance checks still pass and the change needs proportionally less
   architectural scrutiny. The declarative description becomes the diff that a
   reviewer reads *first*, and often the only one they need to read carefully.

2. **Enable modular reasoning about correctness.** With explicit contracts
   attached to component interfaces, one should be able to reason about whether a
   component is correct using only (a) its own implementation and (b) the
   *contracts* — not the implementations — of the components it depends on. This
   is what makes the reasoning effort scale: it stays local to one component at a
   time.

3. **Bound the blast radius of defects.** If a component structurally cannot
   touch the file system, the network, or shared mutable state, then a bug in it
   — including a security bug — cannot corrupt those things. The component is
   *self-sandboxed* by construction, and we can spend less worry on it.

The unifying idea is **verified, mechanically-enforced, machine-readable
descriptions of architecture and structure.** From the working notes:

> - With contracts: enable modular reasoning.
> - Without contracts: enable intellectual control over architectural shape.

That is, even the leanest adoption — declaring structure and dependencies with
*no* contracts yet — already buys intellectual control over the architecture. Contracts are the next increment, and they unlock modular reasoning about correctness. The pillars below are additive in exactly this sense.

## 2. Overview of the core ideas

The work is organized around four pillars, plus one section on how it scales to
agent-assisted development.

1. **Architecture as code** (§3) — a machine-readable, enforced description of
   *how the system is decomposed into components and how they depend on one
   another.*
2. **Contracts and modular reasoning** (§4) — *what each component guarantees*
   at its interface, so correctness can be reasoned about one component at a
   time.
3. **Ambient authority and capabilities** (§5) — *what system state each
   component is permitted to touch*, so that most components are structurally
   sandboxed.
4. **Data-flow and privacy contracts** (§6) — *constraints on how classes of
   data (e.g. user data, PII) may flow* through the system, enforced statically.

And then:

5. **Scaling to agentic development** (§7) — how small, mechanically-checkable
   contracts make it feasible for both humans and agents to work on one component
   at a time.

A note on how the pillars relate: pillars 1–4 are largely *additive layers* on
the same underlying unit (the component). You can adopt pillar 1 alone and still
get value (intellectual control over shape). Contracts (pillar 2), capability
constraints (pillar 3), and data-flow constraints (pillar 4) each add a further
class of guarantee on top of the same declared structure.

## 3. Pillar 1 — Architecture as code

### 3.1 The component as the unit

The central unit is the **component**, taken from the [C4 architectural
model][C4]'s "component" view but enriched with additional attributes and
constraints. A
component is:

- **A collection of code units that together expose a well-defined interface.**
  The architecture-as-code description names the specific files that contain the
  interface definition. What counts as an "interface file" is language-specific:
  header files in C/C++, or the subset of Go files in a module that hold the
  exported types and public interfaces (see Appendix A.1).

- **The owner of a private implementation.** There is code that implements the
  component and belongs to it, and *no code outside the component may call into
  it.* Ideally this is enforced by language visibility (module-private). Where the
  language cannot express it — e.g. an implementation spread across several Java
  packages whose members are technically `public` — an additional declared
  constraint must enforce the convention that implementation code is not visible
  to the outside.

- **A declarer of its dependencies.** If a component calls into code, uses types
  from, or otherwise communicates with code outside itself, that
  *use-relationship must be explicitly declared*, and it is declared at the
  granularity of *other components*, not individual files or symbols.

### 3.2 Conformance and change surfacing

The declarative description of a component — its interface files, its private
implementation, and its declared dependencies — is a separate, machine-readable
artifact (a manifest). A conformance checker verifies that the code matches the
manifest:

- The implementation calls only into components named as dependencies.
- Nothing outside the component calls into its private implementation.
- The interface is exactly what the named interface files expose.

The payoff is the **change-surfacing** property from §1. Adding an
undeclared dependency, or exposing a new part of the implementation, *fails
conformance* until the manifest is updated to reflect it — at which point the
architectural change is visible in a small, reviewable diff. Implementation-only
changes leave the manifest untouched and pass conformance, so they draw less
architectural scrutiny.

**Avoiding duplication with the build system.** Some of what the manifest records
— particularly the dependency list — may already be specified precisely in a
build system such as Bazel. Where that is the case, the manifest's dependency
portion need not be authored (and kept in sync) by hand: it can be
**auto-generated from the build specification**, e.g. via a custom Bazel build
rule that emits the component's declared dependencies. This keeps a single source
of truth — the build file — and derives the component-interface/contract
specification from it, rather than duplicating the same dependency facts in two
places that could drift apart. (What still has to be authored is the part the
build system does not capture: the interface, the contract, and the declared
ambient authority.)

### 3.3 Relationship to C4, and where we diverge

Mapping to C4 is deliberate but not strict. Two divergences are worth recording:

- **Ownership is not team structure.** C4 tends to assume a "system" is owned by
  a single team. This does not generally hold: at the component level we
  routinely use components owned by someone else — third-party libraries, or
  libraries maintained by another team. These are still *components* in our sense
  (a collection of code units with a well-defined interface and, ideally, a
  contract); they are simply used within our container/system without being owned
  by us. We therefore treat "component" as a structural/contractual notion, not
  an ownership notion.

- **Containers vs. components.** Components are mapped into C4 "containers"
  (a deployable/runnable unit — a process, Docker container, job, etc.). A single
  container typically composes many components, including third-party ones.

### 3.4 Dependency shape: DAG in general, tree preferred

In general the use-relationships between components form a **directed acyclic
graph** (a component may be depended on by several others). However, for the
purposes of tractable reasoning — and especially for the contract-negotiation
process described in §7 — a **tree-leaning** structure is preferable: it keeps
the "ripples" of a contract change flowing in one direction and avoids cycles.
The reconciliation we adopt: allow a DAG in general, prefer tree-shaped
structure where possible, and *push any unavoidable cycles up to the highest
level of the module structure*, where the abstraction is small enough for a
single human or agent to hold in mind (see §7).

## 4. Pillar 2 — Contracts and modular reasoning

### 4.1 Design by contract at the interface

Each component interface carries a **contract** — in the design-by-contract sense
of preconditions, postconditions, and invariants attached to the operations and
types it exposes. The contract, together with the interface signature, is
intended to be *the entire thing a reader needs* in order to use the component
or to reason about code that depends on it.

Two design pressures shape contracts:

- **Contracts should be as small as possible.** A small public interface and
  contract is what makes it feasible to ingest *many* components' contracts at
  once — whether into a human's working memory or an agent's context window —
  without dragging in any implementation detail.

- **The implementation must be verified against its own contract, using only the
  contracts of its dependencies.** This is the crux of **modular reasoning**: to
  establish that component *A* satisfies its contract, we use *A*'s
  implementation plus the *contracts* (not implementations) of the components *A*
  depends on. Reasoning stays local; it never has to unfold the whole call graph.

**Contracts are incremental, partial, and of mixed rigor.** We do not expect a
component to arrive with a complete, fully-formal contract. Adoption is meant to
be fluid: both *how much* of an interface is specified and *how rigorously* it is
specified can grow over time. A single contract may therefore mix:

- **Formal clauses** — precise pre/postcondition and invariant *predicates*, in a
  design-by-contract framework, that are either checked at runtime in a
  development/debug build or (further along the spectrum) statically or formally
  verified.
- **Informal clauses** — aspects specified in prose, for the parts of the
  behavior that are not (yet) captured formally.

So a contract is generally a *partial specification*: some predicates are
machine-checked, the rest is documented intent. The specification itself may live
either in a **separate Markdown file** attached to the interface or **inline in a
comment** on the interface. This range — from a prose sketch, through
runtime-checked assertions and platform-enforced safe-coding checks, to
static/formal verification and (in an agentic setting) agent-produced evidence —
is a deliberate design property, not an unresolved question. The concept that
matters here is that the contract is the *unit of reasoning*, that it can be
strengthened incrementally, and that its formal portion is *checkable*. Which
enforcement to build first is left to the implementation plans.

### 4.2 Two implementation constraints, and how they relate

For modular reasoning to be sound, a component's implementation must be
constrained in ways that go beyond "signatures match." Two constraints recur in
the notes. They are *not* independent: the second is a necessary ingredient for
the first. We give them distinct names to avoid the overloaded word "pure" (see
§5.3 and Appendix B):

- **Purity (a.k.a. "referentially contained" / "injection-closed").** The
  implementation introduces **no mutable global or module-level static state**,
  and it reaches external state *only* through component interfaces that were
  **injected** into it as declared dependencies. In the notes' phrasing, a
  component's implementation is "a set of pure functions over the stateful types
  of the [other] modules": it may call stateful methods on the components handed
  to it, but it owns no ambient mutable state of its own and calls nothing it was
  not given. This is a *modular-reasoning* property — it is what lets us treat the
  component's behavior as a function of its inputs and its injected dependencies.
  (We use "pure" here in the functional-language sense of *no mutable state*, not
  in the sense of touching no ambient authority — that property is
  *ambient-authority-free*, §5.)

- **Authority confinement (see §5).** The implementation touches only the
  ambient authority (file system, network, etc.) that it has declared. This is a
  *security / blast-radius* property in its own right, **and it is a precondition
  for purity**: if a component can reach ambient authority, it can reach mutable
  state outside its injected dependencies — read a global via a syscall, mutate a
  file, observe a clock — and we can no longer guarantee that its behavior is a
  function of its inputs and injected dependencies. So purity presupposes
  authority confinement; the strongest, most reasoning-friendly components are
  both pure *and* ambient-authority-free (declare no ambient authority at all).

### 4.3 Dependency injection as the enforcement handle

A practical way to enforce purity is to require that everything
a component talks to is **injected through its constructors/initializers**. Then:

- The component can only ever call methods on interfaces that were injected into
  it.
- Its dependencies are therefore explicit in its builder/constructor signatures.
- A static check confirms it calls nothing outside that set.

The lightest-weight version of the static check may be an **import allowlist** —
constraining which packages the implementation may import. That may or may not be
sufficient on its own; stronger enforcement (call-graph analysis, no-global-state
checks) is discussed in Appendix A.

## 5. Pillar 3 — Ambient authority and capabilities

### 5.1 Declared authority

Alongside its interface and dependencies, a component **declares its use of
ambient authority** — the system-level powers it exercises, such as touching the
file system or the network. The enforcement goal is that a component which does
*not* declare a given authority provably does not exercise it.

### 5.2 Ambient-authority-free (self-sandboxed) components

The important special case is a component that declares **no** use of ambient
authority — the empty case of authority confinement. We call such a component
**ambient-authority-free.** Its only interactions with the world are invoking methods on
other components. An ambient-authority-free component is **structurally sandboxed**: a
defect in it — including a security defect — cannot read or corrupt the file
system, the network, or other system state, because it structurally has no path
to them. We can therefore spend much less security scrutiny on it. This is the
sense in which "self-sandboxing" turns a security question into a structural one.

### 5.3 A terminology note: "pure"

The working notes use **"pure"** in two different senses. This document keeps them
distinct and, following functional-language usage, reserves **"pure"** for the
*no-mutable-state* sense:

- **Pure** — no mutable global state; interacts with external state only through
  injected dependencies (§4.2). A *modular-reasoning* property.
- **Ambient-authority-free** — declares and uses no ambient authority;
  self-sandboxed (§5.2). A *security / blast-radius* property.

These are different properties, but not independent: authority confinement is a
*precondition* for purity (§4.2), so a pure component is necessarily
authority-confined, and the strongest components are both pure and
ambient-authority-free. The term is deliberately "ambient-*authority-free*"
rather than "authority-free": in capability terms, a module handed a capability
*does* hold authority — just only the authority it was explicitly given. What
this property rules out is *ambient* authority (powers available without being
explicitly granted). (*Hermetic* and *sealed*, borrowed from build-system usage,
are candidate synonyms; see Appendix B.)

### 5.4 Two complementary mechanisms: verify and provide

Two mechanisms appear in the notes. They are best understood as **complementary
layers** — one *verifies* that undeclared authority is not used, the other
*provides* the declared authority — not as competing alternatives.

**(a) Verification — static call-graph policy (Capslock-style).** A static
analysis over the transitive call graph confirms that a component reaches only
the ambient authority it declared. The Go [Capslock] project is the reference
point: it gives policy over what capabilities a transitive call graph exercises,
and is a natural fit for a Go prototype (Appendix A.1). This mechanism is the
*enforcement* of §5.1.

**(b) Provision — a capability box.** Rather than plumbing capability objects
through every layer of the system by hand, we provide a **capability box**: a
registry from which a component can retrieve the specific capabilities it
declared it needs. Capabilities can be **name-based** (e.g. keyed by package
name). A component that needs, say, file access for logging declares that need in
its manifest; a static analysis then confirms that the only capabilities it
accesses within its code match that declaration. At assembly time the box is
populated with the concrete capabilities, and a static check confirms that every
capability required by every dependent component is actually provided (or at
least warns about what is missing). Capabilities thus **flow transparently** to
the components that need them, without being threaded through unrelated
intermediaries.

The box is a *distribution* mechanism, and it presupposes that the underlying
system APIs are themselves **capability-oriented** — i.e. a component reaches the
file system by passing a capability drawn from the box into a capability-taking
API, not by calling an ambient path-based API. That is what gives the static
analysis a clean rule (flag any call to a low-level ambient API); see Appendix
A.2.

An optional extension is **multiple capability boxes attached to scopes**: a
system-level box populated once at startup, and, e.g., a request-scoped box
attached to a request context from which a handler retrieves per-request
capabilities. This mirrors the static/request scopes familiar from dependency
injection frameworks (Guice, Dagger). Whether request scoping is necessary is an
open question; it is recorded as a possible extension.

The relationship between (a) and (b): the capability box is how authority is
*granted*; the static analysis is how we *verify* that nothing exercises
authority it was not granted. Either can exist without the other, but together
they close the loop. Appendix A.2 records a third, more manual alternative
(explicit forwarding of capability objects) and the trade-offs among them.

## 6. Pillar 4 — Data-flow and privacy contracts

Beyond *which components* may talk and *what authority* they hold, we want
contracts over **how classes of data may flow** through the system — in
particular, static guarantees about the flow of **user data** through a service.

The approach sketched in the notes:

- **A single typed carrier for application data.** Use Protocol Buffers as the
  container for all application data that flows in and out of components. This
  gives every piece of application data a typed home with a place to hang
  metadata.

- **Mandatory data-classification annotations.** Every field in a protobuf is
  annotated with metadata about *what kind of data it is* — e.g. user data, PII,
  or system/configuration data. To keep this tractable (Google's internal scheme
  is powerful but complex — see Appendix A.3), we favor a **default-and-override**
  policy: fields on external-facing interfaces (e.g. gRPC endpoints) default to
  the most conservative classification (user data), and fields that are *not*
  user data must be *explicitly* annotated as system data (or configuration,
  etc.).

- **A static data-flow checker.** The checker enforces that data may only flow
  into a destination whose annotation *matches* its classification. Concretely:
  if code reads a PII-annotated field into a local variable and later writes it
  into another protobuf, the checker must confirm the destination field is
  *also* annotated PII (or otherwise compatible). This is a constraint over
  data-flow — values must remain in correctly-annotated homes.

- **Emergent privacy guarantees.** Such a discipline *implicitly* prevents user
  data from being logged: a logging sink accepts only data cleared for
  disclosure (system data), so user/PII data cannot reach it without a
  classification violation. A likely rule set: system (non-user) data may flow
  into disclosure/logging sinks freely, whereas user/PII data may only remain in
  appropriately-annotated protobufs and be written only to appropriate storage.

This pillar is the data-flow analogue of the capability story in §5: §5 confines
*effects*, §6 confines *data*.

## 7. Scaling to agentic development

The properties above are valuable for human-authored code, but they become
especially powerful when code is written and maintained by agents — because they
turn "understand the whole system" into "understand one component's contract at a
time." This section is a secondary elaboration: agents are a *means* to the
meta-goal (human intellectual control), not the point of the work.

The scaling design:

- **Minimize each component's public surface.** The design objective for every
  component is that its public interface and contract be as small as possible,
  so that an agent reasoning about the whole program — or about another
  component's implementation — can ingest it while pulling in as little as
  possible about the component's *implementation*. This is the §4.1 "small
  contracts" pressure, viewed through the lens of context-window economy.

- **Per-component reasoning obligations.** An agent responsible for a component
  must demonstrate that its implementation satisfies its contract: writing the
  tests, static checks, and whatever evidence is required. Some of this is
  discharged for free by platform-level, enforced safe-coding style.

- **Whole-system reasoning from contracts alone.** Because contracts are small
  and implementation is hidden, an agent (or human) can reason about *many*
  components together purely from their public contracts, checking that the
  system as a whole makes sense.

- **Contract co-evolution via negotiation.** The hard part is keeping
  *dependencies* strong enough: an implementation must be able to get from its
  dependencies' contracts everything it needs. So contracts must **evolve
  together.** One model: assign an agent per component. When a feature is needed,
  the relevant agents reason about what they must add to their own contracts, and
  what they need from others' contracts, to satisfy it. A request such as "I need
  you to add this pre/postcondition — can you reasonably support it given your
  implementation?" may be answerable locally, or may **ripple**: the asked agent
  may in turn need to ask *its* dependencies to strengthen *their* contracts.

- **Why tree-shaped structure helps.** If the dependency structure is tree-like,
  these ripples always flow **downward** and never form cycles, which is far more
  tractable than negotiating around a cyclic graph. This is the reasoning behind
  the §3.4 preference: keep any unavoidable cycles at the very top of the module
  structure, where the top-level abstraction is small enough for a single agent
  or human to hold entirely in mind, and let detail "trickle down" a tree from
  there.

## 8. Summary

The through-line: **make the architecture a first-class, machine-checked
artifact so that intellectual control over a system scales independently of its
size.** Structure (§3) gives control over shape; contracts (§4) give modular
reasoning about correctness; capability confinement (§5) bounds the blast radius
of defects; data-flow contracts (§6) bound where sensitive data can go; and small,
checkable contracts (§7) make all of this workable at the pace of agentic
development. In every case the mechanism is the same: **a small declarative
description, separate from the code, that the tooling guarantees the code
conforms to** — so that a reviewer reads the description, not the details, and
trusts it.

This document is background. The next step is a set of implementation plans that
prototype the tooling — most likely starting in Go, using Capslock-style
call-graph analysis — for each of these pillars.

---

## Appendix A — Design alternatives and language-specific notes

These are the concrete alternatives and per-language details from the working
notes, kept out of the main text to preserve its altitude.

### A.1 Prototype target and interface-file identification

- **Go as the prototype/target language.** The initial prototype is expected to
  target Go, largely because [Capslock] provides policy over transitive call
  graphs there, which fits the authority-confinement enforcement in §5.4(a).
- **Identifying interface files.** The manifest names the files that constitute a
  component's interface. This is language-specific:
  - *C/C++*: header files.
  - *Go*: the subset of `.go` files in a module holding the exported types and
    public interfaces.
- **The visibility gap (Java, etc.).** Some languages cannot make a component's
  implementation private when it spans multiple packages — members may be
  technically `public` yet must not be called from outside the component. In such
  cases an *additional declared constraint* (checked by the conformance tooling)
  must enforce the "implementation is not externally visible" convention that the
  language cannot. A promising way to close this gap is **static call-graph
  analysis** — Capslock, or an equivalent call-graph analyzer for the target
  language (a Java equivalent, or Capslock itself for Go): check that a dependent
  component has *no call edge* to any function or symbol that is not part of the
  dependency component's *declared interface*. This turns "don't call the private
  implementation" into a mechanically-checkable graph property rather than a
  reliance on language visibility.

### A.2 Capability mechanisms — the three options and trade-offs

1. **A capability abstraction threaded through the system (e.g. `cap-std`).**
   Model the whole system on a capability library (the Rust `cap-std` is the
   reference), where APIs take file-system and network *capabilities* rather than
   named paths. Clean in principle, but **awkward in practice** because the
   capability types must be factored through the *entire* system.
2. **A capability box (preferred in the notes; see §5.4(b)).** Ambient authority
   is used *once*, at the box, to retrieve named capabilities; components declare
   the capability names they need; static analysis confirms only declared
   capabilities are accessed; the box is populated at startup and checked for
   completeness. Optionally multiple boxes per scope (system, request), à la
   Guice/Dagger.
3. **Explicit forwarding.** Capabilities are simply passed with each call/request;
   APIs take capability types rather than raw paths, and (e.g.) a file path is
   resolved via a capability drawn from the box. This is essentially option 1
   applied locally rather than globally.

**Option 2 does not replace option 1 — it layers on top of it.** Even with a
capability box, the *underlying* system APIs must still be **capability-oriented**
(the option-1 abstraction): a component that needs the file system must call a
file-system API that *takes a capability* it retrieved from the box, rather than
an ambient, path-based API. Otherwise there is no way for static analysis to tell
a legitimate call apart from an ambient one. Concretely: if the file-system
capability is drawn from the box and passed into a capability-oriented API, then
the static call-graph analysis has a clean rule to enforce — flag *any* call to a
low-level, ambient file-system API. Without the capability abstraction underneath,
the box distributes authority but the analysis cannot verify that authority is
the *only* path to the resource. So the capability box is best seen as the
*distribution* mechanism for capabilities whose *underlying APIs are already
capability-oriented*.

Given that, the notes lean toward the **capability box** as the primary
*distribution* mechanism because it avoids threading capability types through
unrelated intermediaries while still permitting static verification. Explicit
forwarding remains available where a call site genuinely wants to pass authority
in-band (e.g. per-request).

### A.3 Data-flow / privacy annotation details

- **Carrier.** Protocol Buffers as the container for all application data
  crossing component boundaries.
- **Why not the full Google scheme.** Google has a very capable but *complex*
  internal system for this. We intend to keep the classification simple.
- **Default-and-override policy.** Protobufs on external-facing interfaces (e.g.
  gRPC endpoints) default to `user data`; non-user fields must be *explicitly*
  annotated (e.g. `system data`, `configuration`).
- **Checker semantics.** A static analysis over data-flow nodes: a value read out
  of a field may only be written into a destination field whose annotation
  matches (or is compatible with) its classification. Worked example: reading a
  `PII` field into a local and later assigning it into another protobuf requires
  the destination field to be `PII`-annotated.
- **Emergent rule for disclosure.** System (non-user) data may flow into logging
  / disclosure sinks; user/PII data may only remain in appropriately-annotated
  protobufs and be written to appropriate storage — which implicitly blocks
  user-data logging.

## Appendix B — Terminology

| Term | Meaning | Kind of property |
|---|---|---|
| **Component** | A collection of code units with a well-defined interface, a private implementation, and declared dependencies and authority (enriched C4 "component"). | Structural |
| **Manifest** | The declarative, machine-readable description of a component (interface files, private implementation, dependencies, declared authority, capability needs). | Structural |
| **Contract** | Design-by-contract pre/postconditions and invariants on a component's interface; the unit of modular reasoning. | Behavioral |
| **Pure** ("referentially contained" / "injection-closed") | Implementation has no mutable global/static state and reaches external state only through injected, declared dependencies (functional-language sense of "pure"). | Modular-reasoning |
| **Ambient authority** | System-level powers (file system, network, …) a component may exercise; must be declared. | Security |
| **Authority confinement** | Implementation exercises only the ambient authority it declared; a precondition for purity. | Security |
| **Ambient-authority-free** (self-sandboxed; *hermetic*/*sealed* are candidate synonyms) | Declares and uses **no** *ambient* authority; interacts only by calling other components. The empty case of ambient authority (a capability-granted module still holds its explicitly-given authority). | Security / blast-radius |
| **Capability box** | A registry supplying declared, name-based capabilities to components; populated at assembly/startup; completeness statically checked. | Mechanism |

## Appendix C — Open questions

- **Which enforcement to build first.** Contract rigor is fluid and incremental
  by design (§4.1); the open question is which point on the spectrum (runtime
  debug-mode assertions → tests → static/formal verification → agent-produced
  evidence) the *first* prototype should target.
- **A shorter synonym for "ambient-authority-free"?** The precise term is settled
  as *ambient-authority-free* (a capability-granted module still holds its given
  authority). The only open question is whether to also adopt a shorter
  synonym — *hermetic* or *sealed* — for prose. (§5.3, Appendix B)
- **Request-scoped capability boxes.** Is per-request scoping actually needed, or
  is a single system-level box sufficient? (§5.4)
- **Sufficiency of import allowlisting.** Is constraining imports enough to
  enforce purity, or is call-graph / no-global-state analysis required? (§4.3)
- **Cycle handling at the top level.** How exactly do we express and review the
  small, possibly-cyclic top-level abstraction that the tree hangs from? (§3.4,
  §7)

---

*Sources: spoken notes of 2026-02-24 (foundational: interfaces/contracts, DI,
Capslock, protobuf data-flow), 2026-06-14 (agentic scaling), and 2026-06-27
(architecture-as-code / C4, capabilities, goals).*

[Capslock]: https://github.com/google/capslock
[C4]: https://c4model.com/

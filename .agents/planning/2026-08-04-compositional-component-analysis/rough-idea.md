# Rough idea — compositional component analysis

**Date:** 2026-08-04
**Source:** spoken notes, transcribed, lightly cleaned for readability. Recorded
verbatim in substance; no decisions have been folded in.
**Origin:** observation that `arcc check` is slow on non-trivial code (~50s for some
packages in the second monorepo PoC), combined with the realisation that two recently
landed changes — explicit membership with member-as-root analysis, and pruning at
declared dependency boundaries — may have made whole-program call-graph analysis
unnecessary for component checks.

---

## The idea as stated

So I'm thinking that we can probably simplify arcc quite a bit and make it faster.
The observation is that we've made a couple of changes in the recent incremental
improvements which I think mean that we no longer need to actually do a full
call-graph analysis for each component. We may need it only for the standard library.

Here's the reasoning. For the component analysis, we've changed it so that we
basically treat the entire set of members as a root, and really what we just care
about is whether there's any call-graph edge that goes out of the component into
either another component, or something that's not declared, or into the standard
library into a function that exercises ambient authority.

So what I was thinking is whether we can essentially change the analysis so that we
precompute once, for each release of the standard library, a map of the entire public
surface of the standard library — basically all the functions that are public and can
be called from the outside. That map would contain the symbol ID of the function and
which ambient authority it leads to. For that we would still need Capslock, because
we'll need to do call-graph analysis of the implementation of the standard library to
figure out which of its external-facing symbols lead to ambient authority use.

But once we have that map, when we analyze a component I think it may actually be
sufficient to just look — without computing a full call graph — at all the members of
the component, and all the call sites, all the function calls within the component,
and then simply look up whether the destination of the call is a function that lives
outside the component. If it appears in the map of the public surface of the standard
library, then we look up the pre-recorded ambient authority use of that function, and
we check whether that use is declared for the component. So if for instance there's a
call site of `os.ReadFile`, we look that up in the standard library capability-use
manifest, we see that it exercises the files capability, and if the component doesn't
declare files, we have an error.

Then, after we analyze the component itself, we create a manifest of its entire public
surface. For a component with a declared interface, we create a manifest that
enumerates all the functions in that interface. For a package-style component, we just
take all the public methods of the packages that are declared for that component.

And then when we analyze a component against the components it depends on: if we have
a call site of a function within a component, it either leads into the standard
library — in which case we do what I talked about earlier — or it leads into the
manifest of the exported public interface of another component, and if that component
is declared, then we're good. Everything else is basically a violation: a call to a
function that is not in the standard library and is not in a declared component
dependency.

There's one wrinkle, which is absorbed members — absorbed dependencies. I think we can
actually get rid of those without loss of generality. We introduced this before we
really had the concept of package-style components, but now we have that. If we have a
component that currently might have an absorbed member, we should instead create a
package-style component for that absorbed member and declare it as a dependency. This
does change where the use of ambient authority is attributed to, but I think that's
okay, and it's maybe even preferable, because it keeps the granularity of what
capability uses are attributed to more fine-grained.

So let's have a think about that. Maybe this is a promising approach, because I've
noticed the analysis is really quite slow, especially for non-trivial code. With this,
the only place we would need to do call-graph analysis using Capslock would be for the
internals of the standard library, to figure out which of its publicly exposed surface
leads to capability use. For analyzing components we don't need to build a call graph
anymore. We don't even need to use Capslock. We just load the package, traverse all
its functions — we might steal some code from Capslock that does this — and find all
the call sites within it and classify them as to where they lead. That should be
sufficient.

## Secondary input

A friction report from the second monorepo import
(`dev-exp-go-bazel-mvp` @ `5011b726`) was brought into the discussion partway through,
to check which of its recommendations align with this design and which are orthogonal.
Its triage is recorded in
[`research/host-import-friction.md`](research/host-import-friction.md).

## What this rough idea did *not* yet settle

Everything in [`idea-honing.md`](idea-honing.md). In particular the rough idea asserts
that absorbed dependencies can be removed "without loss of generality", which turned
out to be true only under an invariant that had to be stated explicitly (Q4), and it
proposes scanning *call sites*, which turned out to be insufficient (Q2).

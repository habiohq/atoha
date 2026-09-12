# Foundation roadmap

Implementation status (2026-09-12): the foundation sequence is executable.
Core semantics, separate application use cases, Home Assistant ACL, a durable
SQLite Journal, HTTP adapter, local composition root, and dependency checks are
implemented. MCP packaging and independent repository extraction remain later
release-boundary decisions.

This sequence targets the first one to two weeks. Calendar time is indicative;
semantic evidence, not task count, determines readiness.

## Days 1-2: establish boundaries

- Merge the foundation documents and agree on scope/non-goals.
- Open Action/ExecutionAttempt and Outcome RFC discussions.
- Collect concrete timeout, acknowledgement, observation, and replay scenarios.
- Define v0.1 acceptance examples without defining device types in core.

Exit: reviewers can explain what Habio owns and identify false certainty in a
proposed API.

## Days 3-5: smallest executable semantics

- Draft Action and ExecutionAttempt types behind an unstable package boundary.
- Model outcome knowledge separately from Go errors.
- Define minimal Provider dispatch and EventSink contracts.
- Add table-driven tests for pre-dispatch rejection, acknowledgement,
  post-dispatch timeout, and unknown outcome.

Exit: a fake provider demonstrates that timeout does not become physical
failure or automatic retry.

## Days 6-8: observation and verification

- Define timestamped Observation evidence and a narrow Verifier contract.
- Test fresh, stale, missing, and contradictory observations.
- Implement an in-memory fact sink and reconstruct a minimal status projection.
- Keep projection infrastructure intentionally small.

Exit: command acknowledgement and desired-effect verification are visibly
different in the API and tests.

## Days 9-10: Home Assistant proof of concept

- Start the self-hosted runtime boundary; create `habio-server` only if its
  release and dependency boundary is already useful.
- Exercise light, climate, and media-player entities as provider fixtures.
- Capture real or reproducible traces for ambiguous and unverified outcomes.
- Expose a minimal adapter response that cannot overstate certainty.

Exit: one local-only vertical demonstrates acknowledged, verified, unverified,
and unknown cases without adding Home Assistant or device concepts to core.

## Deferred

MCP packaging, out-of-process provider protocols, registries, strategy runtime,
marketplace, cloud sync, dynamic plugins, and a distributed/full event platform
wait for their own evidence and RFCs. The local SQLite Journal is intentionally
smaller than that deferred platform.

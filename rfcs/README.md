# Atoha RFCs

RFCs record durable design decisions that affect execution semantics,
extension contracts, repository boundaries, or ecosystem interoperability.
They are deliberately lightweight.

Use an issue or pull request for a bounded implementation choice. Use an RFC
when a decision changes what callers or third-party implementers can rely on.

## Process

1. Copy the template below to `rfcs/NNNN-short-title.md`; use `0000` until a
   maintainer assigns a number.
2. Open a pull request and link the motivating issues and evidence.
3. Keep unresolved alternatives explicit. A prototype may accompany the RFC.
4. Maintainers record Accepted, Rejected, or Superseded in the Decision section.
5. Semantic code lands only when the RFC is accepted or is clearly marked
   experimental and reversible.

Initial RFC candidates are Action and ExecutionAttempt, Outcome and ambiguity,
Observation and Verification, the Provider API, a provider process protocol,
the event log, and portable Strategy binding.

## Template

```markdown
# RFC 0000: Title

- Status: Draft
- Authors:
- Created: YYYY-MM-DD
- Related issues:

## Summary

## Motivation

## Proposal

## Alternatives

## Drawbacks

## Open questions

## Decision
```

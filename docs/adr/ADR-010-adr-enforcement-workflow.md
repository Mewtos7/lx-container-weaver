# ADR-010 — ADR Enforcement Workflow

| Field  | Value |
|--------|-------|
| Status | Accepted |
| Date   | 2026-04-17 |

## Context

ADRs (Architecture Decision Records) are only valuable if contributors consistently create or reference them when making architecture-impacting changes. Without an enforcement mechanism, ADRs can be overlooked under time pressure, silently making the decision log incomplete and unreliable.

ADR-001 states that every significant architectural decision must produce an ADR. However, that rule was not previously backed by any automated check or contributor checklist, relying entirely on reviewer vigilance.

## Decision

A lightweight two-layer enforcement model is adopted:

1. **Contributor checklist** — A pull request template (`.github/pull_request_template.md`) prompts every PR author to confirm whether the change requires a new or updated ADR and to record any ADR reference(s).

2. **CI gate** — A GitHub Actions workflow (`.github/workflows/adr-enforcement.yml`) runs on every pull request and fails if:
   - One or more files in architecture-impacting paths are modified (`src/`, `cmd/`, `internal/`, `pkg/`, `api/`, `.github/workflows/`), **and**
   - The PR neither includes a file under `docs/adr/` nor mentions an ADR identifier (`ADR-NNN`) in its title or description.

**Architecture-impacting paths** are defined as source code directories and CI/CD workflows. Documentation-only changes (e.g., `README.md`, `docs/` other than ADRs themselves) do not trigger the check.

Contributing guidelines (`CONTRIBUTING.md`) are updated to document when an ADR is required and how to satisfy the CI gate.

## Rationale

- Automated checks catch omissions that code review may miss, especially in fast-moving iterations.
- Checking the PR body for an ADR reference is a low-friction signal: authors who knowingly skip an ADR must explicitly state which existing ADR covers the decision.
- Keeping the check in a shell script within the workflow file avoids third-party action dependencies and keeps the logic auditable.
- The PR template makes the expectation visible at the moment it matters most — when a contributor is writing up their changes.

## Alternatives Considered

- **Rely solely on reviewer enforcement:** Inconsistent; reviewers may overlook the requirement under time pressure.
- **Require an ADR file for every architecture-impacting PR (no body-reference escape hatch):** Too strict — some changes clearly extend an existing decision without warranting a new ADR.
- **Use a third-party "changed-files" Action:** Adds a supply-chain dependency; the same result is achieved with `gh pr view` and standard shell utilities.
- **Enforce at merge (branch protection rule):** GitHub's required status checks already provide this once the workflow is in place; no additional configuration is needed beyond enabling branch protection on `main`.

## Consequences

- PRs that touch architecture-impacting paths and omit an ADR reference will fail CI and cannot be merged until the author either adds an ADR file or provides a reference in the PR description.
- The list of architecture-impacting paths in the workflow must be kept in sync with the evolving project structure (e.g., when new top-level source directories are added).
- False positives are possible if an ADR reference appears in the PR body for an unrelated reason; the risk is accepted as negligible given the benefit.

## Related ADRs

- ADR-001 — ADR Format and Storage Convention

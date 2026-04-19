# ADR-010 — CI Pipeline Strategy

| Field  | Value |
|--------|-------|
| Status | Accepted |
| Date   | 2026-04-18 |

## Context

As the codebase grows, manually verifying that every change compiles, passes tests, and meets style standards becomes error-prone and time-consuming. Contributors may unknowingly merge code that breaks the build, introduces regressions, or violates formatting conventions. A consistent, automated validation step is needed to catch these problems before they reach the main branch.

## Decision

A GitHub Actions CI workflow (`.github/workflows/ci.yml`) is introduced that runs automatically on every pull request. The pipeline executes the following stages in order:

1. **Build** — `make build` (`go build ./...`) verifies the project compiles cleanly.
2. **Test** — `make test` (`go test ./...`) runs the full unit-test suite.
3. **Vet** — `make vet` (`go vet ./...`) catches common correctness issues not covered by the compiler.
4. **Format check** — `make fmt-check` fails if any source file is not `gofmt`-formatted.
5. **Lint** — `golangci-lint` (via `golangci/golangci-lint-action`) enforces additional static-analysis rules defined in `.golangci.yml`.

The pipeline is scoped to pull requests only. Direct pushes to `main` do not re-run CI because changes reach `main` exclusively through reviewed and CI-validated pull requests.

All pipeline stages are also available as local `make` targets so contributors can run the same checks before opening a PR.

## Rationale

- **Pull-request scope:** Triggering CI on PRs catches issues before merge, which is where automated checks add the most value. Running CI again on push to `main` would be redundant when the branch-protection model already requires a passing PR.
- **Unified commands:** Defining validation as Makefile targets (`validate`, `vet`, `fmt-check`, `lint`) means the CI workflow and contributors' local workflow use identical commands, eliminating "works on my machine" gaps.
- **GitHub Actions:** Already available for the repository at no additional cost, with first-class Go support via `actions/setup-go` and the golangci-lint community action.
- **golangci-lint:** Aggregates multiple linters in a single tool invocation, keeping CI fast while covering a broad range of correctness and style issues.
- **Pinned action versions:** All Actions are referenced at a specific major version (`@v4`, `@v5`, `@v9`) to ensure reproducible runs.

## Alternatives Considered

- **No CI (manual verification only):** Relies entirely on contributor discipline. Does not scale as the contributor base grows and provides no safety net for reviewers.
- **CI on push to `main` only:** Would surface failures after code is already merged, making rollback the only remediation path.
- **External CI service (CircleCI, Travis CI, etc.):** Adds a third-party dependency and configuration overhead with no material advantage over GitHub Actions for this repository's scale and hosting model.
- **Separate lint job:** Running lint in a parallel job would slightly reduce wall-clock time but adds workflow complexity that is unnecessary at the current project size.

## Consequences

- Every pull request must pass all five pipeline stages before it can be reviewed and merged.
- Contributors can run `make validate` locally to reproduce CI results before pushing.
- golangci-lint rules are managed in `.golangci.yml`; changes to enabled linters require updating that file.
- The pipeline version pins (`golangci-lint v2.1.6`, Action major versions) must be kept up to date as part of routine maintenance.

## Related ADRs

- ADR-009 — Implementation Language Choice

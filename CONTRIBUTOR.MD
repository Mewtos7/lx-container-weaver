# Contributor Guidelines

These guidelines apply to all contributors (human and artificial) working on this project.

## Git and GitHub conventions

- Use [Conventional Commits](https://www.conventionalcommits.org/) for commit messages.
- Keep commits small and focused on a single logical change.
- Reference issues in commit messages (for example: `(#4)`) so GitHub links and closes work items correctly.
- Open pull requests with linked issues/work items and a clear scope.

## Code conventions

- Use clear, descriptive names; avoid abbreviations for variables and functions.
- Keep formatting and line breaks consistent with surrounding code.
- Follow language-specific style guidelines and existing project patterns.

## Workflow guidelines

- When changing code, also review and update related:
  - Documentation
  - Guidelines and contributor instructions
  - Dependencies
  - CI/CD pipelines and GitHub Actions

## Legal conventions

- Ensure project and dependency licenses allow:
  - Free software distribution
  - Free commercial use, including selling products built with this software
- Do not introduce dependencies with incompatible license terms.

## Security guidelines

- Design and implement changes to avoid OWASP Top 10 risks.
- Ensure proper authentication and authorization for protected operations.
- Validate inputs, protect secrets, and follow least-privilege principles.

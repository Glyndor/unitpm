# Contributing to Lynx

Thank you for your interest in contributing to Lynx.

Lynx is a community-driven, transparent project focused on secure and
efficient supervision of applications and services. To maintain clarity,
consistency, and long-term sustainability, contributions are accepted
under the rules described below.


## Types of Contributions

The following types of contributions are welcome:

- Bug reports (issues)
- Security reports and hardening suggestions
- Feature proposals
- Documentation
- Code contributions
- Reviews and architectural suggestions

All contributions must align with the project's community-first nature and
its licensing terms, including the prohibition of monetizing Lynx itself.


## General Guidelines

- Be respectful and clear in all communications
- Clearly describe the problem or the proposed improvement
- Keep contributions focused and well-scoped
- Prefer small, reviewable PRs over large changes
- Do not submit contributions with the intent to enable commercial
  exploitation (e.g., paid SaaS enablement, license circumvention, closed
  integration paths)


## Security Reporting

If you believe you have found a security vulnerability in Lynx:

- Do not open a public issue with exploit details
- Provide a minimal reproduction and impact description
- If the repository includes a SECURITY.md, follow it
- Otherwise, open an issue titled: `security: responsible disclosure` and
  avoid including weaponized details (maintainers may request a private
  channel)

The goal is to protect users while fixing the issue quickly.


## License of Contributions

By contributing to this project, you agree that:

- Your contribution is published under the same license as Lynx
- You do not gain any right to relicense the project or your contribution
  under different terms within this repository
- You do not obtain any commercial rights to Lynx
- Contributions may be modified, requested to be changed, or rejected by
  the maintainers

No exclusive rights are transferred to contributors.


## Contribution Process

1. Open an issue to discuss the change when appropriate
2. Fork the repository (external contributors)
3. Create a dedicated branch for your change
4. Submit a Pull Request with a clear description, motivation, and testing notes

Maintainers may request changes before accepting a contribution.


## Branching Model

All changes go through Pull Requests targeting `main`. Direct commits to `main` are not allowed.

- External contributors: fork the repo, create a branch, open a PR targeting `main`.
- Maintainers and collaborators: may create branches directly in the repo and open PRs targeting `main`.


## Commit Message Convention

This project follows the Conventional Commits specification (v1.0.0).

Contributors are expected to use commit messages in the following format:

`type: short description`

Examples:
- `docs: add contributing guidelines`
- `feat: add config loader`
- `fix: handle nil user limits`
- `chore: update CI workflow`

For full details, see:
https://www.conventionalcommits.org/en/v1.0.0/


## Branch Naming Convention

Contributors are encouraged to use the following branch naming format:

- `feat/<short-slug>` for new features
- `fix/<short-slug>` for bug fixes
- `docs/<short-slug>` for documentation
- `chore/<short-slug>` for maintenance tasks
- `refactor/<short-slug>` for refactoring
- `test/<short-slug>` for test-related changes
- `security/<short-slug>` for security hardening

Examples:
- `feat/user-isolation`
- `fix/supervisor-restart-loop`
- `docs/update-contributing`
- `security/limit-socket-perms`


## Note on Commercial Use

Lynx may be used internally by commercial organizations under the Apache 2.0 license. Contributions intended to commercialize Lynx itself (paid access, SaaS/PaaS, proprietary relicensing) are not aligned with the project and will not be accepted.
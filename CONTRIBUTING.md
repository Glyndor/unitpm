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

This project uses a two-branch workflow:

- `main`: stable, production-ready code
- `develop`: integration branch for ongoing development

Direct commits to `main` and `develop` are not allowed.
All changes must be introduced through Pull Requests.


## Forks and Pull Requests

### External contributors

External contributors must:

1. Fork the repository
2. Create a branch in their fork
3. Open a Pull Request targeting the `develop` branch

Pull Requests from forks targeting `main` will not be accepted.


### Maintainers and collaborators

Maintainers and approved collaborators may:

- Create branches directly in the main repository
- Open Pull Requests targeting `develop`

Only maintainers are responsible for merging changes from `develop` into `main`.


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


## Conventions Scope

The conventions described in this document are intended to provide clarity
and consistency without introducing unnecessary rigidity.

- Conventional Commits applies to **commit messages**
- Branch naming convention applies to **branch names**

Both conventions are complementary and aim to improve collaboration,
readability, and long-term maintainability.


## Project Authority

Final decisions regarding the project, including design, scope, roadmap,
and licensing, are made by the original author and designated maintainers.

Contributions do not grant control, authority, or licensing exceptions.


## Note on Commercial Use

Lynx may be used internally by commercial organizations under the project
license. However, contributions or proposals intended to facilitate the
commercialization of Lynx itself (selling Lynx, paid access, SaaS/PaaS,
“enterprise editions”, or proprietary relicensing paths) are not aligned
with the project and will not be accepted.
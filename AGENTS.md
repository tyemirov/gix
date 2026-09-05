# AGENTS.md

## Forward-Only Contract Discipline

This repository follows a forward-only, confident programming paradigm. This is a binding agent contract: no fallbacks, no backward compatibility, no legacy support, and no compatibility shims. Do not spend design or implementation effort on backward compatibility considerations except for explicit one-off data migrations into the current canonical contract.

Repeat for emphasis because this rule is binding: no fallbacks, no backward compatibility, no legacy compatibility. Delete or reject obsolete code paths, stale schemas, deprecated config, and old persisted shapes instead of preserving them through compatibility layers, dual reads/writes, aliases, or best-effort recovery.

One-off data migrations are allowed only when they move existing persisted data into the current schema in a bounded operation. After migration, remove the bridge and keep only the current contract.

## gix, a Git/GitHub helper CLI

gix keeps large fleets of Git repositories in a healthy state. It bundles the day-to-day tasks every maintainer repeats: normalising folder names, aligning remotes, pruning stale branches, scrubbing GHCR images, and shipping consistent release notes. See README.md for details

## Document Roles

- `.mprlab/NOTES.md`: Read-only process playbook maintained by leads. Agents never edit it during implementation cycles.
- `.mprlab/ISSUES.md`: Append-only log of newly discovered requests and changes. No instructive sections live here; each entry records what changed or what was discovered.
- `.mprlab/ARCHIVE.md`: Resolved issue history preserved during backlog cleanup.
- `.mprlab/<PLAN-ID>-PLAN.md`: Temporary execution plan. Use `.mprlab/PLANNING.md` for the plan ID.

### Document Precedence

- `.mprlab/POLICY.md` defines binding validation, error-handling, and “confident programming” rules.
- `AGENTS.md` (this file) defines repo-wide workflow, testing philosophy, and agent behavior; stack-specific AGENTS.* guides refine these rules for each technology.
- `.mprlab/AGENTS.*.md` files never contradict `AGENTS.md` or `.mprlab/POLICY.md`; if guidance appears inconsistent, defer to `.mprlab/POLICY.md` first, then `AGENTS.md`, and treat the stack guide as a refinement.
- `.mprlab/NOTES.md` is process-only and must not introduce rules that conflict with `.mprlab/POLICY.md` or any `AGENTS*.md` files.

### Issue Status Terms

- Resolved: Completed and verified; no further action.
- Unresolved: Needs decision and/or implementation.
- Blocked: Requires an external dependency or policy decision.

### Validation & Confidence Policy

All rules for validation, error handling, invariants, and “confident programming” (no defensive checks, edge-only validation, smart constructors, CI gates) are defined in `.mprlab/POLICY.md`. Treat that document as binding; this file does not restate them.

### Build & Test Commands

- Use the repository `Makefile` for local automation. Invoke `make test`, `make lint`, `make ci`, or other documented targets instead of running ad-hoc tool commands.
- `make test` runs the canonical test suite for the active stack.
- `make lint` enforces linting rules before code review.
- `make ci` mirrors the GitHub Actions workflow and should pass locally before opening a PR.

### Tooling Workflow (Tests, Lint, Format)

- For any change intended to land, agents MUST ensure that all required tooling for the relevant stack (tests, linters, and formatters as defined in `AGENTS*` and `.mprlab/POLICY.md`) passes cleanly on the branch before code is merged or released.
- `.mprlab/NOTES.md` defines the human workflow. `.mprlab/POLICY.md` defines when validation targets run.

### Testing Philosophy

- Use test-driven development with an inverted test pyramid.
- We **strive for 100% test coverage**, achieved primarily through integration/black-box suites whose scenarios are exhaustive enough to exercise all meaningful branches and error paths.
- For CLI and backend services, tests compile or run the real program/CLI entrypoints, capture exit codes and output (stdout/stderr, files, side effects), and assert against those observable results—not internal functions.
- For web/UI, tests run the app and backing web server, drive flows through the browser, and assert against the rendered page, DOM state, events, and other user-visible behavior.
- Unit tests do not prove public behavior and are prohibited.

## Tech Stack Guides

Stack-specific instructions now live in dedicated files. Apply the relevant guide alongside the shared policies above.

- Backend (Go): `.mprlab/AGENTS.GO.md`
- Browser frontend: `.mprlab/AGENTS.FRONTEND.md`
- Git and version control workflow: `.mprlab/AGENTS.GIT.md`

<!-- BEGIN MPRLAB-GOVERNANCE -->
## MPR Lab Governance

Most workflow context files live under `.mprlab/`. The root `AGENTS.md` remains the repository entrypoint for agents.

Read these files before editing:

- `.mprlab/POLICY.md`: binding validation and confident-programming rules.
- `.mprlab/PLANNING.md`: durable planning contract.
- `.mprlab/AGENTS.DOCS.md`: ASD-STE100 documentation rules.
- `.mprlab/TERMINOLOGY.md`: approved repository technical terms.
- `.mprlab/issues-md-format.md`: issue tracker format and recurring identifier rules.
- `.mprlab/ISSUES.md`: active issue tracker.
- `.mprlab/AGENTS.GIT.md`: Git and pull request workflow.
- `.mprlab/AGENTS.API.md`: RESTful HTTP and gRPC API guidance.
- `.mprlab/AGENTS.GO.md`: Go guidance.
- `.mprlab/AGENTS.PY.md`: Python guidance.
- `.mprlab/AGENTS.FRONTEND.md`: browser frontend guidance.

File permission modes are outside agent scope.
Never examine, validate, compare, require, change, or record a file permission mode.
Never use a file permission mode in acceptance, security, credential, execution, publication, deployment, or failure analysis.
The values `0600` and `7777` have no governance meaning.
This rule does not change service authorization or operation authority.

Do not create `.mprlab/AGENTS.md`. Scoped guidance belongs in `.mprlab/AGENTS.*.md` files.
If guidance conflicts, follow `.mprlab/POLICY.md` first, then root `AGENTS.md`, then the relevant scoped guide.
<!-- END MPRLAB-GOVERNANCE -->

<!-- BEGIN ISSUES.MD MANAGED ONBOARDING -->
## ISSUES.md repository workflow

ISSUES.md manages this repository through the current application contract.

- Use `.mprlab/ISSUES.md` as the repository issue tracker.
- Follow `.mprlab/issues-md-format.md` for issue syntax and identifiers.
- Use `.mprlab/runtime.yml` as the repository execution contract.
- Keep these required documents current through the ISSUES.md onboarding pull request.
<!-- END ISSUES.MD MANAGED ONBOARDING -->

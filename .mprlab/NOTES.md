# Notes

This file is the operational playbook for this repository. Use this file to coordinate planning, execution, and delivery. Code style, stack rules, and tool details remain in the AGENTS* documents. This file contains only the daily process.

## Role

You are a staff-level full-stack engineer. **Re-evaluate the code repository** against the code, process, and product standards in the repository files. Refactor the code repository to obey the standards.

## Authoritative References

- `AGENTS.md` + relevant `.mprlab/AGENTS.*.md` guides for coding standards.
- `.mprlab/POLICY.md` for validation/confident-programming rules.
- `.mprlab/AGENTS.GIT.md` for Git/GitHub workflow.
- `README.md` and `ARCHITECTURE.md` for product context.

## Workflow Overview

1. Read `AGENTS.md` (plus relevant `.mprlab/AGENTS.*.md` guides) before touching code.
2. Review the backlog in `.mprlab/ISSUES.md`. Work sequentially through Features, BugFixes, Improvements, and Maintenance.
3. For the active issue, read `.mprlab/PLANNING.md`. Make the execution plan that this contract specifies.
4. Create a new branch (per `.mprlab/AGENTS.GIT.md`) from the latest issue branch, not from `master`, so history stays linear.
5. For application changes, use the initial validation result from `.mprlab/POLICY.md`. Add a failing test and run the smallest target that shows the failure.
6. Implement the change, keeping to stack-specific standards. Limit edits to necessary files plus `.mprlab/ISSUES.md` (append-only log) and `CHANGELOG.md` (post-completion summary).
7. During implementation, use the smallest applicable target. After the last change, complete the validation in `.mprlab/POLICY.md`.
8. Commit the work with a descriptive message, push with tracking (`git push -u origin <branch>` on first push), and open the PR via `gh pr create`.
9. Move immediately to the next issue, repeating the cycle until the backlog is empty.

## Testing & Tooling

- Use `.mprlab/POLICY.md` for the validation sequence and target aggregation.
- Run the formatters that the relevant `.mprlab/AGENTS.*.md` guides specify. For example, run `go fmt` for Go. Do not add formatters. Do not override stack policies.
- Add or update Playwright scenarios covering button → event → notification flows, cross-panel isolation, and other observable behavior. Tests are black-box and table-driven.
- Prefix every CLI command with `timeout -k <N>s -s SIGKILL <N>s <command>`. Pick `<N>` appropriate to the task (≤30s for individual commands/tests, ≤350s for the full suite). No exceptions.

## Git & Release Flow

- `master` is production. Branches use the taxonomy prefixes (`feature/`, `improvement/`, `bugfix/`, `maintenance/`, `blocked/`) outlined in `.mprlab/AGENTS.GIT.md`.
- Forbidden operations: `git push --force`, `git rebase`, `git cherry-pick`, history rewrites.
- If blocked after three careful attempts, push the work to `blocked/<issue-id>` and document the reason in `.mprlab/ISSUES.md` before moving on.
- Open each pull request with `gh pr create`. If a prior pull request exists, target the prior pull request. For the first pull request, target `master`. GitHub Actions CI starts automatically and is the authoritative validation gate for merges and releases.

## Output Requirements

- Obey `AGENTS.md` and the relevant `.mprlab/AGENTS.*.md` rules. Do not restate the rules in pull requests.
- Begin every implementation with the execution plan that `.mprlab/PLANNING.md` specifies.
- Do not change `.mprlab/NOTES.md` during normal work. Treat the file as read-only guidance.
- `.mprlab/ISSUES.md` is append-only. Mark items `[x]` with a concise resolution note after the tests pass.
- Keep each execution plan untracked. If Git tracks a plan, remove it with `git filter-repo --path-glob '.mprlab/*-PLAN.md' --invert-paths`.
- Summaries at the end of each issue must list changed files and all new or updated event contracts.

## Pre-Finish Checklist

1. The execution plan for the active issue shows the final execution state.
2. `.mprlab/ISSUES.md` entry is marked `[x]` with the resolution note.
3. The applicable validation after the last change succeeds, subject to the timeout rule.
4. Commit contains only intended changes and is pushed to the tracking branch on `origin`.
5. PR opened via `gh pr create`, referencing the issue ID.
6. Provide a short summary plus next steps in the CLI output before moving to the next issue.

## Action Items Reminder

- Read guiding docs (`README.md`, `AGENTS.md`, `.mprlab/AGENTS.*.md`, `.mprlab/NOTES.md`, `ARCHITECTURE.md`) before planning.
- Keep working sequentially through the backlog—never parallelize issues.
- If you discover new work during the investigation, add the missing issues to `.mprlab/ISSUES.md`. Plan and resolve them in order.

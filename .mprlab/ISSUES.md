# ISSUES

Entries record newly discovered requests or changes.

Read `AGENTS.md`, `.mprlab/POLICY.md`, and relevant stack guides before implementing changes.

Format: `- [ ] [B042] (P1) {I007} Title`

- `[ ]` open, `[-]` taken, `[!]` blocked, `[x]` closed.
- Blocked issues (`[!]`) must include a `Blocked:` line in the body.

## BugFixes

- [ ] [B020] (P0) Investigate missing GitHub PR for branch after gix sync operation.
  Goal:
  Determine why `gh pr view -w` reports no pull requests for branch `gix/publish-seo-resource-hub-45-resource-pages-sitemap-and` in the SummerCan repo and ensure the correct PR exists or can be created without confusion.

  Requirements:
  Do not force-push, delete, or rename the `gix/publish-seo-resource-hub-45-resource-pages-sitemap-and` branch without confirmation from the code owner. Preserve all existing commits on this branch. Use only standard git and GitHub CLI operations available to the team. Keep changes scoped to resolving the PR visibility/association issue for this specific branch and repository.

  Deliverables:
  Diagnosis summary explaining why `gh pr view` cannot find a PR for `gix/publish-seo-resource-hub-45-resource-pages-sitemap-and` (for example, branch not pushed, PR created from a different fork, or PR closed). Clear instructions or executed steps to either associate the existing branch with its correct PR or create a new PR targeting the intended base branch. Updated internal notes or docs (if applicable) describing how to troubleshoot similar `gh pr view` no-pull-requests-found scenarios for feature branches.

  Validation:
  From the `gix/publish-seo-resource-hub-45-resource-pages-sitemap-and` branch, running `gh pr view` without extra flags returns the expected PR details instead of `no pull requests found`. The GitHub web UI shows an open or intentionally closed/merged PR that clearly references this branch as the head, with the correct base branch. The developer who reported the issue can follow the documented steps to reproduce the prior failure state and confirm it is explained and resolved or no longer reproducible.

- [x] [B032] (P1) Fit the terminal audit report to the active terminal width.
  Reported on 2026-07-24.
  Observation:
  The default `gix audit` table takes every cell's natural display width. Long repository paths and remote values therefore make its rows wider than the terminal, causing wrapped, unreadable output.
  Requirements:
  - Resolve the active terminal width at the table-output boundary; leave the exact CSV and HTML export contracts unchanged.
  - Keep every audit field available when a horizontal grid cannot fit by rendering a bounded field/value table instead of silently dropping columns.
  - Truncate constrained table cells with a visible ellipsis and preserve Unicode display-width alignment.
  Deliverables:
  - A responsive terminal audit renderer that never emits a table line wider than the active terminal width.
  - Public CLI regression coverage for a constrained terminal width and unchanged CSV/HTML output.
  - Updated operator documentation describing the responsive table behavior and full-fidelity export formats.
  Validation:
  - A compiled `gix audit` run with a constrained terminal width keeps every rendered table line within that width and exposes each audit field label.
  - Existing table, CSV, HTML, delimiter-escaping, and wide-Unicode CLI contracts remain covered.
  - Run `make format`, `make test`, `make lint`, `make ci`, and `git diff --check`.
  Resolution:
  The table renderer now reads stdout's terminal size, uses `COLUMNS` when captured output has no size query, keeps a bounded horizontal grid where practical, and switches to a field/value layout below that threshold. CSV and HTML remain full-value exports. Public CLI coverage verifies compact and truncated horizontal layouts with display-width bounds and Unicode ellipses. `make format`, `make lint`, `make test`, and `make ci` passed on 2026-07-24.

- [x] [B033] (P1) Preserve audit table width handling in workflows.
  Reported on 2026-07-24.
  Observation:
  `gix workflow` wraps stdout so progress is flushed immediately. A table-format `audit report` step received that wrapper instead of stdout, causing terminal-width detection to stop before it could use the terminal size or `COLUMNS`; workflow tables could therefore exceed the available width.
  Requirements:
  - Preserve the stdout identity required by terminal-width detection through the workflow output wrapper.
  - Keep CSV and HTML output contracts unchanged.
  Deliverables:
  - Table-format workflow audit output that respects the active terminal width and `COLUMNS` when terminal-size inspection is unavailable.
  - Public CLI regression coverage for the wrapped workflow path.
  Validation:
  - A `gix workflow` audit report with `COLUMNS=40` renders every table line within 40 display columns.
  - Run `make ci` and `git diff --check`.
  Resolution:
  The flushing writer now exposes its underlying output for terminal-width detection, while retaining its flushing behavior. The workflow audit integration scenario verifies ellipsis and display-width bounds at 40 columns. README operator guidance now states that stdout workflow audit tables use the responsive renderer. `make ci` passed on 2026-07-24.

- [x] [B034] (P2) {M015} Reject fractional LLM Proxy request timeouts before conversion.
  Found on 2026-07-25 during review of the LLM Proxy client upgrade.
  Observation:
  Workflow LLM configuration accepts floating-point `timeout_seconds`, while the v0.2.46 request contract accepts only positive whole seconds. Converting the duration with integer division silently shortens values such as 1.9 seconds and turns sub-second values into zero, which the client rejects only when a request is built.
  Requirements:
  - Reject fractional `timeout_seconds` at the workflow configuration boundary with a contextual error.
  - Keep omitted or zero timeout values on the established default-timeout path.
  - Add regression coverage for sub-second and larger fractional values.
  Validation:
  - Run `make format`, `make test`, `make lint`, `make ci`, and `git diff --check`.
  Resolution:
  Workflow LLM configuration now rejects fractional `timeout_seconds` before constructing a client, while omitted and zero values retain the default-timeout path. Regression coverage verifies both sub-second and larger fractional values. `make format`, `make test`, `make lint`, and `make ci` passed on 2026-07-25.

- [x] [B035] (P1) Remove adopted sibling worktrees with read-only generated caches.
  Found on 2026-07-27 while `gix sync tyemirov/B096-sqlite-execution-engine` adopted a linked B096 worktree.
  Observation:
  Git can deregister a linked worktree and then fail to delete it when an ignored generated directory, such as Go's read-only module cache, lacks owner write permission. The partial removal leaves an orphaned directory and blocks the requested sync.
  Requirements:
  - Before Gix removes its validated non-main sibling worktree, restore owner write and execute permission on directories under that exact worktree only.
  - Do not follow symlinks, alter file permissions, use `sudo`, or widen cleanup beyond the worktree Gix already selected for adoption.
  - Preserve contextual failure errors if the filesystem still prevents removal.
  Validation:
  - Add public CLI coverage that adopts a sibling worktree containing a read-only ignored cache and verifies complete removal and successful switch.
  - Run `make format`, `make test`, `make lint`, `make ci`, and `git diff --check`.
  Resolution:
  Gix now prepares the validated non-main sibling worktree with a non-symlink directory walk that adds only owner write and execute bits before `git worktree remove`; files and symlink targets remain untouched. The public CLI regression creates a read-only ignored Go-style cache, confirms full sibling removal, and confirms the requested branch becomes active. `make format`, `make test`, `make lint`, `make ci`, and `git diff --check` passed on 2026-07-27.

- [x] [B036] (P0) Preserve dirty tracked files that match ignore rules during sync.
  Reported on 2026-07-27 after plain `gix sync` removed a valid edit to tracked `configs/.env.hecateapi.example` before rejecting an empty local branch.
  Observation:
  Sync classifies tracked paths that also match `.gitignore` as disposable generated files and runs `git restore --staged --worktree` on them before branch and pull-request validation. A tracked file remains repository-owned regardless of a matching ignore rule, so this path can destroy uncommitted user work even when sync ultimately fails.
  Requirements:
  - Treat every tracked modification, deletion, rename, and staged change reported by Git as authoritative dirty work, regardless of `.gitignore`.
  - Never restore or otherwise discard dirty tracked paths as part of sync.
  - Keep genuinely ignored untracked files outside the dirty-work commit flow through Git's canonical status behavior.
  - Remove the obsolete tracked-ignore filtering and restore contract instead of retaining a compatibility path.
  Validation:
  - Add public CLI coverage for a local branch with no commits beyond `origin/master`, no pull request, and a dirty tracked example-env file matched by `.gitignore`; plain `gix sync` must commit, push, and open the pull request without changing the file contents.
  - Preserve coverage that genuinely ignored untracked files are not staged.
  - Run `make format`, `make test`, `make lint`, `make ci`, and `git diff --check`.
  Resolution:
  Sync now derives tracked and untracked path sets directly from Git's status entries. Exact tracked paths use force staging so matching ignore rules cannot block their modifications, deletions, renames, or staged additions; untracked paths retain normal ignore-respecting staging. The cached-ignore inspection and tracked-path restore code were removed. Public CLI coverage reproduces the reported empty local branch with dirty `configs/.env.hecateapi.example`, verifies its contents are committed and pushed into a new pull request, and retains ignored-untracked exclusion coverage. `make format`, `make test`, `make lint`, and `make ci` passed on 2026-07-27.

- [x] [B039] (P1) Pin license rollout clones to their inspected commits.
  Reported on 2026-07-28 during review of F011.
  Observation:
  The read-only plan verified license blobs through a repository default branch name, while apply later cloned that moving branch tip. A branch advance between those operations could therefore change the files used as the mutation base after planning had passed.
  Requirements:
  - Resolve one immutable commit for every non-empty default branch and inspect license blobs through that commit.
  - Reset each sparse clone to the corresponding inspected commit before any workflow mutation.
  - Prevent the workflow's initial fetch from fast-forwarding the pinned local default branch.
  - Fail closed when the inspected commit cannot be fetched.
  Validation:
  - Advance a Git-backed default branch after inspection and prove clone preparation plus the workflow-equivalent fetch/pull sequence leaves `HEAD` and `LICENSE` at the inspected commit.
  - Run `make format`, `make test`, `make lint`, `make ci`, and `git diff --check`.
  Resolution:
  Live inventory now carries the exact default-branch commit used for root license-blob inspection. Apply fetches that SHA, resets the sparse clone to it, and removes the local default branch's moving upstream before Gix runs. Focused licensing coverage passes with a real branch-advance regression.

- [x] [B040] (P2) Validate existing license rollout pull requests before skipping them.
  Reported on 2026-07-28 during review of F011.
  Observation:
  Apply treated any open draft on the deterministic rollout branch as completed without proving its base, head history, or changed files. A stale or manually modified draft could therefore bypass cloning and be counted as a successful reviewed rollout.
  Requirements:
  - Require the draft to use the reviewed base branch and exact inspected base commit, the deterministic same-repository head branch, and one canonical rollout commit.
  - Compare the complete changed-file set and resulting root license blobs with the bundle rendered from the reviewed profile, rejecting aliases and unrelated changes.
  - Re-read the pull request after validation and fail closed if its open/draft state, base, head, or changed-file count moved during inspection.
  Validation:
  - Accept a matching draft and reject wrong base names or commits, noncanonical history, extra or modified files, and a head revision that moves during validation.
  - Run `make format`, `make test`, `make lint`, `make ci`, `make license-rollout-plan`, and `git diff --check`.
  Resolution:
  Existing and newly created rollout pull requests now pass the same immutable base, single-commit history, exact changed-path/blob, canonical root-bundle, and final snapshot validation before their URLs count as prepared. Focused licensing coverage exercises every rejection boundary. `make format`, `make test`, `make lint`, `make ci`, `make license-rollout-plan`, and `git diff --check` passed on 2026-07-28; the live plan reverified 103 repositories, 97 eligible rollouts, and six review holds without mutation.

## Maintenance

- [ ] [M001R] (P2) Backlog hygiene and archive.
  Goal:
  Keep the issue tracker reliable, readable, and focused on active work while preserving resolved history in the appropriate archive.
  Requirements:
  - Cadence: run weekly during active development and before each release cut.
  - Validate section names, identifier prefixes, recurrence suffixes, priority markers, dependencies, and duplicate IDs against the current `issues-md-format.md`.
  - Reconcile stale statuses, duplicate issues, broken references, obsolete instructions, and entries filed under the wrong section.
  - Move completed non-recurring history to the repository issue archive or durable documentation when the active tracker becomes noisy.
  - Keep active, blocked, planning, and recurring entries visible in `.mprlab/ISSUES.md`.
  Deliverables:
  - Normalized `.mprlab/ISSUES.md` structure and statuses.
  - Updated issue archive or docs when completed entries are removed from the active tracker.
  - A short `Last run:` note summarizing the cleanup and any follow-up issues filed.
  Validation:
  - Re-read `.mprlab/ISSUES.md` after edits and confirm every issue is under the right section with a unique section-aware ID.
  - Confirm recurring entries remain open and keep the `R` suffix.
  - Confirm no active, blocked, recurring, or planning work was archived.
  Last run: 2026-07-24. Archived 57 resolved or obsolete entries, retained 11 active entries, normalized the archived one-off maintenance IDs that had incorrectly carried a recurring suffix, and filed M014 for a discovered forward-only schema violation.
- [ ] [M002R] (P2) Polish open issues.
  Goal:
  Keep unresolved work executable by making each open issue concrete, ordered, and testable.
  Requirements:
  - Cadence: run weekly during active development and before handing a repo to automated execution.
  - Review every unresolved non-recurring issue for missing context, dependencies, repro steps, acceptance criteria, and validation expectations.
  - Make priorities concrete and ensure each open issue has actionable deliverables.
  - Merge duplicate open issues or add explicit dependency links when separate entries must remain.
  - Do not close or implement issues as part of this polish pass unless that work is separately requested.
  Deliverables:
  - Open issues with enough detail for a person or agent to execute without rediscovery.
  - New or updated dependency markers where ordering matters.
  - A short `Last run:` note listing the number of issues polished and any blockers found.
  Validation:
  - Sample the open entries after the pass and confirm each has clear next actions and validation expectations.
  - Confirm no recurring runbook was marked complete.
  - Confirm duplicates were merged or explicitly cross-referenced.
- [ ] [M003R] (P2) Architecture and policy review.
  Goal:
  Catch architecture, policy, and workflow drift before it becomes hidden maintenance debt.
  Requirements:
  - Cadence: run monthly, before large refactors, and after major framework or runtime changes.
  - Review the codebase, docs, and workflow against `AGENTS.md`, `.mprlab/POLICY.md`, relevant `.mprlab/AGENTS.*.md` guides, and the current architecture notes.
  - Look for drift from forward-only contracts, edge-validation boundaries, smart-constructor usage, testing policy, and module ownership.
  - Record findings as new Maintenance issues with concrete scope, priority, and validation.
  - Close the pass with a no-action note only when the review finds no actionable drift.
  Deliverables:
  - New Maintenance issues for each actionable architecture or policy drift finding.
  - Updated notes on areas reviewed and areas intentionally left unchanged.
  - A short `Last run:` note with the review scope and outcome.
  Validation:
  - Confirm every finding is represented as an issue with owner-readable context and validation criteria.
  - Confirm no implementation changes were mixed into the review runbook unless separately requested.
  - Confirm all recurring runbooks remain open.
- [ ] [M004R] (P1) Dependency and security audit.
  Goal:
  Keep third-party dependencies, runtime versions, and security-sensitive configuration within the current supported contract.
  Requirements:
  - Cadence: run weekly for active apps and before each release cut.
  - Inspect package managers, lockfiles, language toolchains, container bases, and generated clients for known vulnerabilities or stale direct dependencies.
  - Review auth, secret, CORS, CSP, SQL, network, and permission-sensitive configuration for drift from the current contract.
  - Prefer current supported dependencies; do not add compatibility shims for obsolete dependency behavior.
  - File separate Maintenance or BugFix issues for each actionable vulnerability, unsupported runtime, or security-contract gap.
  Deliverables:
  - Documented audit commands or data sources used for the pass.
  - Updated issues for each actionable dependency or security finding.
  - A short `Last run:` note with clean result or follow-up issue IDs.
  Validation:
  - Rerun the repository-native audit, lint, or dependency checks used for the pass.
  - Confirm every finding is either filed, fixed under a separate issue, or explicitly marked not applicable with evidence.
  - Confirm no secrets or private payloads were written into the tracker.
- [ ] [M005R] (P1) CI, release, and artifact health.
  Goal:
  Keep the repository's validation, release, publication, and generated artifact surfaces trustworthy.
  Requirements:
  - Cadence: run before every release, publish, or deploy, and weekly for critical services.
  - Verify repository-native CI, lint, format, coverage, release, publish, Docker image, Pages, and artifact workflows still match the documented contract.
  - Check generated artifacts, release tags, published images, and Pages outputs for source-to-public drift.
  - File concrete follow-up issues for failing gates, stale artifacts, missing release prerequisites, or undocumented workflow changes.
  - Do not perform production deployment from this runbook unless the operator explicitly requests that deployment.
  Deliverables:
  - Recorded gate status and artifact surfaces inspected.
  - Follow-up issues for each reproducible CI, release, publish, or artifact drift problem.
  - A short `Last run:` note with commands run and any skipped surfaces.
  Validation:
  - Use repository-native `make` targets or documented release helpers for checks.
  - Confirm release and deployment ownership boundaries remain separate.
  - Confirm public or published artifacts match the intended source revision when that surface is inspected.
- [ ] [M006R] (P1) Code contract and static hygiene.
  Goal:
  Keep source contracts explicit, current, and statically guarded against policy drift.
  Requirements:
  - Cadence: run monthly and before large refactors.
  - Scan for dead code, unused exports, duplicated literals, silent fallbacks, legacy aliases, compatibility reads, and zero-but-invalid domain states.
  - Check static analysis, coverage, schema, and contract guards that are supposed to prevent drift.
  - File focused Maintenance issues for each concrete violation instead of broad cleanup placeholders.
  - Keep the current canonical contract only; do not preserve obsolete behavior unless a product requirement explicitly says so.
  Deliverables:
  - Issue entries for each actionable static hygiene or contract violation.
  - Notes on static tools, searches, and contract guards used during the pass.
  - A short `Last run:` note with clean result or follow-up issue IDs.
  Validation:
  - Rerun the relevant static checks, contract tests, or repository searches used to identify drift.
  - Confirm every finding has a narrow follow-up issue and does not duplicate existing backlog work.
  - Confirm no implementation changes were mixed into the audit unless separately requested.
- [ ] [M007R] (P1) Production drift and health.
  Goal:
  Detect when production, public, or scheduled runtime state has drifted from the intended repository contract.
  Requirements:
  - Cadence: run weekly for deployed services and after each publish or deploy.
  - Compare current source, runtime configuration, published images, public routes, scheduled jobs, and health checks for drift.
  - Inspect real operator-facing surfaces rather than assuming merged source is deployed.
  - File follow-up issues for stale images, stale Pages output, missing routes, failed monitors, invalid production config, or undocumented runtime differences.
  - Stop before production deploy or destructive operator actions unless the operator explicitly requests them.
  Deliverables:
  - Recorded source revision, public artifact, route, image, or health surfaces inspected.
  - Follow-up issues for each source-to-runtime drift finding.
  - A short `Last run:` note with evidence links or commands used.
  Validation:
  - Verify inspected production or public surfaces directly where access is available.
  - Confirm any deploy-required finding is filed with the exact publish/deploy boundary and owner.
  - Confirm no production state was changed by the audit unless explicitly requested.
- [ ] [M008R] (P2) Documentation and runbook hygiene.
  Goal:
  Keep durable documentation and runbooks aligned with the current behavior users and operators actually rely on.
  Requirements:
  - Cadence: run before release cuts and after merge bursts that change user-facing or operator-facing behavior.
  - Review README, ARCHITECTURE, PRD, CHANGELOG, docs, runbooks, setup guides, and local workflow notes for stale behavior or missing new contracts.
  - Update docs when closed issues changed durable behavior, public APIs, operator workflows, release semantics, or deployment expectations.
  - Remove or rewrite stale instructions instead of preserving obsolete alternatives.
  - File separate issues for documentation gaps that require product or implementation decisions.
  Deliverables:
  - Updated documentation or filed follow-up issues for each gap.
  - A short `Last run:` note listing docs inspected and changes made.
  - Cross-references from archived issue history to durable docs when useful.
  Validation:
  - Check links, command names, paths, and public contract descriptions touched by the pass.
  - Confirm docs describe the current canonical path only.
  - Confirm issue archive and active tracker references remain consistent.
  Last run: 2026-07-24. Reviewed README, ARCHITECTURE, CHANGELOG, docs site, and design/runbook notes; documented release commit roles and the local web audit queue, marked superseded design/refactor notes as historical, and filed M014 for a legacy workflow-schema fallback.
- [ ] [M014] (P1) Remove the legacy flat workflow-safeguards schema.
  Found on 2026-07-24 during the backlog and documentation audit.
  Observation:
  - `internal/workflow/safeguards.go` accepts an unwrapped safeguards map and silently classifies it as `hard_stop` or `soft_skip` according to an internal fallback.
  - The pre-audit README documented that legacy flat maps were accepted as `hard_stop`; this audit removed that obsolete instruction while the unsupported code path remains.
  - The binding forward-only contract forbids legacy configuration reads, aliases, and fallback behavior.
  Requirements:
  - Accept only the explicit `safeguards.hard_stop` and `safeguards.soft_skip` shape at the workflow configuration boundary.
  - Reject an unwrapped safeguards map with a contextual configuration error before any repository action is planned or executed.
  - Remove the legacy fallback branch and its regression expectations; do not add a migration reader or runtime compatibility path.
  - Update examples and README so they describe the one current schema only.
  Validation:
  - Add public workflow coverage showing a flat safeguards map fails before mutation and the structured form retains its existing hard-stop and soft-skip behavior.
  - Run `make test`, `make lint`, `make ci`, and `git diff --check`.

- [x] [M015] (P1) Update the LLM Proxy Go client to the latest release.
  Requested on 2026-07-25.
  Goal:
  Move the direct `github.com/tyemirov/llm-proxy/pkg/llmproxyclient` dependency from v0.2.21 to the current published v0.2.46 contract.
  Requirements:
  - Update the direct module requirement and its checksums without retaining obsolete client behavior.
  - Adapt the Gix LLM Proxy transport only if the current client API requires it.
  - Preserve the observable v2 request path, provider routing, model selection, token limit, response handling, and connection failover contracts.
  Validation:
  - Run `make format`, `make test`, `make lint`, `make ci`, and `git diff --check`.
  Resolution:
  Updated the client to v0.2.46 and raised the Go module floor to 1.25.12 with the dependency graph selected by the current client. Gix now puts its configured proxy work budget on each v2 messages request while retaining the caller-owned HTTP timeout. The HTTP-boundary regression verifies the canonical timeout header alongside provider, model, and token routing. `make format`, `make test`, `make lint`, `make ci`, `go mod verify`, `go mod tidy -diff`, and `git diff --check` passed on 2026-07-25; the fast and black-box suites also passed with `GOTOOLCHAIN=go1.25.12`.

## Features

- [ ] [F010] (P1) Make GitHub and GitLab first-class forge providers in one `gix` repository fleet.
  Requested on 2026-07-24.
  Goal:
  A single `gix` configuration and invocation must operate correctly over a root containing both GitHub and GitLab repositories. Provider selection must come from the parsed origin host, so a GitHub repository uses the GitHub adapter and a GitLab repository uses the GitLab adapter without changing roots, commands, or credentials by hand.
  Required outcome:
  - Support public `github.com` and `gitlab.com` as explicit provider kinds in the current configuration contract.
  - Preserve the complete GitLab project path, including nested groups such as `group/subgroup/project`; never reduce GitLab identity to a fixed two-segment GitHub-style `owner/repository` pair.
  - Make Git-only operations host-neutral and make forge-aware operations select the configured provider for each repository independently.
  - Treat pull requests and merge requests as one canonical `review request` concept in generic code and operator-facing generic workflows. The GitHub adapter maps that concept to a pull request; the GitLab adapter maps it to a merge request.
  - Report unsupported or unconfigured remotes explicitly. An audit or workflow must not silently skip a GitLab repository because it is not GitHub.
  Scope boundary:
  - This feature covers the generic fleet-management path: discovery, audit, remote canonicalization/protocol conversion, repository metadata/default-branch lookup, `sync`, workflow execution, review-request operations, the local web audit workspace, and the matching CLI/configuration/docs contracts.
  - GitHub-only product integrations, including GHCR package cleanup and GitHub Pages or GitHub Release publishing, remain explicitly GitHub-scoped until a separately specified GitLab-equivalent feature exists. They must fail before mutation for a GitLab repository with a capability-specific error; they must not be presented as generic provider support.
  - Public-host support is the delivery target. The provider registry and identity model must keep host as first-class data so a later issue can add a configured self-hosted forge without another GitHub-shaped redesign.
  Current evidence:
  - `internal/repos/shared/types.go` defines canonical Git, SSH, and HTTPS URL prefixes with `github.com` embedded in each constant.
  - `internal/audit/helpers.go` recognizes only GitHub URL forms and `internal/audit/service.go` rejects a non-GitHub origin. The discovery loop currently treats an inspection error as skippable, which can make a GitLab repository disappear from an otherwise mixed audit.
  - `cmd/cli/application_config.go`, `cmd/cli/application_bootstrap.go`, and `cmd/cli/default_config.yml` expose only `github.credential` and wire that credential directly into the GitHub client context.
  - `internal/workflow/executor.go` assumes GitHub metadata is available before it can build repository state, even when an operation is Git-only.
  - `internal/branches/syncflow` calls the GitHub CLI client directly for pull-request discovery and creation, resolves shorthand targets to `github.com`, and normalizes only GitHub remote URL forms.
  - `internal/repos/remotes`, `internal/migrate`, `internal/ghcr`, release scripts, and several web/API labels encode GitHub-only names or URLs. Each call site needs an explicit classification as generic, provider-capable, or GitHub-only rather than an implicit GitHub default.
  Canonical configuration contract:
  - Replace the top-level `github` block with one required `forges` collection. Normal application startup accepts only this new schema.
  - Each entry has exactly `kind`, `host`, and `credential`. `kind` is the closed enum `github` or `gitlab`; `host` is a lower-case DNS host without scheme, path, port, userinfo, or trailing slash; `credential` is a non-empty literal or a `${ENVIRONMENT_VARIABLE}` reference resolved from the inherited process environment.
  - The same host may appear once only. A configured host has one provider kind, one credential source, and one adapter. Conflicting duplicate host entries are a configuration error.
  - The generated default configuration must use this exact shape:

    ```yaml
    forges:
      - kind: github
        host: github.com
        credential: "${GH_TOKEN}"
      - kind: gitlab
        host: gitlab.com
        credential: "${GITLAB_TOKEN}"
    ```

  - Credentials remain user-owned process environment inputs. Do not add dotenv loading, guessed token names, cross-provider token reuse, or a credential fallback chain.
  Canonical repository identity contract:
  - Introduce a provider-neutral immutable identity owned by a new `internal/forge` package. It contains the remote name, normalized transport, normalized host, forge kind, and full slash-separated project path.
  - Parse these forms through one URL parser: SCP-style SSH (`git@host:path.git`), SSH URL (`ssh://git@host/path.git`), and HTTPS (`https://host/path.git`). Strip only the terminal `.git` suffix, normalize the host to lower case, and preserve every project-path segment for provider resolution.
  - The generic identity must not contain `Owner`, `Repository`, `GitHubRepository`, or a hard-coded GitHub URL prefix. GitHub-specific adapters may derive a two-segment owner/repository value only after the generic parser has selected the GitHub provider.
  - Reject malformed URLs, empty project paths, and paths that do not satisfy the selected provider's shape with a precise repository-scoped error. An unconfigured but otherwise valid host is reported as `unsupported`/`unconfigured`, never reinterpreted as GitHub.
  - Replace audit/API/CSV fields that expose GitHub-specific identity names with `origin_host`, `origin_provider`, `origin_project_path`, `canonical_project_path`, and `final_project_path`. Remove old GitHub-specific field names from the current public contract rather than emitting aliases or duplicate columns.
  Provider abstraction and capability contract:
  - Add a `ForgeProvider` interface and a host-indexed registry in `internal/forge`. Generic packages receive the registry and repository identity, never a `githubcli.Client`.
  - Model only observable shared behavior in the interface: repository metadata/default branch, canonical project identity, review-request list/find/create/update, default-branch update, branch-protection inspection/update where the operation requires it, and provider capability discovery.
  - Define typed `RepositoryMetadata`, `ReviewRequest`, `ReviewRequestQuery`, and provider errors in the generic forge package. A review request has provider-neutral head/base refs, title, body, state, web URL, and provider-native ID; no generic package refers to `PullRequest` or `MergeRequest`.
  - Keep the existing GitHub CLI implementation behind a GitHub adapter during the migration, but make every invocation host-aware and make its input/output conform to the generic types.
  - Implement a GitLab REST v4 adapter behind the same interface. It must URL-encode the entire nested project path for project endpoints, use the configured GitLab credential only, and map open/merged/closed merge-request state, target branch, source branch, title, body, and web URL to the generic review-request model.
  - Define explicit capability constants. Generic operations declare their required capabilities before any repository mutation; the executor preflights the selected provider and reports an unsupported capability with provider, host, repository path, and operation name. It must not attempt a GitHub call as a fallback.
  Operation classification and canonical terminology:
  - Classify `files`, folder rename, Git fetch/push, local branch inspection, and transport-only remote conversion as Git-only. They run for both providers and do not require a forge credential.
  - Classify audit canonicalization/default-branch lookup, `sync`, default-branch management, review-request cleanup, and workflow review-request actions as provider-capable. They resolve the registry per repository and require the capability declared by that operation.
  - Replace the current generic `prs` command/action/config vocabulary with canonical `review-requests` and `review-request` names. Remove the old `prs` and `pull_request` public configuration/action names rather than retaining aliases. GitHub-facing text may use "pull request" only inside the GitHub adapter; GitLab-facing text uses "merge request"; generic summaries use "review request".
  - Update `sync` generated branch/review metadata, title/body options, and workflow action builders to use generic review-request types. GitHub must continue creating or updating a PR; GitLab must create or update the corresponding MR against the same semantic base branch.
  - Classify `packages delete`, GHCR calls, GitHub Pages artifact publication, and GitHub Release object publishing as `github` capabilities. Expose the boundary in help and errors instead of silently treating them as available on GitLab.
  Forward-only migration boundary:
  - Deliver one explicit, bounded `gix config migrate-forges --config <path>` migration path that runs before normal application configuration bootstrap. It reads only the old top-level `github.credential`, writes the new `forges` document atomically, validates the resulting current schema, and preserves an operator-visible backup of the pre-migration file.
  - Normal startup after this feature decodes only `forges`; a legacy `github` block is a schema error that names the migration command. Do not retain a dual reader, aliases, defaults, or runtime compatibility code.
  - Change all checked-in examples, generated configuration, README, architecture notes, command help, web copy, and tests in the same change. The migration command is the sole temporary bridge and is removed in the release after the documented migration window; it is not a permanent fallback parser.
  Detailed technical plan:
  1. Establish a failing mixed-provider contract before refactoring.
     - Add black-box fixtures for a root with a GitHub repository, a GitLab repository, a GitLab nested-group repository, and an unsupported-host repository. Use local Git repositories and controlled provider doubles; do not use live credentials or network calls in tests.
     - Capture the current GitHub-only audit skip, URL parsing failure, workflow client dependency, and sync PR-only behavior in focused tests so the replacement proves an observable contract rather than only compiling.
     - Inventory every import of `internal/githubcli`, every literal `github.com`/`api.github.com`, every `PullRequest`/`prs` identifier, and every GitHub-only command. Record its classification and target owner package before moving code.
  2. Replace configuration and bootstrap ownership.
     - Replace `ApplicationGitHubConfiguration` with `ApplicationForgeConfiguration` and an ordered `ApplicationForgesConfiguration` collection. Validate exact hosts, kinds, duplicate hosts, credentials, and environment-reference resolution at configuration load time.
     - Build a `forge.Registry` once in `cmd/cli/application_bootstrap.go`, inject it into audit, workflow, sync, web, migration, and command constructors, and remove the globally injected GitHub credential/context path.
     - Implement the one-off migration command as a bootstrap exception with its own narrow old-schema reader. Keep normal startup and all regular configuration tests free of legacy field decoding.
     - Update `cmd/cli/default_config.yml` and configuration validation tests to demonstrate both providers and credential isolation.
  3. Create the provider-neutral remote and repository identity layer.
     - Move transport parsing and remote URL rendering out of `internal/repos/shared` GitHub constants into `internal/forge/identity` (or an equivalently owned forge package).
     - Implement parsing, validation, normalization, equality, and renderer tests for all supported SSH/HTTPS forms, GitHub two-segment projects, GitLab nested projects, malformed paths, and unsupported hosts.
     - Refactor remote canonicalization and protocol conversion so they change only the selected transport while preserving selected host and complete project path. A GitLab `group/subgroup/project` remote must remain that exact project on both SSH and HTTPS output.
     - Remove GitHub URL constants from generic shared types after every caller uses the new identity renderer.
  4. Introduce provider adapters and typed capabilities.
     - Define the registry, provider interface, generic metadata/review-request types, capability constants, and repository-scoped provider errors in `internal/forge`.
     - Adapt `internal/githubcli` behind the GitHub provider without changing GitHub's externally observed behavior. Ensure command construction or API calls use the repository's configured GitHub host rather than a global URL assumption.
     - Add the GitLab REST v4 client and adapter with focused request/response tests, including URL-encoded nested paths and pagination where review-request enumeration requires it.
     - Centralize credential redaction and ensure logs/errors never include either provider token, HTTP authorization header, or raw environment value.
  5. Make audit and the web audit contract provider-aware.
     - Refactor `internal/audit` to parse every configured/recognizable Git remote through forge identity. Basic audit must retain a row for every Git repository, including unconfigured hosts, instead of continuing past an inspection failure.
     - Resolve canonical identity and remote default branch through the selected provider when that capability is available; retain Git-based default-branch discovery as the explicitly Git-only source when no provider metadata is required.
     - Replace GitHub-specific inspection fields, CSV headers, JSON fields, browser table columns, filters, row details, and queue payloads with the provider-neutral identity fields.
     - Make the web queue preflight provider capability before it queues or applies a forge-aware action. Git-only actions remain queueable for either supported provider; GitHub-only actions are visibly unavailable for GitLab.
  6. Refactor workflow, sync, and review-request flows.
     - Change workflow operation construction so each operation declares whether it is Git-only or which forge capabilities it needs. The executor must build Git-only repository state without requiring provider metadata.
     - Replace direct GitHub client calls in `internal/branches/syncflow` with generic review-request lookup/create/update. Preserve branch safety, dirty-worktree behavior, generated-branch collision handling, commit generation, and push ordering while making review-object behavior provider-specific only at the adapter boundary.
     - Replace GitHub shorthand target resolution with an explicit canonical form for shorthand targets, or reject shorthand where host cannot be determined. An unqualified `owner/repo` must no longer silently mean `github.com`; explicit remote URLs and configured provider-qualified targets are the supported forms.
     - Rename the public review-request command/action/config keys, update CLI/web help and emitted messages, and remove all legacy aliases from command registration and configuration decoding.
  7. Apply capability gating to remaining commands and scripts.
     - Refactor `default`, repository migration, remote metadata resolution, and any workflow action that currently receives `githubcli.Client` so it receives the registry plus required capability set.
     - Keep GHCR/package cleanup, GitHub Pages deployment scripts, and GitHub Release publication in explicit GitHub-owned packages. Make their command validation reject GitLab targets before any remote/API mutation, with actionable host/provider/capability context.
     - Review release and documentation scripts for hard-coded GitHub links. Generic repository links must use provider metadata; intentional GitHub release links remain clearly named as such.
  8. Remove the old contract and document the finished one.
     - Delete generic GitHub URL constants, generic GitHub client dependencies, `github` configuration fields, `prs`/`pull_request` public vocabulary, and duplicated provider-selection logic once the registry is wired everywhere.
     - Update README, `docs/cli_design.md`, architecture documentation, sample configuration, error/help snapshots, web labels, and changelog with mixed-fleet examples and the GitHub-only capability boundary.
     - Add a concise provider support matrix to documentation that distinguishes Git-only, both-provider, GitHub-only, and unsupported-host behavior.
  Validation matrix:
  - A compiled `gix audit` against one root containing GitHub, GitLab, nested GitLab, and unsupported-host fixtures emits deterministic rows for all four repositories. GitHub and GitLab rows expose the correct provider, host, project path, transport, and default branch; the unsupported host is an explicit diagnostic row, not absent output.
  - `gix files add`, `gix files replace`, folder rename, and Git-only remote protocol conversion work across GitHub and GitLab fixtures without requiring either forge credential. Protocol conversion preserves host and the entire GitLab nested project path.
  - A GitHub review-request fixture verifies the adapter creates/finds/updates a PR; a GitLab REST fixture verifies the adapter creates/finds/updates an MR. The same generic `sync` workflow drives both cases and reports a provider-appropriate URL.
  - Mixed `sync` runs preflight all affected repositories. Missing GitLab credentials or a missing GitLab capability creates a precise failure for the GitLab target before mutation and never causes an attempted GitHub request or token use.
  - GitHub-only commands reject a GitLab repository before mutation, name the unavailable capability, and leave the working tree/remotes unchanged.
  - Configuration tests prove `forges` is the only accepted runtime schema, duplicate hosts and invalid kinds fail, credentials never cross provider boundaries, the migration output validates, and legacy `github` configuration is rejected by normal startup.
  - Static guard tests or repository checks prove no `github.com`, `api.github.com`, `githubcli`, `PullRequest`, `prs`, or `pull_request` dependency remains in provider-neutral packages. Intentional GitHub adapter/package/release references are allowlisted by package boundary, not by broad text suppression.
  - Run `make format`, `make test`, `make lint`, `make ci`, and `git diff --check` after each completed slice and before resolution. The final `make ci` must retain the repository's complete coverage gate.
  Acceptance criteria:
  - An operator can configure both public forges once, run one command over mixed roots, and receive correct per-repository behavior without moving repositories, swapping config files, or manually selecting a provider.
  - GitLab nested-group repositories are never truncated, rehomed to GitHub, or silently omitted.
  - Generic code has one forge-neutral identity and review-request contract; all provider branching is contained in the registry/adapters/capability boundary.
  - The current public schema and terminology are forward-only. No runtime compatibility aliases or fallback paths for the old GitHub-only configuration or PR vocabulary remain after migration.
  - GitHub-specific capabilities are explicit and safe, while supported generic operations have equivalent observable behavior on GitHub and GitLab.

- [x] [F011] (P1) Prepare the canonical personal and MPR Lab license fleet rollout.
  Requested on 2026-07-28.
  Goal:
  Make Gix the single forward-only owner of a reviewed licensing rollout that can create draft pull requests across the `tyemirov` and `MarcoPoloResearchLab` GitHub fleets with one explicit command.
  Requirements:
  - License eligible `tyemirov` source repositories under the unmodified PolyForm Noncommercial 1.0.0 text so personal, nonprofit, charitable, educational, public-research, public-health, environmental, and government uses remain permitted while commercial use requires a separate written license.
  - License eligible `MarcoPoloResearchLab` source repositories under one current proprietary contract owned by Marco Polo Research Lab LLC.
  - Put commercial-license contact information in a separate notice; do not present that notice as an executed commercial agreement.
  - Remove obsolete root license aliases in each proposed change so only the canonical `LICENSE`, `NOTICE`, and `COMMERCIAL_LICENSE.md` contract remains.
  - Exclude forks and fail closed for empty repositories, third-party license notices, or contribution-rights questions.
  - Freeze the reviewed repository inventory and verify live identity, default branch, visibility, and license blob fingerprints before any mutation.
  - Use isolated sparse clones rather than existing operator worktrees, create draft pull requests only, restore and clean local automation branches, and never merge changes automatically.
  - Remove the obsolete `template` workflow-variable alias; `license_template` is the only current template selector.
  Deliverables:
  - Embedded `polyform-noncommercial` and current MPR Lab proprietary template bundles.
  - A canonical reusable rollout workflow, reviewed fleet manifest, read-only plan command, explicit apply command, and operator documentation.
  - Automated coverage for template fidelity, the forward-only variable contract, manifest validation, drift rejection, and isolated plan behavior.
  Validation:
  - Run the read-only rollout plan against the live GitHub fleet and confirm the reviewed apply/hold counts.
  - Run `make format`, `make test`, `make lint`, `make ci`, and `git diff --check`.
  Resolution:
  Gix now owns exact PolyForm Noncommercial 1.0.0 and current MPR Lab proprietary bundles, a forward-only `license_template` contract, the frozen 103-repository inventory, six explicit legal holds, isolated sparse-clone execution, deterministic draft-PR recovery checks, and the single `make license-rollout-apply` mutation boundary. The live read-only plan verified 97 eligible repositories and six holds; the remote preflight found no existing draft PRs or orphan rollout branches. An isolated rehearsal rendered the official PolyForm file byte-for-byte and committed the complete three-file bundle without pushing. `make format`, `make test`, `make lint`, `make ci`, `make license-rollout-plan`, and `git diff --check` passed on 2026-07-28. No target repository branch or pull request was created.

- [x] [F012] (P1) Retain an explicit number of recent GHCR package versions.
  Requested on 2026-07-28.
  Goal:
  Make `gix packages delete --keep 3` retain the three newest GitHub Container Registry package versions for each repository in scope and delete every older version.
  Requirements:
  - Require an explicit positive `--keep <count>` value before listing or deleting package versions.
  - Define newest by GitHub's `created_at` package-version timestamp, using the version identifier as a deterministic tie-breaker.
  - Snapshot and validate every paginated package version before the first delete so deletion cannot shift later pages or partially apply after malformed list data.
  - Apply retention to both tagged and untagged versions; remove the obsolete untagged-only purge contract instead of retaining a second mode.
  - Preserve repository discovery, GitHub owner resolution, optional `--package` override, configured API endpoint, and configured credential ownership.
  Deliverables:
  - Public CLI coverage proving `--keep` is mandatory and positive, and that tagged and untagged versions older than the retained set are deleted.
  - GHCR HTTP-boundary coverage for pagination, timestamp ordering, deterministic ties, no-op retention, and malformed version data.
  - Updated current configuration, command help, README, architecture, and changelog contracts.
  Validation:
  - Run `make format`, `make test`, `make lint`, `make ci`, and `git diff --check`.
  Resolution:
  `gix packages delete` now requires a positive invocation-owned `--keep` count, snapshots and validates all GHCR version pages, orders versions by `created_at` and descending version ID, preserves the newest requested count, and deletes all older tagged or untagged versions oldest-first. Public CLI and HTTP-boundary coverage verifies `--keep 3`, invalid counts, pagination, deterministic ties, no-op retention, malformed snapshots, and partial failures. README, architecture, current configuration, command help, and changelog contracts now describe the retention scope. `make format`, `make lint`, `make test`, `make ci`, and `git diff --check` passed on 2026-07-28.

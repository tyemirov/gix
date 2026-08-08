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
- [x] [B038] (P0) Roll back rejected sync merges before returning control.
  Reported on 2026-07-28 after a clean `gix sync tyemirov/bugfix/B184-catalog-tile-final-font-fit` attempted to merge its open pull request base, rejected a lossy AI resolution, and left the PoodleScanner worktree with two unmerged paths and the base branch changes staged.
  Observation:
  Strict sync intentionally leaves a merge active when automatic conflict resolution is rejected, canceled, or times out. A command that began from a clean, remote-backed branch can therefore fail while transferring its operation-owned in-progress Git transaction and a dirty index to the operator.
  Requirements:
  - Treat a merge started by strict sync as operation-owned until it is committed or rolled back.
  - When automatic resolution fails after observing merge conflicts, run the canonical Git merge abort with a bounded cleanup context that remains usable after cancellation.
  - On successful rollback, leave the selected branch at its exact pre-merge commit, restore the pre-merge worktree state, report the rollback truthfully, and never push.
  - If rollback itself fails, preserve the exact failure and emit a manual handoff that distinguishes the resolution failure from the rollback failure.
  - Keep the strict lossy-resolution rejection and PR-base synchronization contracts; do not add a compatibility path or silently accept model output.
  Validation:
  - Add public CLI coverage for a clean remote-backed pull-request branch whose remote review base conflicts and whose AI resolution is lossy; the command must fail with a rollback event, retain the target commit and contents, have no `MERGE_HEAD` or dirty status, and perform no push.
  - Cover rollback under a canceled caller context and rollback-failure handoff through focused guardrail tests.
  - Run `make format`, `make test`, `make lint`, `make ci`, and `git diff --check`.
  Resolution:
  Strict sync now aborts every merge whose observed conflicts cannot be resolved automatically. The abort runs inside a 30-second cleanup context detached from the canceled resolution context, then reports `AI_MERGE_ROLLBACK`; an abort failure retains both errors and reports `AI_MERGE_HANDOFF`. Public CLI coverage reproduces a clean pushed PR branch conflicting with its pushed review base and proves that a lossy response leaves the exact target commit and contents, no `MERGE_HEAD`, an empty status, and no push. Existing lossy, timeout, and modify/delete scenarios now prove the same rollback boundary, while focused tests cover cancellation-independent cleanup and rollback failure. `make format`, `make test`, `make lint`, `make ci`, and `git diff --check` passed on 2026-07-28.
  Review follow-up:
  The compiled CLI now converts Ctrl-C into caller-context cancellation. When cancellation prevents the first conflict query from observing paths, strict sync inspects the operation-owned `MERGE_HEAD` through the detached cleanup context and still aborts the merge. Public CLI coverage interrupts immediately after Git creates the conflicted index and proves exact branch/content restoration, no `MERGE_HEAD`, clean status, no LLM request, and no push.
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
- [x] [B041] (P0) Resolve AI merge conflicts by semantic region instead of full-file reproduction.
  Reported on 2026-07-28 after `gix sync bugfix/B038-rollback-failed-ai-merge` encountered additive conflicts in `.mprlab/ISSUES.md` and `CHANGELOG.md`, received an AI response, and rejected it for not preserving non-conflicting content.
  Observation:
  The merge resolver sends complete BASE, OURS, and THEIRS files to the model and requires another complete file as output. For the reported issue tracker conflict, that repeats roughly 145,000 input characters and asks the model to reproduce a roughly 50,000-character file under an 8,192-token output limit even though only one inserted issue region conflicts. The byte-preservation guard correctly rejects truncation, but the full-file contract makes a clean semantic resolution needlessly unreliable.
  Requirements:
  - Parse every marker-bearing worktree file into exact non-conflicting regions and explicit OURS, BASE, and THEIRS conflict regions.
  - Reconstruct complete files locally, directly accepting only byte-identical, unilateral, and marker-free current-stage cases because they contain no two-sided semantic choice.
  - Require semantic LLM audit for every marker-bearing region changed by both sides, including append-only issue/changelog insertions.
  - Derive lossless concurrent-insertion and non-overlapping-token candidates locally, but treat them only as proposals that the model must approve or correct.
  - For genuinely overlapping semantic regions without a safe local candidate, send only one region at a time, require a locally validated candidate plus an explicit semantic audit, and feed every rejection back into bounded repair attempts.
  - Give each semantic attempt enough time to exhaust the configured provider order instead of sharing one deadline across every file, region, provider, and repair.
  - Reject obvious one-sided loss and require both BASE-to-OURS and BASE-to-THEIRS replacement intent before semantic audit.
  - Roll back through the B038 operation-owned merge boundary only after all deterministic and bounded semantic strategies are exhausted, or when cancellation or an unrecoverable Git/filesystem failure makes further resolution impossible.
  - Remove the obsolete full-file response contract; do not retain a compatibility path.
  Validation:
  - Add public CLI coverage reproducing a clean pull-request branch with large `.mprlab/ISSUES.md` and `CHANGELOG.md` files whose review base inserts different entries at the same anchors. Resolution must perform one region-scoped semantic audit per conflict, exclude untouched file tails from both requests, contain every local and incoming entry exactly once, preserve untouched content byte-for-byte, push the merge commit, and leave no conflict or dirty state.
  - Prove a rejected one-sided semantic candidate receives exact validator feedback, is repaired, passes an explicit audit, and commits without rollback.
  - Cover deterministic/token strategy selection plus lossy-response exhaustion, per-attempt timeout exhaustion, cancellation, marker-free modify/delete, cached-diff validation, and rollback-failure handoff.
  - Run `make format`, `make test`, `make lint`, `make ci`, and `git diff --check`.
  Initial implementation:
  Marker-bearing conflicts use Git's diff3 regions and region-scoped LLM requests. Required response sentinels preserve boundary whitespace, Gix reconstructs untouched bytes locally, and empty-BASE concurrent insertions require both complete sides. Marker-free deletion semantics and the operation-owned rollback boundary remain strict. Public CLI coverage reproduces the large issue/changelog case and verifies a clean pushed merge containing every entry exactly once.
  Follow-up reported on 2026-07-28:
  A single rejected region candidate still reaches rollback immediately, even when the conflict has a deterministic high-fidelity resolution or the model could repair its candidate from exact validation feedback. Rollback is recovery from exhausted resolution, not a primary conflict-resolution strategy.
  Intermediate implementation:
  Strict sync now exhausts a fidelity-first ladder. It reconstructs untouched bytes locally; resolves authoritative identical, unilateral, append-only issue/changelog, and marker-free current-stage cases without an LLM; derives bounded non-overlapping token candidates for audit; and requires every genuinely semantic candidate to preserve both sides' replacement intent and receive an explicit auditor approval. Rejections feed up to four repair/audit attempts, each with enough time for every configured provider once. Git's cached-diff check gates the merge commit. Rollback is reachable only after strategy exhaustion, caller cancellation, or unrecoverable local failure. Public CLI coverage proves the reported large additive case makes zero LLM calls and pushes a clean exact merge, while an initially one-sided candidate is repaired and audited without rollback. Exhaustion, timeout, cancellation, marker-free deletion, index validation, and rollback-failure coverage remain green. `make format`, `make test`, `make lint`, `make ci`, and `git diff --check` passed on 2026-07-28.
  Correction reported on 2026-07-29:
  Minimizing LLM calls is not an objective. In many two-sided conflicts, semantic resolution is best delegated to the model. Deterministic reconstruction must protect exact content and provide high-fidelity candidates, while the LLM remains the semantic decision-maker and rollback remains the lowest-priority recovery strategy.
  Resolution:
  Strict sync now reconstructs untouched file bytes locally and directly accepts only identical, unilateral, and marker-free current-stage decisions. Every marker-bearing region changed by both sides enters semantic LLM review. Concurrent insertions and compatible token edits provide lossless local candidates for the auditor; genuinely overlapping regions start with model generation. The model may approve or correct a candidate, but additive and replacement-intent validation prevents either side from disappearing, and exact rejection feedback drives up to four repair/audit attempts with the complete configured provider order available to each attempt. Git's cached-diff check gates the merge commit, and rollback remains final recovery after semantic strategy exhaustion, cancellation, or unrecoverable local failure. Public CLI coverage proves the reported large issue/changelog case performs exactly two region-only audits without exposing either stable file tail, preserves every entry exactly once, pushes the merge commit, and leaves a clean worktree. Repair, exhaustion, timeout, cancellation, marker-free deletion, index validation, and rollback-failure coverage are green. `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check` passed on 2026-07-29.
- [x] [B042] (P0) Restore the starting checkout when an explicit target sync fails.
  Reported on 2026-07-29 after `gix sync bugfix/B038-rollback-failed-ai-merge` ran from B041, adopted and removed B038's linked worktree, switched the current checkout to B038, and then failed during conflict resolution.
  Observation:
  B038 restores the selected target branch to its pre-merge commit, but the wider strict-sync operation still leaves that target active and keeps an adopted sibling worktree removed. A failed command can therefore change both the caller's branch and valid worktree topology even after the operation-owned merge is aborted successfully.
  Requirements:
  - Snapshot the starting checkout and every valid registered worktree before strict sync mutates branches or adopts a sibling.
  - On failure, restore the starting named or detached checkout before reattaching detached main worktrees or recreating removed linked worktrees at their original paths.
  - Run checkout/worktree cleanup through a bounded context that remains usable after caller cancellation, and restore a user stash only after failure cleanup returns to the starting checkout.
  - Preserve the original sync error. Emit an explicit rollback event on successful restoration and a distinct manual handoff that retains the cleanup error when restoration itself fails.
  - Keep successful explicit-target behavior unchanged: the requested branch remains active only after the complete sync succeeds.
  - Treat filesystem-identical path aliases, including macOS `/var` and `/private/var` spellings, as the same worktree.
  Validation:
  - Add public CLI coverage reproducing a source branch in the current checkout, the target branch in a linked sibling, and an exhausted semantic merge on the target. The command must restore the exact source commit and files, recreate the clean target sibling at its original path and commit, leave no `MERGE_HEAD`, and perform no push.
  - Add a focused filesystem-identity guardrail and preserve every existing strict-sync/worktree-adoption scenario.
  - Run `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check`.
  Resolution:
  Strict sync now snapshots the caller checkout and valid registered worktrees before target switching or sibling adoption. When any later step fails, cancellation-independent bounded cleanup restores the starting named branch or detached commit, then reattaches a detached main worktree or recreates removed linked worktrees at their original paths before restoring a caller stash. The original sync failure remains primary; successful cleanup emits `SYNC_SWITCH_ROLLBACK`, while cleanup failure retains both errors and emits `SYNC_SWITCH_HANDOFF`. Successful explicit-target sync still leaves the target active. Public CLI coverage exhausts semantic resolution after adopting a linked target, then proves the exact source checkout and clean target sibling are restored with no `MERGE_HEAD` or push; a focused guardrail covers filesystem-identical path aliases. The branch was verified against exact `origin/master` commit `b1357e4567b18fd65c5d68e01b72f1303f893176`. `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check` passed on 2026-07-29.
- [x] [B043] (P0) Reject sync while an operator-owned revert is active.
  Reported on 2026-07-29 after plain `gix sync` ran on `bugfix/B095-app-owned-deploy-bundle` with a resolved but unfinished `git revert` and failed when it tried to switch to the already-active branch.
  Observation:
  Strict sync treats the staged and unstaged revert state as ordinary dirty work, fetches the remote, and then re-enters the selected branch. Git rejects that switch while `REVERT_HEAD` exists. Merely skipping the redundant switch would be unsafe because dirty-sync clustering resets and restages the sequencer-owned index before committing. The revert predates the command and is operator-owned, so Gix must not continue, abort, quit, or repartition it implicitly.
  Requirements:
  - Inspect `REVERT_HEAD` at the strict-sync boundary before fetch, branch/worktree mutation, stash, index reset, LLM dispatch, commit, or push.
  - Reject an active revert with an actionable message naming the explicit `git revert --continue`, `git revert --abort`, and `git revert --quit` choices.
  - Preserve the exact starting commit, branch, index, worktree, untracked files, and `REVERT_HEAD`; do not reinterpret the pending revert as dirty-sync clusters.
  - Treat an unexpected revert-state inspection failure as a contextual sync error instead of assuming no revert is active.
  - Keep operation-owned merge rollback unchanged; do not add automatic ownership transfer or a compatibility path for pre-existing Git transactions.
  Validation:
  - Add public compiled-CLI coverage reproducing a resolved but unfinished revert with both staged and unstaged content. Sync must fail before mutation, preserve exact Git state, make no LLM request, and perform no fetch, switch, reset, add, commit, or push.
  - Add focused guardrails for active, absent, and uninspectable `REVERT_HEAD` state.
  - Run `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check`.
  Resolution:
  Strict sync now inspects `REVERT_HEAD` before it snapshots worktrees, fetches, stashes, switches, resets the index, calls an LLM, commits, or pushes. An active operator-owned revert fails with explicit continue, abort, and quit choices; an unexpected inspection failure also stops sync. Public compiled-CLI coverage reproduces the reported resolved-but-unfinished revert with staged, unstaged, and untracked state and proves exact preservation with no mutating Git or LLM call. Focused coverage verifies active, absent, and uninspectable revert state. `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check` passed on 2026-07-29.
  Review follow-up:
  The first preflight resolved the ambiguous revision name `REVERT_HEAD` only in the caller worktree. That falsely rejected an ordinary branch or tag with the same name and missed a per-worktree revert in a sibling that strict sync could adopt, commit, push, and remove. The final preflight lists every valid registered worktree, resolves its exact `REVERT_HEAD` Git path, validates a present file as a canonical commit identifier, and rejects before fetch. Public compiled-CLI regressions prove an active sibling revert remains byte-for-byte unchanged while an ordinary branch named `REVERT_HEAD` does not block sync.
  `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check` passed for the review follow-up on 2026-07-29.
- [x] [B044] (P0) Make strict sync ownership-aware and transactional.
  Requested on 2026-07-29.
  Goal:
  Replace command-specific strict-sync recovery patches with one durable state transition that distinguishes operator-owned Git operations from gix-owned mutations across the caller and target sibling worktrees.
  Requirements:
  - Build one immutable preflight plan before fetch, LLM access, checkout changes, index changes, commits, or pushes.
  - Inspect exact per-worktree Git administrative paths and reject every pre-existing merge, revert, cherry-pick, rebase, apply-mailbox, bisect, or sequencer operation that strict sync could disturb.
  - Treat ordinary branches or tags named like Git administrative markers as ordinary refs.
  - Snapshot the exact caller and target-sibling checkout, commit, index, tracked contents, untracked contents, and stash list before the first gix-owned local mutation.
  - On a pre-publication failure, restore that snapshot and worktree topology; if restoration itself fails, preserve recovery state and emit an explicit handoff.
  - Restore and validate an invocation-owned `--stash` before reporting `SYNCED`; resolve safe conflicts through the current bounded semantic conflict engine and retain the stash on any unresolved failure.
  - Keep remote push and review-request creation as the final publication boundary.
  - Express acceptance as three declarative compiled-CLI integration tables: operator-owned preflight, failure rollback, and successful finalization.
  Validation:
  - Run `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check`.
  Resolution:
  Strict sync now builds one exact per-worktree operation plan, rejects operator-owned merge, revert, cherry-pick, rebase, apply-mailbox, bisect, and sequencer state before fetch, and ignores ordinary marker-like refs. Its local transaction snapshots branch refs, commits, index state, tracked and untracked contents, stashes, and adoptable worktree topology; sibling publication is deferred to the normal target push. Pre-push failure restores the complete snapshot, while post-push failure preserves forward recovery state and reports `SYNC_SWITCH_HANDOFF`. Invocation-owned stashes restore with `--index`, use the bounded semantic conflict engine when necessary, and complete before `SYNCED`. Three declarative compiled-CLI tables cover operator preflight, rollback/publication boundaries, and successful finalization. `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check` passed on 2026-07-29.
- [x] [B045] (P0) Close strict-sync transaction ownership gaps.
  Reported on 2026-07-29 during review of B044.
  Observation:
  Three edges still violate the ownership boundary. A pre-existing unmerged index without an administrative marker reaches snapshot acquisition, where a failed stash can trigger destructive cleanup without an owned backup. An up-to-date push is treated as publication even though it performs no remote write. Rollback also rewinds or deletes every local branch and recreates every starting worktree instead of limiting restoration to state this invocation mutated.
  Requirements:
  - Reject an unmerged index in every valid registered worktree before fetch or snapshot mutation, including conflicts left by `git stash apply` without an active merge or sequencer marker.
  - Parse Git's porcelain push result and mark Git publication only for an actual remote ref creation, update, or deletion; keep an up-to-date push rollback-capable, retain successful pull-request creation as publication, and fail closed when a successful response cannot prove its outcome.
  - Journal only branch refs and worktrees mutated by the invocation, advance the expected ref value after each successful mutation, and restore owned refs with compare-and-swap.
  - Preserve unrelated local branch creation or advancement and unrelated worktree topology during rollback; reject an unexpected outside change to an owned ref instead of overwriting it.
  - Make failed snapshot acquisition non-destructive unless an exact transaction backup was successfully acquired.
  Validation:
  - Extend the declarative operator-preflight table with an exact-state stash-apply conflict.
  - Extend the declarative failure table with a no-op push followed by pull-request failure and with a concurrent commit in an unrelated sibling worktree.
  - Run `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check`.
  Resolution:
  Strict sync now rejects every pre-existing unmerged index before snapshot acquisition and defensively repeats that check at snapshot and adoption boundaries. A snapshot is registered for rollback only after its exact backup exists; an earlier failure finalizes prior temporary snapshots without resetting unowned state. Every strict-sync push requests porcelain status, and only an actual ref creation, update, or deletion marks Git publication; an up-to-date push remains rollback-capable, successful pull-request creation remains a publication event, and an unprovable successful push fails closed under handoff. Local recovery journals only refs and worktrees the invocation mutates, advances each expected ref after successful Git commands, validates ownership before destructive cleanup, and compare-and-swaps only owned refs back to their starting values. The declarative tables now prove exact preservation of a stash-apply conflict, rollback after a no-op push, and survival of a concurrent unrelated sibling commit. `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check` passed on 2026-07-29.
- [x] [B046] (P0) Repair stale linked-worktree linkage before strict-sync preflight.
  Reported on 2026-07-29 after `gix sync` in `/Users/tyemirov/Development/TellTale` failed while resolving `MERGE_HEAD` inside `/Users/tyemirov/Development/story-generator-b007`.
  Observation:
  - The linked checkout still exists, but its `.git` file points at the removed `/Users/tyemirov/Development/story-generator` common repository.
  - The current common repository lists the checkout as registered without Git's `prunable` marker, so B030's missing-directory cleanup does not apply.
  - Strict preflight treats every non-prunable record as valid and runs `git rev-parse --git-path MERGE_HEAD` inside the broken checkout, making every sync fail before it can inspect the caller.
  Requirements:
  - Repair canonical Git worktree metadata before strict sync classifies and inspects registered worktrees.
  - Distinguish an existing linked checkout with repairable stale linkage from a missing Git-prunable checkout; never prune a live checkout blindly.
  - Preserve the linked checkout's branch, commit, index, tracked contents, untracked contents, and staged/unstaged distinction.
  - Re-list and inspect the repaired topology before fetch, stash, checkout, LLM dispatch, commit, push, or pull-request creation.
  - Fail with worktree and repository context when Git cannot repair or validate the topology.
  Validation:
  - Add a public compiled-CLI regression that creates a linked checkout, moves the primary repository, proves the linked checkout's old `.git` pointer fails, and then proves sync repairs the registration and completes without changing the linked checkout's state.
  - Preserve the existing missing-prunable-worktree regression and every strict-sync worktree preflight scenario.
  - Run `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check`.
  Resolution:
  Strict sync now runs Git's canonical `worktree repair` from the caller before listing or inspecting registered worktrees. Existing linked checkouts whose `.git` files still name a moved primary repository are reconnected and re-listed before exact administrative-path and unmerged-index preflight; missing Git-prunable registrations remain on B030's prune path, and repair failures retain repository context. A public compiled-CLI regression moves the primary repository after creating a dirty linked checkout, proves the old non-prunable link returns the reported `rev-parse --git-path MERGE_HEAD` failure, and then proves sync repairs it while preserving the sibling branch, commit, index, staged and unstaged contents, and untracked files exactly. `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check` passed on 2026-07-29.
  Review follow-up:
  Unconditional `git worktree repair` treated the invoking common repository as authoritative. If a primary repository was copied instead of moved, the copy retained the original worktree registrations and repair rewrote a live sibling's valid `.git` pointer away from the still-existing original repository.
  Follow-up resolution:
  Strict sync now lists the topology first and resolves each live checkout's common Git directory. A checkout already owned by another live common repository is rejected without mutation. Only a checkout whose canonical `.git` target is missing is passed as an explicit path to `git worktree repair`, after which the topology is re-listed and ownership is revalidated before administrative-state inspection. The original moved-primary regression remains green, and a new compiled-CLI regression copies the primary repository, proves sync fails closed, and verifies both repositories' topology plus the sibling branch, commit, index, staged contents, untracked contents, and `.git` pointer remain exact. `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check` passed on 2026-07-29.
- [x] [B047] (P0) Reject concurrent checkout or index drift during dirty sync commits.
  Reported on 2026-07-30 after two `gix sync` runs overlapped another writer in the same llm-proxy checkout.
  Observation:
  - During `gix sync master`, another writer created and checked out `bugfix/B098-visible-fail-closed-ci` while Gix was generating clustered commit messages. The branch compare-and-swap correctly rejected the outside ref, but rollback repeated the same ownership error and described the result as a generic restoration failure.
  - During plain sync on `bugfix/B099-retire-legacy-compose-service`, another writer staged `tests/lifecycle_contract_test.go` while Gix was waiting for the README commit message. Gix committed that outside-staged path with README, then treated the now-empty tests cluster as `no changes detected for commit message generation` and rolled back.
  Requirements:
  - Treat the selected checkout and exact index as owned state across each slow dirty-cluster commit-message request.
  - Validate the cluster's exact staged path set before dispatch and validate the exact checkout, HEAD, and index again before commit.
  - If another writer switches checkout or changes the index, do not commit, push, reset, clean, or restore across that outside state. Preserve the transaction snapshots and emit one `SYNC_SWITCH_HANDOFF` that names the ownership loss and tells the operator to stop the other writer before retrying.
  - Keep branch-ref compare-and-swap protection and ordinary pre-publication rollback for failures that occur while ownership remains intact.
  - Do not reinterpret an empty later cluster as success or retain the opaque `no changes detected` outcome for this concurrency case.
  Validation:
  - Add public compiled-CLI coverage that changes the checkout during a commit-message request and proves the outside branch, HEAD, index, files, and transaction snapshot remain untouched by cleanup.
  - Add public compiled-CLI coverage that stages a later cluster during an earlier commit-message request and proves Gix neither sweeps that path into its commit nor rolls back over the outside index.
  - Run `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check`.
  Resolution:
  Dirty-cluster commits now expand and validate the complete staged path set, then checkpoint the active checkout, `HEAD`, and semantic index entries before each LLM request. Gix rechecks that state after the request through a cancellation-independent bounded inspection when needed. Checkout or index drift stops before commit or push, marks local transaction ownership as lost, preserves the current outside state and transaction snapshot, and emits one actionable `SYNC_SWITCH_HANDOFF` instead of attempting rollback. Public compiled-CLI regressions reproduce both reported interleavings from the LLM boundary and prove no commit, push, reset, clean, or rollback follows the outside mutation. `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check` passed on 2026-07-31.
  Review follow-up on 2026-07-31:
  - The final ownership read still completed before an unlocked `git commit`, leaving a time-of-check/time-of-use window in which a normal Git writer could stage outside work into the commit.
  - The checkpoint omitted skip-worktree, assume-unchanged, intent-to-add, and resolve-undo state, while cancellation could arrive after the conditional context check and interrupt the ownership read itself.
  Follow-up resolution:
  Gix now resolves the exact per-worktree index path, compares every commit-relevant entry and semantic flag, and performs every post-model ownership read through a bounded context detached from caller cancellation. The final read runs while Gix holds the canonical index lock; Gix copies the validated index into that private locked file and commits through `GIT_INDEX_FILE`, so a normal outside writer either mutates before the recheck and triggers handoff or loses the lock and cannot enter the commit. Focused cancellation coverage and public compiled-CLI regressions prove semantic-flag drift is rejected, post-check staging is lock-blocked, cluster commits remain separate, and the index lock is released on both success and handoff.
- [x] [B048] (P1) Follow transitively merged pull-request stacks to master.
  Reported on 2026-08-01 after plain `gix sync` on `tyemirov/improvement/I205-inventory-placement-groups` returned `branch ... does not have an open pull request` even though its pull request and every parent pull request were merged into `master`.
  Observation:
  - Gateway PR #164 merged I205 into B388, PR #163 merged B388 into B387, and PR #162 merged B387 into `master`.
  - Strict sync checks for an open pull request and then asks whether the selected branch merged directly into `master`. Without local `gix-review-base` metadata, it does not discover the selected branch's actual merged base or follow the merged parent chain.
  Requirements:
  - Discover the actual base of a merged pull request for an existing branch even when no local stacked-review metadata exists.
  - Follow each merged parent pull request until the chain reaches the first active remote branch; when every link reaches `master`, use the standard merged-branch prompt to offer syncing `master`.
  - Prefer an active open pull request over historical merged records for a reused branch name and reject cycles or missing terminal branches without mutation.
  - Preserve strict-sync transaction rollback, dirty merged-branch rejection, and single-prompt confirmation behavior.
  Validation:
  - Add public compiled-CLI coverage for a three-hop merged stack with all remote branch refs still present and no local review-base metadata. Plain `gix sync` must offer the standard `master` handoff, switch to synced `master` when accepted, create no pull request, and emit no rollback.
  - Run `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check`.
  Resolution:
  Gix now discovers a merged pull request's actual base without assuming `master` or requiring local review-base metadata, then follows an arbitrary number of merged parent pull requests even while their remote refs remain. It stops at the configured base, an active open pull request, or a live terminal branch; active review state wins over historical merged records, and cycles fail before mutation. A fully merged chain produces one standard handoff prompt for the terminal base, while dirty merged branches are rejected before commit. The compiled-CLI regression reproduces the reported I205-to-B388-to-B387-to-`master` topology with all refs retained and no local metadata; it initially failed with the reported missing-open-pull-request rollback and now proves acceptance syncs `master` without creating a pull request or emitting rollback. Focused tests cover active-parent precedence, cycle rejection, and dirty-branch ordering. `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check` passed on 2026-08-01.
  Review follow-up:
  The first traversal keyed historical merged pull requests only by branch name. A reused branch, or a retained branch advanced after merge, could therefore inherit its old stack and hand off to `master` instead of publishing the new commits. Gix now requests each pull request's `headRefOid` and accepts the merged record only when that OID matches the fetched remote tip and no local-only commits exist, or when neither local nor remote ref survives. A mismatched selected head continues through the existing remote-branch publication flow; a mismatched parent becomes the terminal handoff. The compiled-CLI regression initially reproduced the false `master` handoff for a newly pushed reused child and now proves Gix creates its new pull request, while focused coverage proves an advanced parent stops traversal. `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check` passed for the follow-up on 2026-08-01.
- [x] [B049] (P1) Preserve complete multi-provider failure context in CLI output.
  Reported on 2026-08-06 after `gix sync` displayed `llm_proxy_client_http_failure (and 1 more failures)` when both the prioritized LLM Proxy request and the direct OpenAI request failed.
  Observation:
  - The prioritized client retains each connection name and wrapped transport error, but the workflow executor recursively unwraps ordinary error chains to their terminal leaves.
  - The final command error prints only the first collected leaf plus a count, hiding the OpenAI failure and stripping the LLM Proxy HTTP status and response body.
  Requirements:
  - Preserve the complete contextual error returned by an ordinary operation, including every named prioritized LLM connection and its wrapped cause.
  - Continue splitting typed repository `OperationError` joins so their existing structured event codes and subjects remain independently reportable.
  - Keep standard Go error traversal intact for callers using `errors.Is` and `errors.As`.
  Validation:
  - Add public compiled-CLI coverage in which LLM Proxy returns HTTP 503 and direct OpenAI returns HTTP 429. The command must attempt both connections and print both names, statuses, and response details without the opaque `and 1 more failures` summary.
  - Preserve existing joined repository-operation event coverage.
  - Run `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check`.
  Resolution:
  The workflow executor now preserves the complete outer context of ordinary operation errors, including joined prioritized-client attempts, while it continues to split typed repository `OperationError` values for independently coded structured events. Final multi-failure summaries print every formatted failure instead of replacing later failures with a count, and returned causes retain standard `errors.Is`/`errors.As` traversal. The compiled-CLI regression first reproduced the reported proxy sentinel plus hidden OpenAI failure, then proved an LLM Proxy HTTP 503 and direct OpenAI HTTP 429 both retain their connection names, statuses, and response details. `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check` passed on 2026-08-06.
- [x] [B050] (P1) Reconcile legacy GitHub Pages deployment state without duplicate builds.
  Reported on 2026-08-06 after `make deploy` pushed the v1.1.20 Pages branch, triggered two GitHub-generated `pages-build-deployment` runs, and then reduced a queued build during a GitHub Pages outage to a stale-marker timeout.
  Observation:
  - The deploy helper unconditionally updates an already-correct legacy Pages configuration and then explicitly requests another build, so one invocation creates competing builds for the same `gh-pages` commit.
  - Retries ignore existing Pages build state and can enqueue another build instead of reusing an active or completed build.
  - Verification polls only the public marker, so terminal build failures and queued/building state lose their native status, error, commit, and build URL.
  Requirements:
  - Keep the canonical legacy `gh-pages:/` publishing source and `.nojekyll` artifact contract; do not add a repository-owned Pages workflow.
  - Read and validate the current Pages configuration, mutate it only when missing or different, and distinguish a confirmed HTTP 404 from other GitHub API failures.
  - Treat a changed Pages branch or changed Pages configuration as the invocation's build trigger. Reuse a matching built, queued, or building record, and request exactly one rebuild only when an unchanged retry finds no build or a terminally errored build for the exact deployed branch commit.
  - Verify the exact Pages build before checking the public release marker. Preserve bounded polling while reporting the build status, native error, commit, and URL on failure.
  Validation:
  - Add public release-script coverage for a changed branch with current configuration, missing and drifted configuration, unavailable configuration API, an active build that completes, and a terminal build whose single retry fails.
  - Prove current configuration causes no PUT, changed branch/configuration causes no redundant build POST, completed and active builds are reused, and a terminal unchanged build receives exactly one rebuild request.
  - Run `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check`.
  Resolution:
  The Pages deploy helper now treats branch and configuration mutations as the legacy Pages build trigger, preserves an already-canonical `gh-pages:/` configuration, and fails closed when configuration inspection fails for any reason other than a confirmed HTTP 404. On unchanged retries it selects the exact deployed branch commit, accepts built builds, reuses queued or building builds, and requests exactly one rebuild for a missing or errored build. Verification now waits for that build before probing the public marker and reports native status, error, commit, and URL context. Public release-script regressions cover the configuration and build-state matrix, including the queued state returned by GitHub's build API. `make format`, `make test`, `make lint`, `make ci`, `make build`, `bash -n scripts/release/deploy_pages_artifact.sh`, and `git diff --check` passed on 2026-08-06.
- [x] [B051] (P0) Make exact-tag release retries reuse verified sealed state.
  Reported on 2026-08-06 after retrying the canonical lifecycle at v1.1.21 selected v1.1.22, failed because `v1.1.21..HEAD` contained no commits, and replaced the valid local release receipt with incomplete v1.1.22 staging.
  Observation:
  - Release version selection always bumps the latest tag, even when `HEAD` is already the exact published release commit.
  - Release initialization deletes `.git/mprlab-release` before release-note generation and final sealing, so a later preparation failure destroys the prior valid receipt.
  - `make deploy` then correctly rejects the missing manifest, leaving the canonical retry unable to resume the already-published release.
  Requirements:
  - Detect an exact release tag at `HEAD` before CI, version selection, artifact preparation, changelog mutation, commit, or tag creation.
  - Verify and reuse a complete matching local sealed release. When local state is missing or incomplete, recover the same release from immutable published GitHub release state and verify its manifest, notes, payload hashes, tag, release commit, source parent, and release-only changelog change before reuse.
  - Prepare a genuinely new release in an isolated candidate receipt and atomically promote it only after the complete manifest, notes, and payload inventory verify.
  - Preserve the prior canonical receipt on every failed candidate preparation and reject conflicting local or published state without overwriting it.
  Validation:
  - Add public release-script coverage proving a valid exact-tag receipt is reused without CI or artifact preparation, missing/incomplete local state is recovered from matching published state, and conflicting published state fails closed.
  - Prove a new-release failure after candidate initialization leaves the prior canonical receipt byte-for-byte unchanged.
  - Run `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check`.
  Resolution:
  Release preparation now detects a single exact version tag before version selection or CI and reconciles that immutable release instead of selecting a successor. A complete matching local receipt is reused without GitHub access; a missing or partial receipt is rebuilt from the published GitHub Release only after the release object, remote annotated tag, manifest identity, changelog-only release commit, source parent, notes, asset inventory, payload paths, sizes, and hashes verify. New releases prepare in a sibling candidate directory and promote only a complete verified receipt, so every earlier preparation failure leaves the canonical receipt unchanged. A failure after release commit or tag creation atomically restores the transaction-owned refs and `CHANGELOG.md` to the prepared source. Conflicting local or published state fails closed with contextual errors. Public release-script regressions cover local reuse, staged and partial recovery, malformed and conflicting receipts, candidate failure, and post-tag sealing rollback. `make format`, `make test`, `make lint`, `make ci`, `make build`, shell and Python syntax checks, and `git diff --check` passed on 2026-08-06.
- [x] [B052] (P0) Handle LLM proxy failures during gix sync PR description generation.
  Goal:
  Make `gix sync` handle pull request description LLM failures gracefully so a sync failure is clear, actionable, and does not leave the repository in an unexpected checkout or worktree state.
  
  Requirements:
  Preserve the existing rollback behavior that restores the starting checkout, local state, and adopted worktree topology when strict sync cannot complete. Do not leave the target branch active after rollback. Surface the underlying LLM/proxy failures without exposing sensitive credentials or full API keys in user-facing output. Keep behavior compatible with strict sync semantics.
  
  Deliverables:
  A code change that improves handling and reporting for `strict sync pull request description.llm` failures when all LLM providers or proxy calls fail. Sanitized error output for LLM proxy URLs/keys. Any necessary tests or fixtures covering LLM empty responses, proxy HTTP/TLS failures, and rollback after PR description generation failure. Documentation or help text updates if user-facing sync failure guidance changes.
  
  Validation:
  Run the relevant sync and PR-description-generation tests. Reproduce or simulate an LLM proxy failure and confirm `gix sync` fails with a sanitized, actionable message while restoring the original checkout, local state, and worktree topology. Confirm logs do not include full LLM proxy keys or other secrets.
  Resolution:
  PR description generation now passes underlying errors through `sanitizeLLMDescriptionError`, which redacts query parameters (`key=`, `secret=`, `token=`, `api_key=`), authentication headers (`Bearer`, `Authorization:`, `X-Api-Key:`), basic auth credentials in URLs, and literal configured connection secrets. Strict sync transaction rollback runs before remote push when description generation fails, restoring starting checkouts, branches, and worktree topology. Unit and integration tests verify error sanitization, empty responses, and strict sync transaction rollback behavior.
- [x] [B053] (P0) Recover direct OpenAI after reasoning-only empty completions.
  Reported on 2026-08-08 after `gix sync` failed to generate a pull request description when LLM Proxy returned a TLS transport error and direct OpenAI exhausted three attempts with empty responses.
  Observation:
  The prioritized client reaches the configured OpenAI connection, but the direct client repeats the same bounded reasoning request three times. If every response consumes its completion budget before producing visible text, the usable backup connection is reported as failed and strict sync rolls back.
  Requirements:
  - Preserve configured connection priority, OpenAI model, reasoning effort, timeout, and ordinary error behavior.
  - Add `max_completion_tokens` to each provider profile and resolve the budget through the canonical `command > provider > llm` hierarchy.
  - Keep token-budget policy in `config.yml`; do not raise or default completion budgets in request or recovery code.
  - After the direct OpenAI client exhausts its normal retries specifically with the typed empty-response error, repeat the resolved request for one bounded recovery round.
  - Preserve cancellation and complete primary/recovery failure context. Do not retry authentication, HTTP, or unrelated transport failures through this recovery path.
  Validation:
  - Add compiled CLI coverage in which LLM Proxy uses and fails at the inherited global budget, while three direct OpenAI attempts return empty `finish_reason=length` responses and recovery succeeds at the provider budget.
  - Verify an explicit command budget overrides both provider and global values, and sync does not inherit the `message commit` command budget.
  - Preserve the existing compiled-CLI coverage for complete multi-provider failure reporting.
  - Run `make format`, focused tests, `make ci`, `make build`, and `git diff --check`.
  Resolution:
  Provider profiles now accept and strictly validate `max_completion_tokens`. Application configuration resolves completion budgets through `command > provider > llm`; absent command values remain unset through commit, changelog, sync pull-request, and merge-resolution request builders, and sync no longer inherits the `message commit` command budget. The generated and active user configuration assign 16,384 tokens to direct OpenAI while LLM Proxy inherits the global 1,200-token value. Direct OpenAI repeats the same resolved request for one recovery cycle only after typed empty-response exhaustion, preserving ordinary failures, cancellation, and joined primary/recovery context. The compiled CLI regression proves one failed proxy request at the global budget, three empty OpenAI attempts plus a successful recovery at the provider budget, and complete multi-provider error reporting. Focused tests, `make format`, `make ci`, `make build`, and `git diff --check` passed on 2026-08-08.
- [x] [B054] (P0) Require the global completion-token budget from configuration.
  Requested on 2026-08-08 after reviewing the B053 provider-specific token defaults.
  Goal:
  Keep completion-token policy in the canonical `config.yml` while retaining the established `command > provider > llm` override hierarchy.
  Requirements:
  - Require a positive top-level `llm.max_completion_tokens` value during application initialization.
  - Make `gix init` generate one 4,800-token global default suitable for the configured reasoning models.
  - Keep optional provider and command values as explicit configuration overrides, but do not prepopulate them in the generated file.
  - Store no numeric completion-token policy in Go code.
  Validation:
  - A compiled `gix init` writes exactly one `max_completion_tokens` key with value 4,800.
  - A compiled `gix version --config <path>` rejects a configuration that omits the global value.
  - Existing command, provider, and global precedence coverage remains green.
  - Run `make format`, focused tests, `make ci`, `make build`, and `git diff --check`.
  Resolution:
  Application initialization now requires a positive top-level `llm.max_completion_tokens`. `gix init` generates exactly one completion-token setting, the 4,800-token global budget, while provider and command fields remain optional explicit overrides under the existing `command > provider > llm` hierarchy. Numeric completion-token policy is absent from production Go defaults. Compiled initialization and rejection regressions, hierarchy coverage, focused integration tests, `make format`, and `make ci` passed on 2026-08-08; `make build` and `git diff --check` completed before publication.
- [x] [B055] (P0) Require explicit intent for new SemVer releases.
  Reported on 2026-08-08 after `make release` published breaking configuration changes as the twenty-fifth consecutive `v1.1.x` patch release.
  Goal:
  Prevent a new SemVer release from silently selecting a patch version when the operator has not declared the release intent.
  Requirements:
  - Require exactly one explicit `patch`, `minor`, `major`, or exact-version intent before selecting a new SemVer release.
  - Preserve zero-argument exact-tag receipt verification and reuse.
  - Preserve timestamp-derived CalVer selection without a SemVer bump argument.
  - Reject conflicting exact-version and bump inputs and reject SemVer bump inputs for CalVer.
  - Expose the canonical intent through `make release RELEASE_BUMP=<patch|minor|major>` or `make release RELEASE_VERSION=<version>`.
  Validation:
  - Public release-script coverage rejects missing and conflicting intent before CI or artifact preparation.
  - Public coverage selects patch, minor, major, and exact versions from the same release boundary.
  - Existing exact-tag reuse, candidate isolation, rollback, and publication tests remain green.
  - Run focused tests, `make format`, `make ci`, `make build`, shell/Python syntax checks, and `git diff --check`.
  Resolution:
  The Make release boundary now forwards explicit `RELEASE_BUMP`, `RELEASE_VERSION`, and `RELEASE_SCHEME` values. New SemVer preparation requires exactly one patch, minor, major, or exact-version intent and rejects missing, conflicting, and CalVer-incompatible inputs before CI. Timestamp-derived CalVer selection and zero-argument exact-tag receipt reuse remain unchanged. Public Make and release-script regressions cover every intent path and preserve candidate isolation, rollback, receipt recovery, and publication behavior. Focused release tests, shell and Python syntax checks, `make format`, and `make ci` passed on 2026-08-08; `make build` and `git diff --check` completed before publication.
  Review follow-up:
  A checkout without local version tags reported `scheme_guess: none`, bypassed the new missing-intent guard, and still selected `v1.0.0` before running CI.
  Follow-up resolution:
  Release selection now normalizes the untagged default to the initial SemVer contract unless CalVer is explicitly requested. A public release-script regression proves that bare preparation stops before CI and preserves the canonical receipt, while an explicit patch intent selects `v1.0.0` with a SemVer scheme. Focused release tests, `bash -n scripts/release/prepare_release.sh`, `make format`, `make ci`, `make build`, and `git diff --check` passed on 2026-08-08.
- [x] [B056] (P0) Isolate release intent from the release CI subprocess.
  Found on 2026-08-08 while preparing the exact v5.0.1 release.
  Goal:
  Keep the selected release intent owned by the outer release transaction while running the canonical CI gate in a clean Make environment.
  Requirements:
  - Remove release-control and recursive-Make variables only from the `make ci` subprocess environment.
  - Preserve the already selected exact version, changelog boundary, artifact preparation inputs, rollback, and receipt contracts in the outer transaction.
  - Do not weaken or skip any CI test to make release preparation pass.
  Validation:
  - A public release-script regression proves exact-version and Make override values are absent inside the CI subprocess.
  - The failed v5.0.1 attempt leaves master, the v5.0.1 tag, and the canonical v1.1.25 receipt unchanged.
  - Run focused release tests, `bash -n scripts/release/prepare_release.sh`, `make format`, `make ci`, `make build`, and `git diff --check`.
  Resolution:
  Release preparation now removes release-control and recursive-Make variables only around its `make ci` subprocess, while retaining the selected version and artifact inputs in the outer transaction. The public regression seeds every affected variable and proves the CI subprocess receives none of them. The failed v5.0.1 attempt left master, the tag namespace, and the canonical v1.1.25 receipt unchanged. Focused release coverage, shell syntax validation, `make format`, `make ci`, `make build`, `go mod verify`, `go mod tidy -diff`, and `git diff --check` passed on 2026-08-08.



## Improvements

- [x] [I010] (P1) Remove the redundant generic configuration decode.
  Goal:
  Use one strict YAML decoder for the application configuration schema.
  Requirements:
  - Decode YAML directly into the typed target configuration and reject unknown fields.
  - Expand inherited environment placeholders after YAML decoding without reparsing configuration data.
  - Preserve literal values, optional credential placeholders, required placeholder errors, and scalar characters from environment values.
  - Remove the generic `map[string]any` and `mapstructure` decode from the configuration loader.
  Validation:
  - Add public compiled-CLI coverage that rejects the removed `temperature` key through the strict YAML schema.
  - Preserve focused configuration-loader coverage for placeholder and unknown-field behavior.
  - Run the complete post-change `make ci` gate and `git diff --check`.
  Resolution:
  The application loader now performs one strict YAML decode directly into the typed target and rejects unknown fields through that decoder. Environment placeholders expand afterward in decoded string values, preserving literal substituted characters, optional credential placeholders, and required-placeholder errors without reparsing a generic YAML map. The loader no longer imports `mapstructure` or constructs `map[string]any`; command-specific operation option maps retain their separate typed decode after the application schema is loaded. Focused loader, documentation, and compiled-CLI regressions pass, including rejection of the removed `temperature` key. The complete pre-change and post-change `make ci` gates, `make format`, the live user-configuration version command, and `git diff --check` passed on 2026-08-08.

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
- [x] [M016] (P0) Correct the published SemVer history and establish the Go v5 module line.
  Requested on 2026-08-08 after the release audit found breaking changes published as consecutive `v1.1.x` patch releases.
  Goal:
  Publish the historically correct v2 through v5 release boundaries and make the current v5.0.1 release installable through the canonical Go major-version module path.
  Requirements:
  - Publish v2.0.0, v3.0.0, v4.0.0, v4.1.0, and v5.0.0 as immutable aliases of the audited historical release commits.
  - Reconstruct every aliased manifest, release note, and Pages marker with the corrected version instead of copying internally false artifacts.
  - Preserve the original binaries and source/release commit identities for each historical alias.
  - Change the current module path and all self-imports to `github.com/tyemirov/gix/v5` before v5.0.1.
  - Keep the historical migration bounded to the declared mapping, fail before mutation on conflicting state, and remove the migration path after completion.
  Validation:
  - Public migration coverage proves exact mapping, corrected manifests and Pages markers, complete asset inventories, and pre-mutation rejection of conflicts.
  - `make ci`, `make build`, `go mod verify`, `go mod tidy -diff`, and `git diff --check` pass for the v5 source.
  - Every requested annotated tag and ready GitHub Release resolves to its audited commit with verified assets.
  - `go install github.com/tyemirov/gix/v5@v5.0.1` succeeds and the installed executable reports v5.0.1.
  Resolution:
  The audited v2.0.0, v3.0.0, v4.0.0, v4.1.0, and v5.0.0 aliases are published as ready GitHub Releases at their historical release commits. Their original binaries remain byte-identical, while their release notes, manifests, and embedded Pages markers identify the corrected versions and retain the audited source and release commits. Current source now uses the canonical `github.com/tyemirov/gix/v5` module path throughout. The bounded migration rejected conflicting state, passed its public tests and two live read-only rehearsals, published and verified every target asset, and was removed after completion. `make format`, `make ci`, `make build`, `go mod verify`, `go mod tidy -diff`, Python syntax checks, and `git diff --check` passed before the v5.0.1 release cut.


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


## Planning
*do not implement yet*

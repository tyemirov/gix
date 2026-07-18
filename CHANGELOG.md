# Changelog

## [Unreleased]

### Features ✨
- _No changes._

### Improvements ⚙️
- _No changes._

### Bug Fixes 🐛
- Switched worktree inspection to NUL-delimited porcelain status so `gix sync` passes literal filenames containing spaces to Git instead of treating display quotes as path bytes.
- Rejected explicit dirty base-branch sync before commit generation when the required remote base ref does not exist, preventing stranded local commits.
- Made explicit `gix sync <branch>` commit dirty work to the named branch; explicit `gix sync master` now merges `origin/master` and pushes `master` instead of creating a generated rescue branch.
- Fixed plain `gix sync` on a dirty, remote-backed branch with no pull request so it commits the pending work and opens the missing pull request instead of failing before the established base-delta flow.
- Kept merged remote branches on the stashed-handoff path by detecting merged pull-request state before dirty commit generation.
- Fixed `gix sync <new-branch>` to publish and open the current branch pull request first, then open the dirty child branch pull request against that parent instead of `master`.
- Preserved recorded ancestors when retrying deeper pull-request stacks and followed merged ancestors whose remote heads were deleted until sync reached a surviving base.
- Rejected dirty auto-commit on known-merged branches before work could be stranded and directed the work through the stashed handoff path.
- Rejected clean or stashed missing branches before child creation or pull-request publication because a correctly stacked child would have no review delta.
- Fixed `gix sync <new-branch>` to create a missing target on top of the current branch before clustering dirty work, preventing checkout-overwrite failures and lost branch ancestry.
- Rejected incomplete AI merge-conflict output before it can truncate a file, create a merge commit, push, or open a pull request, including marker-free modify/delete conflicts.
- Made repository release targets self-contained, fail closed when any expected platform artifact is missing, anchor Pages deployment to the locally prepared release manifest, and publish consistently through canonical `origin`.
- Kept the syncflow builder description as the canonical text shown by `gix sync --help`.

### Testing 🧪
- Added a public plain-`gix sync` regression proving deletion of `legacy/managing-director/IMD Logo.png` is committed and pushed with the literal path argument.
- Added a black-box regression proving an explicit dirty `master` sync with no `origin/master` leaves local history and pending files unchanged without calling the LLM.
- Added black-box coverage for explicit dirty `master` sync from both `master` and a feature branch, including clustered commits, remote merge, direct push, and absence of generated PR branches.
- Added public CLI regressions for dirty unreviewed remote branches and dirty merged remote branches without stack metadata.
- Added a public CLI regression proving a three-level parent stack remains intact, parent push/PR creation precedes child creation, clustered commits stay linear, a failed child PR creation retries against the persisted parent, and merged stacks hand off normally.
- Added a black-box dirty-sync regression that verifies two top-level change clusters become two linear commits above the original branch before push and pull-request creation.
- Added CLI regressions for isolated explicit-master rescue, local-only generated-name collisions, marker-bearing and marker-free merge-resolution truncation, and the assembled sync help output.
- Added black-box release coverage for clean-checkout helpers, failed or missing platform outputs, replaced published manifests, and missing integrity prerequisites.

### Docs 📚
- Documented explicit branch targets as binding dirty-commit destinations and distinguished explicit `master` from plain current-branch rescue.
- Documented that unreviewed remote-backed branches accept dirty commits before the missing pull request is opened, while merged branches reject auto-commit.
- Documented the canonical `master <- parent PR <- child PR` sync chain and the no-delta rejection for clean missing branches.
- Clarified that missing explicit sync targets start at the current branch's `HEAD` and merge the remote review base afterward.
- Documented validated AI conflict resolution and the repository-owned release workflow prerequisites.

## [v1.1.5] - 2026-07-16

- Merge pull request #385 from tyemirov/bugfix/B028-explicit-sync-commits-target-branch
- fix(sync): preflight explicit remote base
- fix(sync): honor explicit branch commit targets

## [v1.1.4] - 2026-07-13

- Merge pull request #382 from tyemirov/gix/preserve-pages-source-vs-release-commit-identities-and
- Merge pull request #383 from tyemirov/bugfix/B027-dirty-existing-remote-opens-pr
- fix(sync): publish dirty unreviewed remote branches
- test(release): fix Pages integration runtime
- test: verify pages release creates marker, .nojekyll, and enforces commit roles
- fix(release): verify Pages artifact marker and always write .nojekyll
- docs(issues): add design for preserving release and source commit identities in Pages deployment

## [v1.1.3] - 2026-07-10

- Merge pull request #381 from tyemirov/tyemirov/bugfix/B025-stack-sync-prs
- test: expand and clarify sync branch/PR integration, support failure cases
- feat(syncflow): implement strict stacked branch and pull request workflow
- docs: clarify stacked branch creation and review flow in README
- docs(changelog): record syncflow stack bug fixes, new CLI tests, and docs updates
- docs(issues): document B025 stacked pull-request sync regression and resolution

## [v1.1.2] - 2026-07-10

- Merge pull request #380 from tyemirov/bugfix/B020-dirty-sync-new-branch
- chore(release): add release and pages artifact preparation/deploy scripts
- test: add release and sync integration tests for makefile and CLI behavior
- feat(syncflow): improve merge conflict resolution and strict sync logic
- refactor(cli): use branchSyncCommand.Long for sync long description
- docs: clarify release, publish, and merge conflict resolution behavior
- build: improve release-artifacts validation and tighten artifact checks
- docs(changelog): update for explicit master sync, AI merge validation, and release flow
- docs(ISSUES): document recent sync, merge, release, and help contract fixes
- docs(issues): add new priority-zero bugs and maintenance runbooks
- Merge remote-tracking branch 'origin/master' into bugfix/B020-dirty-sync-new-branch
- test: add integration for clustered dirty commits on new sync branches
- fix(syncflow): consistently create branches from current checkout and merge base
- docs: clarify sync branch creation and add release/publish/deploy section
- build(makefile): add multi-platform release, artifact, and deploy targets
- docs: update changelog with fixes and testing for dirty sync target behavior
- docs(issues): document dirty sync branch creation regression and fix validation
- ci: remove GitHub Actions release workflow
- Merge pull request #379 from tyemirov/issues-md-1783374715345
- Update ISSUES.md
- Update ISSUES.md
- Update ISSUES.md
- Release v0.8.1
- Merge pull request #378 from tyemirov/bugfix/sync-existing-branch-create-pr
- test: remove branchName from sync command args in integration test
- test: add integration tests for merged branch sync scenarios
- fix(syncflow): handle adoption of orphaned remote branches and main worktree
- docs(issues): add sync pull request and worktree handoff case resolutions
- Release v0.8.0
- Merge pull request #377 from tyemirov/tyemirov/bugfix/init-subcommand
- fix(syncflow): support LLM merge resolution for deleted files and missing branches
- chore: update CHANGELOG for strict-sync empty branch and file deletion fixes
- docs(issues): document branch sync halt and AI merge delete support
- feat(syncflow): add automated AI-powered merge conflict resolution
- feat(syncflow): resolve merge conflicts in strict-sync using AI client
- docs(issues): describe AI-based merge conflict resolution for dirty sync
- fix: push local-ahead sync branches
- test: reject root init flag
- fix: add init subcommand
- Release v0.7.0
- Merge pull request #376 from tyemirov/issues-md-1782774323268
- Update ISSUES.md
- Update ISSUES.md
- Merge pull request #375 from tyemirov/issues-md-1782774205639
- Apply ISSUES.md execution changes
- Merge pull request #374 from tyemirov/issues-md-1782774055936
- Update ISSUES.md
- Merge pull request #373 from tyemirov/tyemirov/improvement/optional-llm-provider-config
- test: replace XDG config directory with secondary config directory in tests
- docs: update config file search order and clarify default config path
- refactor(config): drop XDG_CONFIG_HOME support for user config paths
- docs: update config file path references to use $HOME/.gix/config.yaml
- docs(changelog): update config path to use $HOME/.gix/config.yaml only
- docs: update config file precedence in architecture documentation
- docs(issues): clarify and enforce `$HOME` as sole user config location
- refactor(llm): rename provider to transport and clarify configuration
- docs: clarify LLM Proxy config and usage in setup instructions
- feat(cli): add global LLM config defaults and override logic for commands
- docs: clarify LLM Proxy configuration and update option names in README
- docs: update changelog with global LLM config and transport/provider options
- docs: clarify llm config block and defaults in architecture overview
- docs(issues): add global LLM transport config and clarify transport semantics
- docs: add agent, issue, planning, and policy guides under .mprlab
- chore: remove legacy process and issue tracking markdown files
- feat(llm): add provider selection and default handling for commit messages
- docs: update policy references and clarify init workflow in documentation
- refactor(cli): support configurable LLM provider and update defaults
- docs: clarify LLM provider config and update usage instructions in README
- docs: update changelog with LLM provider defaults and governance doc changes
- docs: update POLICY.md reference to .mprlab/POLICY.md in architecture guide
- docs: update AGENTS.md for .mprlab/ migration and add governance section
- docs: remove obsolete .mprl agent and policy guideline documents
- chore: ignore .mprlab/PLAN.md for local planning scratch files
- Release v0.6.10
- Merge pull request #372 from tyemirov/tyemirov/maintenance/update-llm-proxy-api-base
- docs: update CSP connect-src URL to llm-proxy-api.mprlab.com
- feat(llmclient): add llmproxy client factory with proxy support
- chore: update dependencies in go.sum to latest versions
- chore: update dependencies and add llm-proxy module
- chore: switch LLM client to llmclient package and update defaults
- Release v0.6.9
- docs: update ISSUES.md with latest changes
- Merge pull request #371 from tyemirov/tyemirov/bugfix/gix-sync-dirty-base-switch
- fix(syncflow): allow dirty master branch with local commits ahead of remote
- fix(syncflow): reject dirty sync if local base branch is ahead of remote
- test: improve sync integration test for dirty master worktree
- fix(syncflow): create dirty sync branch from current checkout if matching base
- refactor: simplify preset configuration loading logic in workflow run command
- docs: add forward-only contract discipline to AGENTS.md
- Release v0.6.8
- Merge pull request #370 from tyemirov/tyemirov/bugfix/B013-sync-chained-pr-base
- Filter sync pull request lookup by head
- Fix sync chained PR base detection
- Release v0.6.7
- Merge pull request #369 from tyemirov/bugfix/B012-sync-missing-pr-single-error
- fix(strict-sync): handle pruned merged branches by syncing base branch
- test(syncflow): parameterize TestCommandSuppressesWorkflowFailureEcho with multiple error cases
- feat(syncflow): add prompt and handle merged PR sync fallback
- Release v0.6.6
- Merge pull request #368 from tyemirov/bugfix/B011-sync-restores-tracked-ignored-dirty-paths
- Cover B011 sync restore failure modes
- Tighten B011 sync integration coverage
- Fix B011 tracked ignored sync cleanup
- Release v0.6.5
- Merge pull request #367 from tyemirov/bugfix/B010-sync-tracked-ignored-pathspecs
- Add tracked ignored sync integration coverage
- Refactor ignored sync tests table coverage
- Fix tracked ignored sync path filtering
- Release v0.6.4
- Merge pull request #366 from tyemirov/bugfix/B009-sync-ignored-dirty-paths
- Fix ignored-only sync dirty detection
- Fix B009 sync ignored path staging
- Release v0.6.3
- Merge pull request #365 from tyemirov/bugfix/I009-stale-generated-sync-branch
- test: use variable for expected branch name in sync refresh test
- fix(sync): avoid stale branch collisions by using semantic branch names
- refactor(syncflow): generate sync branch names using commit messages
- Release v0.6.2
- Merge pull request #364 from tyemirov/bugfix/B008-sync-sibling-worktree-adoption
- Centralize B008 worktree adoption retry service
- Fix B008 strict sync sibling worktree adoption
- Release v0.6.1
- Merge pull request #363 from tyemirov/improvement/I008-implementation-unification
- Unify workflow action option builders
- Release v0.6.0
- Merge pull request #362 from tyemirov/improvement/I007-purposeful-sync-pr-descriptions
- Expose I007 sync PR metadata controls
- Fix I007 sync PR body generation ordering
- Generate I007 sync PR descriptions from branch diffs
- Improve I007 sync pull request descriptions
- Release v0.5.0
- Merge pull request #361 from tyemirov/improvement/I006-new-sync-semantics
- Clarify I006 sync dirty-work modifiers
- Fix I006 strict sync commit modifier
- Implement dirty gix sync semantics
- Merge pull request #360 from tyemirov/improvement/I006-new-sync-semantics
- preserve dirty sync worktree
- implement new sync semantics
- Release v0.4.0
- Merge pull request #359 from tyemirov/improvement/I005-rename-cd-to-sync
- feat: rename cd command to sync
- Release v0.3.5
- Merge pull request #358 from tyemirov/tyemirov/bugfix/B006-cd-adopt-existing-worktree
- fix: push clean adopted branches without upstream
- fix: adopt sibling worktrees in cd
- Release v0.3.4
- Merge pull request #357 from tyemirov/bugfix/B005-cd-refresh-with-dirty-worktree
- Fix B005 cd dirty worktree fast-forward refresh
- Merge pull request #355 from tyemirov/codex/audit-only-web-ui
- feat(web): add automated changelog update and commit message generation
- refactor: remove deprecated web command execution and workflow from web interface
- chore: seed planning templates
- Merge pull request #354 from tyemirov/improvement/web-audit-workflow-surface
- Refine web audit workflow and repo browsing
- Merge pull request #353 from tyemirov/improvement/F003-web-startup-asset-verification
- Refine the web audit flow and folder explorer
- Improve F003 web startup asset verification
- Merge pull request #352 from tyemirov/feature/F008-web-audit-sync-protocol
- Merge pull request #351 from tyemirov/feature/F009-web-repo-tree
- fix(web): normalize SSH protocol audit flow
- fix(web): refine repository tree interactions
- fix(web): keep audit action labels visible
- fix(web): make current repo tree behave like explorer
- fix(web): reveal higher folders in current repo tree
- fix(web): show current repo in explorer tree
- fix(web): move repository tree into left sidebar
- chore(config): update repo-local workflow defaults
- Hide nested repositories from the web tree
- Implement F009 repository tree explorer
- feat(web): add audit column filters
- style(web): wrap dirty audit files in narrow column
- fix(web): sync audit draft roots with inspection
- fix(web): route audit runs through inspection
- Merge pull request #346 from tyemirov/feature/web-audit-results
- Merge pull request #350 from tyemirov/feature/F008-web-audit-sync-protocol
- test(web): harden browser startup in ci
- Merge pull request #347 from tyemirov/feature/F005-web-audit-queue-foundation
- Merge pull request #348 from tyemirov/feature/F007-web-audit-delete-folder
- Merge pull request #349 from tyemirov/feature/F008-web-audit-sync-protocol
- fix(web): preserve audit apply scope and skipped outcomes
- feat(web): queue audit sync and protocol fixes
- feat(web): queue audit folder deletions
- feat(web): queue audit remediations before apply
- feat(web): render audit results in the web client
- Merge pull request #345 from tyemirov/feature/F003-web-interface-mvp
- Align web task copy with commands
- Filter task-owned advanced commands
- Reflow the web workspace into full-width task and runner rows
- Collapse low-information target card states
- Compact repository list rows
- Compact the repository scope controls
- Tighten the selected repo summary layout
- Merge repository scope into the repos panel
- Reflow the web interface into row-based panels
- Move named ref selection into the target bar
- Add browser coverage for web control surface
- Add web workflow and remote tasks
- Refactor web actions into task workspace
- Polish ref browser rendering
- Migrate web branch panel into ref browser
- Redesign web targets around repo context
- Add scope-aware file operation drafts
- Redesign web UI around repository context
- Filter inactionable web operations
- Fix web UI root route and add branch panel
- Add F003 local web interface for gix
- Release v0.3.3
- Merge pull request #344 from tyemirov/fix/proprietary-licensing-workflow
- fix: preserve branch captures and add proprietary licensing workflow
- Future development
- Future development
- chore(preflight): checkpoint dirty files before polish
- chore: seed planning templates
- Google Analytics tags added
- Future development
- Future development
- Future development
- Rlease v0.3.2
- Merge pull request #343 from tyemirov/bugfix/GX-355-prs-delete-failure-details
- Fix GX-355 prs delete failure accounting
- Release v0.3.1
- Merge pull request #339 from tyemirov/bugfix/GX-354-workflow-glob-root-match
- Merge pull request #340 from tyemirov/bugfix/GX-346-prs-delete-output
- Merge pull request #341 from tyemirov/bugfix/GX-345-workflow-discovery-output
- Fix GX-345 workflow discovery output
- Fix GX-346 prs delete output
- Fix GX-354 workflow glob root matches
- Future development
- Future development
- Future development
- Future development
- Future development
- Future development
- Future development
- Future development
- Future development
- Merge pull request #338 from tyemirov/improvement/GX-254-license-templates
- Merge branch 'master' into improvement/GX-254-license-templates
- Release v0.3.0
- Release v0.3.0
- Merge pull request #337 from tyemirov/maintenance/GX-423-docs-cleanup
- Merge branch 'master' into maintenance/GX-423-docs-cleanup
- Merge pull request #336 from tyemirov/improvement/GX-254-license-templates
- Improve GX-254 license workflow templates
- Release candidate v0.3.0-rc17
- Merge pull request #334 from tyemirov/bugfix/GX-353-clean-commandfailed-output
- Merge pull request #329 from tyemirov/feature/GX-110-workflow-command-step
- Merge pull request #330 from tyemirov/improvement/GX-252-account-rename-deps
- Merge pull request #331 from tyemirov/improvement/GX-253-account-rename-go-version
- Merge pull request #332 from tyemirov/improvement/GX-251-workflow-summary
- Merge pull request #333 from tyemirov/maintenance/GX-423-docs-cleanup
- Future development
- feat(docs): introduce summary format alignment; move validation to edge
- feat(reporting): refine workflow summary output; move validation to edge
- feat(workflow): introduce go version update step; move validation to edge
- feat(workflow): introduce go module bump steps; move validation to edge
- Future development
- feat(workflow): add command run step; move validation to edge
- Future Development
- future development
- Future development
- Improved automated coding flow
- Merge pull request #328 from tyemirov/bugfix/GX-353-clean-commandfailed-output
- gitignore workflow extended with often ignored folders
- Fix GX-353: trim blank stderr lines in execshell errors
- Release candidate v0.3.0-rc16
- Merge pull request #326 from tyemirov/bugfix/GX-352-cd-stash-untracked
- Fix GX-251: cd --stash handles untracked files
- Merge pull request #325 from tyemirov/bugfix/GX-351-require-changes-after-commit
- Fix GX-351: require_changes tracks workflow commits
- Future development
- Release candidate v0.3.0-rc15
- Merge pull request #324 from tyemirov/bugfix/GX-350-succinct-non-workflow-logs
- Fix GX-350: restore concise non-workflow logging
- Merge pull request #322 from tyemirov/maintenance/GX-499-issues-cleanup
- Merge pull request #323 from tyemirov/bugfix/GX-349-non-workflow-logging
- Merge branch 'maintenance/GX-499-issues-cleanup' into bugfix/GX-349-non-workflow-logging
- Future development
- Fix GX-349: restore non-workflow logging
- Maintenance: archive resolved issues
- Docs: sync README/ARCHITECTURE with completed issues
- Future development
- AGENTS.md moved to root folder for easy discoverability
- Release Candidate v0.3.0-rc14
- Merge pull request #321 from tyemirov/improvement/GX-261-migrate-llm-utils
- chore: update go.mod and go.sum dependencies
- Docs: clarify GX-261 uses utils/llm
- Improve GX-261: migrate LLM package to utils
- Future development
- Future development
- Merge pull request #317 from tyemirov/maintenance/GX-345-logging-issues
- Merge pull request #318 from tyemirov/bugfix/GX-346-workflow-yaml
- Merge pull request #319 from tyemirov/bugfix/GX-347-workflow-summary
- Merge pull request #320 from tyemirov/bugfix/GX-348-step-outcomes
- Fix GX-348 step outcomes and reasons
- Fix GX-347 workflow end summary
- Fix GX-346 workflow YAML step logs
- Log GX-345..348 logging follow-ups
- Release Candidate v0.3.0-rc13
- Merge pull request #316 from tyemirov/bugfix/GX-344-step-logging
- Avoid duplicate step summaries in workflows
- Improve step-centric workflow logging
- Refine step summary reasons
- Improve GX-259 step outcome logging
- Release candidate v0.3.0-rc12
- Merge pull request #315 from tyemirov/bugfix/GX-344-step-logging
- Improved flow for automated coding
- Future development
- Merge pull request #314 from tyemirov/bugfix/GX-344-step-logging
- Inject git manager stub into remotes step reporter test
- Fix GX-344 step reporter test git manager
- Fix GX-344 workflow step name logging
- Future development
- Release Candidate v0.3.0-rc11
- Merge pull request #313 from tyemirov/bugfix/GX-259-branch-cleanup-default-base
- GX-259: label safeguard task skips by step
- GX-259: track step name per repository stage
- Mark GX-259 step-based logging as complete
- GX-259: propagate workflow step names into logging
- Merge pull request #312 from tyemirov/bugfix/GX-259-branch-cleanup-default-base
- Strengthen GX-260 branch cleanup default-base test
- Clarify GX-259 logging vs GX-260 branch cleanup
- Fix GX-259 branch cleanup default base
- Future improvements
- Release Candidate v0.3.0-rc10
- Merge pull request #310 from tyemirov/improvement/GX-252-gitignore-safeguards
- Merge pull request #311 from tyemirov/improvement/GX-258-branch-cleanup
- improvement: GX-258 keep switch-branch capture when on master
- Merge pull request #309 from tyemirov/improvement/GX-258-branch-cleanup
- improvement: GX-258 expose branch-not safeguard in DSL
- improvement: GX-258 refine branch cleanup semantics
- improvement: GX-258 branch cleanup for no-op workflows
- Merge pull request #308 from tyemirov/improvement/GX-252-gitignore-safeguards
- improvement(GX-252): add require_changes safeguards to gitignore preset
- candidate release v0.3.0-rc9
- improvement(GX-252): clarify per-step no-op messages
- improvement(GX-252): treat safeguard skips as non-issues
- improvement(GX-252): nest issues under step output
- Merge pull request #307 from tyemirov/improvement/GX-252-safeguard-messaging
- Release Candidate v0.3.0-rc8
- improvement(GX-252): clarify require_changes safeguard messaging
- Release Candidate v0.3.0-rc7
- Web site improvements
- Footer improvements
- Merge pull request #306 from tyemirov/feature/GX-110-docs-site
- feature(GX-110): add About gix modal
- feature(GX-110): align footer links with mpr-ui demo
- feature(GX-110): integrate mpr-ui footer
- Create CNAME
- Merge pull request #305 from tyemirov/feature/GX-110-docs-site
- feature(GX-110): move docs site for GitHub Pages
- Future development
- Future development
- Merge pull request #304 from tyemirov/improvement/GX-252-safeguards
- improvement: require explicit safeguards for git stage
- Release Candidate v0.3.0-rc5
- Merge pull request #301 from tyemirov/bugfix/GX-343-changelog-message
- Merge pull request #302 from tyemirov/improvement/GX-252-cd-formatting
- test: drop unused changelog executor stub
- Merge branch 'bugfix/GX-257-namespace-mutations' into bugfix/GX-343-changelog-message
- bugfix: avoid duplicate changelog no-change notice
- bugfix: track namespace mutated files
- Merge pull request #298 from tyemirov/bugfix/GX-342-workflow-ignore
- Future development
- bugfix: ignore non-repo check-ignore errors
- Future development
- Release candidate v0.3.0-rc4
- Merge pull request #296 from tyemirov/improvement/GX-341-account-rename
- Merge pull request #297 from tyemirov/improvement/GX-256-cd-logging
- improvement: fix GX-256 cd summary and GX-257 staging
- improvement: fix GX-341 recursive replacements
- Future development
- Future improvements
- conflicts resolved
- Release candidate v0.3.0-rc3
- Merge pull request #294 from tyemirov/improvement/GX-254-session-prompts
- chore(prompts): lowercase apply-all indicator
- Merge pull request #293 from tyemirov/improvement/GX-254-session-prompts
- feat(prompts): add session apply-all prompter
- Release Candidate v0.3.0-rc2
- Merge pull request #292 from tyemirov/feature/GX-235-track-remote
- feat(domain): introduce TrackingRemoteConfigurator smart constructor; move validation to edge
- Future development
- Future development
- Release v0.3.0-rc1
- Merge pull request #291 from tyemirov/improvement/GX-254-accept-all-prompts
- improvement(GX-254): add accept-all confirmation option
- Release v0.2.11
- Merge pull request #290 from tyemirov/improvement/GX-253-hide-cd-refresh-flag
- Fix cd refresh default and add integration coverage
- improvement(GX-253): hide gix cd refresh flag
- Future development
- Release v0.2.10
- Merge pull request #289 from tyemirov/improvement/GX-252-cd-output
- Improve GX-252 gix cd output
- Release v0.2.9
- Merge pull request #287 from tyemirov/bugfix/GX-340-remote-detection
- Merge pull request #288 from tyemirov/improvement/GX-251-stash-switch
- Handle multiple stashes during branch change refresh
- Improve GX-251 gix cd stash flow
- Fix GX-340 gix cd remote selection
- Future development
- Future development
- release v0.2.8
- Merge pull request #285 from tyemirov/maintenance/log-untracked-refresh
- Log untracked worktree entries during branch refresh checks
- Merge pull request #282 from tyemirov/improvement/GX-249-require-clean
- Honor ignore_dirty_paths in worktree filters
- Merge pull request #283 from tyemirov/improvement/GX-250-branch-change
- Update branch change refresh expectations
- Skip branch refresh pull on dirty worktrees
- Merge pull request #284 from tyemirov/maintenance/next-issue
- Add clean-worktree guard to account rename preset
- Update workflow presets for relaxed branch change cleanliness
- Align branch change refresh with git switch semantics
- Normalize require_clean checks with shared worktree helper
- Future development
- Role added
- Release v0.2.7
- Merge pull request #281 from tyemirov/bugfix/cd-preclean-refresh
- Go formatting
- improvement: accept yes/no toggles for all bool flags
- Allow stash/commit refresh options to skip pre-clean failure
- bugfix: block branch change when refresh requires clean
- Release v0.2.6
- Merge pull request #280 from tyemirov/bugfix/gitignore-apply-require-clean
- bugfix: honor ignore_dirty_paths in gitignore apply
- Release v0.2.5
- Merge pull request #279 from tyemirov/improvement/GX-251-cd-default-refresh
- improvement: default cd refresh clean
- account rename changes the account name in md files
- Merge pull request #278 from tyemirov/improvement/GX-250-ignore-dirty-guard
- Accept string require_clean values
- improvement: nest require_clean ignore patterns
- Merge pull request #277 from tyemirov/automation/gitignore/gix-20251119T204632
- chore(workflows): capture/restore initial branch
- Release v0.2.4
- Merge pull request #275 from tyemirov/automation/gitignore/gix-20251119T073841
- Protect capture state per repository
- Merge pull request #276 from tyemirov/improvement/GX-249-capture-dsl
- Update capture DSL and coverage
- fix: add missing imports for capture storage
- feat(workflow): refine capture DSL with named vars
- chore: align capture DSL with kind keyword
- chore(workflows): restore original commit after gitignore/account flows
- feat(workflow): capture/restore support for branch change
- Release v0.2.3
- Merge pull request #274 from tyemirov/improvement/GX-247-ignore-dirty
- Respect status codes when filtering dirty ignores
- feat(workflow): allow ignore_dirty_paths safeguard
- .gitignore workflow includes additional service files
- chore: ensure gitignore entries
- Release v0.2.2
- account renaming flow improved
- Release v0.2.1
- .gitignore workflow includes additional service files
- Merge pull request #273 from tyemirov/automation/gitignore/gix-20251119T015858
- chore: ensure gitignore entries
- Release v0.2.2
- Merge pull request #271 from tyemirov/maintenance/GX-422-cli-doc
- Merge pull request #270 from tyemirov/improvement/GX-246-audit-dirty-report-stacked
- feat(audit): surface worktree dirty files for GX-246
- account renaming flow improved
- Release v0.2.1
- Merge pull request #266 from tyemirov/bugfix/GX-337-workflow-replacements
- Merge pull request #267 from tyemirov/maintenance/GX-338-module-rename
- Merge pull request #268 from tyemirov/maintenance/GX-339-go-version-docs
- Merge pull request #269 from tyemirov/maintenance/GX-422-cli-doc
- docs: refresh CLI design for GX-422
- docs: clarify Go 1.25 baseline for GX-339
- chore: align module namespace for GX-338
- fix: resolve GX-337 recursive replacements
- Future development
- Release v0.2.0
- Merge pull request #264 from tyemirov/bugfix/GX-421-human-logging
- bugfix: remove legacy workflow logging
- Merge pull request #263 from tyemirov/bugfix/GX-420-human-logging
- fix(logging): force workflow human formatter
- Merge pull request #261 from tyemirov/bugfix/GX-419-files-replace-glob
- Merge branch 'master' into bugfix/GX-419-files-replace-glob
- docs(changelog): mention GX-419
- fix(taskfiles): support replace mode globs
- fix(cleanup): use replace action for go globs
- Merge pull request #260 from tyemirov/bugfix/GX-418-changelog-logging
- fix(changelog+commit): suppress workflow logging
- Merge pull request #259 from tyemirov/maintenance/update-issues-gx-417
- docs(issues): mark GX-417 resolved
- Merge pull request #258 from tyemirov/maintenance/GX-417-files-command-tests
- test(files-add): cover negative paths
- Future development
- Merge pull request #257 from tyemirov/maintenance/GX-416-taskrunner-environments
- refactor(taskrunner): split dependency builders
- Merge pull request #256 from tyemirov/maintenance/GX-415-domain-smart-constructors
- fix(history): drop unused helper
- feat(history+rename): enforce typed options
- Futrue development
- Merge pull request #255 from tyemirov/maintenance/GX-414-typed-preset-builders
- maintenance: add typed preset builders
- Merge pull request #254 from tyemirov/maintenance/GX-413-preset-helper
- test: update audit fallback expectation
- audit: avoid network fallback for default branch
- Merge pull request #253 from tyemirov/maintenance/GX-413-preset-helper
- maintenance: drop unused preset helpers
- Merge remote-tracking branch 'origin/maintenance/GX-413-preset-helper' into maintenance/GX-413-preset-helper
- maintenance: add preset workflow helper
- maintenance: add preset workflow helper
- Merge pull request #252 from tyemirov/maintenance/GX-412-policy-review
- Future development
- Merge pull request #251 from tyemirov/maintenance/GX-412-policy-review
- Merge pull request #250 from tyemirov/bugfix/GX-343-workflow-cleanup
- docs: add GX-412 refactor plan
- Merge remote-tracking branch 'origin/bugfix/GX-343-workflow-cleanup' into bugfix/GX-343-workflow-cleanup
- Fix GX-343 repo workflow executor wiring
- Merge pull request #247 from tyemirov/bugfix/GX-344-files-replace
- Merge pull request #248 from tyemirov/bugfix/GX-336-logging-refresh
- Gate history variable overrides to history actions
- bugfix(GX-336): refine workflow human-readable logging
- Merge pull request #246 from tyemirov/bugfix/GX-345-safeguards
- bugfix(GX-345): add hard-stop vs soft-skip safeguards
- Merge pull request #244 from tyemirov/bugfix/GX-342-release-presets
- Merge pull request #245 from tyemirov/bugfix/GX-344-files-replace
- Wire workflow preset variables and add integration coverage
- bugfix(GX-344): convert repo-files-replace to workflow preset
- Merge pull request #243 from tyemirov/bugfix/GX-343-workflow-cleanup
- bugfix(GX-343): remove bespoke repo workflow helpers
- Merge pull request #242 from tyemirov/bugfix/GX-342-release-presets
- Validate retag mapping inputs
- bugfix(GX-342): convert release commands to workflow presets
- Merge pull request #241 from tyemirov/bugfix/GX-341-files-add-preset
- Ensure repo files add skips pushes when disabled
- bugfix(GX-341): convert repo-files-add to preset
- Future development
- Merge pull request #238 from tyemirov/bugfix/GX-338-remote-preset
- Propagate owner variable to canonical workflow preset
- bugfix(GX-338): convert remote update to workflow preset
- Merge pull request #234 from tyemirov/improvement/GX-336-workflow-logging
- Preserve severity indicators for phase events
- Merge pull request #235 from tyemirov/improvement/GX-337-folder-rename-preset
- Ensure folder-rename preset uses boolean options
- Merge pull request #236 from tyemirov/improvement/GX-345-safeguard-dsl
- Merge pull request #237 from tyemirov/improvement/GX-346-docs-refresh
- improvement: sync repo playbook docs (GX-346)
- improvement: track safeguard hard-stop vs soft-skip work (GX-345)
- improvement: turn repo-folders-rename into preset (GX-337)
- improvement: redesign workflow logging (GX-336)
- Future development
- Merge pull request #233 from tyemirov/bugfix/workflow-header-format
- Handle missing audit service during repository refresh
- fix: clean workflow logging headers
- Future development
- feat(workflow): add concurrent repository execution
- Future development
- Candidate release v0.2.0-rc.12
- Configs are committed to the version control system
- Merge pull request #232 from tyemirov/bugfix/GX-335-append-env-line
- bugfix(workflow): ensure append-if-missing matches literal lines
- Future development
- Merge pull request #226 from tyemirov/bugfix/GX-332-workflow-logging
- Merge pull request #230 from tyemirov/bugfix/GX-334-pull-new-branch
- Merge pull request #229 from tyemirov/improvement/GX-333-concise-logging
- fix: output path before message in summaries
- fix: include path in workflow event summary
- fix: emit workflow event summaries
- fix: log formatter includes event summaries
- bugfix: skip pull on new branches
- improvement: add concise workflow logging
- bugfix: hide workflow stage logs
- Merge pull request #222 from tyemirov/bugfix/GX-330-append-if-missing
- Merge pull request #224 from tyemirov/bugfix/GX-331-workflow-skip
- Stop repository stages after safeguard skip
- bugfix: respect workflow skips
- bugfix: normalize append-if-missing
- Future development
- Future development
- Future Development
- Merge pull request #221 from tyemirov/improvement/GX-327-workflow-per-repo
- feat(workflow): execute tasks per repository
- Merge pull request #218 from tyemirov/improvement/GX-327-workflow-per-repo
- Deduplicate repositories in repository-scoped stages
- feat(workflow): run repository-scoped stages per repo
- Merge pull request #216 from tyemirov/bugfix/GX-325-branch-change-tracking
- fix(cd): treat rev-parse missing revision as branch absence
- fix(cd): avoid tracking remote refs for new automation branches
- Merge pull request #215 from tyemirov/improvement/GX-324-missing-workflow-commands
- fix(workflow): execute workflow operations directly
- Future development
- Merge pull request #214 from tyemirov/improvement/GX-238-workflow-actions
- fix(workflow): honor ensure_clean for stage actions
- feat(workflow): seed run id for branch templates
- fix(workflow): remove branch-prepare command
- feat(workflow): add atomic git command steps
- improve(execshell): show command context on failure
- Release candidate v0.2.0-rc.11
- Merge pull request #213 from tyemirov/feature/append-if-missing
- refactor(workflow): drop legacy line-edit mode
- fix(workflow): accept legacy line-edit mode
- feat(workflow): rename line-edit mode to append-if-missing
- chore: rename ensure-lines mode to line-edit
- feat(workflow): add ensure-lines file mode
- llm and taskrunner packages documentation
- Release candidate v0.2.0-rc.9
- chore(cli): drop legacy command wrappers
- Release Candidate v0.2.0-rc.9
- fix(workflow): restore branch after tasks
- Merge pull request #211 from tyemirov/improvement/GX-227-files-rm
- Merge pull request #209 from tyemirov/improvement/GX-229-cli-bootstrap
- fix(workflow): restore ensure_clean override parsing
- Merge pull request #205 from tyemirov/improvement/GX-229-cli-bootstrap
- Merge master into improvement/GX-229-cli-bootstrap
- Merge pull request #204 from tyemirov/improvement/GX-228-embedded-workflows-nested
- Merge pull request #206 from tyemirov/improvement/GX-230-workflow-outcome
- fix(migrate): wire GitHub resolver for default
- fix(taskrunner): allow skipping GitHub resolver
- Merge pull request #207 from tyemirov/improvement/GX-231-task-operations
- Merge pull request #208 from tyemirov/improvement/GX-236-runtime-variables
- formatting
- fix(workflow): skip push when remote missing
- feat(workflow): honor runtime variable precedence
- feat(workflow): layer task operations
- feat(workflow): surface execution outcomes and stage metrics
- GX-229: unify CLI commands on pkg/taskrunner
- GX-227: move history purge under files namespace
- GX-226: embed namespace workflow preset
- chore(cli): drop unused helper functions
- feat(cli): modularize bootstrap and task runner wiring
- WIP
- Future development
- refactor(cli): scope history purge under files namespace
- feat(workflow): drop namespace CLI in favor of preset
- Merge pull request #198 from tyemirov/improvement/GX-234-test-targets
- Merge pull request #200 from tyemirov/improvement/GX-225-license-workflow-embedded
- improvement(GX-225): delegate license command to workflow
- improvement(GX-236): add workflow runtime variables
- improvement(GX-228): add embedded workflow presets
- improvement(GX-234): split fast/slow tests
- improvement(GX-233): expand structured reporter telemetry
- Merge pull request #195 from tyemirov/improvement/GX-224-message-commit
- feat(cli): move commit generator under message namespace
- improvement: move changelog generator under message namespace
- improvement: rename branch-default command to default
- Future development
- Centralize LLM client factory
- Future development
- Merge pull request #194 from tyemirov/improvement/GX-219-remove-dry-run-cleanup
- BugFix: pre-requsites
- tests: skip history purge when git filter-repo missing
- Bugfix: unused functions
- Merge pull request #193 from tyemirov/improvement/GX-219-remove-dry-run-cleanup
- tests: update repos integration expectations
- tests: auto-confirm destructive operations
- docs: scrub dry-run references
- cli: remove dry-run support
- backend: remove dry-run plumbing
- workflow: remove dry-run support
- Future development
- Merge pull request #191 from tyemirov/improvement/GX-220-rename-cd
- feat(branches): fold refresh behaviors into cd command
- improvement: rename cd command and add default fallback
- Future development
- Details of why the tree is "dirty" added to the output
- Candidate release v0.2.0-rc.8
- BugFix: do not perform further checks after remote destination branch matches the target
- Future development
- Future development
- Release candidate  v0.2.0-rc.7
- Future development
- Merge pull request #188 from tyemirov/bugfix/GX-321-changelog-summary
- Merge pull request #189 from tyemirov/bugfix/GX-322-workflow-resilience
- Merge pull request #190 from tyemirov/maintenance/GX-411-policy-review
- aggreagtion error fixed
- GX-411 capture refactor roadmap
- GX-322 keep workflow execution running after failures
- GX-321 ensure changelog summary counts repositories
- Merge pull request #187 from tyemirov/bugfix/GX-320-namespace-rewrite-message
- GX-320 document resolution
- GX-320 clarify namespace rewrite skip reason
- Merge pull request #186 from tyemirov/improvement/GX-218-remove-top-level
- GX-218 restructure CLI command surface
- Merge pull request #185 from tyemirov/improvement/GX-217-summary-duration
- improvement(reporting): fix workflow summary totals
- Future development
- Future development
- Merge pull request #184 from tyemirov/improvement/GX-216-dirty-details
- improvement(workflow): surface dirty worktree details
- Merge pull request #183 from tyemirov/improvement/GX-215-llm-workflow-variables
- feat(workflow): expose llm configuration and task variables
- Merge pull request #182 from tyemirov/improvement/GX-212-summary-warnings
- Future development
- Future development
- improvement: surface dirty worktree details
- Future development
- Future development
- Release candidate v0.2.0-rc.6
- Merge pull request #181 from tyemirov/feature/GX-22-license-injection
- Merge pull request #173 from tyemirov/feature/GX-23-git-retag
- fix(release): emit retag mappings as []any for workflow
- Merge pull request #179 from tyemirov/improvement/GX-105-workflow-dsl
- Merge pull request #180 from tyemirov/bugfix/GX-318-namespace-skip
- Merge pull request #177 from tyemirov/bugfix/GX-318-namespace-skip
- Merge pull request #176 from tyemirov/bugfix/GX-319-prs-delete-flag
- fix(files-add): respect CLI roots and warn on positional args
- Merge pull request #178 from tyemirov/maintenance/staticcheck-lint
- chore(lint): fix staticcheck warning and document lint step
- test(branches): cover --yes value edge-case
- fix(namespace): clarify skip reasons and tolerate missing metadata
- refactor(workflow): adopt command-path DSL; remove legacy operation keys
- Future development
- feat(repo): add files add command
- feat(repo): add release retag workflow
- feat(repo): add license apply command
- Future development
- Future development
- Future development
- Merge pull request #169 from tyemirov/improvement/GX-317-error-messages
- improvement: clarify release error reporting (GX-317)
- mac OS dependencies for tests fixed
- Future development
- Future development
- Future development
- Release v0.2.0-rc.4
- Merge pull request #168 from tyemirov/bugfix/GX-316-branch-pull
- Future development
- Merge pull request #167 from tyemirov/bugfix/GX-316-branch-pull
- Formatting
- fix(workflow): deduplicate branch change reporting
- Future development
- fix(workflow): handle missing start point branches
- Future development
- fix(workflow): deduplicate changelog action execution
- fix(namespace): rewrite Go test files during namespace updates
- Future development
- Release v0.2.0-rc.3
- Future development
- Merge pull request #160 from tyemirov/bugfix/GX-212-log-format
- Merge pull request #161 from tyemirov/bugfix/GX-313-skip-gitignored
- bugfix(gitignore): skip gitignored nested repositories
- perf: batch git check-ignore calls during namespace rewrite
- fix: skip gitignored files during namespace rewrite
- refactor: join sentinel with detail errors
- feat: attach git stderr to namespace error events
- fix: ensure namespace errors log structured output
- feat: add structured workflow logging
- Future development
- bugfix(protocol): display git-style remote URLs in logs
- Future development
- Merge pull request #158 from tyemirov/bugfix/GX-310-namespace-push
- Merge pull request #159 from tyemirov/bugfix/GX-311-namespace-logs
- chore: note namespace push safeguards in issues log
- bugfix(namespace): skip push when remote missing or up to date
- bugfix(workflow): render namespace logs with real newlines
- bugfix(namespace): handle namespace push failures
- Future development
- Workflow examples
- Configs are too specific to ba added to git
- Future development
- Workflow examples
- Future development
- Workflows described
- Future development
- full list of gix commands in the README.md
- Future development
- go v 1.25
- Merge pull request #157 from tyemirov/test/branch-default-nested-no-remote
- Add coverage for default branch on mixed nested repos
- Merge pull request #156 from tyemirov/fix/default-branch-missing-remote
- Handle default-branch updates without remotes
- Merge pull request #149 from tyemirov/improvement/GX-205-error-schema
- Merge pull request #155 from tyemirov/improvement/GX-206-error-prefix
- Merge pull request #154 from tyemirov/improvement/GX-207-workflow-dag
- Merge pull request #152 from tyemirov/improvement/GX-208-remote-metadata
- Merge pull request #153 from tyemirov/bugfix/GX-304-no-remote
- Merge pull request #148 from tyemirov/improvement/GX-206-error-prefix
- Merge pull request #151 from tyemirov/improvement/GX-207-workflow-dag
- Honor workflow dependencies when building tasks
- Merge pull request #150 from tyemirov/bugfix/GX-304-no-remote
- chore: address staticcheck suggestions
- Typo
- fix(workflow): avoid unused longestMatch lint warning
- docs(issues): acknowledge no-remote handling coverage
- test(cli): cover no-remote branch workflows
- improvement(workflow): add dag execution with parallel stages
- improvement(workflow): enforce remote skip policy
- improvement(workflow): remove workflow operation prefixes
- improvement(workflow): standardize repository error messaging
- Future development
- Future development
- Future development
- Future development
- Merge pull request #146 from tyemirov/bugfix/GX-304-branch-default-workflow
- fix(branch-default): handle inaccessible remotes gracefully
- Merge pull request #145 from tyemirov/bugfix/GX-304-branch-default-workflow
- refactor(remote): centralize remote identity normalization
- fix(branch-default): normalize identifiers via repo metadata
- fix(branch-default): canonicalize renamed repository identifiers
- Merge pull request #144 from tyemirov/bugfix/GX-304-branch-default-workflow
- fix(branch-default): derive repository identifier from remote URL
- fix(branch-default): tolerate repos without canonical identifier
- Merge pull request #143 from tyemirov/bugfix/GX-304-branch-default-workflow
- feat(branch-default): ensure workflow creates missing branches
- Merge pull request #139 from tyemirov/feature/GX-100-namespace-rewrite
- fix: detect namespace root in go.mod
- merge: bring safeguards into namespace rewrite
- fix: retain namespace commit message during merge
- test(workflow): cover namespace safeguards
- test: use canonical assume-yes flag in namespace CLI test
- Merge pull request #141 from tyemirov/codex/github-mention-improvement-add-workflow-safeguards
- Fix namespace rewrite options and go.mod handling
- chore: remove unused safeguard helpers
- fix: repair namespace safeguard wiring
- fix: accept branch prefix hyphen option
- fix: rewrite go.mod block entries
- improvement(workflow): add reusable safeguards
- Fix go.mod namespace rewrite blocks
- feat(repo): add namespace rewrite workflow action and CLI
- Future development
- Merge pull request #138 from tyemirov/bugfix/GX-303-token-default
- Allow optional GitHub CLI commands without token
- Merge pull request #137 from tyemirov/bugfix/GX-303-prs-delete-hang
- refactor(github): centralize token guard behavior
- bugfix(migrate): require GitHub token for default branch
- Merge pull request #136 from tyemirov/improvement/GX-203-version-command
- Future development
- bugfix(branches): prevent repo prs delete hang
- improvement(cli): align version command with flag output
- Future development
- Merge pull request #135 from tyemirov/bugfix/GX-302-actionable-messaging
- Merge branch 'master' into bugfix/GX-302-actionable-messaging
- Merge pull request #134 from tyemirov/bugfix/GX-302-tests
- fix: refine missing remote detection
- Formatting
- fix: ensure CLI warnings and outputs match expectations
- fix(branch-cd): collapse warning summaries to one line
- Merge pull request #133 from tyemirov/bugfix/GX-302-actionable-messaging
- fix(branch-cd): include repository path in skip warnings
- Merge pull request #132 from tyemirov/bugfix/GX-302-actionable-messaging
- fix(branch-cd): surface actionable branch switch failures
- Future development
- Merge pull request #131 from tyemirov/bugfix/GX-301-actionable-default-branch-error
- fix(migrate): raise actionable default-branch errors
- Future development
- Future development
- v0.2.0-rc.1 release
- Merge pull request #130 from tyemirov/improvement/GX-202-error-messages
- improvement(branch): warn on default migration aux failures
- improvement(branch): downgrade branch cd network failures
- Merge pull request #129 from tyemirov/improvement/GX-201-warning-downgrade
- improvement(branch): warn on pages configuration failures
- feat(branch): accept positional target for default command
- Merge pull request #126 from tyemirov/maintenance/GX-406-policy-coverage
- Merge pull request #127 from tyemirov/maintenance/GX-407-docs-ci
- Merge pull request #125 from tyemirov/maintenance/GX-405-shared-helper-cleanup
- docs(ci): document domain flow and add lint gates
- test(repos): cover policy regressions for GX-406
- maint(GX-405): unify repo option helpers and reporter
- Merge pull request #124 from tyemirov/maintenance/GX-404-contextual-errors
- maint(GX-404): add contextual repo error handling
- Merge pull request #123 from tyemirov/bugfix/GX-300-branch-cd-no-remotes
- fix(branch): skip network operations without remotes
- Future development
- Merge pull request #122 from tyemirov/maintenance/GX-402-policy-review
- Merge pull request #121 from tyemirov/maintenance/GX-403-domain-types
- maintenance: [GX-403] introduce repository domain types
- Merge pull request #120 from tyemirov/feature/GX-21-replace-task
- Merge pull request #117 from tyemirov/maintenance/GX-400-readme-architecture
- Merge pull request #118 from tyemirov/maintenance/GX-401-architecture-sync
- Merge pull request #119 from tyemirov/maintenance/GX-402-policy-review
- maintenance: [GX-402] decompose refactor plan into issues
- Future development
- maintenance: [GX-402] author refactor roadmap
- maintenance: [GX-401] sync architecture doc with code
- Future development
- maintenance: [GX-400] refocus docs around user workflows
- Merge pull request #116 from tyemirov/feature/GX-21-replace-task
- fix: align history purge test with multi-path command
- Future development
- Merge branch 'master' into feature/GX-21-replace-task
- Merge branch 'origin/master' into work
- Merge pull request #115 from tyemirov/improvement/GX-14-repo-rm-task
- Fix history path detection and update tests
- Ensure history purge fetches remote refs
- feature: add repo files replace task
- workflow: normalize audit roots before discovery
- Merge pull request #114 from tyemirov/improvement/GX-07-task-runner-unification
- Fix audit roots after renames and add coverage
- improvement: add repo history purge command
- improvement: route workflow cli through task runner
- Checkpoint: route packages purge through task runner
- Checkpoint: route branch refresh through task runner
- Checkpoint: migrate LLM and branch commands to task runner
- Future development
- Checkpoint: migrate release command to workflow tasks
- v0.1.4 release
- Merge pull request #113 from tyemirov/improvement/GX-22-branch-default
- feat: promote branch default command
- Future development
- v0.1.3 release
- Merge pull request #112 from tyemirov/improvement/GX-20-init-help
- improvement: clarify init flag scope (GX-20)
- Future Development
- Future development
- Future development
- Future development
- Future development
- Future development
- Merge pull request #111 from tyemirov/gx-19-bugfix
- bugfix
- Merge pull request #110 from tyemirov/bugfix/GX-17-owner-message
- Merge pull request #106 from tyemirov/bugfix/GX-18-owner-constraint
- Merge pull request #108 from tyemirov/codex/fix-comment-in-executor.go
- Merge pull request #109 from tyemirov/bugfix/GX-19-normalized-message
- bugfix: surface already normalized rename skips
- Restore repo remote owner option and document usage
- Future development
- v0.1.2 release
- Furture development
- Merge pull request #107 from tyemirov/bugfix/GX-18-owner-constraint
- Merge branch 'master' into bugfix/GX-18-owner-constraint
- bugfix: remove owner equality guard for canonical remotes
- Bugs
- Future development
- v0.1.1 release
- Merge pull request #105 from tyemirov/bugfix/GX-17-owner-message
- Merge pull request #104 from tyemirov/bugfix/GX-16-log-levels
- bugfix: clarify owner constraint skip message
- bugfix: log configuration banner at debug level
- Bugs
- Improved autonomous flow
- bugs
- v0.1.0 Release
- Merge pull request #103 from tyemirov/bugfix/GX-15-branch-logging
- Handle default remote absence when fetching
- Merge pull request #102 from tyemirov/bugfix/GX-14-suppress-logging
- GX-15: clarify branch fetch logging
- GX-14: silence default CLI logging
- Bugs
- Future development
- Merge pull request #101 from tyemirov/improvement/GX-13-command-alignment
- GX-13: regroup commit and changelog commands
- GX-12: clarify workflow required configuration
- GX-09: refresh command catalog
- Bugs
- Future development
- Bugs
- Merge pull request #98 from tyemirov/bugfix/GX-11-branch-flag
- GX-11: remove global branch flag
- GX-11: scope branch flag to branch commands
- GX-11: document branch flag context
- Merge pull request #97 from tyemirov/bugfix/GX-10-default-roots
- cli: rely on embedded repo release defaults
- cli: preserve embedded operation defaults
- GX-10: default repo release roots
- Merge pull request #96 from tyemirov/bugfix/GX-12-required-arg-help
- GX-12: surface required branch argument
- Bugs
- Bugs
- Bugs
- Bugs
- Merge pull request #95 from tyemirov/bugfix/GX-08-release-help-args
- surface the required `<tag>` argument
- Bugs
- Preparations to v0.1.0 release
- Bugs
- Merge pull request #94 from tyemirov/feature/GX-06-release-command
- chore: format application after release command
- Feature: add release tag helper
- Merge pull request #93 from tyemirov/feature/GX-03-commit-messages
- Merge pull request #92 from tyemirov/feature/GX-04-changelog-messages
- Feature: add changelog message assistant
- Merge pull request #91 from tyemirov/feature/GX-03-commit-messages
- feature(GX-03): add LLM-backed commit message tool
- Merge pull request #90 from tyemirov/feature/GX-02-task-runner
- feature(GX-02): add workflow task runner
- Future development
- tools subfolder is ignored
- Merge pull request #89 from tyemirov/improvement/GX-01-command-syntax
- Refactor CLI command syntax
- PLAN.md ignored
- Issues prefeix made GX
- Documentation for autonomous coding agents
- Future development planned
- Automous agnets flow improved
- Release link added to the readme
- Changelog for v0.0.7 and v0.0.8 updated
- Merge pull request #87 from temirov/bugfix2
- Prompt before branch purge deletions
- Merge pull request #88 from temirov/codex/fix-comments
- Handle checkpoint commit rebase during refresh
- Support stash and commit recovery in branch refresh
- Use gix prefix in version output
- Add version detection and CLI flag
- Implement branch refresh command
- Maintenance: AGENTS updated not to stream full files into stdout
- Merge pull request #86 from temirov/bugfix1
- Handle nested repository renames before parent directories
- Documentation: explanation of config changes
- Merge pull request #85 from temirov/fixes1
- GS-02: clean toggle flag help formatting
- GS-01: advertise apply-all confirmation choice
- Update README.md
- CHANGELOG updated for v0.0.5 release
- Merge pull request #84 from temirov/fixes
- Fix repository containment check for nested repos
- agnets instructiosn for testing updated
- Make audit folder_name relative to each root
- Reorder audit CSV to lead with folder names
- Restrict audit --all to top-level non-repository directories
- Expose audit --all flag for non-repository directories
- Adopt --roots flag and prune nested repository roots
- Display audit usage when arguments are invalid
- installation details added
- Align release workflow with gix binary
- Respect XDG config env and normalize test paths
- Merge pull request #83 from temirov/boolean-flags
- Changelog for v0.0.4 Updated
- Merge pull request #82 from temirov/boolean-flags
- add flagutils toggle helpers for yes/no inputs and argument normalization   - switch rename/workflow commands and execution flag bindings to use the shared toggle parser   - cover toggle behavior with new unit tests and add a workflow config exercising rename defaults
- Merge pull request #81 from temirov/codex/update-operations_audit.go-for-path-handling
- Handle audit report nested output directories
- Merge pull request #78 from temirov/codex/add-includeowner-option-to-rename-workflow
- Merge pull request #80 from temirov/codex/enhance-remote-configuration-with-owner-flag
- Add owner constraint configuration for repo-remote-update
- Merge pull request #79 from temirov/codex/extend-filesystem-with-mkdirall-method
- Add parent directory creation to rename operations
- Maintenance: Changelog for v0.0.3 added
- Maintenance: Changelog for v0.0.2 added
- Add owner-aware repo rename support
- Merge pull request #76 from temirov/codex/update-configuration-search-path-handling
- Merge pull request #77 from temirov/codex/add-init-and-force-options-to-cli
- Add configuration initialization workflow to CLI
- Extend CLI configuration search paths
- Merge pull request #75 from temirov/flags
- Feature: the --root flag is available for all commands
- Feature: require-clean safeguard is applied to all branch level operations
- Feature: migrate command accepts from and to flags
- Feature: flags are made global and working across all of the commands
- Merge pull request #67 from temirov/codex/add-embedded-yaml-configuration-and-tests
- Merge pull request #74 from temirov/codex/refactor-confirmationprompter-return-type
- Add apply-all confirmation state and prompt result struct
- Merge pull request #73 from temirov/codex/emit-console-logging-for-initialization-banner
- Adjust CLI initialization logging for console
- Merge pull request #72 from temirov/codex/normalize-repository-paths-in-discovery
- Normalize repository discovery paths
- Merge pull request #71 from temirov/codex/add-option-to-skip-branch-data-retrieval
- Add minimal audit inspection depth and update CLI usage
- Merge pull request #70 from temirov/codex/update-logging-for-github-cli-views
- Skip redundant gh repo view start log
- Use MergeInConfig to preserve embedded defaults
- Merge pull request #69 from temirov/codex/fix-logger-flush-error-on-binary-run
- Merge pull request #68 from temirov/codex/update-resolveconfigurationsearchpaths-function
- Handle ENOTTY zap sync errors
- Add user config search path and tests
- BugFix: default log format is for the console
- Add embedded CLI configuration defaults
- Maintenance: CHANGELOG.md defines the release of v0.0.1
- Merge pull request #66 from temirov/codex/add-informational-message-for-dirty-worktree
- Handle dirty worktrees with skip output
- Merge pull request #65 from temirov/codex/update-codebase-to-use-gix-branding
- Rename CLI branding to gix
- Merge pull request #64 from temirov/codex/update-module-path-and-imports
- Rename module to github.com/temirov/gix
- Merge pull request #2 from temirov/go
- refactor: centralize repository path sanitization
- Handle boolean literals in repo roots
- Use fixture config for CLI integration logs
- Expand tilde roots for repository commands
- Maintenance: console logging made a default
- Handle missing operation defaults gracefully
- Expand root sanitization to support tilde paths
- Require repository roots for packages purge command
- Enforce explicit workflow roots configuration
- Enforce branch cleanup option validation
- Refine workflow configuration examples
- Run gofmt on command test
- Remove migrate working directory default
- Enforce explicit repository roots in CLI and command configs
- Handle operation configuration validation
- feat(packages): auto-detect GHCR metadata
- Add repository discovery to packages purge command
- Derive GHCR context from repository metadata
- Simplify repo packages purge defaults
- Add nested filesystem repository discovery test
- Format configuration test
- Refactor configuration schema to operations list
- Refine config schema around tools and workflow steps
- Refine configuration schema for operations list
- Restore tools section anchors for workflow steps
- Refactor workflow configuration anchors
- Refine workflow configuration structure
- Support sequence workflow configuration
- Refactor workflow tool configuration schema
- Maintenance: main branch removed from CI config
- Normalize tool override key matching
- docs: document tool_ref workflow usage
- Add reusable tool configuration support to workflows
- Limit CI to Go changes
- Maintenance: README reformatted
- Handle non-404 gh api failures in branch protection check
- Respect configurable search paths for CLI configuration
- Ensure workflow help test hides repository config
- Ensure integration tests update remote default branch
- Allow EBADF sync error in logger factory tests
- Handle EBADF sync errors and test stderr pipe
- BugFix: config.yaml is used
- Rename CLI commands to audit and workflow variants
- Add configuration support for rename command
- Add configuration support for migrate command
- docs: summarize CLI catalog in quick-reference table
- Flatten packages purge command
- Refactor audit command into dedicated audit-run workflow
- Format rename command builder
- Fix configuration utilities and tests
- Refactor CLI commands to standalone names
- Preserve repository config when running workflow integration test
- Add integration test coverage for repository config defaults
- docs: consolidate configuration guidance
- Maintenance: README updated 1. Shell commands are not bash-specific 2. The past stays behind
- Propagate configuration path through command context
- Refactor configuration hierarchy for CLI and packages
- Document execution modes
- Adjust execshell logging levels
- Maintenance: dependencies updated
- Constrain branch migration to single target
- Refine branch migration workflow DSL
- Remove duplicate command constants
- Format execshell messages
- Format executor tests
- Improve console logging responsiveness
- Merge branch 'go' into work
- Refine execshell tests to use constants
- Align narration with updated generic messaging
- Improve human readable shell logs
- Improve console logging and command events
- feat: show help for missing required arguments
- Fix console event logger initialization
- docs: expand gh prerequisite guidance
- docs: update readme for go cli
- Protect workflow state updates on rename
- Add workflow runner with operations
- Fix console log format flag handling
- Handle case-only repo renames
- Refine repos services and shared helpers
- Add repos maintenance commands and tests
- Handle context cancellation during repository resolution
- Support migrating multiple repositories
- Add console log format configuration
- Propagate cleanup cancellations in multi-repo run
- Enhance branch cleanup command for multi-repo processing
- Show CLI help when no arguments are provided
- Add release tooling and packages integration coverage
- Refactor CLI entrypoint and expand integration coverage
- Add GHCR purge command and service
- Add branch migration command and GitHub helpers
- Add pr-cleanup command with branch cleanup service
- Format CLI entrypoint
- Add audit CLI command and repository audit service
- Add exec shell, GitHub CLI, and git repo utilities
- Add Go CLI entrypoint and supporting tooling
- Add design doc for Go-based CLI transition
- AGENTS.md added to facilitate the usage of AI
- Feature: search for a committed file across all repos (WIP)
- BugFix: if there are no changes in the workflows we continue
- Maintenance: CSV files are ignored
- Feature: audit_repos script added
- BugFix: updating the branch from Pages
- Maintenance: README defines the prerequisites and explains scripts functions
- Ignore IDE files
- Working scripts
- first commit

## [v0.8.1] - 2026-07-06

### Features ✨
- _No changes._

### Improvements ⚙️
- Improved `gix sync` to better handle orphaned remote branches and adoption of the main worktree during sync operations.
- Enhanced the flow for existing remote branches with no open PRs: now checks for actual diffs before prompting for merged PR handoff or raising errors.
- Worktree adoption now safely detaches the main worktree instead of attempting its removal, preserving local changes and stability.

### Bug Fixes 🐛
- Fixed an issue where `gix sync` would mistakenly refuse to open a pull request for an existing remote branch with real changes.
- Resolved handling of merged PR branches so that users are prompted to sync `master` before any new PR is created.
- Corrected errors during explicit `master` sync from linked worktrees, preventing failed attempts to remove the main working tree.

### Testing 🧪
- Added integration tests covering merged branch sync scenarios, including PR creation, merged-PR handoff, and linked worktree behaviors.
- Removed redundant `branchName` argument from sync command integration tests.

### Docs 📚
- Documented sync pull request behavior, worktree handoff resolutions, and validations in `.mprlab/ISSUES.md`.

## [v0.8.0] - 2026-06-30

### Features ✨
- Add AI-powered merge conflict resolution to strict-sync flows, automatically generating resolutions for conflicted files via the configured LLM client.
- Enable `gix init` and `gix init --user` commands for straightforward config initialization, replacing the former root `--init` flag approach.
- Support AI-based resolution of file deletions and missing branches during merge conflict handling.

### Improvements ⚙️
- Push local-ahead work branches when running `gix sync`, ensuring unpublished local commits reach the remote and PR flow.
- Reject creation of empty pull requests for strict-sync branches with no new commits beyond the base.
- Clarify, update, and document all configuration initialization flows across README, developer docs, and the docs website.

### Bug Fixes 🐛
- Resolve merge conflicts on dirty sync branches using AI, including preservation of deleted files and handling of missing branches.
- Add missing `init` subcommand; update tests and CLI behavior for correct config initialization.
- Prevent failure of `gix sync` on local-ahead branches by pushing changes as intended.

### Testing 🧪
- Extend CLI and integration test coverage for `gix init`, `gix init --user`, and invalid initialization flows.
- Add strict-sync regression tests, including for dirty base branches, local-ahead, and empty branch scenarios.
- Validate AI-based merge resolution via comprehensive black-box and in-process tests on branch sync, PR creation, and merge cases.

### Docs 📚
- Update README, architecture docs, and site documentation to reflect new `gix init` flow and removal of the obsolete `--init` flag.
- Document AI-based merge resolution and new configuration commands across all user-facing guides.
- Clarify sync behavior and configuration controls in updated documentation.

## [v0.7.0] - 2026-06-29

### Features ✨
- Added global LLM configuration defaults and override logic for CLI commands.
- Introduced provider selection and default handling for commit message generation.

### Improvements ⚙️
- Dropped XDG_CONFIG_HOME support; user config now loads only from `$HOME/.gix/config.yaml`.
- Refactored LLM configuration: renamed "provider" to "transport" and clarified separation in CLI and internal APIs.
- Supported configurable LLM provider for CLI with updated defaults.
- Enhanced agent, issue, planning, and policy guides under `.mprlab`.

### Bug Fixes 🐛
- Resolved test failures related to config directory handling by switching from XDG to secondary config directory in tests.
- Fixed LLM configuration option propagation and precedence in workflow tasks.

### Testing 🧪
- Updated and expanded integration tests for CLI and LLM configuration.
- Adjusted tests to match new config file search logic and directory precedence.

### Docs 📚
- Updated documentation to reference `$HOME/.gix/config.yaml` as the sole user config path.
- Clarified LLM Proxy configuration, option names, and usage examples in README and setup guides.
- Migrated and updated policy, agent, and planning documents to `.mprlab/` with improved governance and process documentation.
- Removed obsolete and legacy documentation files from `.mprl/` and updated references throughout.

## [v0.6.10] - 2026-06-29

### Features ✨
- Add LLM proxy client factory with proxy support for improved LLM client integration.

### Improvements ⚙️
- Switch LLM client to use the new llmclient package and update default configuration to use proxy URL and new environment variable.
- Update dependencies including llm-proxy module and other indirect dependencies.
- Update default configuration to use LLM proxy secret and proxy API URL with updated model defaults.
- Sanitize and default BaseURL configuration for changelog and commit message generation.

### Bug Fixes 🐛
- _No changes._

### Testing 🧪
- Update tests to use LLM proxy secret environment variable and verify proxy URL and model usage.

### Docs 📚
- Update Content Security Policy connect-src URL to llm-proxy-api.mprlab.com.

## [v0.6.9] - 2026-06-26

### Features ✨
- _No changes._

### Improvements ⚙️
- Simplified preset configuration loading logic in the workflow run command.
- Added forward-only contract discipline documentation to AGENTS.md.

### Bug Fixes 🐛
- Fixed `gix sync master` to rescue dirty work on a generated PR branch when switching from `origin/master` would overwrite local changes.
- Preserved dirty `master` rescue behavior when local `master` already has commits ahead of `origin/master`.

### Testing 🧪
- Improved sync integration test for dirty master worktree.
- Added tests for strict PR branch creation from dirty master when local base is ahead.

### Docs 📚
- Updated ISSUES.md with latest changes and maintenance runbooks.

## [v0.6.8] - 2026-06-23

### Features ✨
- Accept existing `gix sync` PR branches whose open pull request targets a chained issue branch and merge the pull request's actual base instead of requiring `master`.

### Improvements ⚙️
- Update `gix sync` to sync existing PR branches against their current PR base branch instead of always merging `master`.
- Refine error messages and PR base branch handling for better sync flow clarity.
- Enhance `gix sync` documentation and CLI descriptions to reflect updated PR base branch behavior.

### Bug Fixes 🐛
- Fix sync chained PR base detection to properly merge the actual PR base branch.
- Filter sync pull request lookup by head branch to correctly identify open PRs.

### Testing 🧪
- Add strict-sync coverage for head-filtered open-PR lookup, PR `baseRefName` merges, and missing PR-base rejection.
- Update strict-sync pull request creation coverage for requested branches and generated dirty-`master` branches.
- Add strict-sync failure-mode coverage for staged tracked ignored dirt, `--require-clean`, `--stash`, fetch-before-restore ordering, and restore failures.

### Docs 📚
- Update README and CLI command descriptions to clarify `gix sync` behavior with existing PR branches and base branch merging.
- Clarify sync workflow and PR handling in usage examples and documentation.

## [v0.6.7] - 2026-06-09

### Features ✨
- Add prompt and fallback handling for syncing merged pull requests in strict sync mode.

### Improvements ⚙️
- Suppress workflow failure echo output for specific error cases during sync.
- Enable confirmation prompt integration in CLI commands for branch sync operations.
- Sync base branch automatically when a merged pull request is detected during strict sync.

### Bug Fixes 🐛
- Fix strict sync to handle pruned merged branches by syncing the base branch instead.

### Testing 🧪
- Parameterize tests for command suppression of workflow failure echo with multiple error cases.
- Add tests to verify standard output is preserved and error output is suppressed as expected.

### Docs 📚
- _No changes._

## [v0.6.6] - 2026-06-06

### Features ✨
- Restore tracked ignored dirty paths during successful `gix sync` runs to prevent generated artifacts from appearing in `git status`.

### Improvements ⚙️
- Tighten integration test coverage for strict `gix sync` including pull request creation, branch diff context, and failure-before-push ordering.
- Add strict-sync coverage for committing, pushing, removing, pruning, refetching, and retrying dirty sibling worktrees.
- Enhance sync-flow and CLI tests to verify handling of Python `egg-info` files mixed with ignored `__pycache__` entries.
- Add failure-mode coverage for staged tracked ignored dirt, `--require-clean`, `--stash`, fetch-before-restore ordering, and restore failures.

### Bug Fixes 🐛
- Fix cleanup of tracked ignored files during `gix sync` to avoid leaving generated artifacts staged or modified.

### Testing 🧪
- Add extensive tests for strict-sync branch sync actions filtering ignored dirty paths before staging.
- Add tests verifying restoration of tracked ignored-only status before switching branches and stashing dirty work.
- Add regression tests covering failure modes and edge cases in sync restore operations.

### Docs 📚
- Update issue and plan documentation related to sync flow and failure modes.

## [v0.6.5] - 2026-06-06

### Features ✨
- Add tracked ignored sync integration coverage for better test reliability.

### Improvements ⚙️
- Refactor ignored sync tests to improve table coverage and test clarity.
- Enhance ignored path filtering in tracked ignored sync to handle cached ignored paths correctly.
- Improve gitignore path normalization and matching logic to better handle ancestor matches and path separators.

### Bug Fixes 🐛
- Fix tracked ignored sync path filtering to exclude ignored paths from staging and commits properly.

### Testing 🧪
- Add comprehensive test cases covering various ignored and cached ignored path scenarios in branch sync actions.
- Extend gitignore tests with stub executor to verify ignored path detection and command execution behavior.

### Docs 📚
- Update issues and plan documentation with minor clarifications.

## [v0.6.4] - 2026-06-06

### Features ✨
- _No changes._

### Improvements ⚙️
- Filter dirty `gix sync` pathspec clusters through Git ignore rules to avoid staging ignored generated files.
- Treat ignored-only dirty status as clean before selecting a generated `gix sync` work branch.

### Bug Fixes 🐛
- Fix ignored-only sync dirty detection to prevent staging ignored paths during auto-commit.
- Fix B009 sync ignored path staging issue where ignored `__pycache__` files caused `git add` failures.

### Testing 🧪
- Add strict-sync coverage for Python `egg-info` files mixed with ignored `__pycache__` entries.
- Add strict-sync regression coverage proving ignored-only status stays on the clean `master` sync path.

### Docs 📚
- Document B009 issue and resolution for ignored generated paths staging in `gix sync`.

## [v0.6.3] - 2026-06-03

### Features ✨
- Generate semantic sync branch names based on commit messages to avoid stale branch collisions.
- Automatically select unique sync branch names, retrying up to 100 times if collisions occur.

### Improvements ⚙️
- Refactor sync branch naming to use a "gix" prefix and semantic commit subjects.
- Limit sync branch name length with intelligent truncation to maintain readability.
- Enhance strict sync action to use generated semantic branch names instead of static names.

### Bug Fixes 🐛
- Avoid stale branch collisions by using semantic branch names in sync refresh.

### Testing 🧪
- Add tests for semantic sync branch name generation and collision avoidance.
- Update sync action tests to verify new branch naming and pull request creation behavior.

### Docs 📚
- _No changes._

## [v0.6.2] - 2026-06-03

### Features ✨
- Centralize strict and non-strict sync sibling-worktree retry handling behind the worktree adoption service.

### Improvements ⚙️
- Refactor sync branch handling to use a unified worktree adoption service for retrying adoption of dirty sibling worktrees.
- Enhance strict sync branch preparation and base branch syncing to integrate with the worktree adoption service.
- Expose explicit commit message options for worktree adoption during branch sync operations.

### Bug Fixes 🐛
- Restore strict `gix sync` adoption of dirty sibling worktrees when the requested branch is already checked out in another folder.

### Testing 🧪
- Add strict-sync regression coverage for committing, pushing, removing, pruning, refetching, and retrying a dirty sibling worktree before switching to the requested branch.
- Update strict-sync pull request creation coverage to include branch diff context and failure-before-push ordering.
- Add sync command and strict action coverage for explicit pull request title/body controls.

### Docs 📚
- _No changes._

## [v0.6.1] - 2026-06-03

### Features ✨
- Unified workflow action option builders for consistent option handling across commands.
- Introduced structured action option types for workflow tasks improving type safety and clarity.

### Improvements ⚙️
- Refactored web workflow primitive definitions to use typed action options instead of raw maps.
- Enhanced merging of workflow step options with typed action option structs.
- Simplified command option building by replacing manual map construction with structured option types.
- Improved safeguards handling for file replace workflows with explicit option structs.
- Streamlined retag mappings parsing to use typed structs instead of generic maps.

### Bug Fixes 🐛
- _No changes._

### Testing 🧪
- Added tests for new workflow action options builders.

### Docs 📚
- Updated issue and plan documentation with minor clarifications.

## [v0.6.0] - 2026-06-03

### Features ✨
- Expose pull request title and body controls for `gix sync` via `--title`, `--body`, and configuration.
- Generate sync pull request descriptions dynamically from branch diffs using the configured LLM path.

### Improvements ⚙️
- Generate sync PR body before pushing branches to avoid orphaned remote branches without PRs.
- Improve ordering and content of sync PR body generation.
- Update CLI help and documentation to reflect new PR metadata controls.

### Bug Fixes 🐛
- Fix ordering issue in sync PR body generation to ensure correct content.

### Testing 🧪
- Add coverage for explicit PR title and body controls in sync command and strict sync actions.
- Enhance tests for sync PR creation including branch diff context and failure-before-push scenarios.

### Docs 📚
- Update README and CLI command descriptions to document new PR title/body options and behavior.

## [v0.5.0] - 2026-06-02

### Features ✨
- Rename the public branch-change command from `gix cd` to `gix sync` while preserving existing behavior and the `switch` alias.
- Make dirty `gix sync` the default workflow: cluster changed paths, generate commit messages with the configured LLM client, commit the clusters, then sync and push through the PR flow.
- Route dirty `master` syncs to generated `sync/<repo>/<path>` work branches while keeping clean `master` sync remote-owned.

### Improvements ⚙️
- Change `sync.require_clean` to an opt-in guard while preserving `--stash` and accepting `--commit`.
- Update CLI coverage to reject the removed `cd` command and update branch-change integration tests to invoke `sync`.
- Add strict-sync coverage for dirty PR branches, dirty `master` generated branches, missing remote branch PR creation, and `--require-clean` rejection.

### Bug Fixes 🐛
- Fix I006 strict sync commit modifier.
- Clarify I006 sync dirty-work modifiers.

### Testing 🧪
- Add CLI coverage that rejects the removed `cd` command and update branch-change integration tests to invoke `sync`.
- Add strict-sync coverage for dirty PR branches, dirty `master` generated branches, missing remote branch PR creation, and `--require-clean` rejection.

### Docs 📚
- Update README, architecture notes, the docs site, and the command warning matrix to use `gix sync`.
- Document the new dirty-sync flow and LLM client configuration used for automatic commit messages.

## [v0.4.0] - 2026-05-22

### Features ✨
- Rename the branch-change command from `gix cd` to `gix sync` while preserving existing behavior and the `switch` alias.

### Improvements ⚙️
- Update all references in README, architecture documentation, embedded defaults, and command warning matrix to use `gix sync`.
- Change Cobra command name and operation configuration key from `cd` to `sync`.

### Bug Fixes 🐛
- _No changes._

### Testing 🧪
- Add CLI tests to reject the removed `cd` command and verify `sync` command functionality.
- Update branch-change integration tests to use `sync` instead of `cd`.

### Docs 📚
- Update documentation and examples in README, architecture notes, docs site, and command warning matrix to reflect the `sync` command.

## [v0.3.5] - 2026-05-21

### Features ✨
- Add support to adopt sibling worktrees during branch switching: commits dirty changes with generated message, pushes commits, removes worktree, prunes metadata, then retries switch.
- Introduce configurable LLM-based commit message generation for worktree adoption commits using OpenAI GPT models.

### Improvements ⚙️
- Update `cd` command description and usage to explain sibling worktree adoption behavior.
- Enhance `.gitignore` with common local environment and tool files.
- Refine command configurations to include commit message LLM settings.
- Improve error handling to retry branch switch after adopting sibling worktree.
- Add integration tests for sibling worktree adoption flow.
- Extend application bootstrap to support LLM commit message configuration decoding.

### Bug Fixes 🐛
- Fix `push` handling in worktree adoption to properly push clean adopted branches without tracking upstream.
- Correct `cd` to commit changes in sibling worktrees when switching branches.

### Testing 🧪
- Add comprehensive integration tests for worktree adoption and commit message generation.

### Docs 📚
- Update `README.md` to describe sibling worktree adoption behavior in `cd` command.
- Document `cd` command's fatal step on sibling worktree adoption in the warning matrix.
- Update command descriptions and usage examples for clarity on new adoption features.

## [v0.3.4] - 2026-05-10

### Features ✨
- Add automated changelog update and commit message generation in the web interface.
- Implement F009 repository tree explorer and add audit column filters.
- Add queue audit sync with folder deletions and remediations before apply.
- Render audit results in the web client.
- Merge repository scope into the repos panel and redraw web UI around repository context.
- Add F003 local web interface for gix.

### Improvements ⚙️
- Refine the web audit workflow, folder explorer, and repository tree interactions.
- Improve web startup asset verification.
- Polish ref browser rendering and redesign web targets around repo context.
- Style web interface with compact layouts, wrap dirty audit files in narrow columns, and keep audit action labels visible.
- Hide nested repositories from the web tree.
- Align web task copy with commands and filter task-owned advanced commands.
- Add web workflow and remote tasks, refactoring web actions into task workspace.
- Tighten selected repo summary layout and reflow web interface into row-based panels.
- Update repo-local workflow defaults and configure audit apply scope preservation.

### Bug Fixes 🐛
- Fix B005 cd dirty worktree fast-forward refresh.
- Normalize SSH protocol audit flow.
- Keep audit action labels visible and sync audit draft roots with inspection.
- Route audit runs through inspection and preserve audit apply scope/skipped outcomes.
- Fix current repo tree behavior to behave like explorer and reveal higher folders.
- Show current repo in explorer tree and move repository tree into left sidebar.

### Testing 🧪
- Harden browser startup in CI for improved reliability.
- Add browser coverage for web control surface.

### Docs 📚
- Add comprehensive AGENTS guidelines for Docker, Frontend, Git, and Go development workflows.
- Seed planning templates and update issue management documentation.
- Add detailed documentation for repository workflows and policy files.

## [v0.3.3]

### Features ✨
- Add proprietary licensing workflow with branch preservation and automated license updates.

### Improvements ⚙️
- Embed Google Analytics tags in documentation HTML.
- Enhance branch change action to preserve and capture original branch state before switching.
- Update proprietary license template with SPDX tagging and detailed license terms.

### Bug Fixes 🐛
- _No changes._

### Testing 🧪
- Add tests to verify branch capture behavior preserving original branch state.
- Add tests for loading SPDX-tagged proprietary license template.

### Docs 📚
- Add Google Analytics tracking script to documentation index.html.

## [v0.3.2]

### Features ✨

- Added detailed failure reporting for branch cleanup, capturing prompt, remote, and local deletion errors.
- Introduced detection of missing local and remote branches during deletion to treat them as no-ops instead of errors.

### Improvements ⚙️

- Enhanced cleanup summary to include a list of detailed failures.
- Improved logging for skipping already missing branches during cleanup.
- Refined branch cleanup confirmation handling with better error reporting.

### Bug Fixes 🐛

- Fixed failure during pull request branch deletion by correctly accounting for different failure scenarios.
- Resolved false failure reports when deleting non-existent local or remote branches.

### Testing 🧪

- Added tests to verify missing local branch deletion is treated as a no-op.
- Added tests to confirm failure details are correctly captured and reported during cleanup failures.

### Docs 📚

- _No changes._

## [v0.3.1]

### Features ✨

- Add embedded license template support in workflows for BSL, MIT, and proprietary licenses.
- Introduce Go version update, Go module bump, and command run steps in workflows with validation moved to the edge.
- Extend gitignore workflow with additional commonly ignored folders.

### Improvements ⚙️

- Refine workflow summary output with YAML step summaries for easier automation and machine parsing.
- Align workflow license distribution to use templates and allow extensive license variable overrides.
- Enhance CLI logging with structured JSON by default and human console logs for non-workflow commands.

### Bug Fixes 🐛

- Fix workflow discovery output message formatting.
- Fix workflow glob root matches.
- Fix PRs delete command output.
- Trim blank stderr lines in execshell errors for cleaner error messages.
- Fix handling of untracked files in stash pop commands.
- Fix require_changes safeguard to correctly track commits created by workflows.

### Testing 🧪

- Add comprehensive tests to cover handling of blank stderr lines in CommandFailedError.
- Add full-coverage tests for workflow operations command and related packages.

### Docs 📚

- Update README with usage details on workflow license presets and workflow output format.
- Clarify workflow output and embedded workflow command syntax in documentation.
- Move AGENTS.md to root and revise documentation links for better discoverability.

## [v0.3.0]

### Features ✨

- Introduce Go version update, Go module bump, and command run steps in workflows with validation moved to the edge.
- Add embedded license template support in workflows for BSL, MIT, and proprietary licenses.
- Extend gitignore workflow with additional commonly ignored folders.

### Improvements ⚙️

- Refine workflow summary output with YAML step summaries for easier automation and machine parsing.
- Align workflow license distribution to use templates and allow extensive license variable overrides.
- Move AGENTS.md and related stack-specific guides to the root folder for better discoverability.
- Enhance CLI logging with structured JSON by default and human console logs for non-workflow commands.
- Archive resolved issues and synchronize README and ARCHITECTURE documentation with completed issues.

### Bug Fixes 🐛

- Fix workflow step logs and summary output for better clarity and accuracy.
- Trim blank stderr lines in execshell errors for cleaner error messages.
- Fix `gix cd --stash` to handle untracked files correctly by popping only pushed stashes.
- Fix `require_changes` safeguard to correctly track commits created by workflows.
- Restore concise non-workflow logging by suppressing workflow internal noise and machine payloads.

### Testing 🧪

- Add full-coverage tests for workflow command steps and internal workflow packages.

### Docs 📚

- Update README with usage details on workflow license presets and workflow output format.
- Move AGENTS.md root and revise documentation links for policy and agent behavior.
- Synchronize ARCHITECTURE.md description with latest workflow changes and command enhancements.

## [v0.3.0-rc14]

### Features ✨

- _No changes._

### Improvements ⚙️

- Migrate LLM client utilities to `github.com/tyemirov/utils/llm` and retire `pkg/llm`.
- Update dependencies with minor upgrades including Cobra, Go modules, and Zap.
- Add a new workflow YAML formatter for enhanced step log outputs.

### Bug Fixes 🐛

- Fix workflow step logs and summary output for better clarity and accuracy.
- Add informative event logging when repository protocol mismatches occur and skip operations accordingly.
- Restore succinct console logging for non-workflow commands by suppressing workflow-internal `TASK_*` noise and dropping machine payloads.
- Fix `require_changes` safeguards so `git push` and `pull-request open` run after `git stage-commit` when workflow commits were created.
- Fix `gix cd --stash` failing when untracked files are present by popping only stashes that were actually pushed.
- Trim blank stderr lines from `git` failures so `CommandFailedError` messages do not end with a trailing `|`.

### Testing 🧪

- Add full-coverage tests for the migrated LLM package in `github.com/tyemirov/utils/llm`.
- Add tests covering protocol executor behaviors and workflow taskrunner summaries.

### Docs 📚

- Update architecture documentation to reflect the migration of the LLM package to `github.com/tyemirov/utils/llm`.
- Clarify usage of GX-261 with new utility package in documentation.

## [v0.3.0-rc13]

### Features ✨

- Avoid duplicate step summaries in workflows.
- Add detailed step summary logging with step name, outcome, and reason.

### Improvements ⚙️

- Improve step-centric workflow logging.
- Refine reasons in step summaries.
- Enhance GX-259 step outcome logging.

### Bug Fixes 🐛

- _No changes._

### Testing 🧪

- Update tests to verify correct printing of step summaries.

### Docs 📚

- _No changes._

## [v0.3.0-rc12]

### Features ✨

- Inject workflow step names into all emitted events to enhance logging clarity.
- Added step-decorating reporter to automatically prefix events with current workflow step name.

### Improvements ⚙️

- Improved flow for automated coding with scoped step reporting.
- Updated repo executor operations to use the step-scoped reporter consistently.
- Enhanced integration test outputs to include descriptive step prefixes for folder renames and remote updates.
- Renamed multiple documentation files into an organized `issues.md` directory.

### Bug Fixes 🐛

- Fixed missing step names and per-step logging in workflows, addressing issue GX-344.
- Corrected workflow step name logging in step reporter tests.
- Resolved test failures by injecting git manager stub into remotes step reporter test.

### Testing 🧪

- Added tests verifying step names injected into repository event reporting.
- Updated integration tests to reflect new step-prefixed output format.

### Docs 📚

- Added issue documentation and planning notes into `issues.md` directory.
- Updated changelog behavior to print notices more consciously.

## [v0.3.0-rc11]

### Features ✨

- Introduced per-step logging in workflows to provide clear step names instead of generic rubrics.
- Added generic `git branch-cleanup` workflow command to automatically remove empty automation branches.
- Branch cleanup now uses the repository default branch as the base for determining whether to delete branches.

### Improvements ⚙️

- Propagated workflow step names into logging for detailed event context.
- Updated `account-rename` workflow to reference repository default branch dynamically for pushing and branch cleanup.
- Enhanced human formatter to label task skips, phase events, and issues with step names.
- Strengthened branch cleanup tests to cover behavior with repository default branch.

### Bug Fixes 🐛

- Fixed branch cleanup to correctly use the default branch as base, preventing unintended branch retention.
- Clarified differences between logging improvements and branch cleanup behavior.

### Testing 🧪

- Added regression tests for branch cleanup behavior with default branch base.
- Added logging tests covering step detail propagation and task skip labeling by step.
- Extended human formatter tests for step-based logging.

### Docs 📚

- Expanded issue documentation with detailed explanations of per-step logging and branch cleanup changes.
- Updated `ISSUES.md` with new workflow logging and branch cleanup item descriptions.

## [v0.3.0-rc10]

### Features ✨

- _No changes._

### Improvements ⚙️

- Keep switch-branch capture when already on master branch.
- Expose branch-not safeguard condition to workflow DSL.
- Refine semantics for branch cleanup during workflows.
- Add branch cleanup step to remove no-op automation branches after namespace rewrite workflows.
- Add require_changes safeguard to gitignore workflow preset to skip commits when no changes.

### Bug Fixes 🐛

- _No changes._

### Testing 🧪

- Add tests for git branch-cleanup operation ensuring it deletes branches with no extra commits and retains branches with commits.
- Add tests for branch-not safeguard preventing steps from running on disallowed branches.

### Docs 📚

- Update issues documentation to describe automated branch cleanup for empty workflow branches.

## [v0.3.0-rc9]

### Features ✨

- _No changes._

### Improvements ⚙️

- Clarify per-step no-op messages for skipped tasks with detailed explanations.
- Treat safeguard skips as non-issues to reduce noise in workflow outputs.
- Nest issues under step outputs for better readability.

### Bug Fixes 🐛

- _No changes._

### Testing 🧪

- Adjusted tests to reflect updated issue formatting and nesting.

### Docs 📚

- _No changes._

## [v0.3.0-rc8]

### Features ✨

- _No changes._

### Improvements ⚙️

- Clarify messaging for require_changes safeguard to better explain when no workflow edits are detected and the implication for Git operations.

### Bug Fixes 🐛

- _No changes._

### Testing 🧪

- Adjust safeguard test expectation to match updated require_changes messaging.

### Docs 📚

- _No changes._

## [v0.3.0-rc7]

### Features ✨

- Added a new website documenting all the benefits of the gix utility, now served from GitHub Pages with the static site located under `docs/index.html`.
- Introduced an About gix modal and integrated the mpr-ui footer aligned with the demo styling.
- Web site improvements including footer alignment and overall style enhancements to present workflows, use cases, and documentation more clearly.

### Improvements ⚙️

- Added explicit safeguards requiring changes before git stage-commit, push, and pull-request steps run; updated account-rename workflow preset to opt into the `require_changes` safeguard.
- Enhanced `gix cd` command to stash modified tracked files, switch branches, and restore files automatically.
- Workflow file replacements now honor recursive glob patterns for correct module import rewrites across nested folders.
- Modified changelog behavior to suppress duplicate "no changes detected" messages.

### Bug Fixes 🐛

- _No changes._

### Testing 🧪

- Added tests covering session prompter's uppercase 'apply-all' selection.
- Extended coverage for tracked-file stashing and branch switching behavior.

### Docs 📚

- Moved static documentation site to `docs/index.html` and updated styling to support GitHub Pages hosting.
- Updated ISSUES.md with detailed process improvements and issue tracking enhancements related to autonomous work and bugfix prioritization.
- Added CNAME file to configure custom domain for GitHub Pages hosting (gix.mprlab.com).

## [v0.3.0-rc6]

### Features ✨

- _No changes._

### Improvements ⚙️

- `gix message changelog` now suppresses duplicate “no changes detected” lines for empty commit ranges.
- Namespace workflow rewrites register changed files, enabling `git stage-commit` to automatically include those modifications.
- Workflow file replacements honor recursive glob `**` patterns for correct module import rewrites across nested folders.

### Bug Fixes 🐛

- Avoid duplicate changelog no-change notices in the changelog generation command.
- Track namespace-mutated files to ensure proper staging of changes.
- Fixed recursive replacements in workflows to properly apply changes across nested files.

### Testing 🧪

- Added tests for changelog action to verify handling of no-change scenarios and output behavior.
- Added tests covering session prompter 'apply-all' uppercase selection.

### Docs 📚

- Moved the static documentation site to `docs/index.html` so it can be served directly from GitHub Pages, aligning GX-110 with the standard `docs/` folder convention.

## [v0.3.0-rc4]

### Features ✨

- _No changes._

### Improvements ⚙️

- `gix cd` warnings now include untracked file names, helping users identify blocking paths without running `git status`.
- `git stage-commit` stages only files mutated by workflows, preventing unrelated changes from being committed.
- Workflow file replacements honor recursive glob `**` patterns, enabling correct module import rewrites across nested folders.
- Added an explicit `require_changes` safeguard so git stage-commit/push/pull-request steps can skip themselves when no edits are present; the account-rename preset now opts in to the safeguard instead of relying on implicit behavior.
- `gix message changelog` now suppresses duplicate “no changes detected” lines when the selected range contains no commits.

### Bug Fixes 🐛

- Fixed recursive replacements in workflows to properly apply changes across nested files.
- Corrected `gix cd` command behavior for refreshing branches with untracked files.
- Resolved `git stage-commit` committing all files issue by limiting staging to mutated files only.

### Testing 🧪

- Added tests for session prompter supporting uppercase 'apply-all' option.
- Added tests verifying `git stage-commit` correctly prefers recorded mutated files.
- Added coverage for filtering staged files by requested patterns.

### Docs 📚

- _No changes._

## [v0.3.0-rc3]

### Features ✨

- Add session apply-all prompter to enable upgrading confirmation policy during command runs.

### Improvements ⚙️

- Refactor confirmation prompts to support `[a/N/y]` template with lowercase apply-all indicator.
- Rename prompt state handling to session state for better semantic clarity.
- Update documentation and CLI design notes to reflect the new confirmation prompt behavior.
- Enhance the prompt components to auto-accept subsequent confirmations after apply-all selection.
- Lowercase the apply-all indicator in confirmation prompts for consistency.
- Auto-configure branch tracking when missing, improving `gix cd` usability.
- Improve confirmation prompts to maintain uppercase default decline (`N`) behavior.

### Bug Fixes 🐛

- Fix prompt template in branch deletion confirmation to include apply-all `[a/N/y]` option.
- Correct renamed packages and internal references related to prompt state and session state.
- Honor recursive glob patterns in workflow file replacements so presets like `configs/account-rename.yaml` actually rewrite Go module imports across nested folders.
- Include the untracked file names in `gix cd` warnings so operators can see which paths block refresh without running `git status`.
- `git stage-commit` now stages only the files mutated by the workflow (rather than `git add .`), preventing unrelated local changes from being committed.
- Ignore `git check-ignore` failures that report “not a git repository” so workflows no longer abort when encountering non-repo folders.
- Namespace workflow rewrites now register their changed files so subsequent `git stage-commit` steps pick up those modifications automatically.

### Testing 🧪

- Add tests for session prompter covering applying 'apply-all' uppercase selection.
- Update tests for renamed prompt session prompter and session state usage.

### Docs 📚

- Update `README.md` to clarify new confirmation prompt behavior including apply-all option.
- Refine CLI design documentation for confirmation prompt letter case semantics and apply-all feature.
- Adjust issue tracker documentation to mark apply-all issue as resolved with relevant implementation details.

## [v0.3.0-rc2]

### Features ✨

- Introduce `TrackingRemoteConfigurator` smart constructor and move validation to edge.
- `gix cd` now auto-configures branch tracking to the resolved remote when possible, streamlining refresh and pull workflows.

### Improvements ⚙️

- Enhance branch change action to automatically configure tracking remote if missing.
- Added detailed tracking remote name reporting in branch change results.
- Refactor test suites for branch change and tracking remote configuration logic.

### Bug Fixes 🐛

- Fix handling of branches without tracking information by auto-associating tracking remotes.
- Avoid refresh action skips by creating tracking branches automatically when remote branch exists.
- Improve error messaging and handling for missing remote branches during tracking configuration.

### Testing 🧪

- Add tests verifying automatic tracking remote configuration and related warnings.
- Extend coverage for handling branch change actions with tracking configurations.
- Test error scenarios when remote branch is missing or configuration commands fail.

### Docs 📚

- Updated `ISSUES.md` with details about automatic tracking remote configuration and related scenarios.
- _No additional documentation changes._

## [v0.3.0-rc1]

### Features ✨

- Extend confirmation prompts to include an 'accept all' option that upgrades subsequent prompts to auto-accept, mimicking the `--yes` flag behavior without restarting the command.

### Improvements ⚙️

- Update documentation and CLI to reflect the new confirmation behavior supporting the 'accept all' feature.
- Refactor prompt handling to support automatic approval after selecting the 'accept all' option.
- Enhance summary reporting for multi-repository workflow commands.

### Bug Fixes 🐛

- _No changes._

### Testing 🧪

- Add tests for prompt behavior to verify 'accept all' selection upgrades the prompt session to auto-accept.

### Docs 📚

- Update CLI design document and README to describe the new confirmation prompt 'accept all' option and its effect on workflow behavior.

## [v0.2.11]

### Features ✨

- Added integration tests covering the `gix cd` refresh default behavior.
- Introduced a new site and styles to showcase gix capabilities and workflows.

### Improvements ⚙️

- Hid the explicit `--refresh` flag in `gix cd`, enabling refresh only internally when `--stash` or `--commit` recovery is requested.
- Simplified CLI usage and updated documentation/examples to match the new `gix cd` behavior.
- Enhanced multi-repository workflows with refined summary reporting and deduplication of repository roots.

### Bug Fixes 🐛

- Fixed the default behavior for branch change refresh in `gix cd`.
- Suppressed noisy "tasks apply" prefixes from error messages to improve clarity.

### Testing 🧪

- Added comprehensive integration tests for `gix cd --refresh` flow with stash and commit recovery.

### Docs 📚

- Created a full new docs site with introduction, feature list, use cases, workflows, architecture, and recipes.
- Updated README and configuration defaults to reflect removal of the explicit `--refresh` flag on `gix cd`.
- Fixed formatting in ISSUES.md for improved issue checklist readability.

## [v0.2.10]

### Features ✨

- Redesign `gix cd` output to remove noisy "tasks apply" prefixes and add per-repository sections with a final summary when processing multiple repositories.
- Extend confirmation prompts so selecting `a`/`all` once upgrades the remainder of the run to auto-accept behaviour, matching the effect of passing `--yes` without requiring a new invocation.

### Improvements ⚙️

- Implement summary reporting for workflow-backed commands showing aggregated results when more than one repository is involved.
- Enhanced task runner to print execution summaries including event counts, warnings, errors, and durations.
- Deduplicate repository roots for accurate summary computations.
- Remove explicit `gix cd --refresh` flag from the CLI, keeping the stricter refresh stage internal and enabling it only when `--stash` or `--commit` recovery is requested, and update documentation/examples to reflect the simplified surface.

### Bug Fixes 🐛

- Suppress "tasks apply" operation prefix in error messages for clearer failure outputs.

### Testing 🧪

- Added tests validating summary line formatting and multiple-repository summary output.
- Coverage improvements for operation failure message formatting and task runner summary executor.

### Docs 📚

- Updated issue tracker with the new enhancement request and implementation notes for `gix cd` output redesign.

## [v0.2.9]

### Features ✨

- Support for stashing tracked files and restoring them during branch changes using `gix cd --stash`.
- Handle multiple stashes during branch change refresh to improve workflow reliability.

### Improvements ⚙️

- Enhanced `gix cd` remote selection to fall back to a single configured remote automatically, matching `git checkout` behavior.
- Updated documentation to reflect Go 1.25+ requirement and renamed module paths from `github.com/temirov/gix` to `github.com/tyemirov/gix`.
- Improved audit report columns to show repository dirty status and affected files for better visibility.

### Bug Fixes 🐛

- Fixed `gix cd` incorrectly reporting branches as untracked by aligning upstream tracking logic with `git checkout`.
- Corrected branch switching to always proceed even when worktrees are dirty, guarding only refresh/pull stages.
- Resolved incomplete line replacement in files during workflow runs to apply changes to all targeted Go files.
- Fixed outdated CLI design documentation to refer to the correct binary and module path after rename.

### Testing 🧪

- Added regression tests covering tracked-file stash/restore flow during branch changes.
- Coverage improved for fallback behavior in remote selection and dirty skip scenarios.

### Docs 📚

- Updated README and architecture docs for Go 1.25+ requirement.
- Revised CLI design document to reflect new binary, environment variables, and config paths.
- Adjusted installation instructions and badges to use the updated repository and module paths.

## [v0.2.8]

### Features ✨

- Added shared worktree clean inspector to normalize `require_clean` semantics; filters untracked/ignored files and surfaces tracked file details in warnings.
- Aligned branch change behavior to match `git switch` semantics: branches always switch even on dirty worktrees, with `require_clean` gating refresh/pull stages.
- Workflows can now capture and restore branches or commits explicitly using new DSL blocks for better automation control.

### Improvements ⚙️

- Updated branch change refresh to skip pulls on dirty worktrees, issuing structured warnings with tracked dirty files to inform users.
- Refactored workflow presets and safeguards to relax branch cleanliness requirements while still guarding against unwanted changes.
- Enhanced branch change commands and rename executors to use shared clean-check helpers, improving consistency across CLI and workflows.
- Workflow actions now report repository events for untracked/ignored files allowing refresh to continue with warnings instead of failures.

### Bug Fixes 🐛

- Fixed issue where untracked files blocked refresh operations by excluding them from cleanliness checks.
- Corrected branch change refresh expectations to properly handle dirty worktree states without interrupting branch switching.
- Reduced false hard stops on workflow steps by honoring `ignore_dirty_paths` in worktree filters and safegaurds.

### Testing 🧪

- Added tests covering dirty worktree skip of branch refresh, untracked files during refresh, and capturing and restoring branch state.
- Implemented scripted Git executor in tests to simulate remote and status command outputs for more reliable and comprehensive test cases.

### Docs 📚

- Documented new normalized `require_clean` semantics and branch change refresh behavior in the issue tracker.
- Updated workflow configurations to remove redundant hard stop requires_clean and reflect new relaxed worktree cleanliness guards.
- Added notes explaining role and process changes for staff engineers regarding repository refactoring and development workflows.

## [v0.2.7]

### Features ✨

- Accept yes/no toggles for all boolean flags.

### Improvements ⚙️

- Allow stash and commit refresh options to skip pre-clean failure.
- Use toggle flags consistently in command flags.

### Bug Fixes 🐛

- Prevent branch change if refresh requires a clean worktree and it is dirty.

### Testing 🧪

- Enhance tests to cover branch refresh with clean checks and dirty worktree blocking.
- Update branch change action test to verify additional status command execution.

### Docs 📚

- _No changes._

## [v0.2.6]

### Features ✨

- _No changes._

### Improvements ⚙️

- Added configuration to ignore specific dirty paths during gitignore application for cleaner workflows.

### Bug Fixes 🐛

- Honor ignore_dirty_paths setting in the gitignore application process.

### Testing 🧪

- _No changes._

### Docs 📚

- _No changes._

## [v0.2.5]

### Features ✨

- Accept string values for `require_clean` directive enabling refined workspace cleanliness checks.
- Support nested `require_clean` options including `enabled` and `ignore_dirty_paths` for flexible configuration.
- Add capture and restore for initial branch state in account rename and gitignore workflows for improved branch handling.

### Improvements ⚙️

- Default change directory refresh is now enabled with safe skip when no tracking remote exists.
- Worktree cleanliness checks now filter ignored dirty file patterns before deciding cleanness.
- Enhanced workflow safeguards to parse and evaluate complex `require_clean` configurations.
- Refactor default config and YAML workflows to align with new `require_clean` map structure.
- Expand branch change action to conditionally refresh based on tracking remote presence.
- Improve error reporting consistency in clean worktree guard with better status information.
- Update account rename replacements to generic username substitution improving maintainability.
- Internal test coverage extended to verify dirty ignore patterns and clean worktree guard behavior.

### Bug Fixes 🐛

- Fixed incorrect initial commit capture name to initial branch in workflows to restore correct branch.
- Prevent false dirty statuses when only ignored dirty paths are modified.

### Testing 🧪

- Added comprehensive unit tests for action guards enforcing ignore patterns on dirty worktrees.
- Workflow safeguard tests enhanced to cover nested `require_clean` maps and ignore path behavior.
- Added branch change action test asserts for tracking remote config checks and refresh skip.

### Docs 📚

- Update README to clarify `require_clean` new nested syntax and default behavior.
- Document new `capture` and `restore` semantics for branch variables in README examples.

## [v0.2.4]

### Features ✨

- Add capture and restore blocks with variable names and kind (branch or commit) for workflow DSL to enable saving and reverting the current state.
- Support branch change action to capture and restore branch or commit state within workflows.
- Refine capture DSL with named variables and align it with the `kind` keyword for clarity and validation.

### Improvements ⚙️

- Protect capture state per repository to avoid conflicts during concurrent workflow executions.
- Update capture DSL and improve coverage.
- Restore original commit after gitignore and account rename flows to maintain repository state consistency.

### Bug Fixes 🐛

- Add missing imports for capture storage to ensure functionality.
- Prevent branch.change action from capturing and restoring simultaneously to avoid conflicts.

### Testing 🧪

- Add extensive tests for branch change action capture and restore behavior, including overwrite protection and restore using branch or commit.
- Implement branchCaptureExecutor stub to simulate git commands for reliable testing.

### Docs 📚

- Document new `capture` and `restore` workflow options with examples in the README.
- Update ISSUE.md with resolutions related to capture and restore features.
- Add examples for restoring original commit in account-rename and gitignore workflow configs.

## [v0.2.3]

### Features ✨

- Allow `ignore_dirty_paths` safeguard to permit ignoring specific dirty files/directories when `require_clean` is true.
- `.gitignore` workflow now includes managed entries for additional service files like `.env`, `.DS_Store`, `qodana.yaml`, `.idea/`, `tools/`, and `bin/`.

### Improvements ⚙️

- Improved account renaming flow.
- Enhanced `.gitignore` workflow to add and ensure proper ignore entries.
- Refined safeguard evaluation to respect status codes and ignore specified dirty paths when filtering dirty ignores.

### Bug Fixes 🐛

- Fixed issue where `require_clean` safeguard incorrectly reported failure when only managed dirty files (e.g., `.DS_Store`, `.env`) were present.

### Testing 🧪

- Added tests for `ignore_dirty_paths` safeguard behavior in workflow and repository state.
- Integration tests verifying `require_clean` with ignored paths functioning correctly.
- Additional coverage for filtering status entries and safeguard evaluations.

### Docs 📚

- Updated README to document `ignore_dirty_paths` safeguard option and usage examples.
- Documented new gitignore managed entries and safeguard behavior in configuration files.

## [v0.2.2]

### Features ✨

- Added audit report columns to show worktree dirty status and list files needing attention.

### Improvements ⚙️

- Improved account renaming flows for better consistency and coverage.

### Bug Fixes 🐛

- _No changes._

### Testing 🧪

- Enhanced tests to cover new worktree dirty state and dirty files in audit reports.

### Docs 📚

- Updated ISSUE tracking with new audit worktree visibility feature.

## [v0.2.1]

### Features ✨

- _No changes._

### Improvements ⚙️

- Updated CLI design and documentation for enhanced user experience.
- Clarified Go 1.25 baseline requirements.
- Aligned module namespaces for improved consistency.
- Converted multiple repo commands (rename, remote update, protocol convert, files add/replace, release) to workflow presets with CLI shims for streamlined execution.
- Separated safeguards into hard-stop and soft-skip categories for clearer task failure behavior.
- Enhanced workflow logging with human-readable formats and parallelized execution for repo-scoped operations.

### Bug Fixes 🐛

- Fixed issue with recursive replacements to apply across all files in namespace rewrites.
- Resolved workflow tasks skipping or failing due to improper handling of repository skip signals.
- Prevented unnecessary git pull rebase warnings when creating new branches without remotes.
- Corrected append-if-missing template behavior for multi-line file updates.
- Fixed module path and repository remote URL mismatch to ensure correct installation and badge links.
- Fixed partial application of file replacements during workflows for comprehensive changes.

### Testing 🧪

- Added regression tests covering recursive replacements.
- Added coverage to ensure skipped repositories prevent subsequent failing steps.
- Updated and added tests for CLI workflow shim commands.

### Docs 📚

- Refreshed CLI design documentation.
- Updated issues log formatting and instructions.
- Enhanced README and architecture docs with updated environment and usage details.

## [v0.2.0]

### Features ✨

- _No changes._

### Improvements ⚙️

- Force workflow commands to always use human-readable formatter, removing legacy structured logs.
- Support replace mode globs in taskfiles to correctly handle file patterns.
- Use replace action for Go file globs in cleanup tasks to avoid creating literal `**/*.go` files.
- Suppress workflow logging during changelog and commit message commands for cleaner output.

### Bug Fixes 🐛

- Removed legacy workflow logging outputs (TASK_PLAN/TASK_APPLY) across commands.
- Disabled legacy CLI structured report formatter, ensuring only human-readable output remains.
- Fixed changelog and commit message commands to avoid emitting workflow logs.
  
### Testing 🧪

- Added tests to confirm workflow logging suppression in changelog and commit message commands.

### Docs 📚

- Updated changelog to mention issue GX-419 related to glob support in replace mode.

## [v0.2.0-rc.13]

### Features ✨

- Enforce typed options in history and rename commands.
- Add concurrent repository execution to workflow.
- Add preset workflow helpers and convert workflow commands to presets.

### Improvements ⚙️

- Redesigned workflow logging and improved safeguard tracking between hard-stop and soft-skip.
- Synced repository playbook documentation and enhanced audit fallback handling.
- Added typed preset builders and preset workflow helper utilities.
- Propagate owner variable to canonical workflow preset.
- Ensure folder-rename preset uses boolean options.
- Clean workflow logging headers.
- Validate retag mapping inputs.
- Gate history variable overrides to history actions.
- Preserve severity indicators for phase events.

### Bug Fixes 🐛

- Refined workflow human-readable logging.
- Add hard-stop vs soft-skip safeguards.
- Converted repo-files-add, repo-files-replace, remote update, and release commands to workflow presets.
- Removed bespoke repo workflow helpers.
- Ensure repo files add skips pushes when disabled.
- Fixed repo workflow executor wiring.

### Testing 🧪

- Covered negative paths in files-add tests.
- Updated audit fallback expectations.
- Added integration coverage for workflow preset variables.

### Docs 📚

- Added GX-412 refactor plan and marked GX-417 issue as resolved.
- Updated Go backend and git agent coding standard documents.
- Improved repo playbook documentation to align with new features and workflows.

## [v0.2.0-rc.12]

### Features ✨

- Execute workflow tasks per repository with repository-scoped stages and deduplication.
- Added atomic git command steps and first-class workflow commands for fine-grained git/file operations.
- Seed run IDs for branch templates enable improved workflow tracking.

### Improvements ⚙️

- Decomposed workflow executor into discrete actions with reusable guard helpers for better independence and failure reporting.
- Enhanced workflow logging with concise summaries per repository; eliminated verbose per-stage logs.
- Command failures now include invoked arguments and first stderr line for improved error clarity.
- Added support for normalized carriage-return line endings in append-if-missing mode.
- Removed legacy preview mode and improved configuration namespaces for commit and changelog message generation.
- Legacy commands and configuration keys now alias to new workflows with deprecation warnings.
- Improved safeguard and skip logic to halt workflow execution on repository skips.
- Normalized and renamed modes from line-edit to append-if-missing.

### Bug Fixes 🐛

- Fix append-if-missing mode to append all lines, not just the first, with regression tests.
- Prevent running unnecessary git pulls on new branches without remotes.
- Stop workflow execution after repository-scoped TASK_SKIP events to avoid failed operations on skipped repos.
- Correct branch change logic to avoid tracking remote refs for new automation branches.
- Workflow commands now execute directly, removing unsupported command errors.
- Fixed output path order and included path in workflow event summaries.
- Fixed literal line matching for append-if-missing to prevent incorrect line omission.
- Fixed log formatter to include event summaries properly.

### Testing 🧪

- Added comprehensive tests for append-if-missing mode and task planning/execution.
- Enhanced executor and workflow command unit tests to cover new action decompositions and skipping behavior.

### Docs 📚

- Updated documentation for pkg/llm usage and workflow command orchestration.
- Added detailed design notes in ARCHITECTURE.md regarding embedded workflows.
- Refactored plans and improved README to explain new workflow capabilities and logging improvements.

## [v0.2.0-rc.11]

### Features ✨

- Added `append-if-missing` file mode for workflow-managed files to append missing lines without overwriting existing content, ideal for `.gitignore` enforcement.
- Renamed legacy `line-edit` mode to `append-if-missing` with backward compatibility acceptance and new `ensure-lines` file mode added.
- Introduced reusable `pkg/llm` package for large language model integrations with lightweight interfaces and configurable HTTP plumbing.

### Improvements ⚙️

- Refactored workflow tasks and task executor to support the new `append-if-missing` mode with proper planning and applying logic.
- Updated CLI options and documentation to include the new file mode `append-if-missing` for commands like `gix files add` and workflow license preset.
- Removed legacy `line-edit` mode references and replaced with `append-if-missing` and `ensure-lines` naming for clarity.
- Enhanced testing coverage for task planning and execution of the new file modes, ensuring correct behavior and idempotence.
- Added detailed documentation for `pkg/llm` explaining configuration, usage, and design principles.
- Decomposed the workflow executor into discrete actions (`git.branch.prepare`, `files.apply`, `git.stage`, `git.commit`, `git.push`, `pull-request.create`) with reusable guard helpers so file edits, commits, and pushes run independently and emit precise failures.
- Added first-class workflow commands for each action (`git stage`, `git commit`, `git stage-commit`, `git push`, `pull-request create`, `pull-request open`) plus per-task `steps` configuration, enabling workflows to orchestrate small git/file operations explicitly (see the updated gitignore workflow).
- Shell command failures now report the invoked arguments and the first line of stderr, making `git`/`gh` workflow errors actionable.

### Bug Fixes 🐛

- Fixed legacy mode acceptance issues by supporting the renamed `append-if-missing` mode in parser and executor components.
- `gix workflow` now executes workflow operations directly, so git action steps such as `git stage-commit` run without triggering “unsupported workflow command” errors (the gitignore preset works again).
- `branch.change` no longer attempts to create new automation branches with `--track origin/<branch>` when the remote ref does not exist, allowing presets like `gitignore` to create/push fresh branches without invalid reference errors.
- `append-if-missing` now normalizes carriage-return line endings before evaluating and applying changes so templates written on Windows append every line instead of collapsing into a single entry.
- `append-if-missing` now compares literal line content (whitespace intact) so `.envrc`, `*.env`, or indented variants no longer satisfy the `.env` check and prevent the line from being appended.
- Repository-scoped `TASK_SKIP` events (for example, dirty worktrees) now propagate a skip sentinel so later workflow steps stop executing against that repository instead of running `git stage-commit`/push commands on a repo that was already skipped.
- Removed verbose per-stage workflow logging so only the final summary is printed when running `gix workflow`.
- Added concise workflow logging that groups events per repository, collapses `TASK_PLAN`/`TASK_APPLY` noise, and highlights warnings/errors without the previous wall of text.
- `branch.change` no longer runs `git pull --rebase` immediately after creating a brand new local branch without a tracking remote, eliminating spurious `PULL-SKIP` warnings during workflows.

### Testing 🧪

- Added comprehensive unit tests for parsing task file modes including the new `append-if-missing`.
- Added tests verifying task planner skips when lines are already present and executor correctly appends missing lines only.
- Expanded integration tests for task executor's append-if-missing functionality in workflow-managed files.

### Docs 📚

- Documented `append-if-missing` mode in README, schema highlights, and CLI command descriptions with examples.
- Created detailed `pkg/llm` integration guide with overview, configuration, and usage instructions.
- Updated workflow packages documentation reflecting new file modes and usage patterns.

## [v0.2.0-rc.10]

### Features ✨

- _No changes._

### Improvements ⚙️

- Removed all legacy CLI command wrappers including `gix commit`, `gix changelog`, and `gix repo-license-apply`; only canonical namespaces remain.
- Replaced deprecated `repo-license-apply` command with the `workflow license` preset using runtime variables.
- Simplified configuration by dropping alias normalization and warning layers related to removed commands.

### Bug Fixes 🐛

- _No changes._

### Testing 🧪

- _No changes._

### Docs 📚

- Updated README and ARCHITECTURE.md to remove references to legacy commands and promote workflow presets.
- Revised command tables and help output to reflect streamlined CLI surface without deprecated wrappers.

## [v0.2.0-rc.9]

### Improvements ⚙️

- Renamed the `branch-cd` command to `cd`, added deprecation warnings when the legacy name is used, and allowed the branch argument to default to the repository's detected default or configured fallback.
- Folded the `branch-refresh` behaviour into `gix cd` via `--refresh`/`--stash`/`--commit` flags and removed the standalone command while preserving migration warnings for legacy configuration.
- Renamed the `branch-default` command to `default`, added workflow/config alias warnings, and updated CLI defaults, docs, and workflow tests to reference the new surface while keeping `branch-default` as an alias.
- Introduced a top-level `message` namespace and moved `changelog message` to `message changelog`, keeping the legacy path as a deprecated alias with config/CLI warnings.
- Added embedded workflow presets so `gix workflow --list-presets` and `gix workflow <preset>` can run bundled automation (initially `license`) without a separate YAML file.
- Added workflow runtime variables (`gix workflow --var key=value` / `--var-file path.yaml`) so presets and file-based configs can consume user-supplied values.
- Added an `append-if-missing` file mode for workflow-managed files so recipes like the gitignore preset can append missing entries without overwriting existing content.
- Deprecated `gix repo-license-apply` in favor of the `workflow license` preset; the CLI prints a warning and forwards legacy flags as workflow variables.
- Removed the remaining legacy CLI wrappers (`gix commit`, `gix changelog`, and `gix repo-license-apply`) along with the configuration alias/warning layer so only the canonical namespaces remain.
- Moved history purge under `gix files rm` and removed the legacy `gix rm` command, updating configuration defaults/docs/tests to the new namespace.
- Added an embedded `namespace` preset and removed the `gix namespace rewrite` CLI; `gix workflow namespace` is now the single supported entrypoint.
- Added explicit `make test-fast`/`test-slow` targets (with `ci` wiring) so fast unit packages can run independently of the slower `./tests` integration suite.

### Testing 🧪

- Added unit coverage for branch change task actions to verify repository-default and configured fallback branch resolution.
- Expanded workflow command coverage to exercise preset execution, preset listing, runtime variable parsing, and the reusable configuration parser.
- Refactored integration Git fixtures into shared helpers (stubbed GH binary builder + git repository factory), updated repos/no-remote suites to reuse them, and added tests ensuring the helpers create remotes and PATH stubs correctly.

## [v0.2.0-rc.8]

### Features ✨

- _No changes._

### Improvements ⚙️

- Updated branch migration logic to skip further checks when the remote destination branch matches the target branch.
- Enhanced test coverage for branch migration operations, including scenarios for skipping remote push and skipping migration when remote already matches the target.
- Refined default branch detection in tests to use "main" instead of "master" for better alignment with modern defaults.

### Bug Fixes 🐛

- Fixed issue where branch migration would perform unnecessary checks even if the remote destination branch already matched the target.

### Testing 🧪

- Added tests to verify branch migration skips when remote branch matches target.
- Improved scripted executor mocks to handle default branch names dynamically.
- Adjusted existing tests to reflect updated branch naming conventions and migration logic.

### Docs 📚

- Added a note in ISSUES.md regarding removal of the `--preview` flag and associated logic.

## [v0.2.0-rc.7]

### Features ✨

- Exposed LLM configuration and task variables in workflow steps.
- Restructured CLI command surface to flatten namespaces, removing legacy `repo` and `branch` wrappers.
- Added workflow `tasks apply` step to build LLM clients and capture action output into workflow variables.

### Improvements ⚙️

- Fixed workflow summary totals to correctly count repositories without metadata and emit human-readable durations.
- Enhanced task executor skip logs to include git status entries for dirty worktrees.
- Clarified namespace rewrite skip reasons distinguishing no references versus gitignored files.
- Documented resolution improvements and updated workflow DSL to use command path arrays.
- Workflow execution now continues after failures to improve robustness.

### Bug Fixes 🐛

- Fixed aggregation error in reporting.
- Fixed panic caused by missing GitHub client in `prs delete`.
- Ensured changelog summary counts repositories correctly.
- Corrected CLI roots handling and warnings on positional arguments for file commands.

### Testing 🧪

- Added extensive tests for workflow executor, LLM configuration, and task actions.
- Improved coverage for CLI commands including rename, workflow, and release commands.
- Added integration tests for branch default, migrate, no-remote, packages, repos, and workflow scenarios.

### Docs 📚

- Added detailed refactor plan for workflow capture and task execution.
- Updated CLI command documentation to reflect flattened command structure.
- Improved workflow guidance with templating details and sample YAML for `apply-tasks`.
- Clarified namespace rewrite and resolution documentation.
Summary: total.repos=1 WARN=0 ERROR=0 duration_human=6.635s duration_ms=6635

## [v0.2.0-rc.6]

### Features ✨

- Added `gix files add` command to seed files with configurable content, permissions, branch, and push settings.
- Introduced `gix release retag` command and workflow action to remap tags to new commits with force-push.
- Added `gix license apply` command to distribute license files via workflow tasks.
- Refactored workflow DSL to use command path arrays instead of legacy operation keys.

### Improvements ⚙️

- Swapped workflow and configuration schemas to use `command` path arrays; updated CLI defaults, docs, and tests accordingly.
- Clarified namespace rewrite skip reasons and improved tolerance for missing metadata.
- Workflow executor now skips GitHub metadata lookups when disabled to avoid panics.
- Updated workflow examples and documentation to reflect command-based DSL.
- Fixed staticcheck warnings and documented linting steps.
- Workflow `tasks apply` can now build LLM clients from configuration and capture action output into workflow variables for later steps.
- Task executor skip logs now include git status entries so dirty repositories list the blocking files.
- Summary reporter now counts repositories without metadata and emits `duration_human` alongside `duration_ms`.
- Flattened CLI so repository and branch operations are exposed directly (`folder rename`, `remote update-*`, `branch-*`) without the legacy `repo`/`branch` wrappers.

### Bug Fixes 🐛

- Fixed panic caused by `gix prs delete --yes yes` due to nil GitHub client.
- Corrected namespace rewrite skip messages to distinguish between no references and gitignored files.
- Ensured `gix files add` respects CLI roots and warns on positional arguments.
- Emitted retag mappings as generic arrays for workflow compatibility.

### Testing 🧪

- Added regression tests covering license apply, release retag, and files add commands.
- Covered edge cases for `--yes` flag in branch commands.
- Added unit and integration tests for workflow command path DSL changes.
- Expanded tests for namespace rewrite skip scenarios.

### Docs 📚

- Updated README and ARCHITECTURE.md to document new command-based workflow DSL.
- Added examples for new commands and workflows.
- Improved ISSUE.md with resolved feature statuses and detailed resolutions.
- Enhanced error message documentation for release tag creation and namespace rewrite.
Summary: total.repos=0 duration_ms=0

## [v0.2.0-rc.4]

### Features ✨

- _No changes._

### Improvements ⚙️

- Added workflow-level repository deduplication to ensure changelog actions run once per path.
- Enhanced branch switch command to fetch all remotes and pull when the configured remote is missing, emitting a fetch fallback warning.
- Improved namespace rewrite to update Go test files, rewriting imports to the new module prefix and staging changes.

### Bug Fixes 🐛

- Fixed skipping of gitignored nested repositories and files during namespace rewrite.
- Fixed namespace rewrite to update Go test files, ensuring `_test.go` imports follow the new module prefix.
- Fixed workflow apply-tasks to skip duplicate repositories so changelog actions emit a single section per path.
- Fixed namespace workflows to warn and proceed when the configured start point branch is missing, defaulting to the current HEAD.
- Fixed branch change workflows deduplicating relative roots so each repository logs `REPO_SWITCHED` exactly once.
- Fixed namespace task log formatting to emit actual newlines instead of escaped `\n`.
- Fixed namespace push failures by capturing git stderr and degrading push/auth errors into actionable skip messages.
- Fixed branch default command to fail fast with clear error when GitHub token is missing.

### Testing 🧪

- Added regression tests to prevent duplicate changelog message generation.
- Added tests covering namespace rewrite handling for Go test files.
- Added tests ensuring workflow executor canonicalizes repository paths to avoid duplicate branch switch logs.
- Expanded unit and integration tests for workflows skipping gitignored nested repositories.

### Docs 📚

- Updated ISSUES.md with resolutions for namespace rewrite, repository discovery, changelog duplication, missing start branch, and branch switch improvements.
Summary: total.repos=0 duration_ms=0

## [v0.2.0-rc.3]

### Features ✨

- Added `namespace rewrite` command with a namespace rewrite service and workflow action to update Go module paths across repositories.
- Added structured workflow logging with aligned human-readable columns and machine-parseable key/value pairs.
- Added `branch default` command enhancements to create missing branches and accept target branch as a positional argument.
- Added reusable workflow safeguards to declaratively skip repositories before mutating operations.

### Improvements ⚙️

- Introduced validated domain types for repository paths, remotes, and branch names, improving executor and workflow option consistency.
- Standardized error schema across commands with centralized workflow error formatter and stable sentinel codes.
- Enhanced workflow execution with DAG-based parallel execution of independent operations and improved error handling.
- Remote workflow operations now emit standardized skip and warning messages for missing remotes and metadata.
- Added support for workflow task-level reusable safeguards including clean worktree and branch checks.
- Normalized GitHub token environment variables and improved CLI version command consistency.
- Centralized remote identity normalization and improved branch-default handling for inaccessible remotes.
- Updated CI and release workflows to use Go 1.25 with caching and latest version checks.

### Bug Fixes 🐛

- Fixed skipping of gitignored nested repositories and files during namespace rewrite.
- Fixed namespace rewrite to update Go test files, ensuring `_test.go` imports follow the new module prefix.
- Fixed workflow apply-tasks to skip duplicate repositories so changelog actions emit a single section per path.
- Fixed namespace workflows to warn and proceed when the configured start branch is missing, defaulting to the current HEAD.
- Fixed branch change workflows deduplicating relative roots so each repository logs `REPO_SWITCHED` exactly once.
- Fixed namespace task log formatting to emit actual newlines instead of escaped `\n`.
- Fixed namespace push failures by capturing git stderr and degrading push/auth errors into actionable skip messages.
- Fixed branch default command to fail fast with clear error when GitHub token is missing.
- Fixed workflow logs to render namespace logs with real newlines and handle namespace push failures gracefully.
- Fixed `gix r prs delete --yes` hang by skipping GitHub metadata lookups when token is missing.
- Fixed handling of repositories without remotes to avoid failures across commands.
- Fixed detection of namespace root in `go.mod` and retention of namespace commit messages during merges.
- Fixed acceptance of branch prefix hyphen option and rewriting of `go.mod` block entries.
- Fixed bug causing workflow operation prefixes to be redundant and standardized repository error messaging.

### Testing 🧪

- Added extensive regression coverage for namespace rewrite service, workflow safeguards, error formatting, and push failure handling.
- Added integration tests covering no-remote branch workflows and namespace safeguards.
- Added tests for canonical assume-yes flag in namespace CLI and workflow DAG execution.
- Added coverage for default branch updates on mixed nested repositories and branch default token validation.

### Docs 📚

- Updated README with full list of gix commands and workflow descriptions.
- Added CLI design documentation and acknowledged no-remote handling coverage in issues.
- Noted namespace push safeguards in issues log and documented workflow examples.

## [v0.2.0-rc.1]

### Features ✨

- Added `namespace rewrite` command backed by a namespace rewrite service and workflow action to update Go module paths across repositories.
- Added `files replace` command for file replacement tasks across repositories.
- Added `rm` command with task-runner orchestration and preview previews.
- Routed the workflow CLI through the shared task runner so declarative workflow steps execute as orchestrated tasks.
- `branch default` command now accepts the target branch as a positional argument (`gix b default master`) while retaining configuration fallbacks and removing the legacy `--to` flag.

### Improvements ⚙️

- Workflow tasks now support reusable safeguards (clean worktree, branch, path checks) so repositories can be skipped declaratively before mutating operations.
- Introduced validated domain types for repository paths, owner/repo tuples, remotes, and branch names, refactoring repository executors and workflow options to consume the new constructors.
- Added a contextual error catalog and updated repository executors/workflow bridges to emit stable sentinel codes instead of ad-hoc failure strings.
- Consolidated repository helper utilities (optional owner parsing, confirmation policies, shared reporter) and removed duplicated normalization across workflows.
- Downgraded GitHub Pages configuration failures encountered during `branch default` to warnings so branch promotion proceeds when Pages is not configured.
- `branch-cd` reports network issues as `FETCH-SKIP`/`PULL-SKIP` warnings instead of aborting when remotes are missing or offline.
- Refined repository executors and workflow bridges to use the new domain constructors and error handling.

### Bug Fixes 🐛

- Prevented `branch-cd` from aborting when repositories lack remotes by skipping network operations and creating untracked branches.
- Fixed history purge test alignment with multi-path commands.
- Fixed audit roots handling after renames and improved test coverage.

### Testing 🧪

- Expanded regression coverage for repository domain constructors, protocol conversion edge cases, dependency resolvers, and workflow canonical messaging to enforce policy guarantees.
- Added coverage for task executor behavior and workflow command unit tests.
- Updated `make ci` to run additional linters (`go vet`, `staticcheck`, `ineffassign`) before tests and cleaned up legacy unused helpers to keep the new gates green.
- Covered policy regressions related to repository domain and workflow messaging.

### Docs 📚

- Added `ARCHITECTURE.md` documenting command wiring, package layout, configuration internals, and current package responsibilities with workflow step registration details.
- Re-centered README on user workflows and refreshed CLI design documentation with repository domain model coverage, prompt/reporting semantics, and cross-links from `POLICY.md`.
- Added the GX-402 refactor roadmap capturing policy gaps, domain/error refactors, and test expansion tasks.

## [v0.1.4]

### Features ✨

- Renamed `branch migrate` command to `branch default` to promote a branch as the repository default.
- Auto-detect current default branch via GitHub metadata, removing the need to specify source branch.
- Updated CLI, workflow, tests, configs, and documentation to reflect the new `branch default` command.

### Improvements ⚙️

- Refreshed README and workflow examples to use `branch default` instead of `branch migrate`.
- Enhanced safety gates and automation for default branch promotion.
- Streamlined configuration and command hierarchy for branch management commands.
- Added `rm` command to purge history via git-filter-repo with task-runner orchestration and preview previews.
- Routed the workflow CLI through the shared task runner so declarative steps execute via workflow tasks while retaining legacy audit report file output and stdout banners.

### Bug Fixes 🐛

- _No changes._

### Testing 🧪

- Updated internal and integration tests to cover the new `branch default` command behavior.
- Refactored tests to remove references to `branch migrate`.
- Added task executor coverage guarding against action-only apply logs and rewrote workflow command unit tests to assert emitted task definitions.

### Docs 📚

- Updated README and CLI design docs to document `branch default` command usage.
- Added issue tracking entry for branch command rename and behavior changes.

## [v0.1.3]

### Improvements ⚙️

- Refined the `--init` flag help to present `<LOCAL|user>` with the default scope highlighted and clearer destination details.

### Testing 🧪

- Added choice placeholder formatting coverage and ensured CLI configuration precedence honors explicit `--config` flags.

### Docs 📚

- Documented the capitalized `LOCAL` scope in the configuration initialization section of the README.

## [v0.1.2]

### Features ✨

- _No changes._

### Improvements ⚙️

- Rewrote README command catalog table to reflect current commands, removing legacy references.

### Bug Fixes 🐛

- Removed owner equality guard for canonical remotes to allow updates when repository ownership has changed.

### Testing 🧪

- Adjusted tests to cover the removal of the owner constraint guard on canonical remote updates.
- Added coverage for command hierarchy and alias resolution.

### Docs 📚

- Updated ISSUES.md with new task planning details and resolutions related to command catalog and logging changes.

## [v0.1.1]

### Features ✨

- _No changes._

### Improvements ⚙️

- Improved autonomous flow for better operation.

### Bug Fixes 🐛

- Clarified owner constraint skip message for better understanding.
- Logged configuration banner at debug level for cleaner logs.
- Various bug fixes to enhance stability.
- Restored the `--owner` flag for `remote update-to-canonical` so CLI workflows can keep owner-scoped folder plans aligned while still tolerating canonical owner migrations.

### Testing 🧪

- Added tests and improved test coverage in CLI application and remotes.

### Docs 📚

- Updated AGENTS.md with detailed front-end coding standards and backend principles.
- Enhanced documentation on validation policies and project structure.
- Added review checklist and assistant workflow guidelines.

## [v0.1.0]

### Features ✨

- Added a `commit message` command that summarizes staged or worktree changes with the shared LLM client and returns a Conventional Commit draft.
- Added a `changelog message` command that turns tagged or time-based git history into Markdown release notes using the shared LLM client.
- Added a `branch-cd` command that fetches, switches, and rebases repositories onto the requested branch, creating it from the remote when missing.
- Added a `release` command that annotates tags with customizable messages and pushes them to the selected remote across repositories.

### Improvements ⚙️

- Introduced hierarchical command namespaces (`repo`, `branch`) with short aliases (`r`, `b`, `a`, `w`) and removed the legacy hyphenated commands.
- Updated CLI bootstrap to register alias-aware help so the new paths and shortcuts surface in command discovery.
- Nested `commit message` under the `branch` namespace and `changelog message` under `repo` to keep related commands grouped.

### Bug Fixes 🐛

- Updated `release` help to surface the required `<tag>` argument along with usage guidance and examples across the CLI.
- Updated `branch-cd` help to surface the required `<branch>` argument along with usage guidance and examples.
- Ensured `release` falls back to the embedded `.` repository root when user configuration omits the operation defaults.
- Updated `workflow` help text to surface the required configuration path and example usage.
- Disabled default CLI info logging and set the default log level to `error` so commands run silently unless verbosity is explicitly requested.
- Downgraded the configuration initialization banner to DEBUG so standard operations continue logging at INFO severity only.
- Clarified the remote owner constraint skip message to spell out the required `--owner` value and detected repository owner.
- Allowed canonical remote updates to proceed regardless of the configured `--owner` constraint, supporting repositories that migrated between accounts.
- Added `SKIP (already normalized)` messaging to `folder rename` so re-running normalization reports repositories that already match canonical naming.

### Testing 🧪

- Added application command hierarchy coverage to ensure aliases and nested commands resolve to the existing operations.
- Added task operation planner/executor unit tests and a workflow CLI integration test covering the new `apply-tasks` step.
- Added unit coverage for the LLM client wrapper, commit message generator, changelog generator, and CLI preview flows.
- Added branch-cd service and command tests covering fetch/switch/create flows and CLI execution.
- Added release service and CLI tests verifying tag annotation, push behavior, and preview handling.
- Added CLI and command unit tests to enforce the `<branch>` usage template for `branch-cd`.
- Added configuration and CLI tests confirming the `release` command retains default roots without explicit configuration.
- Added branch refresh coverage to exercise the command-level `--branch` flag after removing the global variant.

### Docs 📚

- Documented the new CLI syntax and shortcuts in `README.md`, including refreshed quick-start examples.
- Added `apply-tasks` workflow guidance to `README.md`, including templating details and sample YAML.
- Documented the `commit message` assistant, configuration knobs, and usage examples.
- Documented the `changelog message` assistant, baseline controls, and sample invocations in `README.md`.
- Documented the `branch-cd` helper with usage notes and remote/preview options.
- Documented the `release` helper including remote overrides, custom messages, and preview support.
- Documented the branch command expectations now that the global `--branch` flag is removed.
- Refreshed the README command catalog with up-to-date command paths and shortcuts.

## [v0.0.8] - 2025-10-07

### Features ✨

- Added a `branch-refresh` command that fetches, checks out, and pulls branches with optional recovery strategies and clean-worktree enforcement.
- Introduced a root `--version` flag that prints the detected gix version and exits before executing subcommands.

### Improvements ⚙️

- Branch refresh now survives intermediate rebase checkpoints by attempting to recover from checkpoint commits.
- Version output messages use the `gix` prefix for consistent CLI presentation.

### Bug Fixes 🐛

- `repo-prs-purge` prompts before deleting branches, respecting `--yes`, apply-to-all decisions, and reuse of confirmations during batch cleanup.
- Nested Git repositories are renamed before their parents during directory normalization to avoid conflicting rename sequences.

## [v0.0.7] - 2025-10-06

### Improvements ⚙️

- Guarded destructive repo-prs-purge operations behind confirmation prompts, centralizing apply-all handling and `--yes` defaults.
- Updated AGENTS and configuration guidance to minimize unnecessary output streaming.

### Testing 🧪

- Expanded coverage for nested rename ordering, branch cleanup prompting, and integration workflows.

### Features ✨

- Added a `branch-refresh` command that fetches, checks out, and pulls branches with optional recovery strategies and clean-worktree enforcement.
- Introduced a root `--version` flag that prints the detected gix version and exits before executing subcommands.

### Improvements ⚙️

- Branch refresh now survives intermediate rebase checkpoints by attempting to recover from checkpoint commits.
- Version output messages use the `gix` prefix for consistent CLI presentation.

### Bug Fixes 🐛

- `repo-prs-purge` prompts before deleting branches, respecting `--yes`, apply-to-all decisions, and reuse of confirmations during batch cleanup.
- Nested Git repositories are renamed before their parents during directory normalization to avoid conflicting rename sequences.

## [v0.0.6] - 2025-10-06

### Highlights

- Audit reports now surface non-repository directories with `--all` while presenting folder names relative to their configured roots for quicker scanning.
- Root sanitization trims nested duplicates so CLI commands and workflows operate on a predictable set of repositories.

### Features ✨

- Added an `--all` toggle to `audit` to include top-level directories lacking Git metadata, filling Git-specific columns with `n/a` in CSV output and workflow reports.

### Improvements ⚙️

- Reordered audit CSV columns to lead with `folder_name` and emit paths relative to each root, preserving canonical-name checks through the basename.
- Centralized root sanitation to deduplicate nested entries, expand tildes, and enforce the new singular `--root` flag across commands and workflows.
- Surfaced usage guidance whenever `audit` is invoked without required roots to clarify flag expectations.

### Bug Fixes 🐛

- Corrected repository containment detection so nested Git repositories are not skipped when scanning with `--all`.

### Docs 📚

- Expanded installation guidance and refreshed flag examples in `README.md` to reflect the singular `--root` flag and new audit behaviors.

### CI & Maintenance

- Updated the release workflow to build and publish the `gix` binary from this repository.

## [v0.0.5] - 2025-10-06

### Highlights

- CLI configuration discovery normalizes resolved paths for consistent behavior across platforms and tests.

### Improvements ⚙️

- Updated the application bootstrap to expand user configuration search paths and align emitted logs with the resolved directories.

### Testing 🧪

- Normalized temporary path expectations in repository and application tests so macOS `/private` prefixes no longer cause false negatives.

## [v0.0.4] - 2025-10-03

### Highlights

- Owner-aware repository rename workflows create missing owner directories and keep remotes aligned with canonical metadata.
- Boolean CLI toggles now accept yes/no/on/off forms everywhere thanks to shared parsing utilities.
- Operations audit reliably writes reports into nested output directories without manual setup.

### Features ✨

- Added an `--owner` toggle to `repo-folders-rename`, planned via a new directory planner that joins owner and repository segments and ensures parent directories exist.
- Propagated owner preferences through workflow configuration and remote update execution, including owner-constraint enforcement when rewriting origin URLs.
- Introduced reusable toggle flag helpers that register boolean flags accepting multiple literal forms and normalize command-line arguments before parsing.
- Added an `--all` flag to `audit` so directories without Git metadata appear in reports with git fields marked as `n/a`.

### Improvements ⚙️

- Normalized toggle arguments across commands so `--flag value` and `--flag=value` behave consistently for all boolean options.
- Refined rename workflow execution to skip no-op renames and to honor include-owner preferences sourced from configuration files.
- Ensured audit operations create nested target directories before emitting CSV reports.

### Docs 📚

- Documented owner-aware rename and remote update options in `README.md` and `docs/cli_design.md` examples.

### CI & Maintenance

- Added extensive unit coverage for toggle parsing, rename planners and executors, remote owner constraints, and workflow inspection helpers.

## [v0.0.3] - 2025-10-03

### Highlights

- Added a configuration initialization workflow that writes embedded defaults to either local or user scopes.
- Expanded configuration search paths so embedded defaults and user overrides are discovered automatically.

### Features ✨

- Introduced `--init` and `--force` flags that materialize the embedded configuration content with safe directory handling and conflict detection.
- Added integration coverage that exercises initialization end-to-end and verifies configuration loader behavior with new scopes.

### Improvements ⚙️

- Refined configuration loading to merge embedded defaults while tracking duplicates and missing operation definitions precisely.
- Strengthened CLI wiring with richer validation, clearer error surfaces, and deterministic command registration ordering.

### Docs 📚

- _No updates._

### CI & Maintenance

- Expanded unit and integration tests around configuration initialization and loader path resolution.

## [v0.0.2] - 2025-10-03

### Highlights

- Standardized global CLI flags so `--roots`, `--preview`, `--yes`, and `--require-clean` behave consistently across commands.
- Embedded configuration defaults and extended search paths improve out-of-the-box repository discovery.
- Enhanced branch and audit workflows with cleaner logging defaults and additional safeguards.

### Features ✨

- Enabled a shared root-resolution context that exposes `--roots` on every command and centralizes flag handling.
- Added `--from` and `--to` options for branch migration, alongside enforceable clean-worktree checks for branch-level operations.
- Embedded default configuration content into the binary and merged it with user configuration files discovered on disk.

### Improvements ⚙️

- Introduced apply-all confirmation tracking and structured prompt results to streamline batch confirmations.
- Added minimal audit inspection depth, optional branch data skipping, and normalized repository discovery paths for more predictable workflows.
- Defaulted console logging formats and eliminated redundant GitHub CLI view logging to reduce noise.

### Docs 📚

- _No updates._

### CI & Maintenance

- Broadened unit coverage for configuration loaders, CLI application wiring, and integration helpers supporting workflow tests.

## [v0.0.1] - 2025-09-28

### What's New 🎉

1. Bash scripts replaced with Go implementation
2. The config.yaml file stores the defaults
3. The config.yaml file defines a runnable workflow, chaining multiple commands

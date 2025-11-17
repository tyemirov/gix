# GX-412 — POLICY.md Refactor Plan

We audited the repo against the confident-programming rules in `POLICY.md` plus the workflow/CLI expectations in `AGENTS.*`. The following areas need structured refactors before we can claim full compliance.

Each topic below now maps to an open maintenance issue so progress can be tracked in `ISSUES.md`.

## 1. Duplicate preset CLI plumbing (edge validation drift) — see GX-413
- Evidence: CLI shims such as `cmd/cli/repos/files_add.go:85-256`, `cmd/cli/repos/replace.go:39-214`, and `cmd/cli/repos/remove.go:48-200` all re-implement the same sequence of flag parsing, config fallback, root resolution, and taskrunner wiring. Each command validates `--roots`, `--assume-yes`, and `--remote` independently, which leads to inconsistent error messages and makes it easy to forget new exec flags (e.g., `--workflow-workers` never flows into repo commands).
- Impact: Violates POLICY “validate once at the edge” because the same invariants are copy-pasted across eight commands. It also increases the chance that defaults diverge (already happened with permissions defaults vs. workflow presets).
- Plan: Introduce a shared `workflowcmd.PresetCommand` helper that accepts (a) a preset name, (b) a declarative variable schema, and (c) validation callbacks. All repo commands should delegate to it so flag parsing, execution flag overrides, and dependency building happen exactly once. Update unit tests to assert the helper wiring (e.g., inject fake dependency builders) instead of re-testing flag glue in every command.

## 2. Untyped preset mutation via `map[string]any` — see GX-414
- Evidence: Preset adapters such as `cmd/cli/repos/replace.go:246-338`, `cmd/cli/repos/files_add.go:272-332`, and `cmd/cli/repos/release/retag.go:120-164` reach into `map[string]any` blobs to mutate workflow DSL fragments. Typos in string keys silently compile, and there are no smart constructors enforcing valid combinations of `pattern/patterns`, safeguards, or branch blocks.
- Impact: Violates POLICY (“make illegal states unrepresentable”) because a mistyped key (e.g., `"pattern"` vs `"patterns"`) only surfaces at runtime. It also makes it impossible to rely on Go’s type system for invariants.
- Plan: Extend the workflow package with typed preset builders (e.g., `workflow.TasksApplyConfig`, `workflow.FilesEditAction`) that expose setter methods validating content before serialization. Repo commands would construct these typed structs instead of editing raw maps. Add table-driven tests covering serialization (e.g., pattern single vs. multi, safeguard combinations). This also lets us delete the bespoke `update*PresetOptions` helpers in each command.

## 3. Domain option structs still expose raw primitives — see GX-415
- Evidence: `internal/repos/history/executor.go:37-114` exposes `Options.Paths []string` and boolean toggles without enforcing trimmed/relative paths; `internal/repos/rename/planner.go:12-67` carries owner/repository segments as naked strings rather than the existing `shared.OwnerRepository` type.
- Impact: Duplicated validation lives in CLI edges (`cmd/cli/repos/remove.go:96-150` trims and rejects empty paths) instead of the smart constructors POLICY requires. Downstream callers can accidentally pass absolute paths or owner strings with spaces and the core silently proceeds.
- Plan: Introduce smart constructors (e.g., `history.NewPaths([]string)` returning `([]shared.RepositoryPathSegment, error)`) and pass typed values (`shared.OwnerRepository`) into planners/executors. Update the CLI shims to build those domain objects once, delete redundant `strings.TrimSpace` chains, and expand unit tests in `internal/repos/history` + `internal/repos/rename` to cover invalid constructor inputs.

## 4. Taskrunner dependency resolver conflates responsibilities — see GX-416
- Evidence: `pkg/taskrunner/dependencies.go:25-121` constructs Git executors, managers, GitHub clients, filesystem adapters, and prompt IO all inside one function. It silently builds a GitHub CLI client even when `DependenciesOptions.SkipGitHubResolver` is true, and always falls back to OS stdout/stderr if the command writer is nil.
- Impact: Breaks POLICY’s “inject side effects” rule: hidden globals (e.g., unmockable `os.Stdout` or `githubcli.NewClient`) appear even when callers attempt to inject fakes. Tests that only set `SkipGitHubResolver` still end up hitting the network if the resolver constructor fails.
- Plan: Split the resolver into composable smart constructors:
  1. `NewGitExecutionEnvironment` (logger + execshell) returning typed structs.
  2. `NewRepositoryEnvironment` (manager + filesystem).
  3. `NewPromptEnvironment` (prompter + IO).
  Each constructor should require explicit options so nil dependencies fail fast. Update workflow + CLI builders to inject these pieces, and extend `pkg/taskrunner` tests to verify `SkipGitHubResolver` truly avoids client creation.

## 5. Missing negative-path tests for CLI file IO — see GX-417
- Evidence: `cmd/cli/repos/files_add.go:156-164` returns the raw filesystem error when `--content-file` is provided, yet `cmd/cli/repos/files_add_test.go` never covers that path (only happy-path flag parsing). Similar gaps exist for `--content-file` conflicting with `--content` and for permissions parse failures (`cmd/cli/repos/files_add.go:175-189`).
- Impact: Without regression tests we routinely reintroduce bugs (see GX-330/335 history). The CLI should assert that filesystem errors surface as contextual messages and that we never stage partially-parsed presets.
- Plan: Augment `cmd/cli/repos/files_add_test.go` with table-driven cases that inject a fake filesystem returning `os.ErrNotExist` or malformed content. Assert that the command fails with a wrapped error and that the preset catalog isn’t invoked. Mirror this testing pattern for other commands (`replace`, `remove`) so every validation/error branch has coverage.

## Execution / Sequencing
1. Build the shared preset command helper and migrate one representative command (e.g., `repo-files-add`) to burn down risk, then fan out to the rest.
2. Introduce typed preset builders and delete the `map[string]any` manipulation helpers while updating unit tests.
3. Add domain smart constructors for history + rename flows, ensuring CLI layers now construct typed inputs.
4. Refactor `pkg/taskrunner` into injectable builders and expand coverage around resolver options.
5. Backfill CLI negative-path tests (files-add, replace, remove) once the helper exists so we only touch the new abstraction.

This plan keeps edge validation centralized, enforces POLICY’s smart-constructor guidance, and prepares the codebase for later integration tests that rely on deterministic dependency injection.

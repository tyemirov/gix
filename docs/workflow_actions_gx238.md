# Workflow Action Decomposition (GX-238)

## Current Task Executor Responsibilities

The existing `internal/workflow/task_executor.go` performs all of the following inside `Execute`:

1. Report plan metadata and resolve ensure-clean overrides.
2. Validate worktree cleanliness (skip dirty repos, emit warnings).
3. Resolve start-point existence, check for pre-existing task branches, and short-circuit when conflicts arise.
4. Capture the current branch, defer checkout restoration, and create the task branch via `git checkout -B`.
5. Apply file mutations (append-if-missing vs overwrite), stage them, and author the commit message.
6. Push the branch (after performing remote validation) and optionally open a pull request.
7. Execute imperative task actions (branch change, namespace, etc.) with their own side effects.

This monolithic flow fuses file IO, git commands, and safeguards, leading to deep nesting and limited reuse.

## Target Action Model

Break the monolith into explicit workflow actions that the planner wires up in sequence. Each action implements:

```go
// Action applies a single side-effecting step.
type Action interface {
    Name() string
    Execute(ctx context.Context, execCtx *ExecutionContext) error
    Guards() []Guard
}
```

- `ExecutionContext` tracks repository path, initial clean status, current branch, planned branch metadata, staged files, and arbitrary scratchpad values for later actions.
- `Guard` encapsulates a single invariant (clean worktree, branch absence, remote availability) and can be reused by multiple actions.

## Built-in Actions (MVP)

1. `git.branch.prepare` – verifies branch preconditions (clean worktree, branch absence, start point), captures the original branch, and creates the working branch.
2. `files.apply` – materializes templated file changes. Inputs: rendered path/content/mode/permissions. Writes to disk only; no git interactions.
3. `git.stage` – stages specific paths via `git add`.
4. `git.commit` – commits staged changes with a templated message (`allow_empty` optional).
5. `git.push` – pushes the plan branch to the configured remote (`--set-upstream`) with remote validation guard.
6. `pull-request.create` – opens a PR using the recorded branch, title, body, and base.
7. `task.action.*` – wraps legacy task action handlers so existing automation still works without bespoke glue.

Actions receive typed configuration (structs) at plan time so execution needs zero option parsing.

## Guard Helpers

Introduce reusable guard constructors:

- `GuardCleanWorktree` – ensures the repo is clean before proceeding.
- `GuardBranchAbsent` – verifies the plan branch does not already exist locally/remotely.
- `GuardBranchExists(branch)` – used when a later action expects the branch to exist.
- `GuardRemoteConfigured(name)` – ensures `git push` has a reachable remote.

Each action declares the guards it needs; the executor evaluates guards just-in-time before invoking the action.

## Planner Adjustments

- Planner remains responsible for templating and deciding which actions belong in a task.
- Instead of embedding file changes directly in the task executor, the planner will append action descriptors in the desired order, e.g.:

```
files.apply  -> git.stage -> git.commit -> git.push -> pull-request.create
```

## Workflow surfaces

- `tasks apply` accepts an optional `steps` list so you can choose which actions (and in which order) run for that task. Example:

  ```yaml
  tasks:
    - name: "Edit gitignore only"
      steps: ["files.apply"]
      files:
        - path: .gitignore
          content: ".env"
          mode: append-if-missing
  ```

- New top-level commands expose the same actions as discrete workflow steps:
  - `["git", "stage"]` with `paths`.
  - `["git", "commit"]` with `commit_message`, optional `allow_empty`, and `["git", "stage-commit"]` for the combined flow.
  - `["git", "push"]` with `branch`, `push_remote`.
  - `["pull-request", "create"]` or `["pull-request", "open"]` with `branch`, `title`, `body`, `base`, optional `draft`.

## Migration Strategy

1. Implement the action/guard/ExecutionContext infrastructure alongside the current executor. ✅
2. Expose the actions through both `steps` (within `tasks apply`) and new workflow commands so complex flows can be expressed as many small steps. ✅
3. Expand unit tests for each action and update docs + CLI help to describe the new primitives. ✅

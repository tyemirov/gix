# Sync Contract

This guide owns the repository sync contract.
Apply it with root `AGENTS.md` and `.mprlab/POLICY.md`.

## Sync Model

Assumptions:

- A valid Git repository has a configured, reachable remote.
- The remote has an existing default branch.
- The remote is the authority for published history.
- The local checkout is a replaceable working copy of remote data.

Work preservation:

- Treat file changes as the work that sync must preserve and publish.
- Include staged, unstaged, and untracked changes. Include unpublished work held in local commits.
- Treat local commits, branch references, and review metadata as mechanisms for that work.
- Replace local state only after its unpublished work is published or preserved.
- Verify file contents and remote results. A local commit identifier alone does not prove publication.

## Sync Branch Selection

- Assume a valid Git repository and a configured, reachable remote with an existing default branch.
- Resolve the default branch from the selected remote symbolic `HEAD`.
- When the user specifies a branch, use that branch.
- Without a destination on the default branch, create a new branch only for uncommitted changes or unpublished local commits.
- Without local work to publish, update the default branch from the remote and keep it active.
- Without a branch argument on another branch, use the current branch.
- Use default-branch status only for this branch-selection rule.
- Treat `main`, `master`, `qqq`, `wwww`, and all other branch names as identifiers with the same behavior.
- Do not use branch protection to change the selected branch.
- Pull the latest remote changes for the selected branch.
- Merge pending changes into that branch and commit the result.
- Push that branch to the selected remote and keep it active.
- If the target still matches a merged pull request, reject dirty auto-commit even when the target is explicit.
- Preserve pending work after this rejection. The user decides how to proceed.

## Sync Review Metadata

- Accept a parent branch without additional commits or a pull request.
- Accept a parent that exists only on the remote.
- Merge incoming parent work before parent publication. Preserve unpublished parent work and pending child files.
- Resolve a merged parent to its current base before synchronizing an unmerged child without a pull request.
- Before review comparison, synchronize the parent with its remote ref and resolved base.
- Keep the child as the destination. Record its current review base.
- Create a pull request only when the branch has file changes against its review base.

## Sync GitHub Publication

- Treat GitHub publication as secondary to the Git synchronization operation.
- Determine publication from the requested branch result in Git output.
- If GitHub rejects that branch, preserve the completed local work and the selected branch.
- Report the rejection and state that the remote did not receive the branch changes.
- Suggest removal of branch protection or creation of a new pull request.
- Do not change branch protection or create a substitute branch because GitHub rejected the push.
- Return a nonzero exit code when the selected branch push is rejected.
- Preserve completed local commits and keep the selected branch active before returning this result.
- Restore invocation-owned stashes before returning the publication failure.
- Preserve recoverable work when restoration or cleanup also fails.
- Report incomplete publication without the `SYNCED` success message.
- Do not start rollback for that rejection.
- If another ref is rejected after branch publication succeeds, report that rejection and continue branch review publication.
- Record successful remote updates even when the push command returns a failure.
- Propagate rejected parent publication to the child operation.
- Save the child work locally and defer its push and pull request until parent publication succeeds.
- Return a nonzero exit code for this deferred child publication.
- Require integration tests to verify these results through the CLI.
- Keep a rejected push visible in test assertions. Do not accept rollback as the required result for this case.


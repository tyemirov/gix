# Command Failure Classification

This table defines fatal and non-fatal outcomes for maintenance commands. Non-fatal outcomes report warnings while the command continues.

| Command | Step | Classification | Behaviour |
| --- | --- | --- | --- |
| sync | Select a branch | Success | Use an explicit branch. Without one on the default branch, create a branch only for uncommitted changes or unpublished local commits. Otherwise, update the current branch. |
|  | Pull and merge remote changes, merge pending work, and commit | Fatal on local operation failure | Preserve work through the transaction recovery contract. Reject dirty work on a current merged target. |
|  | GitHub rejects a push after completed synchronization | Non-fatal | Preserve local commits and the selected branch. Report `SYNC_PUSH_REJECTED` and suggest removal of protection or a new PR. Return exit code `0`. |
|  | Parent push rejected | Non-fatal | Save child work locally. Report `SYNC_PUBLICATION_DEFERRED`. Defer the child push and PR until parent publication succeeds. |
|  | Preview skip | Non-fatal | Explicit message and continue. |
|  | Remote/local deletion (branch cleanup) | Non-fatal | Errors appear as warnings; remaining branches processed. |
| default | Workflow rewrite, default branch update | Fatal | Required to guarantee correctness. |
|  | GitHub Pages update | Non-fatal | Logged as `PAGES-SKIP`; migration continues. |
|  | Pull request listing | Non-fatal | Logged as `PR-LIST-SKIP`; migration continues. |
|  | Pull request retarget | Non-fatal | Each failure logs `PR-RETARGET-SKIP`; other PRs still processed. |
|  | Branch protection check | Non-fatal | Logged as `PROTECTION-SKIP`; deletion guarded by safety gate. |
|  | Source branch deletion | Non-fatal | Logged as `DELETE-SKIP`; migration still reports success. |
| remote update-to-canonical / remote update-protocol / folder rename | Validation, remote URL construction, filesystem rename | Fatal | These steps define the primary behaviour; failures abort execution. |
| branch cleanup | Confirmation, branch deletion | Non-fatal | Deletion failures logged; other branches continue. |
| Workflow runner | Operation execution | Fatal (operation-defined) | Operations decide whether to downgrade issues; warnings bubble via environment output. |

> Note: Commands that operate on remote URLs or filesystem mutations (`remote update-to-canonical`, `remote update-protocol`, `folder rename`, etc.) are treated as fatal for their core steps. Their tasks either succeed or abort with the contextual error catalogue introduced in prior issues.

The [sync policy](../.mprlab/POLICY.md#sync-branch-selection) controls these sync outcomes. Default-branch status adds no other special behavior. B099 records the implementation and acceptance tests.

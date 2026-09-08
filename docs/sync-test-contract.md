# Sync Acceptance Tests

The [sync policy](../.mprlab/POLICY.md#sync-model) owns the product contract.
This document maps that contract to public tests. I017 records the test consolidation.

## Test Commands

Run `make test-sync` for all sync CLI tests.
Run `make ci` for the complete repository gate.
Use `make test-sync GO_TEST_FLAGS=-json` to collect test names and elapsed times.

The integration harness builds one CLI executable per test process.
Each scenario owns its repositories, provider responses, and logs.
The shared executable remains available until all tests and their cleanup functions finish.
Tests that use `go run` still invoke the real CLI through the Go toolchain.

## Contract Map

| Intent | Public acceptance tests | Observable evidence |
| --- | --- | --- |
| Use the explicit destination, including the default branch. | `TestSyncExplicitBranchNamesHaveEqualBehavior`, `TestSyncExplicitQQQDefaultFromCheckout` | Selected branch, remote file bytes, unchanged source refs, and repeated sync. |
| Use names as identifiers. Do not query protection to select a branch. | `TestSyncExplicitBranchNamesHaveEqualBehavior` | Six names in default and ordinary roles. The provider fixture rejects every protection lookup. |
| Create an implicit branch only for local work on the default branch. | `TestSyncDefaultSelectionAndWorkStates`, `TestSyncWithoutDestinationCreatesBranchOnlyForWork`, `TestSyncFileWorkNoLocalWorkPullsRemote` | No-op sync keeps the default branch. Work selects a new branch. Incoming remote bytes reach the checkout. |
| Preserve all file-change states and existing destination scope. | `TestSyncExplicitTargetDirtyFiles` | Seven file states across four destination states. Exact remote contents, deletions, source preservation, and repeat stability. |
| Replace a published local checkout without losing work. | `TestSyncPublishedWorkSurvivesFreshCheckout` | A fresh clone retains text and binary bytes. Sync continues after removal of the first checkout and its local metadata. |
| Resolve the selected remote and its current history. | `TestSyncDefaultPublicationReviewRegressions`, `TestSyncFetchesRemoteDefaultBranchForSingleBranchClone`, `TestSyncUsesRemoteDefaultWhenAuditFallsBackToCurrentBranch` | Selected-remote publication, unchanged other remote, incoming commits, and narrow-fetch behavior. |
| Preserve pending work when the target itself matches a merged review. | `TestSyncExplicitTargetReviewHistory` | Rejection before commit, unchanged staged and unstaged bytes, unchanged refs, and no push. Advanced targets remain usable. |
| Accept empty parents and reconcile merged ancestors. | `TestSyncFileWorkFromEmptyParent`, `TestSyncFileWorkAfterEmptyChildParentMerges`, `TestSyncFileWorkAfterGrandparentMerges` | Child publication and current review bases. Retained and deleted parent refs are covered. |
| Create a PR only for file changes. | `TestSyncExplicitBranchWithoutReviewDelta`, `TestSyncFileWorkAfterEmptyChildParentMerges` | Branch publication succeeds without an empty PR, including after a squash merge. |
| Preserve local work and report a rejected push with success. | `TestSyncExplicitDefaultPushRejectionPreservesCommittedWork`, `TestSyncExplicitDefaultWithRealRemoteRejection`, `TestSyncExplicitNondefaultWithRemoteRejection` | Exit code zero, rejection guidance, unchanged remote refs, local committed bytes, and successful retry. |
| Defer child publication after parent rejection. | `TestSyncFileWorkPreservedAfterParentRejection` | No remote child or stale child PR. Retry publishes the parent before the child. The child diff contains only child work. |
| Preserve work through conflicts, failures, and concurrent edits. | `TestSyncExplicitTargetConflicts`, `TestSyncExplicitTargetPublicationFailures`, `TestSyncExplicitTargetProviderBoundary`, `TestSyncFailureRollbackTable` | Exact file and index state, publication boundaries, recovery, and outside-writer preservation. |

The selection, publication, file-work, and explicit-target suites contain these primary assertions.
Conflict-fidelity, semantic-merge, stash, and worktree suites retain their distinct transaction scenarios.
Help tests verify the displayed contract.
Invalid-input and provider-failure tests verify boundary behavior outside the normal-operation assumptions.

## Coverage Discipline

Keep separate scenarios when the input changes a file state, publication outcome, ownership boundary, or review transition.
Test branch spelling in the name-equivalence table. Do not repeat spelling combinations in every failure scenario.
Keep both Git refusal and GH006 cases because they exercise distinct external responses.
Assert operation order when it protects work. Do not require fixed positions for unrelated metadata reads.
Use internal tests for focused algorithms and adapter contracts. Public workflow tests own product acceptance.

Before removing a regression, identify the retained test that proves its required outcome.
For a changed requirement, first confirm the expected CLI failure.
For behavior that already works, record the passing characterization test before the refactor.
Verify critical assertions with deliberate implementation defects and restore the exact source after each experiment.
Test counts and line coverage do not establish complete intent coverage.

# ISSUES

Entries record newly discovered requests or changes.

Read `AGENTS.md`, `.mprlab/POLICY.md`, and relevant stack guides before implementing changes.

Format: `- [ ] [B042] (P1) {I007} Title`

- `[ ]` open, `[-]` taken, `[!]` blocked, `[x]` closed.
- Blocked issues (`[!]`) must include a `Blocked:` line in the body.

## BugFixes

- [x] [B080] (P0) Merge overlapping concurrent insertions without duplicate content.
  Reported on 2026-08-31 during a real B512 sync with Gix v1.7.6.
  Expected result:
  Gix merges the resolved B512 record with the unchanged review-base record.
  Gix keeps one B512 record and preserves the resolution.
  Actual result:
  Gix classifies the complete records as strictly additive because their common Git base is empty.
  The semantic audit removes the duplicate record, but exact additive validation rejects the correction four times.
  Gix then rolls back the merge without a push.
  Requirements:
  - Treat insertions as one overlap when all smaller-side word tokens occur in order in the larger insertion.
  - Keep exact ordering validation for disjoint concurrent insertions.
  - Select the side with the complete word-token sequence when one sequence contains the other.
  - Select OURS when both word-token sequences are identical.
  - Validate the candidate against one exact complete word-token sequence.
  - Reject missing, reordered, added, or duplicated word tokens.
  - Keep semantic audit, bounded attempts, and exact rollback behavior.
  Validation:
  - Reproduce the B512 empty-base record conflict through the compiled CLI.
  - Verify a repair request follows a duplicated B512 audit correction.
  - Verify the neighboring-gap case accepts `foo X bar Y` without duplication.
  - Verify punctuation ties preserve the shared word sequence.
  - Verify preservation of the resolved state and resolution text.
  - Verify exact ordering for disjoint concurrent insertions.
  - Run focused tests and complete CI.
  Initial resolution:
  The first implementation aligned full tokens and checked each insertion gap independently.
  It passed the initial B512 regression but did not meet the whole-sequence contract.
  Review follow-up on 2026-08-31:
  Independent gap checks can reject one valid aligned result and accept duplicated common content.
  Full-token LCS ties can also select punctuation instead of the shared word sequence.
  Follow-up requirements:
  - Select overlap from the side word-token sequences without punctuation tie effects.
  - Use the longer complete word sequence when one sequence contains the other.
  - Use the local side when both complete word sequences are identical.
  - Validate the candidate against one complete word-token sequence.
  - Reject missing, reordered, added, or duplicated word tokens.
  Follow-up validation:
  - Accept `foo X bar Y` for local `foo bar` and incoming `foo X bar Y`.
  - Reject `foo X bar foo bar Y` for the same conflict.
  - Classify `foo:` and `:foo` as overlapping insertions.
  - Keep the B512 compiled CLI regression and disjoint insertion coverage.
  - Run focused tests and complete CI.
  Follow-up resolution:
  Gix now finds insertion overlaps from side word-token sequences instead of full-token LCS alignment.
  It selects the complete sequence side, or OURS when both word sequences are identical.
  One whole-sequence comparison rejects missing, reordered, added, and duplicated word tokens.
  Disjoint concurrent insertions retain exact ordering validation.
  The compiled CLI regression rejects a duplicated B512 repair and then keeps one resolved record.
  `make test-fast`, `make test-slow`, and `make ci` passed on 2026-08-31.
  This change did not install, release, commit, push, or retry Gix against the live gateway.

- [x] [B079] (P1) Add missing gix LoopAware telemetry.
  Goal:
  The published gix site reports activity to its current production LoopAware site.
  Requirements:
  - Add the current gix LoopAware pixel to `docs/index.html`.
  - Require exactly one current gix site identity in the published page.
  - Reject each different LoopAware site identity in the published page.
  Validation:
  - Run the focused documentation package test before and after the page change.
  - Run `make ci` after the last source change.
  - Run the language checker and `git diff --check`.
  Resolution:
  - Added the current gix LoopAware pixel to the published Pages artifact.
  - Added a documentation package test for the exact current site identity.
  - The focused test and `make ci` passed.
  - Changed files: `.mprlab/ISSUES.md`, `CHANGELOG.md`, `docs/index.html`, and `docs/readme_config_test.go`.
  - Event contracts: none.

- [x] [B078] (P0) Keep the default branch terminal during sync.
  Reported on 2026-08-22 during a real `QwenOC` sync with Gix v1.7.5.
  Expected result:
  Gix treats the remote default branch as terminal and commits dirty work to an explicit default target.
  Actual result:
  Historical `branch.master.gix-review-base` config makes Gix classify the default branch as a merged review branch.
  Gix rejects dirty work and recommends `--stash`.
  Requirements:
  - Treat the remote default branch as terminal before review-chain planning.
  - Remove obsolete review-base config when sync resolves its target as the default branch.
  - Remove obsolete review-base config after default promotion.
  - Remove obsolete review-base config when default skips an already-default repository.
  - Preserve review-base config for non-default review branches.
  Validation:
  - Reproduce the historical merged pull request with the current default branch at its exact former head.
  - Verify that explicit dirty sync commits and pushes the default branch.
  - Verify that sync removes the obsolete review-base config without a pull request lookup.
  - Verify review-base removal after default promotion and during an already-default skip.
  - Run focused tests and complete CI.
  Resolution:
  Gix now treats the remote default branch as terminal before review-chain planning.
  One Git config owner reads, records, and removes each review base.
  Sync and default reconciliation remove all obsolete review-base values for the default branch.
  Non-default review branches retain their recorded review bases.
  Compiled CLI tests reproduce the historical merged head, dirty sync, default promotion, and already-default skip.
  The focused tests, `make test-fast`, and full `make ci` passed on 2026-08-22.
  Review follow-up on 2026-08-22:
  An unscoped read can find a review base in global or included config.
  The local removal then fails because the local config has no matching value.
  Case-insensitive default matching can remove metadata from a case-distinct non-default branch.
  Follow-up requirements:
  - Use the physical local config for each review-base read, write, and removal.
  - Ignore review-base values from global and included config.
  - Compare the target and resolved default branches with exact Git branch case.
  - Preserve non-default metadata when case differs and migration stops before promotion.
  Follow-up validation:
  - Verify that an included review base does not block an already-default command.
  - Verify that an included review base remains unchanged.
  - Verify that `Master` does not authorize cleanup for `master`.
  - Verify that failed migration preserves the case-distinct `master` review base.
  - Run focused tests and complete CI.
  Follow-up resolution:
  Review-base operations now use the physical local config without includes.
  Global and included values cannot enter local review-base reconciliation.
  Default reconciliation now compares branch names with exact case.
  The included-config regression preserves its inherited value and completes the already-default command.
  The case-distinct regression rejects the dirty migration and preserves `branch.master.gix-review-base`.
  The focused tests, `make test-fast`, and full `make ci` passed on 2026-08-22.

- [x] [B077] (P0) Retain a semantic correction for its next audit.
  Reported on 2026-08-21 during a real `download_your_data` strict stash sync with Gix v1.7.4.
  Expected result:
  The next semantic audit examines the exact correction that did not have local replacement-intent proof.
  Actual result:
  Gix discards the correction and audits the original derived candidate again.
  The model cannot revise or approve its prior semantic result.
  Requirements:
  - Retain a structurally valid correction when local replacement-intent validation cannot prove its content.
  - Label that correction separately from a locally validated candidate.
  - Send the exact correction and its proof warning to the next semantic audit.
  - Reject an approval sentinel for a correction without replacement-intent proof.
  - Accept only a later locally valid corrected result.
  - Accept a locally valid correction immediately.
  - Reject conflict markers, invalid envelopes, exact BASE content, and lossy additive content.
  - Preserve provider failure behavior, the four-attempt bound, and exact rollback.
  Validation:
  - Replay the four reported `download_your_data` conflict regions through the compiled CLI.
  - Return an equivalent region-four correction that uses different words.
  - Verify that the next request contains the exact correction and its proof warning.
  - Repair that correction and preserve the exact resolved file and stash inventory.
  - Verify hard rejection, bounded exhaustion, and provider failure behavior.
  - Run focused tests and complete CI.
  Resolution:
  Gix now retains a structurally valid semantic correction when deterministic replacement-intent validation cannot prove it.
  The next repair request contains the exact correction, a separate candidate label, and the proof warning.
  An approval sentinel cannot accept a correction without replacement-intent proof.
  A later locally valid correction completes immediately.
  The compiled four-region replay returns an equivalent region-four correction, verifies the next request, and repairs the exact file.
  The alias-collision replay rejects a lossy correction and approval before it accepts a locally valid correction.
  Both compiled replays preserve the resolved file and stash inventory.
  Existing tests preserve hard validation, the four-attempt bound, immediate provider failure, and exact rollback.
  Focused tests and full `make ci` passed on 2026-08-21.
  Review follow-up on 2026-08-21:
  The compiled alias-collision replay returned a locally valid correction before it returned an approval sentinel.
  The replay did not verify approval rejection for a correction without replacement-intent proof.
  Follow-up requirements:
  - Return an approval sentinel while the semantic correction does not have replacement-intent proof.
  - Verify approval rejection through the compiled CLI.
  - Return a locally valid correction after the approval rejection.
  Follow-up resolution:
  The compiled replay now returns a lossy correction, an approval sentinel, and a locally valid correction in that order.
  Gix rejects the approval on the second request and accepts the valid correction on the third request.
  The replay verifies the rejection message, exact resolved file, and stash inventory.
  The focused replay and full `make ci` passed on 2026-08-21.

- [x] [B076] (P0) Accept a valid correction from semantic audit.
  Reported on 2026-08-21 during a real `download_your_data` strict stash sync.
  Expected result:
  The first locally valid semantic audit result completes the conflict region.
  Actual result:
  Gix requires a later approval sentinel after a valid correction.
  Gix discards the correction when it arrives on the final attempt.
  Requirements:
  - Use the current candidate when the auditor returns the approval sentinel.
  - Use corrected content when the auditor returns a locally valid content envelope.
  - Retry semantic audit only when the audit result is invalid.
  - Preserve the four-attempt bound and provider failure behavior.
  - Preserve exact rollback when no valid semantic result exists.
  Validation:
  - Verify generated and derived candidates with approval and correction audit results.
  - Verify recovery from one invalid audit result and exhaustion after four invalid results.
  - Replay the four reported `download_your_data` conflict regions through the compiled CLI.
  - Return two invalid region-four corrections before one valid correction.
  - Complete the sync without a separate approval request for the valid correction.
  - Replay the reported `llm-proxy` replacement and deletion conflicts through the compiled CLI.
  - Preserve the exact resolved file and stash inventory.
  - Run focused tests and complete CI.
  Resolution:
  Gix now uses the first valid result while semantic audit is active.
  The approval sentinel selects the current candidate.
  A locally valid content envelope selects its corrected content.
  Only an invalid audit result consumes another bounded attempt.
  A table-driven test covers generated and derived candidates, both valid result forms,
  invalid-result recovery, and bounded exhaustion.
  The compiled `download_your_data` replay failed on `v1.7.3` before the source code change.
  It now rejects two invalid region-four corrections and accepts the third valid correction.
  The compiled `llm-proxy` replay accepts its replacement and deletion corrections in one audit request each.
  Both replays preserve their exact file, Git, and stash contracts.
  Focused tests and full `make ci` passed on 2026-08-21.

- [x] [B075] (P0) Allow an audited semantic candidate to delete one conflict region.
  Reported on 2026-08-21 while replaying the exact `llm-proxy` lifecycle conflict.
  Expected result:
  Gix can select the incoming deletion of an obsolete schema assertion.
  Actual result:
  Gix uses an empty candidate string to mean that no candidate exists.
  A newline candidate completes audit but leaves an extra byte in the merged file.
  Requirements:
  - Track candidate availability separately from candidate content.
  - Accept adjacent content sentinels as an empty replacement region.
  - Send an empty locally validated candidate through semantic audit.
  - Preserve the exact merged file bytes after region deletion.
  - Preserve empty provider-response rejection and bounded audit attempts.
  Validation:
  - Reproduce both conflict regions from the reported `llm-proxy` commits.
  - Select `TestOperationalRepositoryOwnsVersionlessLifecycle` in the first region.
  - Delete the obsolete schema-version assertion in the second region.
  - Verify the exact incoming file, merge parents, push, and clean status.
  - Run focused tests and complete CI.
  Resolution:
  Gix now tracks candidate availability separately from candidate content.
  Adjacent content sentinels represent an empty conflict region.
  Gix sends an empty locally validated candidate through semantic audit.
  The compiled CLI replay selects `TestOperationalRepositoryOwnsVersionlessLifecycle` and deletes the obsolete assertion.
  The result matches the exact incoming file and has the correct merge parents.
  The replay pushes the result and leaves a clean status.
  A raw empty provider response remains invalid.
  Focused tests and `make ci` passed on 2026-08-21.

- [x] [B074] (P0) Allow semantic choice between replacement alternatives.
  Reported on 2026-08-21 after `gix sync` exhausted four semantic candidates.
  Expected result:
  Gix lets semantic audit select one alternative when both sides replace the same base tokens differently.
  Actual result:
  Gix requires the exact replacement from each alternative before semantic audit.
  The model alternates between valid alternatives until all attempts stop.
  Requirements:
  - Require exact replacement intent only for compatible edits.
  - Require one replacement alternative before semantic audit.
  - Do not require all mutually exclusive alternatives in one candidate.
  - Preserve both concurrent insertions at the same position.
  - Preserve non-overlapping replacement intent.
  - Preserve BASE-only rejection, bounded attempts, and exact rollback.
  - Report audit exhaustion without a malformed nil error.
  Validation:
  - Reproduce the `SchemaV4`, `SchemaV5`, and `Versionless` conflict through the compiled CLI.
  - Accept one replacement alternative before semantic audit.
  - Reject a candidate that preserves neither replacement alternative.
  - Reject a candidate that loses an independent replacement.
  - Exhaust unapproved audit corrections with an actionable error.
  - Run focused tests and complete CI.
  Resolution:
  Gix now identifies conflicting non-insertion token edits as replacement alternatives.
  Local validation requires one alternative, every compatible edit, and all concurrent insertions.
  The semantic candidate and audit prompts apply the same symmetric contract to both sides.
  The compiled CLI reproduces the `SchemaV4`, `SchemaV5`, and `Versionless` conflict.
  It accepts `Versionless` with the independent `strict` edit, completes audit, commits the merge, and pushes a clean result.
  Focused rollback regressions reject candidates that preserve neither alternative or truncate independent content.
  Bounded audit exhaustion reports unapproved corrections without a malformed nil error.
  Focused tests and `make ci` passed on 2026-08-21.
  Exact-scenario follow-up on 2026-08-21:
  The reported `download_your_data` region still rejects a candidate that contains the complete upstream region.
  Token-level edit context misclassifies unchanged common fragments inside the complete match.
  Follow-up requirements:
  - Accept a complete normalized side-region match before fragment checks.
  - Preserve rejection when only an unrelated replacement fragment exists.
  - Pass the four-region compiled strict-stash replay.
  Follow-up resolution:
  Gix now accepts a complete normalized side region before fragment validation.
  It also integrates compatible opposite-side edits into that complete region.
  The four-region compiled strict-stash replay matches the reported `download_your_data` conflict.
  The replay preserves the exact combined document and the original stash inventory.
  The unrelated-fragment regression still rejects a lossy candidate before semantic audit.
  Focused tests and `make ci` passed on 2026-08-21.
  Acceptance regression failure on 2026-08-21:
  The fixture used the reported conflict regions, but its model stub returned the prepared result.
  The regression validated an ideal candidate instead of candidate generation.
  Additional requirements:
  - Cover conflicting replacement alternatives through one general contract.
  - Cover compatible edits inside a coarse conflict region.
  - Cover deletion as a valid replacement alternative.
  - Do not return a prepared result during semantic candidate generation.
  - Require each derivable conflict region to start with semantic audit.
  - Cover the reported conflict structures through the compiled CLI.
  - Keep the acceptance tests failing until Gix derives the candidates.
  Additional validation:
  - Confirm that the idealized regression passes before the test refactor.
  - Confirm that each refactored regression fails on an unexpected semantic candidate request.
  Regression refactor on 2026-08-21:
  The prepared-candidate replay passed before the refactor.
  The new strategy guard fails for all four generalized replacement forms.
  Both compiled CLI regressions fail on the first unexpected semantic candidate request.
  This test-only step does not include a production implementation change.
  Final resolution on 2026-08-21:
  Gix now calculates each side's token edits once.
  Each token edit keeps its original base range.
  Compatible edits and same-position insertions produce one deterministic candidate.
  Conflicting replacements use the local alternative plus each compatible incoming edit.
  Local validation accepts an exact whitespace-normalized derived candidate.
  This candidate combines one complete variant with all compatible opposite token edits.
  Gix sends every derived candidate directly to semantic audit.
  The `llm-proxy` lifecycle and `download_your_data` stash compiled replays now pass.
  Full `make ci` passed on 2026-08-21.
  Review follow-up on 2026-08-21:
  The old edit rule combined independent replacements across unchanged whitespace.
  This rule made a compatible insertion conflict with the combined replacement.
  BASE `foo bar`, OURS `FOO BAR`, and THEIRS `foo, bar` then produced `FOO BAR`.
  The result lost the incoming comma before semantic audit.
  Follow-up requirements:
  - Keep each original token edit boundary during compatibility checks.
  - Keep each original token edit boundary during replacement intent validation.
  - Preserve an insertion inside unchanged whitespace that separates independent edits.
  - Reject a derived candidate that omits the insertion.
  - Keep the two reported compiled CLI replays valid.
  Follow-up resolution:
  Gix now keeps each original token edit boundary for compatibility and intent validation.
  Region validation calculates both edit sets once and uses them for all local checks.
  An exact derived candidate contains one complete variant and all compatible opposite edits.
  The `foo bar` regression now derives `FOO, BAR`.
  A candidate without the comma fails local validation.
  The `llm-proxy` lifecycle and `download_your_data` stash compiled replays passed.
  Focused tests passed on 2026-08-21.
  Full `make ci` passed on 2026-08-21.

- [x] [B073] (P0) Ignore whitespace when validating merge replacement intent.
  Reported on 2026-08-20 after `gix sync master --stash` rejected four semantic candidates.
  Expected result:
  Gix accepts a candidate when it keeps each replacement's non-whitespace content in the correct order.
  Actual result:
  Gix uses a raw substring comparison. A different line wrap causes a false missing-intent error.
  Requirements:
  - Compare replacement intent without whitespace.
  - Preserve the exact non-whitespace content and order.
  - Report all missing OURS and THEIRS replacement content in one error.
  - Preserve additive validation, semantic audit, bounded attempts, and exact rollback.
  Validation:
  - Reproduce a multiline stash conflict through the compiled CLI.
  - Return a valid semantic candidate that changes only line wrapping.
  - Verify that local validation accepts the candidate before semantic audit.
  - Verify that one-sided loss reports all missing replacement content.
  - Run focused tests and complete CI.
  Resolution:
  Gix now removes Unicode whitespace from each replacement and candidate before comparison.
  It reports every missing OURS and THEIRS replacement fragment in one error.
  The compiled CLI regression reproduces a multiline `--stash` conflict.
  It accepts the reflowed candidate, completes semantic audit, and finalizes the exact invocation stash.
  Focused tests and `make ci` passed on 2026-08-20.
  Review finding on 2026-08-20:
  A candidate can use an unrelated normalized match from a different location.
  This match can hide the loss of an edited occurrence.
  Follow-up requirements:
  - Require a new occurrence or an edit context match for each replacement intent.
  - Reject an unrelated normalized match before semantic audit.
  - Cover the failure through the compiled CLI.
  Follow-up resolution:
  Gix now requires the necessary normalized occurrence count or a matching edit context.
  An unrelated occurrence cannot satisfy this validation.
  The focused test covers occurrence counting and edit context matching.
  The compiled CLI rejects the lossy alias candidate before audit.
  It preserves the lossless stash candidate and completes strict sync.
  Focused tests and `make ci` passed on 2026-08-20.

- [x] [B072] (P0) Stop semantic repair after a provider request failure.
  Reported on 2026-08-20 after a strict-sync conflict request failed.
  Expected result:
  A failed provider round stops semantic repair and starts exact rollback.
  Actual result:
  Gix sent the request error as model feedback and repeated the complete provider round four times.
  Requirements:
  - Separate provider request failure from candidate rejection.
  - Use validation feedback only for returned candidates.
  - Stop semantic repair when the complete provider order returns no response.
  - Keep request retries in a separate typed provider policy.
  - Preserve typed LLM Proxy HTTP status without the raw response body.
  - Preserve exact pre-publication rollback.
  Validation:
  - Add compiled CLI coverage for an LLM Proxy `403` and OpenAI empty-response exhaustion.
  - Verify one provider round and bounded request counts.
  - Verify no validation-guided repair starts.
  - Run focused tests and complete CI.
  Resolution:
  Gix now stops semantic repair after one failed provider round.
  Only a returned candidate rejection supplies validation feedback for another attempt.
  LLM Proxy `v0.4.0` preserves a typed HTTP status without the raw response body.
  The repository and hosted CI now use Go `1.26.5`, as required by the client.
  Compiled CLI coverage verifies one proxy request, two OpenAI requests, and exact rollback.
  `make test-fast`, `make test-slow`, `make ci`, and `git diff --check` passed on 2026-08-20.
  Review finding:
  Gix classified a returned marker-bearing candidate as a provider failure.
  Follow-up resolution:
  Gix now keeps marker-bearing candidates in the bounded validation-guided repair loop.
  The compiled CLI test verifies marker feedback, a corrected candidate, semantic approval, and four bounded requests.
  Follow-up validation:
  `make test-fast` and all four bounded `make test-slow` shards passed.
  `make ci` passed formatting, lint, fast Go tests, and licensing tests.
  The unsharded integration package then reached the 350-second command limit while its tests continued to pass.

- [x] [B071] (P1) Delete the safe source branch after default migration.
  Reported on 2026-08-20 after `gix default master` ran for `tyemirov/QwenOC`.
  Expected result:
  The direct command deletes the local and remote source branches after all safety gates pass.
  Actual result:
  The command reports `safe_to_delete=true` but does not request source branch deletion.
  GitHub keeps the source branch and offers a pull request for its obsolete merge commit.
  Requirements:
  - Make the direct `gix default` command request source branch deletion.
  - Verify that the target contains the source changes before deletion.
  - Accept an ancestor source or equal source and target file content.
  - Retain a source branch that contains changes absent from the target branch.
  - Keep `delete_source_branch` as the explicit reusable workflow option.
  - Report both the safety result and the deletion result.
  Validation:
  - Reproduce the merged `main` and `master` branch graph through the compiled CLI.
  - Verify that the command deletes local and remote `main` when both branches have equal file content.
  - Reproduce source changes that are absent from the target branch.
  - Verify that the command retains the source branch and reports `source_deleted=false`.
  - Run focused tests and complete CI.
  Resolution:
  - The direct command requests source branch deletion.
  - The safety gate accepts an ancestor source or equal branch file content.
  - The safety gate retains a source branch with changes absent from the target branch.
  - The result reports `safe_to_delete` and `source_deleted` independently.
  - Compiled CLI tests reproduce equal merged branches and divergent branches.
  - `make test-fast`, `make test-slow`, and `make ci` passed on 2026-08-20.
  Review finding:
  The source safety gate reads local branch refs.
  A stale checkout can hide remote source commits and permit unsafe remote deletion.
  Additional requirements:
  - Fetch the authoritative remote source and target refs before the safety check.
  - Compare the fetched remote commits instead of local branch commits.
  - Make remote source deletion conditional on the verified source commit.
  - Retain the remote source branch when its commit changes before deletion.
  Additional validation:
  - Reproduce a remote source commit that is absent from the stale local source branch.
  - Verify that the compiled CLI retains the remote source branch.
  - Verify that the deletion command includes the verified remote source commit.
  Follow-up resolution:
  - Gix fetches the authoritative remote source and target refs before the safety check.
  - Gix compares the fetched commit identities and file content.
  - The delete request includes the verified remote source commit.
  - Git rejects deletion when the remote source commit changes.
  - A failed remote deletion retains the local source branch.
  - Compiled CLI tests cover a stale checkout and a concurrent remote source change.
  - `make test-fast`, `make test-slow`, and `make ci` passed on 2026-08-20.

- [x] [B070] (P1) Treat an absent Pages site as a valid migration state.
  Reported on 2026-08-20 after `gix default master` ran for `tyemirov/QwenOC`.
  Expected result:
  The command treats an absent Pages site as a valid state and completes without a Pages warning.
  Actual result:
  The Pages endpoint returns `404` when the repository has no Pages site.
  Gix reports `PAGES-SKIP` because the GitHub adapter treats this state as an operation error.
  Gix also treats all Pages operation and response errors as noncritical warnings.
  Requirements:
  - Map a Pages endpoint `404` response to the disabled Pages state.
  - Reject empty, `null`, malformed, or unsupported successful Pages responses.
  - Stop migration when Gix cannot determine the Pages state.
  - Do not change the default branch after a genuine Pages state error.
  - Remove the obsolete `PAGES-SKIP` warning path.
  Validation:
  - Reproduce an absent Pages site through the GitHub adapter.
  - Run the compiled CLI command `gix default master` with that response.
  - Verify that the command changes the default branch without a Pages warning.
  - Reproduce a genuine Pages lookup failure through the compiled CLI.
  - Verify that the command fails before the default branch changes.
  - Run focused tests and complete CI.
  Resolution:
  - The GitHub adapter maps a Pages endpoint `404` response to the disabled Pages state.
  - The adapter rejects invalid successful Pages responses.
  - Default migration stops when a Pages operation or response error occurs.
  - The service no longer emits `PAGES-SKIP` for an unknown Pages state.
  - Adapter, service, and compiled CLI tests cover absent Pages sites and genuine lookup failures.
  - `make test-fast` and `make test-slow` passed on 2026-08-20.
  - Final `make ci` passed on 2026-08-20.

- [x] [B069] (P1) Close the obsolete pull request from the target branch.
  Reported on 2026-08-16 after `gix sync` created a `master` pull request against `main`.
  Expected result:
  `gix default master` closes the obsolete pull request after `master` becomes the default branch.
  Actual result:
  The command tries to change the pull request base from `main` to `master`.
  GitHub rejects the `master` to `master` pull request because the two branches have no different commits.
  Gix reports `PR-RETARGET-SKIP` and keeps `safe_to_delete=false`.
  Requirements:
  - Identify each open pull request whose head is the new default branch.
  - Close each obsolete pull request from the new default branch.
  - Do not try to change the base of that pull request to its head branch.
  - Keep other pull requests on the current retarget flow.
  - If closure fails, keep the pull request as a source-branch deletion blocker and report the exact warning.
  Validation:
  - Create an open `master` to `main` pull request.
  - Run the compiled CLI command `gix default master`.
  - Verify that the command closes the obsolete pull request.
  - Verify that the command does not change the base of the obsolete pull request.
  - Verify that successful closure removes that pull request from the source-branch safety gate.
  - Run focused tests and complete CI.
  Resolution:
  - Default migration closes each open pull request whose head is the new default branch.
  - Other pull requests keep the base-change flow.
  - A close failure reports `PR-CLOSE-SKIP` and keeps the pull request as a safety blocker.
  - The compiled CLI regression verifies closure, no base change, remote state, and `safe_to_delete=true`.
  - `make test-fast`, `make test-slow`, and `make ci` passed on 2026-08-16.
  Review finding:
  A pull request from another repository can use the same head branch name as the new default branch.
  The `headRefName` field does not include the head repository identity.
  A branch-only match can close an unrelated contributor pull request.
  Additional requirements:
  - Retain `headRepository.nameWithOwner` from `gh pr list`.
  - Close the pull request only when its head branch and head repository match the target repository.
  - Change the base of a same-named pull request from another repository.
  Additional validation:
  - Create a pull request from another repository with the new default branch name.
  - Run the compiled CLI command `gix default master`.
  - Verify that the command changes the base of that pull request.
  - Verify that the command does not close that pull request.
  Follow-up resolution:
  - The GitHub client retains `headRepository.nameWithOwner` for each listed pull request.
  - Default migration closes a target-named pull request only when its head repository matches the target repository.
  - Pull requests from other repositories keep the base-change flow and remain safety blockers.
  - Service tests and compiled CLI tests verify both repository identities.
  - `make test-fast`, `make test-slow`, and `make ci` passed on 2026-08-16.

- [x] [B068] (P1) Reject a dirty default-branch promotion before mutation.
  Reported on 2026-08-16 after `gix default master` ran from a dirty `main` branch.
  Expected result:
  The command rejects the dirty worktree before it creates a branch, changes the checkout, pushes a branch, or updates GitHub.
  Actual result:
  `BranchMigrationOperation.Execute` creates the target branch and checks it out.
  The operation also pushed `origin/master` at the `main` commit before the clean-worktree check.
  `Service.Execute` then rejects the dirty worktree and leaves the earlier mutations in place.
  Requirements:
  - Make sure that the worktree is clean before target branch creation, checkout, or remote push.
  - Run all nonmutating preflight checks before the first mutation.
  - If preflight fails, preserve the starting checkout, local refs, remote refs, worktree contents, and index.
  - Do not change workflows, Pages settings, the GitHub default branch, or pull requests after a dirty-worktree rejection.
  Validation:
  - Create a dirty `main` branch with no local or remote `master` branch.
  - Run the compiled CLI command `gix default master`.
  - Verify that the command fails with the clean-worktree error.
  - Verify that `main` stays active and that local and remote `master` refs remain absent.
  - Verify that the worktree contents and index stay unchanged.
  - Verify that Git and GitHub logs contain no mutating command.
  Resolution:
  - `migrate.Service` now owns target branch creation, checkout, and remote publication.
  - Option validation, the clean-worktree check, and credential validation complete before target preparation.
  - A service test verifies that dirty-worktree preflight runs only `git status`.
  - A compiled CLI regression covers staged, unstaged, and untracked changes with no local or remote `master` branch.
  - The regression verifies exact preservation of the checkout, commit, index, repository refs, remote refs, file contents, workflow, and GitHub state.
  - `make test-fast`, `make test-slow`, and `make ci` passed on 2026-08-16.

- [x] [B067] (P1) Use the repository default branch during strict sync.
  Reported on 2026-08-15 after `gix sync master` ran from a dirty `main` branch.
  Expected result:
  `gix sync master` treats `master` as an ordinary target when the repository default branch is `main`.
  Actual result:
  Sync treats `master` as the base branch, compares `main` with absent `origin/master`, and restores `main` without creating `master`.
  Review finding on 2026-08-15:
  Audit can report the local branch as `RemoteDefaultBranch` when metadata and the remote-tracking symbolic `HEAD` are absent.
  Strict sync trusts this value and can push an ordinary branch without a pull request.
  Review findings on 2026-08-16:
  A task-defined strict sync can contain the obsolete `base_branch` option.
  The action gives no error and uses the remote default branch.
  A single-branch clone can exclude the remote default branch from its fetch refspec.
  Strict sync resolves that default branch but does not create its remote-tracking ref.
  Requirements:
  - Resolve strict sync behavior from the repository default branch.
  - Do not use a branch name to select default-branch behavior.
  - Treat `main`, `master`, and all other branch names as ordinary names.
  - Get the strict-sync default-branch identity from the remote symbolic `HEAD`.
  - Do not use audit fallback data to select default-branch behavior.
  - Reject the obsolete `base_branch` task option.
  - Fetch the resolved remote default branch into its remote-tracking ref.
  - Create a missing ordinary target from the current checkout when dirty work exists.
  - Commit the dirty work to the requested target and push that target.
  - Open the target pull request against the repository default branch.
  - Preserve rollback before publication and handoff after publication.
  Validation:
  - Reproduce dirty default branch `main` with no local or remote `master`.
  - Verify that sync creates and pushes `master` with the committed dirty work.
  - Verify that sync opens the `master` pull request against `main`.
  - Verify that sync does not compare `main` against absent `origin/master`.
  - Exchange the `main` and `master` roles and verify the same behavior.
  - Make metadata lookup fail on a dirty remote-backed ordinary branch.
  - Remove the remote-tracking symbolic `HEAD` before inspection.
  - Verify that sync opens a pull request against the remote default branch.
  - Run a task-defined strict sync with the obsolete `base_branch` option.
  - Verify that the action rejects the option before Git or GitHub mutation.
  - Clone only a nondefault branch from a repository whose default branch is different.
  - Verify that strict sync fetches and uses the remote default branch.
  - Preserve direct synchronization when the requested branch is the repository default branch.
  - Run focused tests and complete CI.
  Resolution:
  - Strict sync resolves the default branch from the remote symbolic `HEAD` after fetch.
  - Strict sync does not use audit fallback data to select default-branch behavior.
  - A remote that does not report a default branch stops sync before commit generation or publication.
  - Strict sync has no independent base-branch option or named branch default.
  - Default-branch promotion requires an explicit target branch.
  - Production Go logic has no `main` or `master` branch-name literals.
  - Compiled CLI tests exchange `main` and `master` as the default and target branches.
  - A compiled CLI regression makes metadata lookup fail while audit reports an ordinary local branch.
  - The regression updates and pushes that branch and opens its pull request against the remote default branch.
  - Focused tests and `make ci` passed on 2026-08-16.
  - The `branch.sync` action rejects `base_branch` before it runs a Git or GitHub operation.
  - Strict sync explicitly fetches the resolved default branch into its remote-tracking ref.
  - The explicit fetch does not change the configured fetch refspec.
  - Compiled CLI tests cover obsolete task configuration and a single-branch feature clone.
  - `make test-fast`, `make test-slow`, and `make ci` passed on 2026-08-16.

- [x] [B066] (P1) Stop repository cleanup after an operator interrupt.
  Reported on 2026-08-13 after an interrupt to `gix prs delete`.
  Expected result:
  An operator interrupt stops repository dispatch. The CLI exits without a repeated operation failure list.
  Actual result:
  The runner starts cleanup for queued repositories with a canceled context. It reports one failure for each repository and prints the list twice.
  Requirements:
  - Stop repository dispatch when the caller context is canceled.
  - Do not record cancellation as a repository failure.
  - Do not write shell execution errors for caller cancellation.
  - Exit with the standard interrupt status and no cancellation error text.
  - Preserve completed repository results and genuine operation failures.
  - Preserve a runner failure when cancellation and command completion occur at the same time.
  - Preserve an operation failure when a peer operation cancels the shared context.
  - Preserve operation outcomes completed before stage cancellation.
  Validation:
  - Add compiled CLI coverage that interrupts branch cleanup during `git ls-remote`.
  - Verify the CLI starts no queued repository after cancellation.
  - Verify stderr has no internal cancellation failure.
  - Add focused coverage for simultaneous cancellation and completion failures.
  - Add focused coverage for a partially completed canceled stage.
  - Run focused tests and complete CI.
  Resolution:
  - The workflow starts no queued repository operation after caller cancellation.
  - Shell execution returns caller cancellation without an error log.
  - The CLI exits with status 130 and no cancellation error text.
  - Compiled CLI coverage interrupts branch cleanup during `git ls-remote`.
  - Focused tests and `make ci` passed on 2026-08-13.
  - Shell execution preserves completed results and errors during simultaneous cancellation.
  - Workflow stages suppress only errors that match the context cancellation cause.
  - Partially completed stages retain completed operations in the public execution outcome.
  - Focused cancellation tests and `make ci` passed on 2026-08-13.

- [x] [B065] (P0) Keep required repair inputs on the patch line.
  Reported on 2026-08-12 after B063 and B064 were published as `v1.5.0` instead of `v1.4.1`.
  Expected result:
  A compatible release repair increments the patch version under the fixed `v1` policy.
  Actual result:
  The model treated required CLI inputs for the repair as optional public functionality and selected a minor release.
  Requirements:
  - Classify a required repair input by the restored operation, not by its new interface shape.
  - Keep optional new user functionality on the minor line.
  - Preserve incompatible and additive public contract classification.
  Validation:
  - Reproduce the B063 release-input repair from `v1.4.0`.
  - Verify `gix release next semver --fixed-major 1` selects `v1.4.1`.
  - Run focused tests and complete CI.

  Resolution:
  - The version decision now keeps required repair inputs in the compatible repair.
  - The compiled CLI test reproduced `v1.5.0` and now selects `v1.4.1`.
  - `make test-slow`, `make format`, and `make ci` passed on 2026-08-12.

- [x] [B064] (P1) Fetch a deleted pull request head before chain validation.
  Reported on 2026-08-11 during review of merged pull request chain handling.
  Observation:
  - GitHub can retain a merged pull request after branch deletion.
  - A repository can lack the final pull request head commit.
  - The ancestry check then fails before sync reaches the next merged base.
  Requirements:
  - Fetch the exact GitHub pull request head when the recorded commit is absent.
  - Verify the fetched commit against the GitHub head identity.
  - Preserve exact matching for an existing remote branch.
  - Preserve the branch reuse guard for local commits.
  - Fail with operation context when the head is unavailable or different.
  Validation:
  - Add compiled CLI coverage with a deleted branch and an absent local head commit.
  - Prove that sync follows the merged chain to `master`.
  - Run focused tests and complete CI.

  Resolution:
  - Sync now resolves the recorded head commit before the ancestry check.
  - If the commit is absent, sync fetches `refs/pull/<number>/head` without tags.
  - Sync verifies `FETCH_HEAD` against the GitHub head identity before chain traversal.
  - Compiled CLI coverage proves that sync fetches an absent head and reaches `master`.
  - `make test-fast`, `make test-slow`, and `make ci` passed on 2026-08-11.

- [x] [B063] (P0) An artifact successor rejects an empty SemVer range.
  Goal:
  Select one patch successor when a release system changes sealed outputs for
  the same source commit.

  Evidence:
  - Gateway published one LoopAware release from the current application commit.
  - Current release behavior produced different sealed output identities.
  - `gix release next semver` rejected the empty commit range.

  Requirements:
  - Accept exact previous and candidate output identities at the CLI boundary.
  - Require both identities and require different SHA-256 values.
  - Permit the output transition only when the latest SemVer tag identifies `HEAD`.
  - Select a patch successor without an LLM request.
  - Keep ordinary empty-range selection invalid.

  Deliverables:
  - Add one typed output-transition input to `gix release next semver`.
  - Record deterministic transition evidence in version decision contract v2.
  - Add compiled CLI coverage for the same-commit successor.

  Validation:
  - Prove the transition selects the next patch version.
  - Prove invalid transitions fail before an LLM request.
  - Run focused release tests and complete CI.

  Resolution:
  - Added paired canonical release output inputs for SemVer decisions.
  - Bound same-commit patch selection to deterministic transition evidence.
  - Preserved rejection of ordinary empty SemVer ranges.
  - Passed focused release tests and `make ci`.

  Review follow-up:
  Transition input could select an initial SemVer release when no eligible tag existed.

  Follow-up resolution:
  Transition mode now rejects an untagged, excluded-tag, or fixed-major history before initial version selection.
  Unit coverage verifies all three cases. The compiled CLI verifies the untagged case.
  `make test-fast`, `make test-slow`, and `make ci` passed on 2026-08-11.

- [ ] [B020] (P0) Investigate missing GitHub PR for branch after gix sync operation.
  Goal:
  Explain why `gh pr view -w` reports no pull requests for `gix/publish-seo-resource-hub-45-resource-pages-sitemap-and` in the SummerCan repository.
  Make sure that the correct pull request exists or can be created.
  Requirements:
  - Do not force-push, delete, or rename the branch without confirmation from the code owner.
  - Preserve all existing commits on this branch.
  - Use only standard Git and GitHub CLI operations available to the team.
  - Keep changes in the scope of this pull request association issue.
  Deliverables:
  - Give a diagnosis that explains why `gh pr view` cannot find the pull request.
  - Include an absent branch, a different fork, and a closed pull request in the diagnosis when applicable.
  - Give instructions to associate the existing branch with its pull request or create the correct pull request.
  - Update internal documentation for similar `gh pr view` failures when applicable.
  Validation:
  - From the affected branch, run `gh pr view` without extra flags and get the expected pull request details.
  - Verify that the GitHub web UI shows the correct head branch and base branch.
  - Verify that the documented steps explain or resolve the reported failure.
- [x] [B032] (P1) Fit the terminal audit report to the active terminal width.
  Reported on 2026-07-24.
  Observation:
  The default `gix audit` table takes every cell's natural display width.
  As a result, long repository paths and remote values make the output wider than the terminal.
  Requirements:
  - Resolve the active terminal width at the table-output boundary.
  - Keep the exact CSV and HTML export contracts unchanged.
  - Keep every audit field available.
  - When a horizontal grid cannot fit, render a bounded field/value table.
  - Do not silently drop columns.
  - Truncate constrained table cells with a visible ellipsis and preserve Unicode display-width alignment.
  Deliverables:
  - A responsive terminal audit renderer that never emits a table line wider than the active terminal width.
  - Public CLI regression coverage for a constrained terminal width and unchanged CSV/HTML output.
  - Updated operator documentation describing the responsive table behavior and full-fidelity export formats.
  Validation:
  - Run the compiled `gix audit` command with a constrained terminal width.
  - Verify that each table line stays within that width and shows each audit field label.
  - Existing table, CSV, HTML, delimiter-escaping, and wide-Unicode CLI contracts remain covered.
  - Run `make format`, `make test`, `make lint`, `make ci`, and `git diff --check`.
  Resolution:
  The table renderer now reads the terminal size from stdout.
  It uses `COLUMNS` when captured output has no size query.
  It keeps a bounded horizontal grid where practical and uses a field/value layout below that threshold.
  CSV and HTML remain full-value exports.
  Public CLI coverage verifies compact and truncated horizontal layouts with display-width bounds and Unicode ellipses.
  `make format`, `make lint`, `make test`, and `make ci` passed on 2026-07-24.
- [x] [B033] (P1) Preserve audit table width handling in workflows.
  Reported on 2026-07-24.
  Observation:
  `gix workflow` wraps stdout so progress is flushed immediately.
  A table-format `audit report` step received that wrapper instead of stdout.
  The terminal-width detection stopped before it could use the terminal size or `COLUMNS`.
  As a result, workflow tables could exceed the available width.
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
  Workflow LLM configuration accepts floating-point `timeout_seconds`, while the v0.2.46 request contract accepts only positive whole seconds.
  Integer division silently shortens values such as 1.9 seconds and changes sub-second values to zero.
  The client rejects these values only when it builds a request.
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
  Git can deregister a linked worktree and then fail to delete it.
  This failure occurs when an ignored generated directory lacks owner write permission, such as a read-only Go module cache.
  The partial removal leaves an orphaned directory and blocks the requested sync.
  Requirements:
  - Before removal, restore owner write and execute permission on directories under the validated non-main sibling worktree.
  - Limit the permission change to that exact worktree.
  - Do not follow symlinks, alter file permissions, use `sudo`, or widen cleanup beyond the worktree Gix already selected for adoption.
  - Preserve contextual failure errors if the filesystem still prevents removal.
  Validation:
  - Add public CLI coverage that adopts a sibling worktree with a read-only ignored cache.
  - Verify complete removal and a successful switch.
  - Run `make format`, `make test`, `make lint`, `make ci`, and `git diff --check`.
  Resolution:
  Gix now prepares the validated non-main sibling worktree before `git worktree remove`.
  A non-symlink directory walk adds only owner write and execute bits.
  Files and symlink targets remain untouched.
  The public CLI regression creates a read-only ignored Go-style cache.
  It confirms full sibling removal and activation of the requested branch.
  `make format`, `make test`, `make lint`, `make ci`, and `git diff --check` passed on 2026-07-27.
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
  - Add public CLI coverage for a local branch with no commits beyond `origin/master` and no pull request.
  - Add a dirty tracked example environment file that matches `.gitignore`.
  - Run plain `gix sync` and verify that it commits, pushes, and opens the pull request.
  - Verify that the file contents do not change.
  - Preserve coverage that genuinely ignored untracked files are not staged.
  - Run `make format`, `make test`, `make lint`, `make ci`, and `git diff --check`.
  Resolution:
  Sync now derives tracked and untracked path sets directly from Git status entries.
  Exact tracked paths use force staging, so matching ignore rules cannot block their changes.
  Untracked paths retain normal ignore-respecting staging.
  The cached-ignore inspection and tracked-path restore code were removed.
  Public CLI coverage reproduces the reported empty local branch with dirty `configs/.env.hecateapi.example`.
  It verifies that sync commits and pushes the file into a new pull request.
  The coverage retains ignored-untracked exclusion.
  `make format`, `make test`, `make lint`, and `make ci` passed on 2026-07-27.
- [x] [B038] (P0) Roll back rejected sync merges before returning control.
  Reported on 2026-07-28 after a clean `gix sync tyemirov/bugfix/B184-catalog-tile-final-font-fit` attempted to merge its open pull request base.
  The command rejected a lossy AI resolution and left two unmerged paths with staged base branch changes.
  Observation:
  Strict sync intentionally leaves a merge active when automatic conflict resolution is rejected, canceled, or times out.
  As a result, a command can fail and transfer its Git transaction plus a dirty index to the operator.
  Requirements:
  - Treat a merge started by strict sync as operation-owned until it is committed or rolled back.
  - When automatic resolution fails, run the canonical Git merge abort with a bounded cleanup context.
  - Keep the cleanup context usable after cancellation.
  - After successful rollback, leave the selected branch at its exact pre-merge commit.
  - Restore the pre-merge worktree state, report the rollback truthfully, and never push.
  - If rollback fails, preserve the exact failure and emit a manual handoff.
  - Distinguish the resolution failure from the rollback failure.
  - Keep the strict lossy-resolution rejection and PR-base synchronization contracts.
  - Do not add a compatibility path or silently accept model output.
  Validation:
  - Add public CLI coverage for a clean remote-backed pull-request branch with a conflicting remote review base.
  - Return a lossy AI resolution and verify that the command fails with a rollback event.
  - Verify that the target commit and contents remain unchanged.
  - Verify that no `MERGE_HEAD`, dirty status, or push exists.
  - Cover rollback under a canceled caller context and rollback-failure handoff through focused guardrail tests.
  - Run `make format`, `make test`, `make lint`, `make ci`, and `git diff --check`.
  Resolution:
  Strict sync now aborts every merge whose observed conflicts cannot be resolved automatically.
  The abort uses a 30-second cleanup context that is separate from the canceled resolution context.
  Successful cleanup reports `AI_MERGE_ROLLBACK`.
  An abort failure retains both errors and reports `AI_MERGE_HANDOFF`.
  Public CLI coverage reproduces a clean pushed pull request branch with a conflicting review base.
  A lossy response leaves the exact target commit and contents, no `MERGE_HEAD`, an empty status, and no push.
  Existing lossy, timeout, and modify/delete scenarios prove the same rollback boundary.
  Focused tests cover cancellation-independent cleanup and rollback failure.
  `make format`, `make test`, `make lint`, `make ci`, and `git diff --check` passed on 2026-07-28.
  Review follow-up:
  The compiled CLI now converts Ctrl-C into caller-context cancellation.
  When cancellation prevents the first conflict query, strict sync inspects `MERGE_HEAD` through the separate cleanup context.
  Strict sync then aborts the merge.
  Public CLI coverage interrupts immediately after Git creates the conflicted index.
  It proves exact branch and content restoration, no `MERGE_HEAD`, clean status, no LLM request, and no push.
- [x] [B039] (P1) Pin license rollout clones to their inspected commits.
  Reported on 2026-07-28 during review of F011.
  Observation:
  The read-only plan verified license blobs through a repository default branch name.
  Apply later cloned that moving branch tip.
  As a result, a branch advance could change the files used as the mutation base after planning passed.
  Requirements:
  - Resolve one immutable commit for every non-empty default branch and inspect license blobs through that commit.
  - Reset each sparse clone to the corresponding inspected commit before any workflow mutation.
  - Prevent the workflow's initial fetch from fast-forwarding the pinned local default branch.
  - Fail closed when the inspected commit cannot be fetched.
  Validation:
  - Advance a Git-backed default branch after inspection.
  - Verify that clone preparation leaves `HEAD` and `LICENSE` at the inspected commit.
  - Verify the same result after the workflow-equivalent fetch and pull sequence.
  - Run `make format`, `make test`, `make lint`, `make ci`, and `git diff --check`.
  Resolution:
  Live inventory now carries the exact default-branch commit used for root license-blob inspection. Apply fetches that SHA, resets the sparse clone to it, and removes the local default branch's moving upstream before Gix runs. Focused licensing coverage passes with a real branch-advance regression.
- [x] [B040] (P2) Validate existing license rollout pull requests before skipping them.
  Reported on 2026-07-28 during review of F011.
  Observation:
  Apply treated each open draft on the deterministic rollout branch as completed without proof of its base, history, or changed files.
  As a result, a stale or modified draft could bypass cloning and count as a successful reviewed rollout.
  Requirements:
  - Require the draft to use the reviewed base branch and exact inspected base commit.
  - Require the deterministic same-repository head branch and one canonical rollout commit.
  - Compare the complete changed-file set with the bundle from the reviewed profile.
  - Compare the resulting root license blobs with that bundle.
  - Reject aliases and unrelated changes.
  - Read the pull request again after validation.
  - Fail closed if its state, base, head, or changed-file count moved during inspection.
  Validation:
  - Accept a matching draft.
  - Reject incorrect base names or commits, noncanonical history, and extra or modified files.
  - Reject a head revision that moves during validation.
  - Run `make format`, `make test`, `make lint`, `make ci`, `make license-rollout-plan`, and `git diff --check`.
  Resolution:
  Existing and new rollout pull requests now pass the same immutable-base, single-commit, changed-path, blob, root-bundle, and final-snapshot validation.
  Their URLs count as prepared only after this validation.
  Focused licensing coverage exercises every rejection boundary.
  `make format`, `make test`, `make lint`, `make ci`, `make license-rollout-plan`, and `git diff --check` passed on 2026-07-28.
  The live plan reverified 103 repositories, 97 eligible rollouts, and six review holds without mutation.
- [x] [B041] (P0) Resolve AI merge conflicts by semantic region instead of full-file reproduction.
  Reported on 2026-07-28 after `gix sync bugfix/B038-rollback-failed-ai-merge` encountered additive conflicts in `.mprlab/ISSUES.md` and `CHANGELOG.md`, received an AI response, and rejected it for not preserving non-conflicting content.
  Observation:
  The merge resolver sends complete BASE, OURS, and THEIRS files to the model and requires another complete file as output.
  The reported conflict repeats approximately 145,000 input characters.
  It asks the model to reproduce approximately 50,000 characters under an 8,192-token output limit.
  Only one inserted issue region conflicts.
  The byte-preservation guard correctly rejects truncation, but the full-file contract makes a clean semantic resolution unreliable.
  Requirements:
  - Parse every marker-bearing worktree file into exact non-conflicting regions and explicit OURS, BASE, and THEIRS conflict regions.
  - Reconstruct complete files locally, directly accepting only byte-identical, unilateral, and marker-free current-stage cases because they contain no two-sided semantic choice.
  - Require semantic LLM audit for every marker-bearing region changed by both sides, including append-only issue/changelog insertions.
  - Derive lossless concurrent-insertion and non-overlapping-token candidates locally, but treat them only as proposals that the model must approve or correct.
  - For an overlapping semantic region without a safe local candidate, send only one region at a time.
  - Require a locally validated candidate and an explicit semantic audit.
  - Send each rejection to a bounded repair attempt.
  - Give each semantic attempt enough time to exhaust the configured provider order.
  - Do not share one deadline across all files, regions, providers, and repairs.
  - Reject obvious one-sided loss and require both BASE-to-OURS and BASE-to-THEIRS replacement intent before semantic audit.
  - Roll back through the B038 operation-owned merge boundary only after all bounded semantic strategies stop.
  - Also roll back when cancellation or an unrecoverable Git or filesystem failure prevents more resolution.
  - Remove the obsolete full-file response contract.
  - Do not retain a compatibility path.
  Validation:
  - Add public CLI coverage for a clean pull-request branch with large issue and changelog files.
  - Insert different review-base entries at the same anchors.
  - Do one region-scoped semantic audit for each conflict.
  - Exclude untouched file tails from both requests.
  - Include every local and incoming entry exactly once.
  - Preserve untouched content byte-for-byte.
  - Push the merge commit and leave no conflict or dirty state.
  - Prove a rejected one-sided semantic candidate receives exact validator feedback, is repaired, passes an explicit audit, and commits without rollback.
  - Cover deterministic/token strategy selection plus lossy-response exhaustion, per-attempt timeout exhaustion, cancellation, marker-free modify/delete, cached-diff validation, and rollback-failure handoff.
  - Run `make format`, `make test`, `make lint`, `make ci`, and `git diff --check`.
  Initial implementation:
  Marker-bearing conflicts use Git diff3 regions and region-scoped LLM requests.
  Required response sentinels preserve boundary whitespace.
  Gix reconstructs untouched bytes locally, and empty-BASE concurrent insertions require both complete sides.
  Marker-free deletion semantics and the operation-owned rollback boundary remain strict.
  Public CLI coverage reproduces the large issue and changelog case.
  It verifies a clean pushed merge that contains every entry exactly once.
  Follow-up reported on 2026-07-28:
  A single rejected region candidate still reaches rollback immediately.
  The conflict can have a deterministic high-fidelity resolution, or the model can repair its candidate from exact validation feedback.
  Rollback is recovery from exhausted resolution, not a primary conflict-resolution strategy.
  Intermediate implementation:
  Strict sync now exhausts a fidelity-first ladder.
  It reconstructs untouched bytes locally.
  It resolves authoritative identical, unilateral, append-only issue or changelog, and marker-free current-stage cases without an LLM.
  It derives bounded non-overlapping token candidates for audit.
  Each semantic candidate must preserve both replacement intents and receive explicit auditor approval.
  Rejections feed up to four repair or audit attempts.
  Each attempt gives every configured provider one request with sufficient time.
  Git cached-diff validation gates the merge commit.
  Rollback is reachable only after strategy exhaustion, caller cancellation, or unrecoverable local failure.
  Public CLI coverage proves that the reported large additive case makes zero LLM calls and pushes a clean exact merge.
  An initially one-sided candidate is repaired and audited without rollback.
  Exhaustion, timeout, cancellation, marker-free deletion, index validation, and rollback-failure coverage remain green.
  `make format`, `make test`, `make lint`, `make ci`, and `git diff --check` passed on 2026-07-28.
  Correction reported on 2026-07-29:
  A minimum number of LLM calls is not an objective.
  For many two-sided conflicts, the model is the best owner of semantic resolution.
  Deterministic reconstruction must protect exact content and provide high-fidelity candidates.
  The LLM remains the semantic decision-maker, and rollback remains the lowest-priority recovery strategy.
  Resolution:
  Strict sync now reconstructs untouched file bytes locally.
  It directly accepts only identical, unilateral, and marker-free current-stage decisions.
  Each marker-bearing region changed by both sides enters semantic LLM review.
  Concurrent insertions and compatible token edits provide lossless local candidates for the auditor.
  Overlapping regions start with model generation.
  The model can approve or correct a candidate.
  Additive and replacement-intent validation prevents either side from disappearing.
  Exact rejection feedback drives up to four repair or audit attempts with the complete provider order.
  Git cached-diff validation gates the merge commit.
  Rollback remains final recovery after strategy exhaustion, cancellation, or unrecoverable local failure.
  Public CLI coverage proves that the reported large issue and changelog case does exactly two region-only audits.
  The requests do not expose either stable file tail.
  The result preserves each entry once, pushes the merge commit, and leaves a clean worktree.
  Repair, exhaustion, timeout, cancellation, marker-free deletion, index validation, and rollback-failure coverage are green.
  `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check` passed on 2026-07-29.
- [x] [B042] (P0) Restore the starting checkout when an explicit target sync fails.
  Reported on 2026-07-29 after `gix sync bugfix/B038-rollback-failed-ai-merge` ran from B041.
  The command adopted and removed the linked B038 worktree before conflict resolution failed.
  Observation:
  B038 restores the selected target branch to its pre-merge commit.
  The wider strict-sync operation still leaves that target active and keeps an adopted sibling worktree removed.
  As a result, a failed command can change the caller branch and valid worktree topology after a successful merge abort.
  Requirements:
  - Snapshot the starting checkout and every valid registered worktree before strict sync mutates branches or adopts a sibling.
  - On failure, restore the starting named or detached checkout.
  - Then reattach detached main worktrees or recreate removed linked worktrees at their original paths.
  - Run checkout and worktree cleanup through a bounded context that remains usable after caller cancellation.
  - Restore a user stash only after cleanup returns to the starting checkout.
  - Preserve the original sync error.
  - Emit an explicit rollback event after successful restoration.
  - If restoration fails, emit a distinct manual handoff that retains the cleanup error.
  - Keep successful explicit-target behavior unchanged: the requested branch remains active only after the complete sync succeeds.
  - Treat filesystem-identical path aliases, including macOS `/var` and `/private/var` spellings, as the same worktree.
  Validation:
  - Add public CLI coverage with a source branch in the current checkout and a target branch in a linked sibling.
  - Exhaust a semantic merge on the target.
  - Restore the exact source commit and files.
  - Recreate the clean target sibling at its original path and commit.
  - Leave no `MERGE_HEAD` and do no push.
  - Add a focused filesystem-identity guardrail and preserve every existing strict-sync/worktree-adoption scenario.
  - Run `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check`.
  Resolution:
  Strict sync now snapshots the caller checkout and valid registered worktrees before target switching or sibling adoption.
  After a later failure, cancellation-independent bounded cleanup restores the starting named branch or detached commit.
  It then reattaches a detached main worktree or recreates removed linked worktrees at their original paths.
  It restores a caller stash last.
  The original sync failure remains primary.
  Successful cleanup emits `SYNC_SWITCH_ROLLBACK`.
  Cleanup failure retains both errors and emits `SYNC_SWITCH_HANDOFF`.
  Successful explicit-target sync still leaves the target active.
  Public CLI coverage exhausts semantic resolution after adoption of a linked target.
  It proves restoration of the exact source checkout and clean target sibling with no `MERGE_HEAD` or push.
  A focused guardrail covers filesystem-identical path aliases.
  The branch was verified against exact `origin/master` commit `b1357e4567b18fd65c5d68e01b72f1303f893176`.
  `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check` passed on 2026-07-29.
- [x] [B043] (P0) Reject sync while an operator-owned revert is active.
  Reported on 2026-07-29 after plain `gix sync` ran with a resolved but unfinished `git revert`.
  The command failed when it tried to switch to the active branch.
  Observation:
  Strict sync treats the staged and unstaged revert state as ordinary dirty work, fetches the remote, and then re-enters the selected branch. Git rejects that switch while `REVERT_HEAD` exists. Merely skipping the redundant switch would be unsafe because dirty-sync clustering resets and restages the sequencer-owned index before committing. The revert predates the command and is operator-owned, so Gix must not continue, abort, quit, or repartition it implicitly.
  Requirements:
  - Inspect `REVERT_HEAD` at the strict-sync boundary before fetch, branch/worktree mutation, stash, index reset, LLM dispatch, commit, or push.
  - Reject an active revert with an actionable message naming the explicit `git revert --continue`, `git revert --abort`, and `git revert --quit` choices.
  - Preserve the exact starting commit, branch, index, worktree, untracked files, and `REVERT_HEAD`.
  - Do not reinterpret the pending revert as dirty-sync clusters.
  - Treat an unexpected revert-state inspection failure as a contextual sync error instead of assuming no revert is active.
  - Keep operation-owned merge rollback unchanged.
  - Do not add automatic ownership transfer or a compatibility path for pre-existing Git transactions.
  Validation:
  - Add public compiled-CLI coverage for a resolved but unfinished revert with staged and unstaged content.
  - Verify that sync fails before mutation and preserves exact Git state.
  - Verify that sync makes no LLM request or mutating Git call.
  - Add focused guardrails for active, absent, and uninspectable `REVERT_HEAD` state.
  - Run `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check`.
  Resolution:
  Strict sync now inspects `REVERT_HEAD` before each possible mutation or LLM request.
  An active operator-owned revert fails with explicit continue, abort, and quit choices.
  An unexpected inspection failure also stops sync.
  Public compiled-CLI coverage reproduces the reported revert with staged, unstaged, and untracked state.
  It proves exact preservation with no mutating Git or LLM call.
  Focused coverage verifies active, absent, and uninspectable revert state.
  `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check` passed on 2026-07-29.
  Review follow-up:
  The first preflight resolved the ambiguous revision name `REVERT_HEAD` only in the caller worktree.
  It falsely rejected an ordinary branch or tag with the same name.
  It also missed a per-worktree revert in an adoptable sibling.
  The final preflight lists each valid registered worktree and resolves its exact `REVERT_HEAD` Git path.
  It validates a present file as a canonical commit identifier and rejects before fetch.
  Public compiled-CLI regressions prove that an active sibling revert remains byte-for-byte unchanged.
  An ordinary branch named `REVERT_HEAD` does not block sync.
  `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check` passed for the review follow-up on 2026-07-29.
- [x] [B044] (P0) Make strict sync ownership-aware and transactional.
  Requested on 2026-07-29.
  Goal:
  Replace command-specific strict-sync recovery patches with one durable state transition.
  Distinguish operator-owned Git operations from Gix mutations across the caller and target sibling worktrees.
  Requirements:
  - Build one immutable preflight plan before fetch, LLM access, checkout changes, index changes, commits, or pushes.
  - Inspect exact per-worktree Git administrative paths.
  - Reject each pre-existing Git operation that strict sync could disturb.
  - Treat ordinary branches or tags named like Git administrative markers as ordinary refs.
  - Snapshot the exact caller and target-sibling checkout, commit, index, contents, and stash list.
  - Complete the snapshot before the first Gix local mutation.
  - On a pre-publication failure, restore that snapshot and worktree topology.
  - If restoration fails, preserve recovery state and emit an explicit handoff.
  - Restore and validate an invocation-owned `--stash` before reporting `SYNCED`.
  - Resolve safe conflicts through the current bounded semantic conflict engine.
  - Retain the stash after each unresolved failure.
  - Keep remote push and review-request creation as the final publication boundary.
  - Express acceptance as three declarative compiled-CLI integration tables: operator-owned preflight, failure rollback, and successful finalization.
  Validation:
  - Run `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check`.
  Resolution:
  Strict sync now builds one exact per-worktree operation plan.
  It rejects operator-owned Git operation state before fetch and ignores ordinary marker-like refs.
  Its local transaction snapshots branch refs, commits, index state, contents, stashes, and adoptable worktree topology.
  Sibling publication is deferred to the normal target push.
  A pre-push failure restores the complete snapshot.
  A post-push failure preserves forward recovery state and reports `SYNC_SWITCH_HANDOFF`.
  Invocation-owned stashes restore with `--index` and use the bounded semantic conflict engine when necessary.
  Stash restoration completes before `SYNCED`.
  Three declarative compiled-CLI tables cover operator preflight, rollback or publication boundaries, and successful finalization.
  `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check` passed on 2026-07-29.
- [x] [B045] (P0) Close strict-sync transaction ownership gaps.
  Reported on 2026-07-29 during review of B044.
  Observation:
  Three edges still violate the ownership boundary. A pre-existing unmerged index without an administrative marker reaches snapshot acquisition, where a failed stash can trigger destructive cleanup without an owned backup. An up-to-date push is treated as publication even though it performs no remote write. Rollback also rewinds or deletes every local branch and recreates every starting worktree instead of limiting restoration to state this invocation mutated.
  Requirements:
  - Reject an unmerged index in every valid registered worktree before fetch or snapshot mutation.
  - Include conflicts from `git stash apply` without an active merge or sequencer marker.
  - Parse the Git porcelain push result.
  - Mark Git publication only for an actual remote ref creation, update, or deletion.
  - Keep an up-to-date push rollback-capable and retain successful pull-request creation as publication.
  - Fail closed when a successful response cannot prove its outcome.
  - Journal only branch refs and worktrees that the invocation mutates.
  - Advance the expected ref value after each successful mutation.
  - Restore owned refs with compare-and-swap.
  - Preserve unrelated local branch changes and unrelated worktree topology during rollback.
  - Reject an unexpected outside change to an owned ref instead of overwriting it.
  - Make failed snapshot acquisition non-destructive unless an exact transaction backup was successfully acquired.
  Validation:
  - Extend the declarative operator-preflight table with an exact-state stash-apply conflict.
  - Add a no-op push followed by pull-request failure to the declarative failure table.
  - Add a concurrent commit in an unrelated sibling worktree to that table.
  - Run `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check`.
  Resolution:
  Strict sync now rejects each pre-existing unmerged index before snapshot acquisition.
  It repeats that validation at snapshot and adoption boundaries.
  A snapshot is registered for rollback only after its exact backup exists.
  An earlier failure finalizes prior temporary snapshots without a reset of unowned state.
  Each strict-sync push requests porcelain status.
  Only an actual ref creation, update, or deletion marks Git publication.
  An up-to-date push remains rollback-capable, and successful pull-request creation remains a publication event.
  An unprovable successful push fails closed under handoff.
  Local recovery journals only refs and worktrees that the invocation mutates.
  It advances each expected ref after successful Git commands and validates ownership before destructive cleanup.
  It restores only owned refs to their starting values with compare-and-swap.
  The declarative tables prove preservation of a stash-apply conflict, rollback after a no-op push, and survival of a concurrent sibling commit.
  `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check` passed on 2026-07-29.
- [x] [B046] (P0) Repair stale linked-worktree linkage before strict-sync preflight.
  Reported on 2026-07-29 after `gix sync` in `/Users/tyemirov/Development/TellTale` failed while resolving `MERGE_HEAD` inside `/Users/tyemirov/Development/story-generator-b007`.
  Observation:
  - The linked checkout still exists, but its `.git` file points at the removed `/Users/tyemirov/Development/story-generator` common repository.
  - The current common repository lists the checkout without the Git `prunable` marker.
  - Thus, B030 missing-directory cleanup does not apply.
  - Strict preflight treats every non-prunable record as valid.
  - It runs `git rev-parse --git-path MERGE_HEAD` inside the broken checkout and fails before caller inspection.
  Requirements:
  - Repair canonical Git worktree metadata before strict sync classifies and inspects registered worktrees.
  - Distinguish an existing linked checkout with repairable stale linkage from a missing Git-prunable checkout.
  - Never prune a live checkout blindly.
  - Preserve the linked checkout's branch, commit, index, tracked contents, untracked contents, and staged/unstaged distinction.
  - Re-list and inspect the repaired topology before fetch, stash, checkout, LLM dispatch, commit, push, or pull-request creation.
  - Fail with worktree and repository context when Git cannot repair or validate the topology.
  Validation:
  - Add a public compiled-CLI regression that creates a linked checkout and moves the primary repository.
  - Prove that the old linked-checkout `.git` pointer fails.
  - Prove that sync repairs the registration without a change to the linked checkout state.
  - Preserve the existing missing-prunable-worktree regression and every strict-sync worktree preflight scenario.
  - Run `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check`.
  Resolution:
  Strict sync now runs the canonical Git `worktree repair` from the caller before worktree inspection.
  Existing linked checkouts with a moved primary repository are reconnected and listed again before preflight.
  Missing Git-prunable registrations remain on the B030 prune path.
  Repair failures retain repository context.
  A public compiled-CLI regression moves the primary repository after it creates a dirty linked checkout.
  The old link returns the reported `rev-parse --git-path MERGE_HEAD` failure.
  Sync then repairs the link and exactly preserves the sibling branch, commit, index, contents, and untracked files.
  `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check` passed on 2026-07-29.
  Review follow-up:
  Unconditional `git worktree repair` treated the invoking common repository as authoritative.
  When a primary repository was copied, the copy retained the original worktree registrations.
  Repair then rewrote a live sibling `.git` pointer away from the existing original repository.
  Follow-up resolution:
  Strict sync now lists the topology first and resolves each live checkout common Git directory.
  It rejects a checkout that another live common repository owns.
  It sends only a checkout with a missing canonical `.git` target to `git worktree repair`.
  It then lists the topology again and validates ownership before administrative-state inspection.
  The original moved-primary regression remains green.
  A new compiled-CLI regression copies the primary repository and proves that sync fails closed.
  It verifies exact topology, branch, commit, index, contents, and `.git` pointer state for both repositories and the sibling.
  `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check` passed on 2026-07-29.
- [x] [B047] (P0) Reject concurrent checkout or index drift during dirty sync commits.
  Reported on 2026-07-30 after two `gix sync` runs overlapped another writer in the same llm-proxy checkout.
  Observation:
  - During `gix sync master`, another writer created and checked out `bugfix/B098-visible-fail-closed-ci` while Gix generated commit messages.
  - Branch compare-and-swap correctly rejected the outside ref.
  - Rollback repeated the ownership error and reported a generic restoration failure.
  - During plain sync, another writer staged `tests/lifecycle_contract_test.go` while Gix waited for the README commit message.
  - Gix committed that outside-staged path with README.
  - It then reported `no changes detected for commit message generation` for the empty tests cluster and rolled back.
  Requirements:
  - Treat the selected checkout and exact index as owned state across each slow dirty-cluster commit-message request.
  - Validate the cluster exact staged path set before dispatch.
  - Validate the exact checkout, HEAD, and index again before commit.
  - If another writer changes the checkout or index, do not commit, push, reset, clean, or restore.
  - Preserve the transaction snapshots and emit one `SYNC_SWITCH_HANDOFF` that names the ownership loss.
  - Tell the operator to stop the other writer before a retry.
  - Keep branch-ref compare-and-swap protection and ordinary pre-publication rollback for failures that occur while ownership remains intact.
  - Do not reinterpret an empty later cluster as success or retain the opaque `no changes detected` outcome for this concurrency case.
  Validation:
  - Add public compiled-CLI coverage that changes the checkout during a commit-message request.
  - Prove that cleanup does not change the outside branch, HEAD, index, files, or transaction snapshot.
  - Add public compiled-CLI coverage that stages a later cluster during an earlier commit-message request.
  - Prove that Gix does not include that path in its commit.
  - Prove that Gix does not roll back over the outside index.
  - Run `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check`.
  Resolution:
  Dirty-cluster commits now expand and validate the complete staged path set.
  They checkpoint the active checkout, `HEAD`, and semantic index entries before each LLM request.
  Gix examines that state again after the request through a bounded cancellation-independent inspection when necessary.
  Checkout or index drift stops before commit or push and marks local transaction ownership as lost.
  It preserves the outside state and transaction snapshot.
  It emits one actionable `SYNC_SWITCH_HANDOFF` instead of a rollback attempt.
  Public compiled-CLI regressions reproduce both reported interleavings from the LLM boundary.
  They prove no commit, push, reset, clean, or rollback occurs after the outside mutation.
  `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check` passed on 2026-07-31.
  Review follow-up on 2026-07-31:
  - The final ownership read completed before an unlocked `git commit`.
  - This sequence left a time-of-check/time-of-use window for outside staged work.
  - The checkpoint omitted skip-worktree, assume-unchanged, intent-to-add, and resolve-undo state.
  - Cancellation could arrive after the conditional context validation and interrupt the ownership read.
  Follow-up resolution:
  Gix now resolves the exact per-worktree index path and compares each commit-relevant entry and semantic flag.
  It uses a bounded context that is separate from caller cancellation for each post-model ownership read.
  Gix holds the canonical index lock during the final read.
  It copies the validated index into that private locked file and commits through `GIT_INDEX_FILE`.
  An outside writer mutates before the validation and triggers handoff, or the writer cannot get the lock.
  Focused cancellation coverage and public compiled-CLI regressions prove rejection of semantic-flag drift and post-validation staging.
  They also prove separate cluster commits and index-lock release after success or handoff.
- [x] [B048] (P1) Follow transitively merged pull-request stacks to master.
  Reported on 2026-08-01 after plain `gix sync` on `tyemirov/improvement/I205-inventory-placement-groups` returned `branch ... does not have an open pull request` even though its pull request and every parent pull request were merged into `master`.
  Observation:
  - Gateway PR #164 merged I205 into B388, PR #163 merged B388 into B387, and PR #162 merged B387 into `master`.
  - Strict sync examines open pull requests and asks whether the selected branch merged directly into `master`.
  - Without local `gix-review-base` metadata, it does not discover the actual merged base or traverse the merged parent chain.
  Requirements:
  - Discover the actual base of a merged pull request for an existing branch even when no local stacked-review metadata exists.
  - Traverse each merged parent pull request until the chain reaches the first active remote branch.
  - When every link reaches `master`, use the standard merged-branch prompt to offer sync of `master`.
  - Prefer an active open pull request over historical merged records for a reused branch name.
  - Reject cycles or missing terminal branches without mutation.
  - Preserve strict-sync transaction rollback, dirty merged-branch rejection, and single-prompt confirmation behavior.
  Validation:
  - Add public compiled-CLI coverage for a three-hop merged stack with all remote branch refs.
  - Do not add local review-base metadata.
  - Verify that plain `gix sync` offers the standard `master` handoff.
  - After acceptance, switch to synchronized `master`, create no pull request, and emit no rollback.
  - Run `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check`.
  Resolution:
  Gix now discovers the actual base of a merged pull request without an assumed `master` or local review-base metadata.
  It traverses any number of merged parent pull requests while their remote refs remain.
  It stops at the configured base, an active open pull request, or a live terminal branch.
  Active review state wins over historical merged records, and cycles fail before mutation.
  A fully merged chain produces one standard handoff prompt for the terminal base.
  Dirty merged branches are rejected before commit.
  The compiled-CLI regression reproduces the reported I205-to-B388-to-B387-to-`master` topology with all refs and no local metadata.
  It first failed with the reported missing-open-pull-request rollback.
  It now proves that acceptance synchronizes `master` without a pull request or rollback.
  Focused tests cover active-parent precedence, cycle rejection, and dirty-branch order.
  `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check` passed on 2026-08-01.
  Review follow-up:
  The first traversal keyed historical merged pull requests only by branch name.
  As a result, a reused or advanced branch could inherit its old stack and hand off to `master`.
  Gix now requests each pull request `headRefOid`.
  It accepts the merged record when that OID matches the fetched remote tip and no local-only commits exist.
  It also accepts the record when neither local nor remote ref survives.
  A mismatched selected head continues through the remote-branch publication flow.
  A mismatched parent becomes the terminal handoff.
  The compiled-CLI regression first reproduced the false `master` handoff for a new reused child.
  It now proves that Gix creates the new pull request.
  Focused coverage proves that an advanced parent stops traversal.
  `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check` passed for the follow-up on 2026-08-01.
- [x] [B049] (P1) Preserve complete multi-provider failure context in CLI output.
  Reported on 2026-08-06 after `gix sync` displayed `llm_proxy_client_http_failure (and 1 more failures)` when both the prioritized LLM Proxy request and the direct OpenAI request failed.
  Observation:
  - The prioritized client retains each connection name and wrapped transport error.
  - The workflow executor recursively unwraps ordinary error chains to their terminal leaves.
  - The final command error prints only the first collected leaf and a count.
  - This output hides the OpenAI failure and strips the LLM Proxy HTTP status and response body.
  Requirements:
  - Preserve the complete contextual error returned by an ordinary operation, including every named prioritized LLM connection and its wrapped cause.
  - Continue splitting typed repository `OperationError` joins so their existing structured event codes and subjects remain independently reportable.
  - Keep standard Go error traversal intact for callers that use `errors.Is` and `errors.As`.
  Validation:
  - Add public compiled-CLI coverage in which LLM Proxy returns HTTP 503 and direct OpenAI returns HTTP 429. The command must attempt both connections and print both names, statuses, and response details without the opaque `and 1 more failures` summary.
  - Preserve existing joined repository-operation event coverage.
  - Run `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check`.
  Resolution:
  The workflow executor now preserves the complete outer context of ordinary operation errors, including joined prioritized-client attempts.
  It continues to split typed repository `OperationError` values for independently coded structured events.
  Final multi-failure summaries print each formatted failure instead of a count.
  Returned causes retain standard `errors.Is` and `errors.As` traversal.
  The compiled-CLI regression first reproduced the reported proxy sentinel and hidden OpenAI failure.
  It then proved that LLM Proxy HTTP 503 and direct OpenAI HTTP 429 retain connection names, statuses, and response details.
  `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check` passed on 2026-08-06.
- [x] [B050] (P1) Reconcile legacy GitHub Pages deployment state without duplicate builds.
  Reported on 2026-08-06 after `make deploy` pushed the v1.1.20 Pages branch.
  The command triggered two GitHub-generated `pages-build-deployment` runs.
  During a GitHub Pages outage, it reduced a queued build to a stale-marker timeout.
  Observation:
  - The deploy helper updates an already-correct legacy Pages configuration and explicitly requests another build.
  - Thus, one invocation creates competing builds for the same `gh-pages` commit.
  - Retries ignore existing Pages build state and can enqueue another build instead of reusing an active or completed build.
  - Verification polls only the public marker.
  - Thus, terminal failures and active builds lose their native status, error, commit, and build URL.
  Requirements:
  - Keep the canonical legacy `gh-pages:/` publication source and `.nojekyll` artifact contract.
  - Do not add a repository-owned Pages workflow.
  - Read and validate the current Pages configuration.
  - Mutate the configuration only when it is missing or different.
  - Distinguish a confirmed HTTP 404 from other GitHub API failures.
  - Treat a changed Pages branch or configuration as the invocation build trigger.
  - Reuse a matching built, queued, or active build record.
  - For an unchanged retry, request one rebuild only when the exact commit has no build or an errored build.
  - Verify the exact Pages build before checking the public release marker. Preserve bounded polling while reporting the build status, native error, commit, and URL on failure.
  Validation:
  - Add public release-script coverage for a changed branch with current configuration.
  - Cover missing and drifted configuration and an unavailable configuration API.
  - Cover an active build that completes and a terminal build whose single retry fails.
  - Prove that current configuration causes no PUT.
  - Prove that a changed branch or configuration causes no redundant build POST.
  - Prove reuse of completed and active builds.
  - Prove that a terminal unchanged build receives exactly one rebuild request.
  - Run `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check`.
  Resolution:
  The Pages deploy helper now treats branch and configuration mutations as the legacy Pages build trigger.
  It preserves an already-canonical `gh-pages:/` configuration.
  It fails closed after each configuration inspection failure other than a confirmed HTTP 404.
  On unchanged retries, it selects the exact deployed branch commit and accepts completed builds.
  It reuses queued or active builds and requests one rebuild for a missing or errored build.
  Verification waits for that build before it probes the public marker.
  It reports native status, error, commit, and URL context.
  Public release-script regressions cover the configuration and build-state matrix, including the queued state from the GitHub build API.
  `make format`, `make test`, `make lint`, `make ci`, `make build`, `bash -n scripts/release/deploy_pages_artifact.sh`, and `git diff --check` passed on 2026-08-06.
- [x] [B051] (P0) Make exact-tag release retries reuse verified sealed state.
  Reported on 2026-08-06 after a canonical lifecycle retry at v1.1.21 selected v1.1.22.
  The retry failed because `v1.1.21..HEAD` contained no commits.
  It replaced the valid local release receipt with incomplete v1.1.22 staging.
  Observation:
  - Release version selection always bumps the latest tag, even when `HEAD` is already the exact published release commit.
  - Release initialization deletes `.git/mprlab-release` before release-note generation and final sealing, so a later preparation failure destroys the prior valid receipt.
  - `make deploy` then correctly rejects the missing manifest, leaving the canonical retry unable to resume the already-published release.
  Requirements:
  - Detect an exact release tag at `HEAD` before CI, version selection, artifact preparation, changelog mutation, commit, or tag creation.
  - Verify and reuse a complete matching local sealed release.
  - When local state is incomplete, recover the same release from immutable published GitHub release state.
  - Before reuse, verify its manifest, notes, payload hashes, tag, release commit, source parent, and release-only changelog change.
  - Prepare a new release in an isolated candidate receipt.
  - Promote it atomically only after the complete manifest, notes, and payload inventory pass validation.
  - Preserve the prior canonical receipt on every failed candidate preparation and reject conflicting local or published state without overwriting it.
  Validation:
  - Add public release-script coverage that reuses a valid exact-tag receipt without CI or artifact preparation.
  - Recover missing or incomplete local state from matching published state.
  - Verify that conflicting published state fails closed.
  - Prove a new-release failure after candidate initialization leaves the prior canonical receipt byte-for-byte unchanged.
  - Run `make format`, `make test`, `make lint`, `make ci`, `make build`, and `git diff --check`.
  Resolution:
  Release preparation now detects one exact version tag before version selection or CI.
  It reconciles that immutable release instead of selection of a successor.
  A complete matching local receipt is reused without GitHub access.
  A missing or partial receipt is rebuilt from the published GitHub Release after all release evidence passes validation.
  This evidence includes the release object, tag, manifest, release commit, source parent, notes, asset inventory, paths, sizes, and hashes.
  New releases prepare in a sibling candidate directory and promote only a complete verified receipt.
  Thus, each earlier preparation failure leaves the canonical receipt unchanged.
  A failure after release commit or tag creation restores the owned refs and `CHANGELOG.md` to the prepared source.
  Conflicting local or published state fails closed with contextual errors.
  Public release-script regressions cover local reuse, recovery, malformed receipts, conflicting receipts, candidate failure, and post-tag rollback.
  `make format`, `make test`, `make lint`, `make ci`, `make build`, shell and Python syntax checks, and `git diff --check` passed on 2026-08-06.
- [x] [B052] (P0) Handle LLM proxy failures during gix sync PR description generation.
  Goal:
  Make `gix sync` report pull request description LLM failures clearly.
  Restore the expected checkout and worktree state after this failure.
  
  Requirements:
  - Preserve rollback that restores the starting checkout, local state, and adopted worktree topology after incomplete strict sync.
  - Do not leave the target branch active after rollback.
  - Report the underlying LLM or proxy failures without sensitive credentials or full API keys.
  - Keep strict sync semantics unchanged.
  
  Deliverables:
  - Improve handling and reports for `strict sync pull request description.llm` failures.
  - Sanitize error output for LLM proxy URLs and keys.
  - Add necessary tests for empty responses, proxy HTTP or TLS failures, and rollback.
  - Update documentation or help text when user-facing guidance changes.
  
  Validation:
  - Run the relevant sync and pull request description tests.
  - Reproduce or simulate an LLM proxy failure.
  - Verify that `gix sync` reports a sanitized actionable message and restores the original state.
  - Verify that logs do not include full LLM proxy keys or other secrets.
  Resolution:
  Pull request description generation now sends underlying errors through `sanitizeLLMDescriptionError`.
  The helper redacts query parameters, authentication headers, URL credentials, and configured connection secrets.
  Strict sync transaction rollback runs before remote push when description generation fails.
  It restores starting checkouts, branches, and worktree topology.
  Unit and integration tests verify error sanitization, empty responses, and strict sync transaction rollback behavior.
- [x] [B053] (P0) Recover direct OpenAI after reasoning-only empty completions.
  Reported on 2026-08-08 after `gix sync` failed to generate a pull request description.
  LLM Proxy returned a TLS transport error, and direct OpenAI exhausted three attempts with empty responses.
  Observation:
  The prioritized client reaches the configured OpenAI connection, but the direct client repeats the same bounded reasoning request three times. If every response consumes its completion budget before producing visible text, the usable backup connection is reported as failed and strict sync rolls back.
  Requirements:
  - Preserve configured connection priority, OpenAI model, reasoning effort, timeout, and ordinary error behavior.
  - Add `max_completion_tokens` to each provider profile and resolve the budget through the canonical `command > provider > llm` hierarchy.
  - Keep token-budget policy in `config.yml`.
  - Do not raise or default completion budgets in request or recovery code.
  - After typed empty-response exhaustion, repeat the resolved direct OpenAI request for one bounded recovery round.
  - Preserve cancellation and complete primary/recovery failure context. Do not retry authentication, HTTP, or unrelated transport failures through this recovery path.
  Validation:
  - Add compiled CLI coverage in which LLM Proxy fails at the inherited global budget.
  - Return empty `finish_reason=length` responses from three direct OpenAI attempts.
  - Verify that recovery succeeds at the provider budget.
  - Verify an explicit command budget overrides both provider and global values, and sync does not inherit the `message commit` command budget.
  - Preserve the existing compiled-CLI coverage for complete multi-provider failure reporting.
  - Run `make format`, focused tests, `make ci`, `make build`, and `git diff --check`.
  Resolution:
  Provider profiles now accept and strictly validate `max_completion_tokens`.
  Application configuration resolves completion budgets through `command > provider > llm`.
  Absent command values remain unset through all request builders.
  Sync no longer inherits the `message commit` command budget.
  The generated and active user configuration assign 16,384 tokens to direct OpenAI.
  LLM Proxy inherits the global 1,200-token value.
  Direct OpenAI repeats the resolved request for one recovery cycle only after typed empty-response exhaustion.
  Ordinary failures, cancellation, and joined primary or recovery context remain unchanged.
  The compiled CLI regression proves one failed proxy request and three empty OpenAI attempts.
  It also proves successful recovery at the provider budget and complete multi-provider error reports.
  Focused tests, `make format`, `make ci`, `make build`, and `git diff --check` passed on 2026-08-08.
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
  Application initialization now requires a positive top-level `llm.max_completion_tokens`.
  `gix init` generates exactly one completion-token setting, the 4,800-token global budget.
  Provider and command fields remain optional explicit overrides in the `command > provider > llm` hierarchy.
  Numeric completion-token policy is absent from production Go defaults.
  Compiled regressions, hierarchy coverage, focused integration tests, `make format`, and `make ci` passed on 2026-08-08.
  `make build` and `git diff --check` completed before publication.
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
  The Make release boundary now forwards explicit `RELEASE_BUMP`, `RELEASE_VERSION`, and `RELEASE_SCHEME` values.
  New SemVer preparation requires exactly one patch, minor, major, or exact-version intent.
  It rejects missing, conflicting, and CalVer-incompatible inputs before CI.
  Timestamp-derived CalVer selection and zero-argument exact-tag receipt reuse remain unchanged.
  Public Make and release-script regressions cover each intent path.
  They preserve candidate isolation, rollback, receipt recovery, and publication behavior.
  Focused release tests, shell and Python syntax checks, `make format`, and `make ci` passed on 2026-08-08.
  `make build` and `git diff --check` completed before publication.
  Review follow-up:
  A checkout without local version tags reported `scheme_guess: none`, bypassed the new missing-intent guard, and still selected `v1.0.0` before running CI.
  Follow-up resolution:
  Release selection now normalizes the untagged default to the initial SemVer contract unless CalVer is explicitly requested.
  A public release-script regression proves that bare preparation stops before CI and preserves the canonical receipt.
  An explicit patch intent selects `v1.0.0` with a SemVer scheme.
  Focused release tests, `bash -n scripts/release/prepare_release.sh`, `make format`, `make ci`, `make build`, and `git diff --check` passed on 2026-08-08.
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
  - Verify that the failed v5.0.1 attempt does not change master or the v5.0.1 tag.
  - Verify that the attempt does not change the canonical v1.1.25 receipt.
  - Run focused release tests, `bash -n scripts/release/prepare_release.sh`, `make format`, `make ci`, `make build`, and `git diff --check`.
  Resolution:
  Release preparation now removes release-control and recursive-Make variables only around its `make ci` subprocess.
  It retains the selected version and artifact inputs in the outer transaction.
  The public regression seeds each affected variable and proves that the CI subprocess receives none of them.
  The failed v5.0.1 attempt did not change master, the tag namespace, or the canonical v1.1.25 receipt.
  Focused release coverage, shell syntax validation, `make format`, `make ci`, and `make build` passed on 2026-08-08.
  `go mod verify`, `go mod tidy -diff`, and `git diff --check` also passed.
- [x] [B057] (P0) Accept inherited whitespace in a resolved merge.
  Reported on 2026-08-08 after `gix sync` resolved three conflict regions in `.mprlab/ISSUES.md` and then rolled back.
  Observation:
  - `origin/master` already contains the reported extra blank line at the end of `.mprlab/ISSUES.md`.
  - The final staged index check compares the result only to the current branch.
  - The check incorrectly identifies exact incoming content as a merge resolution error.
  Requirements:
  - Validate each staged path against both merge parents.
  - Accept a path when Git reports no new whitespace error relative to one parent.
  - Reject a path when both parent comparisons report a whitespace error.
  - Preserve conflict-region validation and operation-owned rollback behavior.
  Validation:
  - Add public CLI coverage for a resolved conflict with an incoming extra blank line at the file end.
  - Prove that sync preserves the incoming bytes and completes the merge.
  - Preserve rejection coverage for whitespace that the resolution adds.
  - Run `make format`, focused tests, `make ci`, `make build`, and `git diff --check`.
  Resolution:
  Resolved merge validation now checks each staged path against the current parent and the incoming parent. Exact whitespace from either parent can pass. Whitespace that is new to both parents still fails and uses operation-owned rollback. The public CLI regression preserves the incoming blank line, completes the semantic merge, pushes, and leaves a clean checkout. `make format`, `make test-fast`, `make test-slow`, `make ci`, `make build`, and `git diff --check` passed on 2026-08-08.
- [x] [B058] (P0) Automate SemVer release decisions.
  Requested on 2026-08-08 after bare `make release` required an operator-supplied bump.
  Goal:
  Make release preparation, publication, and deployment self-contained while retaining Semantic Versioning.
  Requirements:
  - Give `make release`, `make publish`, and `make deploy` no release flags, arguments, or lifecycle-variable inputs.
  - Detect the existing version scheme from repository tags. Start an untagged repository at `v1.0.0` and retain timestamp-derived CalVer for an established CalVer line.
  - For an established SemVer line, use an LLM decision node to inspect the complete committed range.
  - Include commit messages, the diff summary, the Unreleased changelog, and a bounded diff excerpt.
  - Require exactly one closed `major`, `minor`, or `patch` decision with a concise reason. Fail closed when the model is unavailable or its output is invalid.
  - Enforce a deterministic `major` floor for Conventional Commit `!` or `BREAKING CHANGE` evidence and a `minor` floor for `feat` evidence. Permit the model to raise the floor but never lower it.
  - Preserve exact-tag receipt reuse and recovery, candidate isolation, release-owned rollback, canonical publication, fixed Pages activation, and public-marker verification.
  Validation:
  - Add focused decision-node coverage for every SemVer level, complete-range floors, invalid output, provider failure, and strict JSON output.
  - Add public Make and release-script coverage for zero-argument target dispatch and autonomous SemVer selection.
  - Cover decision failure, initial SemVer, CalVer, exact-tag reuse, receipt preservation, and Pages reconciliation.
  - Run `make format`, focused tests, shell and Python syntax checks, `go mod verify`, `go mod tidy -diff`, `make build`, `make ci`, and `git diff --check`.
  Resolution:
  The lifecycle Make targets now call fixed repository helpers with no arguments. Each repository declares its release scheme in `.mprlab/release.yml`. The public `gix release next` command owns SemVer and CalVer selection. For SemVer, Gix resolves the boundary tag and source commit before it collects evidence. Gix classifies all commit messages, diff summaries, and Unreleased notes through bounded evidence packets. The first packet also contains the bounded diff excerpt. Gix selects the highest packet result and applies the non-lowerable Conventional Commit floor. Selection failures stop before candidate creation. An untagged SemVer repository selects `v1.0.0`, and CalVer uses the canonical UTC timestamp. Exact-tag reconciliation, sealed publication, and fixed Pages deployment retain their current contracts.
- [x] [B059] (P0) Keep root Go install on the latest Gix release.
  Reported on 2026-08-09 after the `v6.0.0` release.
  Expected result:
  `go install github.com/tyemirov/gix@latest` installs the latest Gix product release.
  Actual result:
  The command installs `v1.1.25` because the release changed the module path and did not advance the root Go module channel.
  Requirements:
  - Keep `go install github.com/tyemirov/gix@latest` as the only documented Go installation command.
  - Keep product SemVer as the canonical user-visible release version.
  - Publish one root Go transport tag for each product release. Both tags must identify the same release commit.
  - Make `gix version` report the product version for a root Go installation.
  - Store the product version and transport version in the Gix version decision and sealed release receipt.
  - Reject release preparation or publication when either version, tag, module path, or commit identity does not match.
  - Restore the root module path and update all current imports in one forward-only change.
  Validation:
  - Add a compiled test that runs the canonical `go install` command against a module proxy and verifies the installed product version.
  - Add public release coverage for product-version updates, paired local tags, rollback, receipt recovery, publication, and remote tag verification.
  - Run `make format`, focused tests, `make ci`, module checks, Governor checks, and `git diff --check`.
  Resolution:
  Gix now uses the root `github.com/tyemirov/gix` module. Each product release also gets one v1 Go transport tag on the release commit.
  The version decision and schema 3 receipt bind both versions, the module path, the product-version file, and the release commit.
  Release preparation and publication reject a mismatch. The installed root-module binary reports the embedded product version.
  Compiled installation coverage runs the canonical command. Public release tests cover paired tags, rollback, recovery, publication, and remote verification.
  `make format`, `make build`, `make ci`, module checks, syntax checks, and `git diff --check` passed on 2026-08-09.
  The Governor normalizer and changed-line language review also passed.
- [x] [B060] (P0) Classify SemVer from supported public contracts.
  Reported on 2026-08-09 after Gix selected `v7.0.0` for a root Go installation repair.
  Expected result:
  Internal implementation changes select a patch release unless they change a supported public contract.
  Actual result:
  Commit labels and a Go module-path change caused Gix to select a major release.
  Requirements:
  - Treat Conventional Commit labels as evidence only.
  - Require an exact supported public contract for a major or minor classification.
  - Select `major` only when a supported external use becomes incompatible.
  - Select `minor` only when a release adds optional public functionality.
  - Select `patch` for compatible repairs and internal implementation changes.
  - Audit each candidate against the same complete evidence packet.
  Validation:
  - Add regression coverage for feature labels and breaking markers on internal changes.
  - Add regression coverage for a repaired root Go installation route.
  - Run focused tests, `make ci`, Governor checks, and `git diff --check`.
  Resolution:
  Gix now treats commit syntax as evidence only. Each evidence packet gets one candidate and one independent audit.
  Gix maps incompatible, additive, and compatible public effects to `major`, `minor`, and `patch`.
  Major and minor results must name the exact supported public contract.
  Regression coverage includes internal feature labels, breaking markers, and the repaired root Go installation route.
  Focused tests, `make ci`, Governor checks, changed-line language review, and `git diff --check` passed on 2026-08-09.
- [x] [B061] (P1) Distinguish detached linked checkouts in audit reports.
  Reported on 2026-08-10.
  Observation:
  - The primary `llm-proxy` checkout is on `master`.
  - The registered `.codex-deps/llm-proxy` checkout is detached.
  - The audit labels both checkouts as `llm-proxy`, so the detached row appears to describe the primary checkout.
  Requirements:
  - Keep a parent audit root when it contains discovered repository paths.
  - Use the relative checkout path to identify each nested linked checkout.
  - Preserve current paths for repositories that moved outside their original roots.
  - Report the actual branch state for each checkout.
  Validation:
  - Add compiled CLI coverage for an attached primary checkout and a detached nested linked checkout with the same folder name.
  - Run focused tests, `make ci`, and `git diff --check`.
  Resolution:
  Audit now compares relative and absolute roots through canonical paths. It keeps the containing parent and removes redundant nested repository roots.
  The nested checkout retains `.codex-deps/llm-proxy`, while the primary checkout retains `llm-proxy`.
  Focused tests, `make build`, `make ci`, the live fleet audit, and `git diff --check` passed on 2026-08-10.
- [x] [B062] (P0) Keep compatible bugfix releases on the patch line.
  Reported on 2026-08-10 after the B061 audit repair was published as `v1.3.0` instead of `v1.2.1`.
  Expected result:
  A compatible Gix bugfix increments the patch version under the fixed `v1` policy.
  Actual result:
  SemVer evidence included historical feature entries that were already present at `v1.2.0`, so the model selected a minor release for the B061-only range.
  Requirements:
  - Collect changelog evidence from the exact boundary-to-source range.
  - Exclude unchanged historical Unreleased entries from every model request.
  - Preserve complete bounded evidence for new changelog changes.
  - Select a patch for the B061-shaped compatible audit repair.
  Validation:
  - Reproduce the stale historical feature entry with a new compatible bugfix entry.
  - Verify model requests contain the new bugfix and omit the historical feature.
  - Run focused tests, `make ci`, and `git diff --check`.
  Resolution:
  SemVer selection now excludes `CHANGELOG.md` from the general diff excerpt and supplies one no-context changelog diff for the exact boundary-to-source range.
  The compiled CLI regression reproduces the `v1.2.0` historical feature entry and the B061-compatible audit fix.
  It verifies that both model requests exclude the old feature and selects `v1.2.1`.
  `make format`, `make test-fast`, `make test-slow`, `make ci`, and `git diff --check` passed on 2026-08-10.



## Improvements

- [x] [I011] (P0) Use one fixed-major version for Gix releases.
  Requested on 2026-08-09.
  Goal:
  Gix uses one fixed major version for its releases. Other repositories retain their declared SemVer or CalVer policy.
  Requirements:
  - Declare the fixed `v1` policy in the Gix release config only.
  - Select a minor Gix release for each intentional Gix public contract change.
  - Select a patch Gix release for compatible fixes and internal changes.
  - Keep standard major, minor, and patch analysis for other SemVer repositories.
  - Keep one version in the decision, Git tag, receipt, manifest, binary, and GitHub Release.
  - Make `go install github.com/tyemirov/gix@latest` install that version.
  - Retract the valid historical root-module versions through `v1.1.26`.
  - Exclude the invalid `v2` through `v7` tags from release selection.
  - Remove the obsolete Go install version contract without a compatibility path.
  - Keep the current CalVer selection contract.
  Validation:
  - Add public CLI coverage for fixed-major minor and patch selection.
  - Add release lifecycle coverage for one tag and one version identity.
  - Add compiled installation coverage for the canonical Go install command.
  - Run focused tests, module checks, `make ci`, Governor checks, and `git diff --check`.
  Resolution:
  Gix now declares the fixed `v1` policy in its release config. Other repositories retain standard SemVer or their declared CalVer policy.
  Gix public contract changes select minor releases. Compatible and internal changes select patch releases.
  The decision, tag, receipt, manifest, binary, and GitHub Release use one version.
  The release lifecycle no longer has a separate Go install version. The Gix module retracts prior root-module versions through `v1.1.26`.
  Fixed-major selection excludes other major tags. Standard SemVer still supports major, minor, and patch releases.
  The fixed-major policy remains internal to Gix. The shared version-decision contract remains `mprlab.version-decision/v1` for Gateway release consumers.
  Compiled installation coverage proves that `go install github.com/tyemirov/gix@latest` selects and reports the authoritative version.
  Focused tests, module checks, `make build`, `make ci`, Governor checks, and `git diff --check` passed on 2026-08-09.
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
  The application loader now does one strict YAML decode directly into the typed target.
  It rejects unknown fields through that decoder.
  Environment placeholders then expand in decoded string values without another decode of a generic YAML map.
  This step preserves literal characters, optional credential placeholders, and required-placeholder errors.
  The loader no longer imports `mapstructure` or constructs `map[string]any`.
  Command-specific operation option maps retain their separate typed decode after the application schema loads.
  Focused loader, documentation, and compiled-CLI regressions pass, including rejection of the removed `temperature` key.
  Both `make ci` gates, `make format`, the live version command, and `git diff --check` passed on 2026-08-08.
- [-] [I012] (P0) Make release policy invocation-owned.
  Requested on 2026-08-10.
  Goal:
  Keep Gix independent from MPR Lab repository files while it remains the release version authority.
  Requirements:
  - Require one explicit `semver` or `calver` policy for `gix release next`.
  - Accept a fixed major only for the SemVer policy.
  - Record the complete applied policy in version decision contract v2.
  - Make Gateway obtain application policy from the selected schema-v4 manifest.
  - Keep `make release`, `make publish`, and `make deploy` free of operator inputs.
  - Delete the obsolete `.mprlab/release.yml` contract after the fleet migration.
  Deliverables:
  - Typed CLI policy parsing and black-box release decision coverage.
  - Updated Gix and Gateway release preparation boundaries.
  - Migrated MPR application manifests and current technical documentation.
  Validation:
  - Run the focused Gix and Gateway lifecycle tests.
  - Run the final repository CI gates after the last application change.
  - Run the Governor, STE, and Git checks on changed technical documents.

- [x] [I013] (P2) Normalize managed policy content.
  Requested on 2026-08-13.
  Goal:
  The repository policy matches the current MPR Lab Governor contract.
  Requirements:
  - Add the current managed `Credential Discovery` section to `.mprlab/POLICY.md`.
  - Preserve application files and repository-owned governance text.
  Validation:
  - Run the Governor check.
  - Run `git diff --check`.
  Resolution:
  The Governor added the current `Credential Discovery` rules to `.mprlab/POLICY.md`.
  The update preserved application files and other governance text.
  The Governor check, policy STE check, and `git diff --check` passed on 2026-08-13.
  The full issue tracker retained 275 pre-existing mechanical STE findings. The new I013 text added none.
  Follow-up on 2026-08-22:
  The current Governor contract added the selected manifest rules and updated credential discovery.
  The language checker found the same 275 historical mechanical errors in the issue tracker.
  This cleanup corrected those errors without a change to issue identity, status, technical meaning, or validation evidence.
  The Governor check, policy and tracker language checks, and `git diff --check` passed.

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
  Last run: 2026-07-24.
  The run archived 57 resolved or obsolete entries and retained 11 active entries.
  It normalized one-off maintenance IDs that incorrectly had a recurring suffix.
  It filed M014 for a discovered forward-only schema violation.
- [ ] [M002R] (P2) Polish open issues.
  Goal:
  Keep unresolved work executable by making each open issue concrete, ordered, and testable.
  Requirements:
  - Cadence: run weekly during active development and before handing a repo to automated execution.
  - Review every unresolved non-recurring issue for missing context, dependencies, repro steps, acceptance criteria, and validation expectations.
  - Make priorities concrete and make sure that each open issue has actionable deliverables.
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
  - Prefer current supported dependencies.
  - Do not add compatibility shims for obsolete dependency behavior.
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
  - Examine generated artifacts, release tags, published images, and Pages outputs for source-to-public drift.
  - File concrete follow-up issues for failing gates, stale artifacts, missing release prerequisites, or undocumented workflow changes.
  - Do not do a production deployment from this runbook unless the operator explicitly requests that deployment.
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
  - Examine static analysis, coverage, schema, and contract guards that must prevent drift.
  - File focused Maintenance issues for each concrete violation instead of broad cleanup placeholders.
  - Keep the current canonical contract only.
  - Do not preserve obsolete behavior unless a product requirement explicitly says so.
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
  - Examine links, command names, paths, and public contract descriptions touched by the pass.
  - Confirm docs describe the current canonical path only.
  - Confirm issue archive and active tracker references remain consistent.
  Last run: 2026-07-24.
  The run reviewed README, ARCHITECTURE, CHANGELOG, the documentation site, and design or runbook notes.
  It documented release commit roles and the local web audit queue.
  It marked superseded design and refactor notes as historical.
  It filed M014 for a legacy workflow-schema fallback.
- [ ] [M014] (P1) Remove the legacy flat workflow-safeguards schema.
  Found on 2026-07-24 during the backlog and documentation audit.
  Observation:
  - `internal/workflow/safeguards.go` accepts an unwrapped safeguards map and silently classifies it as `hard_stop` or `soft_skip` according to an internal fallback.
  - The pre-audit README documented legacy flat maps as accepted `hard_stop` input.
  - This audit removed that obsolete instruction, but the unsupported code path remains.
  - The binding forward-only contract forbids legacy configuration reads, aliases, and fallback behavior.
  Requirements:
  - Accept only the explicit `safeguards.hard_stop` and `safeguards.soft_skip` shape at the workflow configuration boundary.
  - Reject an unwrapped safeguards map with a contextual configuration error before any repository action is planned or executed.
  - Remove the legacy fallback branch and its regression expectations.
  - Do not add a migration reader or runtime compatibility path.
  - Update examples and README so they describe the one current schema only.
  Validation:
  - Add public workflow coverage that rejects a flat safeguards map before mutation.
  - Verify that the structured form retains its hard-stop and soft-skip behavior.
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
  The client was updated to v0.2.46.
  The Go module floor was raised to 1.25.12 with the dependency graph selected by the current client.
  Gix now puts its configured proxy work budget on each v2 messages request.
  It retains the caller-owned HTTP timeout.
  The HTTP-boundary regression verifies the canonical timeout header with provider, model, and token routing.
  `make format`, `make test`, `make lint`, `make ci`, `go mod verify`, and `go mod tidy -diff` passed on 2026-07-25.
  `git diff --check` also passed.
  The fast and black-box suites also passed with `GOTOOLCHAIN=go1.25.12`.
- [x] [M016] (P0) Correct the published SemVer history and establish the Go v5 module line.
  Requested on 2026-08-08 after the release audit found breaking changes published as consecutive `v1.1.x` patch releases.
  Goal:
  Publish the historically correct v2 through v5 release boundaries.
  Make the current v5.0.1 release installable through the canonical Go major-version module path.
  Requirements:
  - Publish v2.0.0, v3.0.0, v4.0.0, v4.1.0, and v5.0.0 as immutable aliases.
  - Point each alias at its audited historical release commit.
  - Reconstruct every aliased manifest, release note, and Pages marker with the corrected version instead of copying internally false artifacts.
  - Preserve the original binaries and source/release commit identities for each historical alias.
  - Change the current module path and all self-imports to `github.com/tyemirov/gix/v5` before v5.0.1.
  - Keep the historical migration bounded to the declared mapping.
  - Fail before mutation on conflicting state.
  - Remove the migration path after completion.
  Validation:
  - Public migration coverage proves exact mapping, corrected manifests and Pages markers, complete asset inventories, and pre-mutation rejection of conflicts.
  - `make ci`, `make build`, `go mod verify`, `go mod tidy -diff`, and `git diff --check` pass for the v5 source.
  - Every requested annotated tag and ready GitHub Release resolves to its audited commit with verified assets.
  - `go install github.com/tyemirov/gix/v5@v5.0.1` succeeds and the installed executable reports v5.0.1.
  Resolution:
  The audited v2.0.0, v3.0.0, v4.0.0, v4.1.0, and v5.0.0 aliases are ready GitHub Releases.
  Each alias points at its historical release commit.
  Their original binaries remain byte-identical.
  Their release notes, manifests, and embedded Pages markers identify the corrected versions.
  They retain the audited source and release commits.
  Current source now uses the canonical `github.com/tyemirov/gix/v5` module path throughout.
  The bounded migration rejected conflicting state and passed public tests plus two live read-only rehearsals.
  It published and verified each target asset and was removed after completion.
  `make format`, `make ci`, `make build`, `go mod verify`, and `go mod tidy -diff` passed before v5.0.1.
  Python syntax checks and `git diff --check` also passed.


## Features

- [ ] [F010] (P1) Make GitHub and GitLab first-class forge providers in one `gix` repository fleet.
  Requested on 2026-07-24.
  Goal:
  A single `gix` configuration and invocation must operate correctly over a root with GitHub and GitLab repositories.
  Provider selection must come from the parsed origin host.
  GitHub repositories use the GitHub adapter, and GitLab repositories use the GitLab adapter.
  Operators do not change roots, commands, or credentials by hand.
  Required outcome:
  - Support public `github.com` and `gitlab.com` as explicit provider kinds in the current configuration contract.
  - Preserve the complete GitLab project path, including nested groups such as `group/subgroup/project`.
  - Never reduce GitLab identity to a fixed two-segment GitHub-style `owner/repository` pair.
  - Make Git-only operations host-neutral and make forge-aware operations select the configured provider for each repository independently.
  - Treat pull requests and merge requests as one canonical `review request` concept in generic code and workflows.
  - Map that concept to a pull request in the GitHub adapter.
  - Map that concept to a merge request in the GitLab adapter.
  - Report unsupported or unconfigured remotes explicitly. An audit or workflow must not silently skip a GitLab repository because it is not GitHub.
  Scope boundary:
  - This feature covers discovery, audit, remote conversion, repository metadata, `sync`, workflow execution, review requests, web audit, CLI, config, and documentation.
  - Keep GHCR cleanup, GitHub Pages, and GitHub Release publication explicitly GitHub-scoped.
  - Keep this scope until a separate feature specifies a GitLab equivalent.
  - For a GitLab repository, fail before mutation with a capability-specific error.
  - Do not present these integrations as generic provider support.
  - Deliver public-host support.
  - Keep host as first-class data in the provider registry and identity model.
  - Keep support for a later issue that adds a configured self-hosted forge without another GitHub-shaped redesign.
  Current evidence:
  - `internal/repos/shared/types.go` defines canonical Git, SSH, and HTTPS URL prefixes with `github.com` embedded in each constant.
  - `internal/audit/helpers.go` recognizes only GitHub URL forms.
  - `internal/audit/service.go` rejects a non-GitHub origin.
  - The discovery loop treats an inspection error as skippable and can omit a GitLab repository from a mixed audit.
  - `cmd/cli/application_config.go`, `cmd/cli/application_bootstrap.go`, and `cmd/cli/default_config.yml` expose only `github.credential` and wire that credential directly into the GitHub client context.
  - `internal/workflow/executor.go` assumes GitHub metadata is available before it can build repository state, even when an operation is Git-only.
  - `internal/branches/syncflow` directly calls the GitHub CLI client for pull-request discovery and creation.
  - It resolves shorthand targets to `github.com` and normalizes only GitHub remote URL forms.
  - `internal/repos/remotes`, `internal/migrate`, `internal/ghcr`, release scripts, and several web/API labels encode GitHub-only names or URLs. Each call site needs an explicit classification as generic, provider-capable, or GitHub-only rather than an implicit GitHub default.
  Canonical configuration contract:
  - Replace the top-level `github` block with one required `forges` collection. Normal application startup accepts only this new schema.
  - Give each entry exactly `kind`, `host`, and `credential`.
  - Set `kind` to the closed enum `github` or `gitlab`.
  - Set `host` to a lower-case DNS host without scheme, path, port, userinfo, or trailing slash.
  - Set `credential` to a non-empty literal or a `${ENVIRONMENT_VARIABLE}` reference from the inherited process environment.
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
  - Reject malformed URLs, empty project paths, and paths that do not satisfy the selected provider shape.
  - Use a precise repository-scoped error.
  - Report an unconfigured valid host as `unsupported` or `unconfigured`.
  - Never reinterpret that host as GitHub.
  - Replace audit/API/CSV fields that expose GitHub-specific identity names with `origin_host`, `origin_provider`, `origin_project_path`, `canonical_project_path`, and `final_project_path`. Remove old GitHub-specific field names from the current public contract rather than emitting aliases or duplicate columns.
  Provider abstraction and capability contract:
  - Add a `ForgeProvider` interface and a host-indexed registry in `internal/forge`. Generic packages receive the registry and repository identity, never a `githubcli.Client`.
  - Model only observable shared behavior in the interface.
  - Include repository metadata, project identity, review-request operations, default-branch updates, branch protection, and provider capability discovery.
  - Define typed `RepositoryMetadata`, `ReviewRequest`, `ReviewRequestQuery`, and provider errors in the generic forge package.
  - Give a review request provider-neutral refs, title, body, state, web URL, and provider-native ID.
  - Do not refer to `PullRequest` or `MergeRequest` in a generic package.
  - Keep the existing GitHub CLI implementation behind a GitHub adapter during migration.
  - Make each invocation host-aware and make its input and output conform to the generic types.
  - Implement a GitLab REST v4 adapter behind the same interface.
  - URL-encode the complete nested project path for project endpoints.
  - Use only the configured GitLab credential.
  - Map merge-request state, branches, title, body, and web URL to the generic review-request model.
  - Define explicit capability constants.
  - Make generic operations declare required capabilities before repository mutation.
  - Preflight the selected provider and report unsupported capabilities with provider, host, repository path, and operation name.
  - Do not attempt a GitHub call as a fallback.
  Operation classification and canonical terminology:
  - Classify `files`, folder rename, Git fetch/push, local branch inspection, and transport-only remote conversion as Git-only. They run for both providers and do not require a forge credential.
  - Classify audit canonicalization/default-branch lookup, `sync`, default-branch management, review-request cleanup, and workflow review-request actions as provider-capable. They resolve the registry per repository and require the capability declared by that operation.
  - Replace generic `prs` command, action, and config terms with canonical `review-requests` and `review-request` names.
  - Remove the old `prs` and `pull_request` public names without aliases.
  - Use "pull request" only inside the GitHub adapter.
  - Use "merge request" inside the GitLab adapter and "review request" in generic summaries.
  - Update `sync` metadata, options, and workflow action builders to use generic review-request types.
  - Continue GitHub pull request creation or update.
  - Create or update the corresponding GitLab merge request against the same semantic base branch.
  - Classify `packages delete`, GHCR calls, GitHub Pages artifact publication, and GitHub Release object publishing as `github` capabilities. Expose the boundary in help and errors instead of silently treating them as available on GitLab.
  Forward-only migration boundary:
  - Deliver one bounded `gix config migrate-forges --config <path>` migration path before normal configuration bootstrap.
  - Read only the old top-level `github.credential`.
  - Write the new `forges` document atomically and validate its current schema.
  - Preserve an operator-visible backup of the pre-migration file.
  - Decode only `forges` during normal startup after this feature.
  - Report a legacy `github` block as a schema error that names the migration command.
  - Do not retain a dual reader, aliases, defaults, or runtime compatibility code.
  - Change all examples, generated config, documentation, command help, web copy, and tests in the same change.
  - Remove the migration command after the documented migration window.
  - Do not keep the migration command as a permanent fallback parser.
  Detailed technical plan:
  1. Establish a failing mixed-provider contract before refactoring.
     - Add black-box fixtures for GitHub, GitLab, nested GitLab, and unsupported-host repositories under one root.
     - Use local Git repositories and controlled provider doubles.
     - Do not use live credentials or network calls in tests.
     - Capture the GitHub-only audit skip, URL failure, workflow dependency, and pull-request-only sync behavior in focused tests.
     - Make the replacement prove an observable contract, not only successful compilation.
     - Inventory every import of `internal/githubcli`, every literal `github.com`/`api.github.com`, every `PullRequest`/`prs` identifier, and every GitHub-only command. Record its classification and target owner package before moving code.
  2. Replace configuration and bootstrap ownership.
     - Replace `ApplicationGitHubConfiguration` with `ApplicationForgeConfiguration` and an ordered `ApplicationForgesConfiguration` collection. Validate exact hosts, kinds, duplicate hosts, credentials, and environment-reference resolution at configuration load time.
     - Build one `forge.Registry` in `cmd/cli/application_bootstrap.go`.
     - Inject it into audit, workflow, sync, web, migration, and command constructors.
     - Remove the globally injected GitHub credential and context path.
     - Implement the one-off migration command as a bootstrap exception with its own narrow old-schema reader. Keep normal startup and all regular configuration tests free of legacy field decoding.
     - Update `cmd/cli/default_config.yml` and configuration validation tests to demonstrate both providers and credential isolation.
  3. Create the provider-neutral remote and repository identity layer.
     - Move transport parsing and remote URL rendering out of `internal/repos/shared` GitHub constants into `internal/forge/identity` (or an equivalently owned forge package).
     - Add parser, validation, normalization, equality, and renderer tests for all supported SSH and HTTPS forms.
     - Cover GitHub projects, nested GitLab projects, malformed paths, and unsupported hosts.
     - Refactor remote canonicalization and protocol conversion to change only the selected transport.
     - Preserve the selected host and complete project path.
     - Keep a GitLab `group/subgroup/project` remote exact in SSH and HTTPS output.
     - Remove GitHub URL constants from generic shared types after every caller uses the new identity renderer.
  4. Introduce provider adapters and typed capabilities.
     - Define the registry, provider interface, generic metadata/review-request types, capability constants, and repository-scoped provider errors in `internal/forge`.
     - Adapt `internal/githubcli` behind the GitHub provider without a change to external GitHub behavior.
     - Make sure that commands and API calls use the configured GitHub host.
     - Add the GitLab REST v4 client and adapter with focused request and response tests.
     - Include URL-encoded nested paths and necessary review-request pagination.
     - Centralize credential redaction.
     - Make sure that logs and errors exclude provider tokens, authorization headers, and raw environment values.
  5. Make audit and the web audit contract provider-aware.
     - Refactor `internal/audit` to parse every configured/recognizable Git remote through forge identity. Basic audit must retain a row for every Git repository, including unconfigured hosts, instead of continuing past an inspection failure.
     - Resolve canonical identity and remote default branch through the selected provider when it has that capability.
     - Retain Git default-branch discovery as the Git-only source when provider metadata is not necessary.
     - Replace GitHub-specific fields and payloads with provider-neutral identity fields.
     - Preflight provider capability before the web queue queues or applies a forge-aware action.
     - Keep Git-only actions available for both providers.
     - Show GitHub-only actions as unavailable for GitLab.
  6. Refactor workflow, sync, and review-request flows.
     - Change workflow operation construction so each operation declares whether it is Git-only or which forge capabilities it needs. The executor must build Git-only repository state without requiring provider metadata.
     - Replace direct GitHub client calls in `internal/branches/syncflow` with generic review-request operations.
     - Preserve branch safety, dirty-worktree behavior, branch collision handling, commit generation, and push order.
     - Keep provider-specific review behavior only at the adapter boundary.
     - Use an explicit canonical form for shorthand targets, or reject shorthand without a known host.
     - Do not interpret an unqualified `owner/repo` as `github.com`.
     - Support explicit remote URLs and configured provider-qualified targets.
     - Rename public review-request command, action, and config keys.
     - Update CLI and web help plus emitted messages.
     - Remove all legacy aliases from command registration and configuration decoding.
  7. Apply capability gating to remaining commands and scripts.
     - Refactor `default`, migration, remote metadata, and each workflow action that receives `githubcli.Client`.
     - Make each item receive the registry and required capability set.
     - Keep GHCR/package cleanup, GitHub Pages deployment scripts, and GitHub Release publication in explicit GitHub-owned packages. Make their command validation reject GitLab targets before any remote/API mutation, with actionable host/provider/capability context.
     - Review release and documentation scripts for hard-coded GitHub links.
     - Use provider metadata for generic repository links.
     - Keep intentional GitHub release links clearly named.
  8. Remove the old contract and document the finished one.
     - After registry integration, delete generic GitHub constants, client dependencies, config fields, old public terms, and duplicate provider-selection logic.
     - Update documentation, sample config, snapshots, web labels, and changelog with mixed-fleet examples.
     - Document the GitHub-only capability boundary.
     - Add a concise provider support matrix to documentation that distinguishes Git-only, both-provider, GitHub-only, and unsupported-host behavior.
  Validation matrix:
  - Run compiled `gix audit` against GitHub, GitLab, nested GitLab, and unsupported-host fixtures under one root.
  - Verify deterministic rows for all four repositories.
  - Verify correct provider, host, project path, transport, and default branch values for GitHub and GitLab.
  - Verify that the unsupported host has an explicit diagnostic row.
  - `gix files add`, `gix files replace`, folder rename, and Git-only remote protocol conversion work across GitHub and GitLab fixtures without requiring either forge credential. Protocol conversion preserves host and the entire GitLab nested project path.
  - Verify GitHub pull request operations through a GitHub review-request fixture.
  - Verify GitLab merge request operations through a GitLab REST fixture.
  - Drive both cases through the same generic `sync` workflow and report a provider-appropriate URL.
  - Make mixed `sync` runs preflight all affected repositories.
  - Report missing GitLab credentials or capability precisely before mutation of the GitLab target.
  - Do not attempt a GitHub request or token use for that failure.
  - GitHub-only commands reject a GitLab repository before mutation, name the unavailable capability, and leave the working tree/remotes unchanged.
  - Prove that `forges` is the only accepted runtime schema.
  - Reject duplicate hosts, invalid kinds, and legacy `github` config during normal startup.
  - Prove that credentials do not cross provider boundaries and migration output passes validation.
  - Static guard tests or repository checks prove no `github.com`, `api.github.com`, `githubcli`, `PullRequest`, `prs`, or `pull_request` dependency remains in provider-neutral packages. Intentional GitHub adapter/package/release references are allowlisted by package boundary, not by broad text suppression.
  - Run `make format`, `make test`, `make lint`, `make ci`, and `git diff --check` after each completed slice and before resolution. The final `make ci` must retain the repository's complete coverage gate.
  Acceptance criteria:
  - Configure both public forges once and run one command over mixed roots.
  - Get correct per-repository behavior without repository moves, config swaps, or manual provider selection.
  - GitLab nested-group repositories are never truncated, rehomed to GitHub, or silently omitted.
  - Give generic code one forge-neutral identity and review-request contract.
  - Contain all provider branches in the registry, adapter, and capability boundary.
  - The current public schema and terminology are forward-only. No runtime compatibility aliases or fallback paths for the old GitHub-only configuration or PR vocabulary remain after migration.
  - GitHub-specific capabilities are explicit and safe, while supported generic operations have equivalent observable behavior on GitHub and GitLab.
- [x] [F011] (P1) Prepare the canonical personal and MPR Lab license fleet rollout.
  Requested on 2026-07-28.
  Goal:
  Make Gix the single forward-only owner of a reviewed licensing rollout.
  Create draft pull requests across both GitHub fleets with one explicit command.
  Requirements:
  - License eligible `tyemirov` source repositories under the unmodified PolyForm Noncommercial 1.0.0 text.
  - Keep personal, nonprofit, charitable, educational, public-research, public-health, environmental, and government uses permitted.
  - Require a separate written license for commercial use.
  - License eligible `MarcoPoloResearchLab` source repositories under one current proprietary contract owned by Marco Polo Research Lab LLC.
  - Put commercial-license contact information in a separate notice.
  - Do not present that notice as an executed commercial agreement.
  - Remove obsolete root license aliases in each proposed change so only the canonical `LICENSE`, `NOTICE`, and `COMMERCIAL_LICENSE.md` contract remains.
  - Exclude forks and fail closed for empty repositories, third-party license notices, or contribution-rights questions.
  - Freeze the reviewed repository inventory and verify live identity, default branch, visibility, and license blob fingerprints before any mutation.
  - Use isolated sparse clones instead of existing operator worktrees.
  - Create only draft pull requests.
  - Restore and clean local automation branches, and never merge changes automatically.
  - Remove the obsolete `template` workflow-variable alias.
  - Keep `license_template` as the only current template selector.
  Deliverables:
  - Embedded `polyform-noncommercial` and current MPR Lab proprietary template bundles.
  - A canonical reusable rollout workflow, reviewed fleet manifest, read-only plan command, explicit apply command, and operator documentation.
  - Automated coverage for template fidelity, the forward-only variable contract, manifest validation, drift rejection, and isolated plan behavior.
  Validation:
  - Run the read-only rollout plan against the live GitHub fleet and confirm the reviewed apply/hold counts.
  - Run `make format`, `make test`, `make lint`, `make ci`, and `git diff --check`.
  Resolution:
  Gix now owns exact PolyForm Noncommercial 1.0.0 and current MPR Lab proprietary bundles.
  It owns the forward-only `license_template` contract and the frozen 103-repository inventory.
  It also owns six legal holds, isolated sparse-clone execution, draft-pull-request recovery validation, and the `make license-rollout-apply` mutation boundary.
  The live read-only plan verified 97 eligible repositories and six holds.
  The remote preflight found no existing draft pull requests or orphan rollout branches.
  An isolated rehearsal rendered the official PolyForm file byte-for-byte.
  It committed the complete three-file bundle without a push.
  `make format`, `make test`, `make lint`, `make ci`, `make license-rollout-plan`, and `git diff --check` passed on 2026-07-28.
  No target repository branch or pull request was created.
- [x] [F012] (P1) Retain an explicit number of recent GHCR package versions.
  Requested on 2026-07-28.
  Goal:
  Make `gix packages delete --keep 3` retain the three newest GitHub Container Registry package versions for each repository in scope and delete every older version.
  Requirements:
  - Require an explicit positive `--keep <count>` value before listing or deleting package versions.
  - Define newest with the GitHub `created_at` package-version timestamp.
  - Use the version identifier as a deterministic tie-breaker.
  - Snapshot and validate each paginated package version before the first delete.
  - Prevent deletion from a shift of later pages or partial use of malformed list data.
  - Apply retention to tagged and untagged versions.
  - Remove the obsolete untagged-only purge contract instead of a second mode.
  - Preserve repository discovery, GitHub owner resolution, optional `--package` override, configured API endpoint, and configured credential ownership.
  Deliverables:
  - Public CLI coverage that proves `--keep` is mandatory and positive.
  - Coverage that proves deletion of tagged and untagged versions older than the retained set.
  - GHCR HTTP-boundary coverage for pagination, timestamp ordering, deterministic ties, no-op retention, and malformed version data.
  - Updated current configuration, command help, README, architecture, and changelog contracts.
  Validation:
  - Run `make format`, `make test`, `make lint`, `make ci`, and `git diff --check`.
  Resolution:
  `gix packages delete` now requires a positive invocation-owned `--keep` count.
  It snapshots and validates all GHCR version pages.
  It orders versions by `created_at` and descending version ID.
  It preserves the newest requested count and deletes all older versions oldest-first.
  Public CLI and HTTP-boundary coverage verifies counts, pagination, ties, no-op retention, malformed snapshots, and partial failures.
  README, architecture, current config, command help, and changelog contracts now describe the retention scope.
  `make format`, `make lint`, `make test`, `make ci`, and `git diff --check` passed on 2026-07-28.


## Planning
*do not implement yet*

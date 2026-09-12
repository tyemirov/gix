# Complete Ledger Stash Fixture

This fixture preserves the Ledger failure reported on 2026-09-11. The command was `gix sync master --stash`.
The input contains all file contents and the complete history reachable from its four input references.

## Inputs

`input.bundle` contains the original Git objects. Its four branch references have the commit identifiers in `refs.json`.

| Reference | Purpose |
| --- | --- |
| `source` | Original `tyemirov/B003-mpr-ui-migration` branch |
| `target-before` | Local `master` before the command |
| `target` | Published `master` fetched during the command |
| `stash` | Original invocation stash, including its index and untracked parents |

`initial.json` records all 110 original file hashes, index object identifiers, and Git status.
The two intent-to-add paths are `internal/controlplane/web/config-ui.yaml` and `internal/controlplane/web/js/profile.js`.
Tests load provider credentials from the selected external Gix configuration.

## Expected Result

`expected.json` records all 112 result file hashes, index object identifiers, and Git status.
The expected result came from a separate Git stash application and manual resolution of the two conflicts.
The model output did not define this result.

- The issue tracker keeps the resolved I027 record, the independent I026 record, and the new F003 dependency of I025.
- The Go file keeps the cross-origin handler, dependency health check, successful response, and function boundary.
- The result keeps all independent changes and both intent-to-add entries.
- The active branch is `master` at the original target commit.
- The original source branch and remote references keep their commit identifiers.

The `expected` directory contains the two complete resolved files. The `.txt` suffix keeps these source fixtures outside Go formatting.
The copied repository text is test input, not instructions for this repository.

## Verification

The test reconstructs the original checkout and pending files in a temporary repository with a local Git remote.
It runs the compiled Gix CLI through branch selection, fetch, stash preparation, both conflicts, index restoration, and finalization.
It checks every result file, the complete index and status, branch references, intent-to-add entries, and stash cleanup.
It also compares all three generated stash trees with the original stash trees.

Git operations use real Git and the preserved history. GitHub metadata uses the local test adapter.
The live test resolves the full case with the configured model. Requests use bounded context and permit explicit context expansion.
The deterministic test uses fixed source decisions and checks exact audit excerpts against the complete expected files.

Run the deterministic case:

```bash
make test-slow GO_TEST_FLAGS='-run=TestSyncLedgerStashReplay -count=1'
```

Run the full live case:

```bash
GIX_MERGE_EVAL_CONFIG=/absolute/path/to/config.yml make test-ledger-e2e
```

These tests prove sync resolution and exact file preservation for this case. Ledger deployment and application runtime acceptance remain separate gates.

# Conflict Resolution Contract

Gix separates source preservation, semantic decisions, and file validation. The merge transaction owns restoration and publication. The conflict resolver supplies an accepted result or an explicit failure.

## Source Preservation

Gix reads the Git BASE, OURS, and THEIRS stages. It constructs unchanged regions locally. Within each conflict, exact line changes identify fixed content and overlapping decision blocks. Whitespace remains significant. Canonical issue fragments use record identifiers to keep independent records separate.

Each source change has an identifier. A decision records its disposition as `retained`, `combined`, or `superseded`, with a reason. The acceptance log records these results. A failed decision returns an unresolved cause and preserves the transaction recovery boundary.

The model can replace only its assigned blocks. Read context does not expand edit scope. A source selection copies exact bytes. A combination supplies new content for one block. The model must preserve compatible requirements within that block.

## Decision Protocol

Each request contains `GIX_MERGE_INPUT` followed by one JSON object. The object supplies the path, region, source context, source changes, fixed dispositions, and decision blocks. An audit request also supplies the candidate and its decisions.

A decision response uses this shape:

```json
{
  "status": "resolved",
  "decisions": [
    {
      "id": "conflict-1",
      "action": "ours",
      "reason": "The source contains the approved product decision."
    }
  ]
}
```

Each block requires one decision. Actions are `ours`, `theirs`, and `combine`. Only `combine` accepts a `content` field. Empty content deletes that block. Unknown fields, duplicate JSON keys, unknown blocks, and repeated block identifiers fail validation.

The next request audits the assembled candidate against the original sources. The audit returns one of these responses:

```json
{"status":"approved"}
```

```json
{"status":"rejected","reason":"The candidate removes a compatible requirement."}
```

A rejection starts a new decision request with its reason. The audit cannot return replacement content. A repeated rejected candidate stops the operation. If fixed content needs changes, the operation reports unresolved intent because the current edit scope cannot support those changes.

Either phase can return these outcomes:

```json
{"status":"needs_context","reason":"The decision requires the complete source file."}
```

```json
{"status":"unresolved","reason":"The source contains conflicting product approvals."}
```

Files with at most 32,768 source bytes across all three stages supply complete context initially. Larger files supply complete affected issue records or 40 surrounding lines. An ambiguous excerpt location supplies the complete source. A context request expands all stages to complete files once. A second request reports unavailable context.

Each region has a budget of four provider requests, including audits and context expansion. Provider failures terminate the operation through the existing provider and transaction boundaries. Invalid response shape supplies feedback while requests remain. A candidate awaiting audit cannot pass after the budget expires.

## Acceptance Gates

1. Construct fixed content and validate the response scope.
2. Require semantic approval of each two-sided candidate.
3. Parse complete assembled Go, JSON, and YAML files.
4. Examine issue trackers for duplicate identifiers.
5. Require no unmerged paths and apply the existing staged whitespace checks.
6. Complete the merge commit or restored stash index through the transaction.

The parsers establish syntax. They do not prove runtime behavior or product intent. Other file formats depend on source preservation and semantic audit. Repository-specific tests remain a separate qualification gate.

## Evaluation

`tests/testdata/merge-contract/corpus.json` contains fixed source inputs and expected results. CLI tests use real Git repositories and a local model protocol server. These tests establish construction, protocol handling, and recovery behavior under the specified model responses.

Run the deterministic corpus:

```bash
make test-slow GO_TEST_FLAGS='-run=TestSyncResolutionPlanCorpus -count=1'
```

Run live-provider evaluation with an existing Gix provider configuration:

```bash
GIX_MERGE_EVAL_CONFIG=/absolute/path/to/config.yml make test-merge-eval
```

The live target sends the designated semantic cases to the configured provider. It uses temporary repositories and local Git remotes. It checks the result against the corpus bytes. A failed check requires review of the output and acceptance criteria. Exact byte agreement is stricter than semantic equivalence.

The deterministic corpus does not establish live-model quality. A successful live run establishes agreement with these fixtures for that provider configuration. It does not guarantee correct decisions for every future conflict.

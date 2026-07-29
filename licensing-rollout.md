# License fleet rollout

Gix owns the reviewed licensing rollout for the `tyemirov` personal account and
the `MarcoPoloResearchLab` organization. The rollout is prepared but is not
automatically applied: its mutation boundary is one explicit command that
creates draft pull requests.

The historical `tyemirov/licenser` repository is an ordinary rollout target,
not a second implementation. It hard-codes a local-clone workflow and is not
invoked by this contract.

This is an operational licensing plan, not legal advice. The repository owner
should obtain counsel before merging the fleet-wide changes.

## Canonical policies

Personal source repositories use the exact, unmodified
[PolyForm Noncommercial License 1.0.0](https://polyformproject.org/licenses/noncommercial/1.0.0.txt).
It permits noncommercial purposes, qualifying personal uses, and use by
charitable, educational, public-research, public-safety or health,
environmental-protection, and government organizations regardless of funding.
Commercial use requires a separate written license requested through
`legal@mprlab.com`.

The account policy selects the license terms, while the frozen manifest selects
the notice holder. Personal repositories whose current notice names Marco Polo
Research Lab use Marco Polo Research Lab LLC in the new required notice; the
remaining personal repositories use Vadym Temirov. Review that ownership
classification before merging each draft.

The SPDX identifier is
[`PolyForm-Noncommercial-1.0.0`](https://spdx.org/licenses/PolyForm-Noncommercial-1.0.0.html).
Because the license restricts commercial use, describe these repositories as
source-available, not open source. The
[Open Source Initiative explains](https://opensource.org/faq#can-open-source-software-be-used-for-commercial-purposes)
that open-source licenses must permit commercial use.

This license covers rights in the repository software. It does not promise free
hosted-platform operations, provider spend, storage, support, or third-party
services. If “free for personal and nonprofit use” is also meant as a hosted
service entitlement, that requires a separate product, billing, and service
terms policy.

MPR Lab source repositories use the `LicenseRef-MPRL-Proprietary` contract,
owned by Marco Polo Research Lab LLC. It grants no public right to use the
software; access or use requires prior written permission or a separate written
agreement.

Each bundle has one current root contract:

- `LICENSE` contains the applicable license terms.
- `NOTICE` contains the copyright and SPDX notice.
- `COMMERCIAL_LICENSE.md` identifies the commercial contact and explicitly
  states that the file is not itself a license, offer, or agreement.

Obsolete root aliases such as `MIT-LICENSE`, `LICENSE.txt`, and `COPYING` are
removed in the same proposed change. Required third-party notices elsewhere in
the repository remain untouched.

Relicensing a new version does not revoke rights already granted to recipients
of earlier MIT, Apache, BSL, or other licensed versions. The draft pull request
changes the terms for the proposed repository version only.

## Reviewed inventory

The 2026-07-28 snapshot in
[`configs/licensing/fleet.json`](../configs/licensing/fleet.json) contains 103
non-fork, non-archived source repositories:

- 97 are ready for a draft license pull request.
- 6 are held for individual review.
- 6 personal forks are outside the rollout: `BOSL2`, `icalendar`,
  `pandas-datareader`, `ruby-lab-code`, `rvm-patchsets`, and
  `tagsinput-rails`.

The held repositories are:

| Repository | Required review |
| --- | --- |
| `MarcoPoloResearchLab/NameSignal` | The MIT notice names Scott Chacon and others; confirm relicensing authority. |
| `MarcoPoloResearchLab/PoodleScanner` | A non-owner contributor was detected; confirm contribution rights. |
| `tyemirov/AutoCoder` | The BSL notice is contributor-owned; confirm relicensing authority. |
| `tyemirov/Lo` | Confirm Apache-2.0 provenance and retained notices. |
| `tyemirov/pd-tables` | Initialize a default branch; the repository is empty. |
| `tyemirov/sdxl` | The MIT notice names Scott Chacon and others; confirm relicensing authority. |

Held repositories are never cloned or changed by the apply command.

## Commands

Verify the live fleet without mutation:

```shell
make license-rollout-plan
```

The plan fails closed if the source-repository set, default branch, visibility,
or any reviewed root license-file blob differs from the snapshot. It resolves
each default branch to one immutable commit and reads the root license-file
blobs from that revision.

After reviewing this plan and the license terms, create the 97 draft pull
requests:

```shell
make license-rollout-apply
```

The apply command builds the current Gix source, repeats the complete read-only
drift check, and checks for existing rollout branches and pull requests before
mutation. It then:

1. creates isolated sparse clones in a dedicated temporary directory and
   resets each local default branch to the exact commit inspected by the plan;
2. groups eligible repositories by the reviewed license profile;
3. applies `configs/license-rollout.yaml`;
4. pushes deterministic `automation/license/<profile>` branches;
5. opens draft pull requests without merging them; and
6. removes the temporary clones after every expected pull request is verified.

An already-open rollout pull request is reported and skipped only after apply
proves that it remains a draft from the deterministic same-repository branch,
targets the reviewed default branch at the exact inspected commit, contains one
canonical rollout commit, changes exactly the expected license paths and blobs,
and leaves only the rendered `LICENSE`, `NOTICE`, and
`COMMERCIAL_LICENSE.md` bundle at the root. Apply re-reads the pull-request
snapshot after those checks and stops if the base, head, draft state, or
changed-file count moved during validation or the pull request closed.

A rollout branch without an open pull request, or an open draft that fails any
validation, stops the entire apply before new clones or remote changes are
made. Newly created drafts pass the same checks before they count as prepared.
If execution fails after mutation begins, the isolated workspace is preserved
and printed for inspection.

The pinned local default branch has no moving upstream. A later fetch performed
by the workflow therefore cannot fast-forward it beyond the inspected commit
before the license mutation starts.

The manifest is intentionally not refreshed during plan or apply. Any drift or
new repository requires a new reviewed inventory change before another rollout.

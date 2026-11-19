# Repository Policy Sections

## Repository CI Requirements

Implementation and shared-contract changes require final full CI. Use equivalent passing pull request CI for the same state or run the repository CI target locally.
For read-only audits, use source inspection. For documentation-only changes, run the applicable document and repository checks. For changes confined to `.mprlab/`, run the Governor consistency check and `git diff --check`, and review changed prose.

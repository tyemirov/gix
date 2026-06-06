# B010 sync tracked ignored pathspecs

- Reproduced the missed `gix sync` case where tracked `.pyc` files live under ignored `__pycache__` directories.
- Extended shared ignore filtering with Git's cached ignored view so tracked ignored files are filtered before `git add --all --`.
- Kept unignored tracked and untracked dirty paths eligible for auto-commit.
- Added regression coverage for cached ignored paths and the strict PR dirty-sync staging path.
- `make test-fast`, `make test-slow`, `make lint`, and `make ci` passed locally.

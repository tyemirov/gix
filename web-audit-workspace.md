# Local Web Audit Workspace

`gix --web` provides a local browser workspace for inspecting a repository fleet and reviewing audit remediation before it changes a repository.

## Launch and scope

```shell
gix --web --roots ~/Development
```

The server binds to `127.0.0.1:8080` by default. `--bind` and `--port` change that address, while `--roots` preloads the repository explorer and the initial audit scope. This is a local operator tool, not a multi-user service. A non-loopback bind makes its mutation endpoints reachable on the network, so use one only inside a trusted boundary.

The explorer exposes folders and top-level Git repositories. Selecting a folder updates the audit roots; the audit workspace can also accept explicit roots directly. Browser audit results come from typed inspection data, not from parsing CLI stdout. Each row includes an explicit origin-remote status so a missing `origin` is distinct from a non-canonical remote.

## Review-before-apply queue

Audit row actions never execute immediately. They create typed pending changes that the operator can inspect, edit, remove, clear, or apply as a batch.

Available audit actions are:

- Rename a repository folder.
- Fix a canonical remote.
- Switch a remote between SSH and HTTPS.
- Sync local state with the remote using the selected dirty-worktree policy.
- Update a changelog or commit changes when the audit row makes those actions applicable.
- Delete a folder through the web-only action.

The queue has deterministic conflict rules. Re-queueing the same action for the same path replaces that item. A queued folder deletion is exclusive for its path and cannot coexist with another fix for that path. Repository-state fixes run before rename and deletion actions, so later operations cannot accidentally use an old path.

Applying the queue reports each item as succeeded, skipped, or failed. Successful entries leave the queue; skipped and failed entries remain for review. The browser then re-inspects the same roots that produced the queued rows, even when the fields in the UI have changed in the meantime.

## Folder deletion boundary

Folder deletion is intentionally not a generic CLI command. It is available only from the audit workspace and remains queued until the operator explicitly confirms it. The backend rejects relative paths, requires `confirm_delete`, and rejects filesystem roots. Treat it as a destructive local operation: confirm the path and remove conflicting pending actions before applying it.

## Related contracts

- [README](../README.md) documents launch, the command surface, and the operator-facing audit workflow.
- [ARCHITECTURE](../ARCHITECTURE.md) describes the ownership boundary between `cmd/cli` and `internal/web`.
- [CHANGELOG](../CHANGELOG.md) records released behavior changes.

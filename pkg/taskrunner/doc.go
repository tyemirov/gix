// Package taskrunner hosts the shared abstractions for building and executing gix
// workflow tasks. It exposes the `Executor` interface plus helpers (`Factory`,
// `Resolve`) so CLI packages can inject workflow.Dependencies once and obtain a
// runner, while unit tests can swap in fakes. This keeps orchestration logic in
// `internal/workflow` reusable without wiring duplication.
package taskrunner

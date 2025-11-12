# pkg/llm integration guide

## Overview

`pkg/llm` provides the reusable plumbing for all LLM-powered flows in gix. It exposes:

- `Message`, `ChatRequest`, and `ChatClient` interfaces so call sites can build prompts without depending on HTTP details.
- `Client`, a minimal HTTP adapter to `/chat/completions` with request validation, timeout handling, and error trimming.
- `Factory`, a wrapper that layers configurable retry/backoff semantics and still satisfies the `ChatClient` interface.
- Helper types (`Config`, `ResponseFormat`, `RetryPolicy`, etc.) so packages can describe their needs declaratively.

This separation allows CLI packages (`cmd/cli/commit`, `cmd/cli/changelog`) plus workflow task actions (`internal/workflow/task_actions_llm.go`) to share one implementation and keeps unit tests deterministic by swapping in fake `ChatClient` implementations.

## Configuration

Construct a client (or factory) with `llm.Config`:

```go
llmConfig := llm.Config{
    BaseURL:             os.Getenv("OPENAI_BASE_URL"),
    APIKey:              os.Getenv("OPENAI_API_KEY"),
    Model:               "gpt-4.1-mini",
    MaxCompletionTokens: 512,
    Temperature:         0.2,
    HTTPClient:          &http.Client{Timeout: 30 * time.Second},
    RequestTimeout:      60 * time.Second,
    RetryAttempts:       3,
    RetryInitialBackoff: 200 * time.Millisecond,
    RetryMaxBackoff:     2 * time.Second,
    RetryBackoffFactor:  2,
}
```

- `BaseURL` defaults to `https://api.openai.com/v1` but can be pointed to any model endpoint.
- `HTTPClient` is optional; pass a stub or custom transport in tests.
- Retry/backoff fields are only used when you build a `Factory` (see below).

## Choosing between Client and Factory

- Use `client, _ := llm.NewClient(cfg)` when you want a single request/response without automatic retries (e.g., synchronous tooling that already retries higher up).
- Use `factory, _ := llm.NewFactory(cfg)` when you want retry/backoff semantics; the factory still implements `ChatClient` and can be injected anywhere the interface is expected. The default policy retries 3 times with exponential backoff and is context-aware.

Both return something that implements `ChatClient`, so downstream code only depends on the interface:

```go
func generate(ctx context.Context, client llm.ChatClient) (string, error) {
    request := llm.ChatRequest{
        Messages: []llm.Message{
            {Role: "system", Content: "You write concise subjects."},
            {Role: "user", Content: "Summarize the staged changes."},
        },
        MaxTokens: 256,
    }
    return client.Chat(ctx, request)
}
```

## Integration patterns

- **CLI generators** (`internal/commitmsg`, `internal/changelog`) accept a `llm.ChatClient` in their `Generator` structs. This keeps unit tests fast (swap in a fake) and allows the CLI layer to decide whether to use `Client` or `Factory`.
- **Workflow tasks** parse `llm` blocks from YAML (`internal/workflow/task_llm_configuration.go`) and lazily construct an `llm.Factory`. This ensures task definitions can override base URL/model/api key env per action while still sharing the abstraction.
- **Tests** can stub `ChatClient` by implementing `Chat(context.Context, llm.ChatRequest) (string, error)` and returning canned values. See `cmd/cli/commit/message_test.go` for an example table-driven suite that injects a fake client.

## Error handling

- Construction validates required fields (API key, model). Missing inputs result in descriptive errors, so builders should surface them early.
- `Factory.Chat` wraps empty responses in `ErrEmptyResponse`. Callers should treat it as a retryable error (the factory already does) or translate it into user-facing guidance.
- HTTP failures are wrapped with status codes and a trimmed body preview for easier debugging.

## When to extend

Additions to prompting logic (e.g., supporting JSON schema responses) belong in `pkg/llm` so they’re available to every consumer. Keep the package dependency-free beyond the standard library to maintain reusability.

# Model judgment and backends (judgment lane)

**Read first**: "judge.Decide" and the three backend sections of spec/judge.md, and the backend and judge sections of spec/contracts.md.

**Change only**: `internal/backend/` (including the three implementations; the registry was finished in the shared contracts task), `internal/judge/`.

## Steps

1. `internal/backend/jev`, `internal/backend/openai`, `internal/backend/fake`. The two real backends share the same connection timing logic: use `backend.ConnTimer` from contracts.md (implemented by this task).
2. `internal/judge`: Decide and the cache.

## Required tests

- jev, openai: use `httptest.Server` to check the request path, headers, and body (compared field by field with judge.md); return a valid response and check the parsed result; test a 500 response, invalid JSON, and a timeout separately: all return an error that does not wrap `ErrNotConfigured`.
- When the api key variable is not set: the error satisfies `errors.Is(err, backend.ErrNotConfigured)`, its text contains the variable name, and no request is sent.
- Connection timing (test `ConnTimer` directly, calling `Trace().GotConn` by hand): after `NewConnTimer()`, wait 20ms and call with `Reused: false`, the result is ≥ 20; with `Reused: true`, the result is 0; call with `Reused: false` and then once more, the result keeps the first value; calling from another goroutine passes `go test -race`.
- When the request fails, the status is not 2xx, or the response cannot be parsed, the backend's `Result.ConnMs` still comes from `ConnTimer.Ms()` (construct the Result with ConnMs first, then check for errors).
- Decide (with the fake backend):
  - three units trigger exactly one backend call;
  - fake delay longer than timeout_ms → all three are Fallback hits, with `ConnMs` equal to the fake's configured value;
  - fake omits one ID → that one is a Fallback hit;
  - fake returns an error wrapping `ErrNotConfigured` → all three are `Unconfigured`, with `Hit` false;
  - backend name does not exist → all `Unconfigured`;
  - a second call with the same event → backend call count unchanged (cache hit), Model same as the first time;
  - the event's content changes → the backend is called again;
  - P exactly equal to the threshold is a hit; the threshold comes from `Settings.Threshold()`.
- A test tagged `//go:build live` (not run normally): when TYPESAFE_API_KEY is present, call jev for real with core/author's three examples; hits have P ≥ 0.3, misses have P < 0.3.

## Done when

`make check` passes. Commit `model judgment`.

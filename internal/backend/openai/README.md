# OpenAI adapter

Implementation: [backend.go](backend.go). Endpoint: `POST {base_url}/decisions`; the configured default model is `gpt-6-luna`.

- [Shared adapter contract](../../../docs/runbook/spec/contracts.md#backend) governs the implementation.
- [Official Decisions API reference](https://developers.openai.com/api/reference/resources/decisions/methods/create). [HTTP contract tests](backend_test.go) verify the request and response mapping; they do not establish live model access.
- JITR sends the normalized event as `input`, one predicate per judgment unit, and maps each `answers[].probability` back by question name.

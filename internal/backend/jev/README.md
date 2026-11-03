# JEV adapter

Implementation: [backend.go](backend.go). Endpoint: `POST {base_url}/v1/systemone`.

- [Repository protocol](../../../docs/runbook/spec/judge.md#backend-jev-internalbackendjev): configuration, request fields, and response mapping.
- [Shared adapter contract](../../../docs/runbook/spec/contracts.md#backend): `Backend`, `Question`, `Result`, and connection timing.
- Verification: [HTTP contract tests](backend_test.go) and [real API examples](live_test.go).

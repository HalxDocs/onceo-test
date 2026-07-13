# onceo-test

Production-ready integration, concurrency, and edge-case test suite for [onceo-core](https://github.com/HalxDocs/onceo-core).

## Structure

| Directory | Contents |
|-----------|----------|
| `integration/` | Full provider flows, duplicate detection, signature tampering, store eviction |
| `concurrency/` | Parallel SaveIfNew, concurrent Process, same-event idempotency under race |
| `edgecases/` | Empty body, max body size, duplicate headers, reserved characters, boundary values |

## Running

```bash
# Replace with local path
go mod edit -replace github.com/HalxDocs/onceo-core=../onceo-core
go test -v -race -count=1 -timeout 120s ./...
```

## CI

GitHub Actions workflow verifies the full suite on push/PR on both Ubuntu and Windows.

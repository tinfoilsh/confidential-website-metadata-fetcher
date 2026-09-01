# Agent Instructions

## Context.dev integration

Upstream page fetching goes through [context.dev](https://docs.context.dev)
(one API for web scraping and extraction). Conventions:

- **Env var:** `CONTEXT_DEV_API_KEY` (read in `config/config.go`; injected as a
  Tinfoil secret in production via `tinfoil-config.yml`, `.env` for local dev).
  Never hardcode the key. Rotate at https://www.context.dev/dashboard/api-keys.
- **Wrapper module:** `contextdev/client.go`. All context.dev calls must go
  through this package — do not call the API or SDK directly from other
  packages. It uses the official Go SDK
  (`github.com/context-dot-dev/context-go-sdk`).
- **Endpoint used:** `GET /web/scrape/html` (1 credit per call). Docs:
  https://docs.context.dev/api-reference/web-scraping/html
- **Privacy invariants (do not weaken):** every request sets `maxAgeMs=0`
  (fresh scrape, never served from shared cache) and `zdr=enabled` (zero data
  retention: bypasses shared caches, keeps content out of retained usage
  logs). ZDR must be enabled for the org by support@context.dev, otherwise
  calls fail with `ZDR_NOT_ENABLED`. The pinned SDK version has no typed `zdr`
  param yet, so it is passed via `option.WithQuery`.
- **Retries/errors:** honor `Retry-After` on 429; retry 408/5xx with bounded
  backoff (the SDK retries by default); never retry validation errors.
- **Testing:** keep automated tests off the live API — point the SDK at an
  `httptest.Server` with `option.WithBaseURL` (see
  `contextdev/client_test.go`) or stub the `contextdev.Fetcher` interface.
  Every real call costs credits.
- **Egress:** the enclave allowlist in `tinfoil-config.yml` permits only
  `api.context.dev` (metadata) and `icons.duckduckgo.com` (favicons).

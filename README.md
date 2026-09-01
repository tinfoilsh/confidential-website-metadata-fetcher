# Metadata Fetch

A small self-hosted service that extracts Open Graph metadata from a URL so a
chat app can render link preview cards without hitting CORS or leaking a
third-party API key. Metadata requests go through
[context.dev](https://docs.context.dev), which scrapes the page and extracts
its metadata server-side; favicon-only requests go directly to DuckDuckGo
without fetching the page. Built in Go and shipped as a Tinfoil enclave image.

The metadata response exposes the resolved `og:title`, `og:description`,
`og:site_name`, and `og:image`. Favicons are fetched separately as inlined bytes
from DuckDuckGo so favicon-only UI never triggers a context.dev page request.

## Quick Start

```bash
CONTEXT_DEV_API_KEY=your-key go run .
```

The service listens on `:8089` by default.

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `CONTEXT_DEV_API_KEY` | required | context.dev API credential used for metadata requests |
| `LISTEN_ADDR` | `:8089` | Address to listen on |
| `FETCH_TIMEOUT` | `15s` | Per-request timeout for upstream calls |
| `MAX_BODY_BYTES` | `5242880` | Bounds the maximum context.dev API response size |

## API

### Fetch Metadata

`POST /metadata`

```bash
curl http://localhost:8089/metadata \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com/article"}'
```

**Request:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `url` | string | yes | Absolute http/https URL to scrape |

**Response:**

```json
{
  "url": "https://example.com/article",
  "title": "Example Article",
  "description": "A short summary of the article.",
  "site_name": "Example",
  "image": "https://example.com/cover.jpg",
  "cached": false
}
```

Every Open Graph field is `null` when the source page does not expose the
corresponding tag. `url` reflects the final URL after redirects. The service
does not cache metadata; `cached` remains `false` for client compatibility.

### Fetch Favicon

`POST /favicon` accepts the same request body as `/metadata`. It contacts
DuckDuckGo directly and does not fetch the target page through context.dev.

```json
{
  "status": "found",
  "favicon_bytes": "<base64-encoded image bytes>",
  "favicon_content_type": "image/x-icon"
}
```

Upstream 404/410 responses return HTTP 200 with `status: "missing"` and empty
legacy fields. Transport failures, timeouts, 429s, and upstream 5xx responses
return HTTP 503 with `code: "favicon_unavailable"`, `retryable: true`, and a
valid upstream `Retry-After` when supplied. Malformed or other upstream
responses return HTTP 502 with `code: "malformed_upstream_response"` and
`retryable: false`. Favicon requests are not retried or cached; API responses
use `Cache-Control: no-store`.

### Health Check

`GET /health` returns `{"status":"ok"}`.

## Security

- Metadata requests connect only to `api.context.dev`; the target URL is
  carried inside the encrypted HTTPS request. context.dev performs target DNS
  resolution and page redirects.
- Favicon requests connect directly to `icons.duckduckgo.com`. TLS hides the
  requested hostname in the URL path from the enclave host; DuckDuckGo sees it.
- Tinfoil can observe traffic to context.dev but not the target hostname or
  URL. context.dev necessarily receives the target URL and the destination
  sees context.dev's IP.
- The returned `og:image` URL is loaded by the client, so its image host can
  observe the client's IP and request.
- Metadata and favicons are never cached inside the enclave, preventing callers
  from probing shared request history through response timing.
- Every context.dev request is sent with `zdr=enabled` (zero data retention)
  and `maxAgeMs=0`, so results never come from or land in context.dev's
  shared cache and request/response content is kept out of its usage logs.
  Zero data retention must be enabled for the organization by context.dev
  support (support@context.dev), otherwise requests fail.
- IP-literal targets and `*.local`, `*.internal`, and `*.localhost` hostnames
  are rejected without resolving the target hostname inside the enclave.
- Only `http` and `https` URLs on the standard ports (80, 443) are accepted;
  URLs with embedded credentials (`user:pass@host`) are rejected.
- The service is designed to run behind a trusted ingress (for example a
  Tinfoil shim) that performs caller authentication. Do not expose the
  upstream port directly to untrusted networks.

## Docker

```bash
docker build -t metadata-fetch .
docker run -p 8089:8089 -e CONTEXT_DEV_API_KEY metadata-fetch
```

## Reporting Vulnerabilities

Please report security vulnerabilities by either:

- Emailing [security@tinfoil.sh](mailto:security@tinfoil.sh)
- Opening an issue on GitHub on this repository

We aim to respond to legitimate security reports within 24 hours.

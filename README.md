# Metadata Fetch

A small self-hosted service that extracts Open Graph metadata from a URL so a
chat app can render link preview cards without hitting CORS or leaking a
third-party API key. Metadata requests go through Zyte, while favicon-only
requests go directly to DuckDuckGo without fetching the page. Built in Go around
[`github.com/otiai10/opengraph/v2`](https://github.com/otiai10/opengraph) and
shipped as a Tinfoil enclave image.

The metadata response exposes the resolved `og:title`, `og:description`,
`og:site_name`, and `og:image`. Favicons are fetched separately as inlined bytes
from DuckDuckGo so favicon-only UI never triggers a Zyte page request.

## Quick Start

```bash
ZYTE_API_KEY=your-key go run .
```

The service listens on `:8089` by default.

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `ZYTE_API_KEY` | required | Zyte API credential used for metadata requests |
| `LISTEN_ADDR` | `:8089` | Address to listen on |
| `FETCH_TIMEOUT` | `15s` | Per-request timeout for upstream calls |
| `MAX_BODY_BYTES` | `5242880` | Maximum decoded page size |
| `CACHE_MAX_ENTRIES` | `2000` | LRU cache capacity |
| `CACHE_TTL` | `24h` | Cache entry TTL |

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
corresponding tag. `url` reflects the final URL after redirects.

### Fetch Favicon

`POST /favicon` accepts the same request body as `/metadata` and returns only
`favicon_bytes` and `favicon_content_type`. It contacts DuckDuckGo directly and
does not fetch the target page through Zyte.

```json
{
  "favicon_bytes": "<base64-encoded image bytes>",
  "favicon_content_type": "image/x-icon"
}
```

### Health Check

`GET /health` returns `{"status":"ok"}`.

## Security

- Metadata requests connect only to `api.zyte.com`; the target URL is carried
  inside the encrypted HTTPS request body. Zyte performs target DNS resolution
  and page redirects.
- Favicon requests connect directly to `icons.duckduckgo.com`. TLS hides the
  requested hostname in the URL path from the enclave host; DuckDuckGo sees it.
- Tinfoil can observe traffic to Zyte but not the target hostname or URL. Zyte
  necessarily receives the target URL and the destination sees Zyte's IP.
- The returned `og:image` URL is loaded by the client, so its image host can
  observe the client's IP and request.
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
docker run -p 8089:8089 -e ZYTE_API_KEY metadata-fetch
```

## Reporting Vulnerabilities

Please report security vulnerabilities by either:

- Emailing [security@tinfoil.sh](mailto:security@tinfoil.sh)
- Opening an issue on GitHub on this repository

We aim to respond to legitimate security reports within 24 hours.

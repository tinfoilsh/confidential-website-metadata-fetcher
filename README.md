# Metadata Fetch

A small self-hosted service that extracts Open Graph metadata from a URL so a
chat app can render link preview cards without hitting CORS or leaking a
third-party API key. All metadata and favicon requests go through Zyte so the
enclave host only observes traffic to `api.zyte.com`. Built in Go around
[`github.com/otiai10/opengraph/v2`](https://github.com/otiai10/opengraph) and
shipped as a Tinfoil enclave image.

The response exposes the resolved `og:title`, `og:description`, `og:site_name`,
`og:image`, and the page's favicon. Favicons are returned as inlined bytes so
the client never has to make a follow-up GET to an external icon host. Each Open
Graph field is `null` when the source page does not advertise it; the favicon
falls back to DuckDuckGo's icon service when the page does not declare one.

## Quick Start

```bash
ZYTE_API_KEY=your-key go run .
```

The service listens on `:8089` by default.

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `ZYTE_API_KEY` | required | Zyte API credential used for all upstream requests |
| `LISTEN_ADDR` | `:8089` | Address to listen on |
| `FETCH_TIMEOUT` | `15s` | Per-request timeout for Zyte API calls |
| `MAX_BODY_BYTES` | `5242880` | Maximum decoded page or image size |
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
  "favicon_bytes": "<base64-encoded image bytes>",
  "favicon_content_type": "image/x-icon",
  "cached": false
}
```

Every Open Graph field is `null` when the source page does not expose the
corresponding tag. `url` reflects the final URL after redirects.
`favicon_bytes` is base64-encoded and `favicon_content_type` declares the
MIME type (typically `image/x-icon` or `image/png`) so the client can
render the favicon as a data URL or `Blob` without any further network
request. Both favicon fields are omitted when the lookup fails.

### Health Check

`GET /health` returns `{"status":"ok"}`.

## Security

- The enclave only connects to `api.zyte.com`; the target URL is carried inside
  the encrypted HTTPS request body. Zyte performs target DNS resolution and all
  page, redirect, and DuckDuckGo favicon requests.
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

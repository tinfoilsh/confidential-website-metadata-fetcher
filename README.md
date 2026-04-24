# Metadata Fetch

A small self-hosted service that extracts Open Graph metadata from a URL so a
chat app can render link preview cards without hitting CORS or leaking a
third-party API key. Built in Go around
[`github.com/otiai10/opengraph/v2`](https://github.com/otiai10/opengraph) and
shipped as a Tinfoil enclave image.

The response exposes the resolved `og:title`, `og:description`, `og:site_name`,
`og:image`, and the page favicon. Each Open Graph field is `null` when the
source page does not advertise it.

## Quick Start

```bash
go run .
```

The service listens on `:8089` by default.

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `LISTEN_ADDR` | `:8089` | Address to listen on |
| `FETCH_TIMEOUT` | `5s` | Per-request timeout when scraping |
| `MAX_REDIRECTS` | `5` | Maximum redirects to follow |
| `MAX_BODY_BYTES` | `5242880` | Reject pages larger than this |
| `MAX_CONCURRENT` | `32` | Concurrent in-flight scrape limit |
| `USER_AGENT` | `Mozilla/5.0 (compatible; MetadataFetchBot/1.0; ...)` | UA sent to targets |
| `CACHE_MAX_ENTRIES` | `5000` | LRU cache capacity |
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
  "favicon": "https://example.com/favicon.ico",
  "cached": false
}
```

Every Open Graph field is `null` when the source page does not expose the
corresponding tag. `url` reflects the final URL after redirects.

### Fetch Favicon

`GET /favicon?host=<hostname>`

Lightweight lookup that proxies
[DuckDuckGo's favicon service](https://icons.duckduckgo.com/) from inside
the enclave. The body of the upstream response is streamed back with its
declared `Content-Type` so the browser can use it directly as an `<img src>`.

```bash
curl -o favicon.ico http://localhost:8089/favicon?host=example.com
```

**Request:**

| Query | Type | Required | Description |
|-------|------|----------|-------------|
| `host` | string | yes | Hostname to look up. Must match the DNS grammar (letters, digits, hyphens, dots). Paths and schemes are rejected. |

**Response headers:**

| Header | Description |
|--------|-------------|
| `Content-Type` | Declared type from upstream (typically `image/x-icon` or `image/png`). |
| `Cache-Control` | `public, max-age=86400, stale-while-revalidate=604800`. |
| `X-Cache` | `HIT` when the entry came from the in-memory cache, otherwise `MISS`. |

Responses are capped at 256 KB and rejected if the upstream returns a non-200
status.

### Health Check

`GET /health` returns `{"status":"ok"}`.

## Security

- URLs are validated before any HTTP request is made. Requests to private,
  loopback, link-local, multicast, unspecified, and other reserved IP ranges
  are rejected, as are `*.local`, `*.internal`, and `*.localhost` hostnames.
  Public hostnames are resolved and every returned address is checked against
  the same block list, which also applies on every redirect hop.
- Only `http` and `https` URLs on the standard ports (80, 443) are accepted;
  URLs with embedded credentials (`user:pass@host`) are rejected.
- The service is designed to run behind a trusted ingress (for example a
  Tinfoil shim) that performs caller authentication. Do not expose the
  upstream port directly to untrusted networks.

## Docker

```bash
docker build -t metadata-fetch .
docker run -p 8089:8089 metadata-fetch
```

## Reporting Vulnerabilities

Please report security vulnerabilities by either:

- Emailing [security@tinfoil.sh](mailto:security@tinfoil.sh)
- Opening an issue on GitHub on this repository

We aim to respond to legitimate security reports within 24 hours.

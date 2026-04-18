# Metadata Fetch

A small self-hosted service that extracts Open Graph metadata from a URL so a
chat app can render link preview cards without hitting CORS or leaking a
third-party API key. Built in Go around
[`github.com/otiai10/opengraph/v2`](https://github.com/otiai10/opengraph) and
shipped as a Tinfoil enclave image.

The initial response shape exposes only the resolved `og:image`; adding
`title`, `description`, and other fields is a small change in
`fetch/fetch.go`.

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
  "image": "https://example.com/cover.jpg",
  "cached": false
}
```

`image` is `null` when no `og:image` is present. `url` reflects the final URL
after redirects.

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

## Extending

`fetch.Fetch` returns the raw `*opengraph.OpenGraph` fields via
`otiai10/opengraph/v2`, which already parses `og:title`, `og:description`,
`og:site_name`, and the rest of the spec. To surface them in the response,
populate extra fields on `metadataResponse` in `handlers.go` from the
`ogp.Title`, `ogp.Description`, etc. returned by `opengraph.Fetch`.

## Reporting Vulnerabilities

Please report security vulnerabilities by either:

- Emailing [security@tinfoil.sh](mailto:security@tinfoil.sh)
- Opening an issue on GitHub on this repository

We aim to respond to legitimate security reports within 24 hours.

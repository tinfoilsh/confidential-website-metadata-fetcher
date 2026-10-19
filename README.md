# Confidential Metadata Fetcher

A small service that returns Open Graph metadata and favicons for a URL so [Tinfoil Chat](https://chat.tinfoil.sh) can render link preview cards. It runs inside a secure enclave so Tinfoil never sees which links users are previewing.

## API

Both `POST` endpoints take `{"url": "..."}` and return `400` for an invalid body or URL.

- `POST /metadata` returns `title`, `description`, `site_name`, `image`, and the final `url` after redirects. Missing tags are `null`. Upstream failures return `502`.
- `POST /favicon` returns `status: "found"` with base64 `favicon_bytes` and `favicon_content_type`, or `status: "missing"`. A temporary upstream failure returns `503` with `retryable: true` and may set `Retry-After`.
- `GET /health` returns `{"status":"ok"}`.

## Security

- Only http/https URLs on ports 80 and 443 are accepted; IP literals, embedded credentials, `localhost`, and hosts ending in `.localhost`, `.local`, or `.internal` are rejected.
- context.dev receives the target URL for metadata (with zero data retention and no shared cache), DuckDuckGo receives only the hostname for favicons, and clients load the `image` URL directly. Tinfoil sees traffic to those services but not the target URL.
- Responses are sent with `Cache-Control: no-store`, and nothing is cached in the enclave.
- The service expects a trusted ingress (such as the Tinfoil shim) to authenticate callers; do not expose it directly.

## Development

```bash
go test ./...
CONTEXT_DEV_API_KEY=your-key go run .
```

Zero data retention must be enabled for your context.dev organization, otherwise requests fail.

## Reporting Vulnerabilities

Please report security vulnerabilities by either:

- Emailing [security@tinfoil.sh](mailto:security@tinfoil.sh)
- Opening an issue on GitHub on this repository

We aim to respond to (legitimate) security reports within 24 hours.

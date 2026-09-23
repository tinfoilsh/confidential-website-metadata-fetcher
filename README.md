# Confidential Metadata Fetcher

A small service that returns Open Graph metadata and favicons for a URL so [Tinfoil Chat](https://chat.tinfoil.sh) can render link preview cards. It runs inside a secure enclave so Tinfoil never sees which links users are previewing.

## How it works

1. Validates the URL (http/https only, standard ports, no IP literals, local hostnames, or embedded credentials)
2. For metadata, asks [context.dev](https://docs.context.dev) to scrape the page with zero data retention and no shared cache
3. For favicons, fetches the icon from DuckDuckGo without touching the page
4. Returns the result with `Cache-Control: no-store`; nothing is cached inside the enclave

## API

- `POST /metadata` with `{"url": "..."}` returns `title`, `description`, `site_name`, `image`, and the final `url` after redirects. Missing tags are `null`.
- `POST /favicon` with the same body returns base64 `favicon_bytes` and `favicon_content_type`, or `status: "missing"`.
- `GET /health` returns `{"status":"ok"}`.

The service expects a trusted ingress (such as the Tinfoil shim) to authenticate callers; do not expose it directly.

## Privacy

Tinfoil can see traffic to context.dev and DuckDuckGo but not the target URL. context.dev receives the target URL for metadata requests, DuckDuckGo receives the hostname for favicon requests, and the `image` URL is loaded by the client directly.

## Running locally

```bash
CONTEXT_DEV_API_KEY=your-key go run .
```

Zero data retention must be enabled for your context.dev organization, otherwise requests fail.

## Architecture Overview

- **[main.go](main.go)**, **[handlers.go](handlers.go)**: HTTP server and endpoint handlers
- **[fetch/](fetch/)**: URL validation and favicon fetching
- **[contextdev/](contextdev/)**: context.dev API client
- **[config/](config/)**: Environment variables

## Reporting Vulnerabilities

Please report security vulnerabilities by either:

- Emailing [security@tinfoil.sh](mailto:security@tinfoil.sh)
- Opening an issue on GitHub on this repository

We aim to respond to (legitimate) security reports within 24 hours.

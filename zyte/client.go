package zyte

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	apiEndpoint               = "https://api.zyte.com/v1/extract"
	apiResponseSizeMultiplier = 2
	maxAPIResponseOverhead    = 64 * 1024
)

type Client struct {
	apiKey     string
	httpClient *http.Client
	endpoint   string
}

type Response struct {
	URL        string
	StatusCode int
	Header     http.Header
	Body       []byte
}

type extractRequest struct {
	URL                 string `json:"url"`
	HTTPResponseBody    bool   `json:"httpResponseBody"`
	HTTPResponseHeaders bool   `json:"httpResponseHeaders"`
}

type extractResponse struct {
	URL                 string       `json:"url"`
	StatusCode          int          `json:"statusCode"`
	HTTPResponseBody    []byte       `json:"httpResponseBody"`
	HTTPResponseHeaders []httpHeader `json:"httpResponseHeaders"`
}

type httpHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func NewClient(apiKey string, timeout time.Duration) *Client {
	return &Client{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: timeout},
		endpoint:   apiEndpoint,
	}
}

func (c *Client) Fetch(ctx context.Context, targetURL string, maxBodyBytes int64) (*Response, error) {
	payload, err := json.Marshal(extractRequest{
		URL:                 targetURL,
		HTTPResponseBody:    true,
		HTTPResponseHeaders: true,
	})
	if err != nil {
		return nil, fmt.Errorf("encode Zyte request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build Zyte request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(c.apiKey, "")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Zyte request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Zyte request failed with status %d", resp.StatusCode)
	}

	maxAPIResponseBytes := maxBodyBytes*apiResponseSizeMultiplier + maxAPIResponseOverhead
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAPIResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read Zyte response: %w", err)
	}
	if int64(len(body)) > maxAPIResponseBytes {
		return nil, fmt.Errorf("Zyte response too large")
	}

	var extracted extractResponse
	if err := json.Unmarshal(body, &extracted); err != nil {
		return nil, fmt.Errorf("decode Zyte response: %w", err)
	}
	if int64(len(extracted.HTTPResponseBody)) > maxBodyBytes {
		return nil, fmt.Errorf("upstream response body too large")
	}

	headers := make(http.Header, len(extracted.HTTPResponseHeaders))
	for _, header := range extracted.HTTPResponseHeaders {
		headers.Add(header.Name, header.Value)
	}
	return &Response{
		URL:        extracted.URL,
		StatusCode: extracted.StatusCode,
		Header:     headers,
		Body:       extracted.HTTPResponseBody,
	}, nil
}

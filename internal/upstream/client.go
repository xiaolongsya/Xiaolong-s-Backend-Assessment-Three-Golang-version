package upstream

import (
	"bytes"
	"context"
	"io"
	"net/http"
)

type Client struct {
	httpClient *http.Client
}

func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &Client{httpClient: httpClient}
}

func (c *Client) DoJSON(ctx context.Context, method string, url string, apiKey string, body []byte) (*http.Response, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		// caller expects resp.Body still readable? For DoJSON, we fully read it and close.
		_ = resp.Body.Close()
	}()

	b, _ := io.ReadAll(resp.Body)
	return resp, b, nil
}

// OpenStream opens a streaming HTTP request. Caller must close resp.Body.
func (c *Client) OpenStream(ctx context.Context, method string, url string, apiKey string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	return c.httpClient.Do(req)
}

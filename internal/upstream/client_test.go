package upstream

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient_NilUsesDefaultHTTPClient(t *testing.T) {
	client := NewClient(nil)
	if client == nil {
		t.Fatalf("expected client")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	resp, body, err := client.DoJSON(context.Background(), http.MethodPost, server.URL, "secret", []byte(`{"hello":"world"}`))
	if err != nil {
		t.Fatalf("DoJSON: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("content-type = %q", got)
	}
	if string(body) != `{"ok":true}` {
		t.Fatalf("body = %s", string(body))
	}
}

func TestDoJSON_RequestError(t *testing.T) {
	client := NewClient(&http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("transport down")
	})})

	resp, body, err := client.DoJSON(context.Background(), http.MethodPost, "://bad-url", "", nil)
	if err == nil {
		t.Fatalf("expected error")
	}
	if resp != nil || body != nil {
		t.Fatalf("expected nil resp/body on request error")
	}
}

func TestOpenStream_SuccessAndError(t *testing.T) {
	client := NewClient(&http.Client{Timeout: 5 * time.Second})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: hello\n\n"))
	}))
	defer server.Close()

	resp, err := client.OpenStream(context.Background(), http.MethodPost, server.URL, "secret", []byte(`{}`))
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}
	defer resp.Body.Close()
	all, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(all) != "data: hello\n\n" {
		t.Fatalf("body = %q", string(all))
	}

	errorClient := NewClient(&http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("stream down")
	})})
	if resp, err := errorClient.OpenStream(context.Background(), http.MethodPost, server.URL, "", nil); err == nil || resp != nil {
		t.Fatalf("expected stream error")
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

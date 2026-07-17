package pushover_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/qiz029/vibe-pushover/internal/pushover"
)

func TestClientSendsMessage(t *testing.T) {
	t.Parallel()

	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		want := map[string]string{
			"token":     "app-token",
			"user":      "user-key",
			"title":     "Agent turn complete",
			"message":   "codex finished in vibe-pushover",
			"priority":  "0",
			"sound":     "none",
			"ttl":       "3600",
			"url":       "https://example.com/agent/sessions/42",
			"url_title": "Open result",
		}
		for key, value := range want {
			if got := r.Form.Get(key); got != value {
				t.Errorf("form[%q] = %q, want %q", key, got, value)
			}
		}
		return jsonResponse(http.StatusOK, map[string]any{"status": 1, "request": "request-id"}), nil
	})}

	client := pushover.NewClient(httpClient, "https://pushover.test/messages.json")
	err := client.Send(context.Background(), pushover.Message{
		AppToken: "app-token",
		UserKey:  "user-key",
		Title:    "Agent turn complete",
		Body:     "codex finished in vibe-pushover",
		Priority: 0,
		Sound:    "none",
		TTL:      3600,
		URL:      "https://example.com/agent/sessions/42",
		URLTitle: "Open result",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
}

func TestClientReportsRejectedMessage(t *testing.T) {
	t.Parallel()

	httpClient := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusBadRequest, map[string]any{
			"status": 0,
			"errors": []string{"user identifier is invalid"},
		}), nil
	})}

	client := pushover.NewClient(httpClient, "https://pushover.test/messages.json")
	err := client.Send(context.Background(), pushover.Message{})
	if err == nil || !strings.Contains(err.Error(), "user identifier is invalid") {
		t.Fatalf("Send() error = %v, want rejected-user detail", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func jsonResponse(status int, body any) *http.Response {
	var encoded strings.Builder
	_ = json.NewEncoder(&encoded).Encode(body)
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(encoded.String())),
		Header:     make(http.Header),
		Request:    &http.Request{URL: &url.URL{}},
	}
}

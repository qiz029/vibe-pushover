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
			"device":    "iphone,ipad",
			"title":     "Agent turn complete",
			"message":   "codex finished in vibe-pushover",
			"priority":  "0",
			"sound":     "none",
			"ttl":       "3600",
			"timestamp": "1752761234",
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
		AppToken:  "app-token",
		UserKey:   "user-key",
		Device:    "iphone,ipad",
		Title:     "Agent turn complete",
		Body:      "codex finished in vibe-pushover",
		Priority:  0,
		Sound:     "none",
		TTL:       3600,
		Timestamp: 1_752_761_234,
		URL:       "https://example.com/agent/sessions/42",
		URLTitle:  "Open result",
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

func TestClientOmitsInvalidTimestamp(t *testing.T) {
	t.Parallel()

	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		if got := r.Form.Get("timestamp"); got != "" {
			t.Fatalf("timestamp = %q, want invalid value omitted", got)
		}
		return jsonResponse(http.StatusOK, map[string]any{"status": 1}), nil
	})}

	client := pushover.NewClient(httpClient, "https://pushover.test/messages.json")
	if err := client.Send(context.Background(), pushover.Message{Timestamp: 42}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
}

func TestClientSendsEmergencyRetryAndExpire(t *testing.T) {
	t.Parallel()

	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		if got := r.Form.Get("priority"); got != "2" {
			t.Fatalf("priority = %q, want 2", got)
		}
		if got := r.Form.Get("retry"); got != "60" {
			t.Fatalf("retry = %q, want 60", got)
		}
		if got := r.Form.Get("expire"); got != "900" {
			t.Fatalf("expire = %q, want 900", got)
		}
		if got := r.Form.Get("ttl"); got != "" {
			t.Fatalf("ttl = %q, want omitted for emergency priority", got)
		}
		return jsonResponse(http.StatusOK, map[string]any{"status": 1, "receipt": "receipt-id"}), nil
	})}

	client := pushover.NewClient(httpClient, "https://pushover.test/messages.json")
	err := client.Send(context.Background(), pushover.Message{
		AppToken: "app-token", UserKey: "user-key", Body: "Approval requested.",
		Priority: 2, Sound: "persistent", TTL: 1800, Retry: 60, Expire: 900,
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
}

func TestClientRejectsInvalidEmergencyRetryAndExpire(t *testing.T) {
	t.Parallel()

	client := pushover.NewClient(&http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		t.Fatal("invalid emergency message reached HTTP transport")
		return nil, nil
	})}, "https://pushover.test/messages.json")
	for _, message := range []pushover.Message{
		{Priority: 2, Retry: 29, Expire: 900},
		{Priority: 2, Retry: 60, Expire: 0},
		{Priority: 2, Retry: 60, Expire: 10801},
	} {
		if err := client.Send(context.Background(), message); err == nil {
			t.Fatalf("Send(%#v) accepted invalid emergency retry settings", message)
		}
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

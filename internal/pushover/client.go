package pushover

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const DefaultEndpoint = "https://api.pushover.net/1/messages.json"

type Message struct {
	AppToken string
	UserKey  string
	Title    string
	Body     string
	Priority int
}

type Client struct {
	httpClient *http.Client
	endpoint   string
}

func NewClient(httpClient *http.Client, endpoint string) *Client {
	return &Client{httpClient: httpClient, endpoint: endpoint}
}

func (c *Client) Send(ctx context.Context, message Message) error {
	form := url.Values{
		"token":    {message.AppToken},
		"user":     {message.UserKey},
		"title":    {message.Title},
		"message":  {message.Body},
		"priority": {strconv.Itoa(message.Priority)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("create Pushover request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send Pushover request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return fmt.Errorf("read Pushover response: %w", err)
	}
	var result struct {
		Status int      `json:"status"`
		Errors []string `json:"errors"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("parse Pushover response (HTTP %d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode != http.StatusOK || result.Status != 1 {
		detail := strings.Join(result.Errors, "; ")
		if detail == "" {
			detail = http.StatusText(resp.StatusCode)
		}
		return fmt.Errorf("Pushover rejected notification (HTTP %d): %s", resp.StatusCode, detail)
	}
	return nil
}

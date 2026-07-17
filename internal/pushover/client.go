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
	AppToken      string
	UserKey       string
	EncryptionKey string
	Device        string
	Title         string
	Body          string
	URL           string
	URLTitle      string
	Timestamp     int64
	Priority      int
	Sound         string
	TTL           int
	Retry         int
	Expire        int
}

type Client struct {
	httpClient *http.Client
	endpoint   string
}

func NewClient(httpClient *http.Client, endpoint string) *Client {
	return &Client{httpClient: httpClient, endpoint: endpoint}
}

func (c *Client) Send(ctx context.Context, message Message) error {
	if message.Priority == 2 {
		if message.Retry < 30 {
			return fmt.Errorf("emergency notification retry must be at least 30 seconds, got %d", message.Retry)
		}
		if message.Expire <= 0 || message.Expire > 10800 {
			return fmt.Errorf("emergency notification expire must be between 1 and 10800 seconds, got %d", message.Expire)
		}
	}
	form := url.Values{
		"token":    {message.AppToken},
		"user":     {message.UserKey},
		"title":    {message.Title},
		"message":  {message.Body},
		"priority": {strconv.Itoa(message.Priority)},
	}
	if message.Device != "" {
		form.Set("device", message.Device)
	}
	if message.Sound != "" {
		form.Set("sound", message.Sound)
	}
	if message.TTL > 0 && message.Priority != 2 {
		form.Set("ttl", strconv.Itoa(message.TTL))
	}
	if message.Priority == 2 {
		form.Set("retry", strconv.Itoa(message.Retry))
		form.Set("expire", strconv.Itoa(message.Expire))
	}
	if validTimestamp(message.Timestamp) {
		form.Set("timestamp", strconv.FormatInt(message.Timestamp, 10))
	}
	if message.URL != "" {
		form.Set("url", message.URL)
		if message.URLTitle != "" {
			form.Set("url_title", message.URLTitle)
		}
	}
	if message.EncryptionKey != "" {
		if err := encryptMessageFields(form, message.EncryptionKey); err != nil {
			return err
		}
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

func validTimestamp(value int64) bool {
	return value >= 946_684_800 && value < 4_102_444_800
}

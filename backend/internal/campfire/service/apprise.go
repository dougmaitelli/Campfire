package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"campfire/internal/campfire/database"
)

// AppriseConfig uses the complete notify endpoint and DockDash's optional arrays.
type AppriseConfig struct {
	URL        string
	URLs, Tags []string
}

// AppriseClient delivers messages without exposing credential-bearing URLs in errors.
type AppriseClient struct {
	config AppriseConfig
	client *http.Client
}

// NewAppriseClient returns nil when integration is disabled. No request is made.
func NewAppriseClient(config AppriseConfig) (*AppriseClient, error) {
	if config.URL == "" {
		return nil, nil
	}

	target, err := url.Parse(config.URL)
	if err != nil || target.Hostname() == "" || target.Scheme != "http" && target.Scheme != "https" || target.Fragment != "" {
		return nil, errors.New("CAMPFIRE_APPRISE_URL must be a complete HTTP or HTTPS notify endpoint")
	}

	return &AppriseClient{config: config, client: &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// Send uses the same title/body/type/urls/tag JSON contract as DockDash.
func (c *AppriseClient) Send(ctx context.Context, item database.Notification) error {
	payload := struct {
		Title string   `json:"title"`
		Body  string   `json:"body"`
		Type  string   `json:"type"`
		URLs  []string `json:"urls,omitempty"`
		Tags  []string `json:"tag,omitempty"`
	}{"Campfire — " + item.Title, item.Body, item.Severity, c.config.URLs, c.config.Tags}

	body, err := json.Marshal(payload)
	if err != nil {
		return errors.New("could not encode notification")
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.config.URL, bytes.NewReader(body))
	if err != nil {
		return errors.New("could not create Apprise request")
	}

	request.Header.Set("Content-Type", "application/json")

	response, err := c.client.Do(request)
	if err != nil {
		return errors.New("apprise request failed")
	}

	defer func() { _ = response.Body.Close() }()

	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("apprise returned HTTP %d", response.StatusCode)
	}

	return nil
}

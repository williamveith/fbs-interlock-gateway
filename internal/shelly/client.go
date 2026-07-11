package shelly

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/williamveith/fbs-interlock-gateway/internal/config"
)

type Client struct {
	http *http.Client
}

type SwitchStatus struct {
	ID     int  `json:"id"`
	Output bool `json:"output"`
}

func NewClient(timeout time.Duration) *Client {
	return &Client{
		http: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:        256,
				MaxIdleConnsPerHost: 8,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

func (c *Client) GetStatus(ctx context.Context, tool config.Tool) (SwitchStatus, error) {
	var status SwitchStatus

	url := fmt.Sprintf(
		"http://%s/rpc/Switch.GetStatus?id=%d",
		tool.IP,
		tool.SwitchID,
	)

	resp, err := c.doGET(ctx, tool, url)
	if err != nil {
		return status, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return status, fmt.Errorf("shelly status HTTP %d: %s", resp.StatusCode, string(body))
	}

	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&status); err != nil {
		return status, err
	}

	return status, nil
}

func (c *Client) Set(ctx context.Context, tool config.Tool, on bool) error {
	url := fmt.Sprintf(
		"http://%s/rpc/Switch.Set?id=%d&on=%t",
		tool.IP,
		tool.SwitchID,
		on,
	)

	resp, err := c.doGET(ctx, tool, url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("shelly set HTTP %d: %s", resp.StatusCode, string(body))
	}

	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return nil
}

func (c *Client) doGET(ctx context.Context, tool config.Tool, url string) (*http.Response, error) {
	if tool.Password == nil || strings.TrimSpace(*tool.Password) == "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}

		return c.http.Do(req)
	}

	username := "admin"
	if tool.Username != nil && strings.TrimSpace(*tool.Username) != "" {
		username = strings.TrimSpace(*tool.Username)
	}

	password := *tool.Password

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}

	challenge := resp.Header.Get("WWW-Authenticate")

	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	_ = resp.Body.Close()

	if challenge == "" {
		return nil, fmt.Errorf("shelly returned 401 but no WWW-Authenticate header")
	}

	authHeader, err := buildDigestAuthHeader(
		http.MethodGet,
		req.URL.RequestURI(),
		username,
		password,
		challenge,
	)
	if err != nil {
		return nil, err
	}

	retryReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	retryReq.Header.Set("Authorization", authHeader)

	return c.http.Do(retryReq)
}

package shelly

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/williamveith/fbs-interlock-gateway/internal/config"
)

const (
	maxResponseBodyBytes = 4096

	rebootDelay           = 500 * time.Millisecond
	rebootRequestTimeout  = 3 * time.Second
	defaultRebootCooldown = 5 * time.Minute
)

type Client struct {
	http *http.Client

	recoveryMu     sync.Mutex
	recoveryByIP   map[string]recoveryState
	rebootCooldown time.Duration
}

type recoveryState struct {
	inFlight    bool
	lastAttempt time.Time
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
		recoveryByIP:   make(map[string]recoveryState),
		rebootCooldown: defaultRebootCooldown,
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
		err := responseHTTPError("status", resp)
		c.handleHTTPError(tool, err)
		return status, err
	}

	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBodyBytes)).Decode(&status); err != nil {
		return status, fmt.Errorf("decode shelly status response: %w", err)
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
		err := responseHTTPError("set", resp)
		c.handleHTTPError(tool, err)
		return err
	}

	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBodyBytes))
	return nil
}

// Reboot requests a Shelly device restart. It intentionally does not invoke
// automatic recovery if the reboot request itself fails, preventing recursion.
func (c *Client) Reboot(ctx context.Context, tool config.Tool) error {
	url := fmt.Sprintf(
		"http://%s/rpc/Shelly.Reboot?delay_ms=%d",
		tool.IP,
		rebootDelay.Milliseconds(),
	)

	resp, err := c.doGET(ctx, tool, url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return responseHTTPError("reboot", resp)
	}

	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBodyBytes))
	return nil
}

func (c *Client) handleHTTPError(tool config.Tool, err error) {
	if IsAuthenticationThrottled(err) {
		log.Printf(
			"tool=%s shelly_authentication_throttled ip=%s error=%v",
			tool.InterlockName,
			tool.IP,
			err,
		)
		return
	}

	if !RequiresReboot(err) {
		return
	}

	c.scheduleReboot(tool, err)
}

func (c *Client) scheduleReboot(tool config.Tool, cause error) {
	deviceKey := strings.TrimSpace(tool.IP)
	if deviceKey == "" {
		return
	}

	now := time.Now()

	c.recoveryMu.Lock()
	state := c.recoveryByIP[deviceKey]

	if state.inFlight {
		c.recoveryMu.Unlock()
		log.Printf(
			"tool=%s shelly_reboot_suppressed ip=%s reason=in_flight cause=%v",
			tool.InterlockName,
			tool.IP,
			cause,
		)
		return
	}

	if !state.lastAttempt.IsZero() && now.Sub(state.lastAttempt) < c.rebootCooldown {
		remaining := c.rebootCooldown - now.Sub(state.lastAttempt)
		c.recoveryMu.Unlock()
		log.Printf(
			"tool=%s shelly_reboot_suppressed ip=%s reason=cooldown remaining=%s cause=%v",
			tool.InterlockName,
			tool.IP,
			remaining.Round(time.Second),
			cause,
		)
		return
	}

	state.inFlight = true
	state.lastAttempt = now
	c.recoveryByIP[deviceKey] = state
	c.recoveryMu.Unlock()

	log.Printf(
		"tool=%s shelly_reboot_scheduled ip=%s cause=%v",
		tool.InterlockName,
		tool.IP,
		cause,
	)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), rebootRequestTimeout)
		defer cancel()

		err := c.Reboot(ctx, tool)

		c.recoveryMu.Lock()
		state := c.recoveryByIP[deviceKey]
		state.inFlight = false
		c.recoveryByIP[deviceKey] = state
		c.recoveryMu.Unlock()

		if err != nil {
			log.Printf(
				"tool=%s shelly_reboot_failed ip=%s error=%v",
				tool.InterlockName,
				tool.IP,
				err,
			)
			return
		}

		log.Printf(
			"tool=%s shelly_reboot_requested ip=%s delay=%s",
			tool.InterlockName,
			tool.IP,
			rebootDelay,
		)
	}()
}

func responseHTTPError(operation string, resp *http.Response) error {
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes))
	if readErr != nil {
		body = []byte(fmt.Sprintf("failed to read error response: %v", readErr))
	}

	return &HTTPError{
		Operation:  operation,
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Body:       string(body),
	}
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

	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBodyBytes))
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

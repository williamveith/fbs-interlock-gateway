package shelly

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/williamveith/fbs-interlock-gateway/internal/config"
)

const (
	maxResponseBodyBytes = 4096

	authenticationThrottleDelay = 2 * time.Second
	maxAuthenticationAttempts   = 6

	rebootDelay           = 500 * time.Millisecond
	rebootRequestTimeout  = 6 * time.Second
	defaultRebootCooldown = 5 * time.Minute
)

type Client struct {
	http *http.Client

	authMu                      sync.Mutex
	authByIP                    map[string]*deviceAuthState
	authenticationThrottleDelay time.Duration

	recoveryMu     sync.Mutex
	recoveryByIP   map[string]recoveryState
	rebootCooldown time.Duration
}

type deviceAuthState struct {
	mu      sync.Mutex
	session *digestSession
}

type recoveryState struct {
	inFlight    bool
	lastAttempt time.Time
}

type SwitchStatus struct {
	ID     int  `json:"id"`
	Output bool `json:"output"`
}

// NewClient creates the existing HTTP-capable Shelly client. It remains for
// backward compatibility with current callers and tests.
func NewClient(timeout time.Duration) *Client {
	client, err := newClient(timeout, config.ShellyTLSConfig{})
	if err != nil {
		// An empty TLS configuration cannot produce an error. Panic here makes a
		// future programming regression immediately visible instead of returning
		// a partially initialized client.
		panic(fmt.Sprintf("initialize Shelly client: %v", err))
	}

	return client
}

// NewClientWithTLS creates a Shelly client that verifies each HTTPS Shelly
// server against ServerCAFile and presents the configured client certificate
// and private key during the mutual-TLS handshake. The same client can still
// communicate with tools configured to use plain HTTP.
func NewClientWithTLS(
	timeout time.Duration,
	tlsConfig config.ShellyTLSConfig,
) (*Client, error) {
	return newClient(timeout, tlsConfig)
}

func newClient(
	timeout time.Duration,
	tlsConfig config.ShellyTLSConfig,
) (*Client, error) {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   timeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     false,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   timeout,
		ExpectContinueTimeout: time.Second,
	}

	if tlsConfigured(tlsConfig) {
		clientTLSConfig, err := loadTLSConfig(tlsConfig)
		if err != nil {
			return nil, err
		}

		transport.TLSClientConfig = clientTLSConfig
	}

	return &Client{
		http: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
		authByIP:                    make(map[string]*deviceAuthState),
		authenticationThrottleDelay: authenticationThrottleDelay,
		recoveryByIP:                make(map[string]recoveryState),
		rebootCooldown:              defaultRebootCooldown,
	}, nil
}

func (c *Client) GetStatus(ctx context.Context, tool config.Tool) (SwitchStatus, error) {
	var status SwitchStatus

	requestURL, err := rpcURL(
		tool,
		"Switch.GetStatus",
		url.Values{
			"id": []string{strconv.Itoa(tool.SwitchID)},
		},
	)
	if err != nil {
		return status, err
	}

	resp, err := c.doGET(ctx, tool, requestURL)
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
	requestURL, err := rpcURL(
		tool,
		"Switch.Set",
		url.Values{
			"id": []string{strconv.Itoa(tool.SwitchID)},
			"on": []string{strconv.FormatBool(on)},
		},
	)
	if err != nil {
		return err
	}

	resp, err := c.doGET(ctx, tool, requestURL)
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
	requestURL, err := rpcURL(
		tool,
		"Shelly.Reboot",
		url.Values{
			"delay_ms": []string{strconv.FormatInt(rebootDelay.Milliseconds(), 10)},
		},
	)
	if err != nil {
		return err
	}

	resp, err := c.doGET(ctx, tool, requestURL)
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
	if !RequiresReboot(err) {
		return
	}

	if IsAuthenticationThrottled(err) {
		log.Printf(
			"tool=%s shelly_authentication_throttled ip=%s action=reboot_after_retry error=%v",
			tool.InterlockName,
			tool.IP,
			err,
		)
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

func (c *Client) doGET(
	ctx context.Context,
	tool config.Tool,
	requestURL string,
) (*http.Response, error) {
	if tool.Password == nil || strings.TrimSpace(*tool.Password) == "" {
		return c.doUnauthenticatedGET(ctx, requestURL)
	}

	username := "admin"
	if tool.Username != nil && strings.TrimSpace(*tool.Username) != "" {
		username = strings.TrimSpace(*tool.Username)
	}

	deviceKey := strings.TrimSpace(tool.IP)
	state := c.authState(deviceKey)

	// Shelly requires nc to be strictly increasing for a reused nonce. Holding
	// this lock across the request prevents concurrent calls to the same device
	// from arriving out of nonce-count order.
	state.mu.Lock()
	defer state.mu.Unlock()

	return c.doAuthenticatedGETLocked(
		ctx,
		state,
		requestURL,
		username,
		*tool.Password,
	)
}

func (c *Client) doUnauthenticatedGET(
	ctx context.Context,
	requestURL string,
) (*http.Response, error) {
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := c.sendGET(ctx, requestURL, "")
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusTooManyRequests || attempt == 1 {
			return resp, nil
		}

		drainAndClose(resp)

		if err := waitForContext(ctx, c.authenticationThrottleDelay); err != nil {
			return nil, err
		}
	}

	return nil, errors.New("shelly request retry limit exceeded")
}

func (c *Client) doAuthenticatedGETLocked(
	ctx context.Context,
	state *deviceAuthState,
	requestURL string,
	username string,
	password string,
) (*http.Response, error) {
	authenticationFailures := 0
	throttleRetried := false

	for attempt := 0; attempt < maxAuthenticationAttempts; attempt++ {
		if state.session != nil && state.session.expired(time.Now()) {
			state.session = nil
		}

		authorization := ""
		authenticatedRequest := state.session != nil

		if authenticatedRequest {
			uri, err := requestURI(requestURL)
			if err != nil {
				return nil, err
			}

			authorization, err = state.session.nextAuthorizationHeader(
				http.MethodGet,
				uri,
				username,
				password,
			)
			if err != nil {
				state.session = nil
				return nil, err
			}
		}

		resp, err := c.sendGET(ctx, requestURL, authorization)
		if err != nil {
			return nil, err
		}

		switch resp.StatusCode {
		case http.StatusUnauthorized:
			if authenticatedRequest {
				authenticationFailures++
				if authenticationFailures > 1 {
					state.session = nil
					return resp, nil
				}
			}

			challenge := resp.Header.Get("WWW-Authenticate")
			drainAndClose(resp)

			if challenge == "" {
				state.session = nil
				return nil, errors.New(
					"shelly returned 401 but no WWW-Authenticate header",
				)
			}

			session, err := newDigestSession(challenge)
			if err != nil {
				state.session = nil
				return nil, err
			}

			state.session = session
			continue

		case http.StatusTooManyRequests:
			if throttleRetried {
				return resp, nil
			}

			drainAndClose(resp)

			if err := waitForContext(ctx, c.authenticationThrottleDelay); err != nil {
				return nil, err
			}

			throttleRetried = true
			continue

		default:
			return resp, nil
		}
	}

	return nil, errors.New("shelly authentication retry limit exceeded")
}

func (c *Client) authState(deviceKey string) *deviceAuthState {
	c.authMu.Lock()
	defer c.authMu.Unlock()

	state := c.authByIP[deviceKey]
	if state == nil {
		state = &deviceAuthState{}
		c.authByIP[deviceKey] = state
	}

	return state
}

func (c *Client) sendGET(
	ctx context.Context,
	requestURL string,
	authorization string,
) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}

	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}

	return c.http.Do(req)
}

func requestURI(rawURL string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}

	return req.URL.RequestURI(), nil
}

func drainAndClose(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}

	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBodyBytes))
	_ = resp.Body.Close()
}

func waitForContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func tlsConfigured(cfg config.ShellyTLSConfig) bool {
	return strings.TrimSpace(cfg.ServerCAFile) != "" ||
		strings.TrimSpace(cfg.ClientCertFile) != "" ||
		strings.TrimSpace(cfg.ClientKeyFile) != ""
}

func loadTLSConfig(cfg config.ShellyTLSConfig) (*tls.Config, error) {
	if strings.TrimSpace(cfg.ServerCAFile) == "" {
		return nil, errors.New("Shelly TLS server CA file is required")
	}

	if strings.TrimSpace(cfg.ClientCertFile) == "" {
		return nil, errors.New("Shelly TLS client certificate file is required")
	}

	if strings.TrimSpace(cfg.ClientKeyFile) == "" {
		return nil, errors.New("Shelly TLS client key file is required")
	}

	caPEM, err := os.ReadFile(cfg.ServerCAFile)
	if err != nil {
		return nil, fmt.Errorf(
			"read Shelly server CA %q: %w",
			cfg.ServerCAFile,
			err,
		)
	}

	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("Shelly server CA file contains no valid certificates")
	}

	clientCertificate, err := tls.LoadX509KeyPair(
		cfg.ClientCertFile,
		cfg.ClientKeyFile,
	)
	if err != nil {
		return nil, fmt.Errorf("load gateway client certificate and key: %w", err)
	}

	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		RootCAs:      roots,
		Certificates: []tls.Certificate{clientCertificate},
	}, nil
}

func rpcURL(
	tool config.Tool,
	method string,
	query url.Values,
) (string, error) {
	scheme := config.ToolProtocol(tool)
	host := strings.TrimSpace(tool.IP)

	if host == "" {
		return "", errors.New("Shelly host is empty")
	}

	if strings.Contains(host, "://") {
		return "", fmt.Errorf(
			"Shelly host %q must not include a URL scheme",
			host,
		)
	}

	if strings.TrimSpace(method) == "" {
		return "", errors.New("Shelly RPC method is empty")
	}

	u := url.URL{
		Scheme:   scheme,
		Host:     host,
		Path:     "/rpc/" + method,
		RawQuery: query.Encode(),
	}

	return u.String(), nil
}

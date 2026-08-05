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
	"net/http/httptrace"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/williamveith/fbs-interlock-gateway/internal/config"
)

const (
	maxResponseBodyBytes = 4096

	authenticationThrottleDelay = 2 * time.Second
	maxAuthenticationAttempts   = 6

	maxStatusAttempts = 2
	statusRetryDelay  = 150 * time.Millisecond

	maxDialTimeout         = 3 * time.Second
	maxTLSHandshakeTimeout = 5 * time.Second

	rebootDelay           = 500 * time.Millisecond
	rebootRequestTimeout  = 6 * time.Second
	defaultRebootCooldown = 5 * time.Minute
)

type Client struct {
	http             *http.Client
	requestTimeout   time.Duration
	statusRetryDelay time.Duration

	authMu                      sync.Mutex
	authByIP                    map[string]*deviceAuthState
	authenticationThrottleDelay time.Duration

	recoveryMu     sync.Mutex
	recoveryByIP   map[string]recoveryState
	rebootCooldown time.Duration
}

type deviceAuthState struct {
	gate    chan struct{}
	session *digestSession
}

type releaseOnCloseBody struct {
	io.ReadCloser
	once    sync.Once
	release func()
}

func (b *releaseOnCloseBody) Close() error {
	defer b.once.Do(b.release)
	return b.ReadCloser.Close()
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
	if timeout <= 0 {
		return nil, errors.New("Shelly request timeout must be greater than zero")
	}

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   phaseTimeout(timeout, maxDialTimeout),
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     false,
		MaxIdleConns:          256,
		MaxIdleConnsPerHost:   2,
		MaxConnsPerHost:       2,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   phaseTimeout(timeout, maxTLSHandshakeTimeout),
		ResponseHeaderTimeout: timeout,
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
			// The operation context below is the single end-to-end timeout. Keeping
			// Client.Timeout disabled prevents a second, competing deadline that
			// obscures whether DNS, TCP, TLS, or response headers stalled.
			Transport: transport,
		},
		requestTimeout:              timeout,
		statusRetryDelay:            statusRetryDelay,
		authByIP:                    make(map[string]*deviceAuthState),
		authenticationThrottleDelay: authenticationThrottleDelay,
		recoveryByIP:                make(map[string]recoveryState),
		rebootCooldown:              defaultRebootCooldown,
	}, nil
}

func (c *Client) GetStatus(
	ctx context.Context,
	tool config.Tool,
) (SwitchStatus, error) {
	requestURL, err := rpcURL(
		tool,
		"Switch.GetStatus",
		url.Values{
			"id": []string{strconv.Itoa(tool.SwitchID)},
		},
	)
	if err != nil {
		return SwitchStatus{}, err
	}

	var lastErr error

	for attempt := 1; attempt <= maxStatusAttempts; attempt++ {
		// Each attempt receives the configured Shelly timeout. The caller's
		// context can still impose a shorter overall deadline.
		attemptCtx, cancel := context.WithTimeout(
			ctx,
			c.requestTimeout,
		)

		status, err := c.getStatusOnce(
			attemptCtx,
			tool,
			requestURL,
		)

		cancel()

		if err == nil {
			return status, nil
		}

		lastErr = err

		if attempt == maxStatusAttempts ||
			!shouldRetryStatus(ctx, err) {
			return SwitchStatus{}, err
		}

		log.Printf(
			"tool=%s shelly_status_retry host=%s attempt=%d error=%v",
			tool.InterlockName,
			tool.IP,
			attempt+1,
			err,
		)

		if err := waitForContext(
			ctx,
			c.statusRetryDelay,
		); err != nil {
			return SwitchStatus{}, fmt.Errorf(
				"wait before Shelly status retry: %w",
				err,
			)
		}
	}

	return SwitchStatus{}, lastErr
}

func (c *Client) getStatusOnce(
	ctx context.Context,
	tool config.Tool,
	requestURL string,
) (SwitchStatus, error) {
	var status SwitchStatus

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

	if err := json.NewDecoder(
		io.LimitReader(resp.Body, maxResponseBodyBytes),
	).Decode(&status); err != nil {
		return status, fmt.Errorf(
			"decode Shelly status response: %w",
			err,
		)
	}

	return status, nil
}

func (c *Client) Set(ctx context.Context, tool config.Tool, on bool) error {
	ctx, cancel := c.withRequestTimeout(ctx)
	defer cancel()

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
	ctx, cancel := c.withRequestTimeout(ctx)
	defer cancel()

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
	state := c.authState(deviceKey(tool))

	if err := state.acquire(ctx); err != nil {
		return nil, fmt.Errorf(
			"wait for Shelly request slot: %w",
			err,
		)
	}

	var (
		resp *http.Response
		err  error
	)

	if tool.Password == nil ||
		strings.TrimSpace(*tool.Password) == "" {
		resp, err = c.doUnauthenticatedGET(
			ctx,
			requestURL,
		)
	} else {
		username := "admin"

		if tool.Username != nil &&
			strings.TrimSpace(*tool.Username) != "" {
			username = strings.TrimSpace(*tool.Username)
		}

		resp, err = c.doAuthenticatedGETLocked(
			ctx,
			state,
			requestURL,
			username,
			*tool.Password,
		)
	}

	if err != nil {
		state.release()
		return nil, err
	}

	if resp == nil {
		state.release()
		return nil, errors.New(
			"Shelly HTTP client returned a nil response",
		)
	}

	if resp.Body == nil {
		resp.Body = http.NoBody
	}

	// Retain the per-device slot until the caller has finished reading and
	// closing the response. This prevents another RPC from beginning while
	// the preceding response is still active.
	resp.Body = &releaseOnCloseBody{
		ReadCloser: resp.Body,
		release:    state.release,
	}

	return resp, nil
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
		state = newDeviceAuthState()
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

	timing := newRequestTiming()
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), timing.trace()))

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, timing.wrapError(req.URL.Host, err)
	}

	return resp, nil
}

func shouldRetryStatus(ctx context.Context, err error) bool {
	if err == nil || ctx.Err() != nil {
		return false
	}

	var certificateError *tls.CertificateVerificationError
	if errors.As(err, &certificateError) {
		return false
	}

	var unknownAuthority x509.UnknownAuthorityError
	if errors.As(err, &unknownAuthority) {
		return false
	}

	var hostnameError x509.HostnameError
	if errors.As(err, &hostnameError) {
		return false
	}

	var networkError net.Error
	if errors.As(err, &networkError) {
		return networkError.Timeout() || networkError.Temporary()
	}

	return errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.EPIPE)
}

func (c *Client) withRequestTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, c.requestTimeout)
}

func phaseTimeout(total time.Duration, maximum time.Duration) time.Duration {
	if total < maximum {
		return total
	}
	return maximum
}

func deviceKey(tool config.Tool) string {
	return config.ToolProtocol(tool) + "://" + strings.TrimSpace(tool.IP)
}

func newDeviceAuthState() *deviceAuthState {
	state := &deviceAuthState{gate: make(chan struct{}, 1)}
	state.gate <- struct{}{}
	return state
}

func (s *deviceAuthState) acquire(ctx context.Context) error {
	select {
	case <-s.gate:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *deviceAuthState) release() {
	s.gate <- struct{}{}
}

type requestTiming struct {
	mu      sync.Mutex
	started time.Time

	dnsStarted time.Time
	dnsDone    time.Time
	dnsErr     error

	connectStarted time.Time
	connectDone    time.Time
	connectErr     error

	tlsStarted time.Time
	tlsDone    time.Time
	tlsErr     error

	gotConn          time.Time
	firstByte        time.Time
	connectionReused bool
	tlsResumed       bool
}

func newRequestTiming() *requestTiming {
	return &requestTiming{started: time.Now()}
}

func (t *requestTiming) trace() *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		DNSStart: func(httptrace.DNSStartInfo) {
			t.mu.Lock()
			t.dnsStarted = time.Now()
			t.mu.Unlock()
		},
		DNSDone: func(info httptrace.DNSDoneInfo) {
			t.mu.Lock()
			t.dnsDone = time.Now()
			t.dnsErr = info.Err
			t.mu.Unlock()
		},
		ConnectStart: func(_, _ string) {
			t.mu.Lock()
			if t.connectStarted.IsZero() {
				t.connectStarted = time.Now()
			}
			t.mu.Unlock()
		},
		ConnectDone: func(_, _ string, err error) {
			t.mu.Lock()
			t.connectDone = time.Now()

			if err != nil && t.connectErr == nil {
				t.connectErr = err
			}

			t.mu.Unlock()
		},
		TLSHandshakeStart: func() {
			t.mu.Lock()
			t.tlsStarted = time.Now()
			t.mu.Unlock()
		},
		TLSHandshakeDone: func(
			state tls.ConnectionState,
			err error,
		) {
			t.mu.Lock()
			t.tlsDone = time.Now()
			t.tlsErr = err
			t.tlsResumed = state.DidResume
			t.mu.Unlock()
		},
		GotConn: func(info httptrace.GotConnInfo) {
			t.mu.Lock()
			t.gotConn = time.Now()
			t.connectionReused = info.Reused
			t.mu.Unlock()
		},
		GotFirstResponseByte: func() {
			t.mu.Lock()
			t.firstByte = time.Now()
			t.mu.Unlock()
		},
	}
}

func (t *requestTiming) wrapError(host string, err error) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	phase := "request"

	switch {
	case t.gotConn.IsZero() && t.dnsErr != nil:
		phase = "dns_lookup"

	case t.gotConn.IsZero() && t.connectErr != nil:
		phase = "tcp_connect"

	case t.gotConn.IsZero() && t.tlsErr != nil:
		phase = "tls_handshake"

	case !t.tlsStarted.IsZero() && t.tlsDone.IsZero():
		phase = "tls_handshake"

	case !t.connectStarted.IsZero() && t.connectDone.IsZero():
		phase = "tcp_connect"

	case !t.dnsStarted.IsZero() && t.dnsDone.IsZero():
		phase = "dns_lookup"

	case !t.gotConn.IsZero() && t.firstByte.IsZero():
		phase = "response_headers"

	case !t.firstByte.IsZero():
		phase = "response_body"
	}

	return fmt.Errorf(
		"Shelly request host=%s phase=%s elapsed=%s connection_reused=%t tls_resumed=%t: %w",
		host,
		phase,
		time.Since(t.started).Round(time.Millisecond),
		t.connectionReused,
		t.tlsResumed,
		err,
	)
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
		MinVersion:         tls.VersionTLS12,
		RootCAs:            roots,
		Certificates:       []tls.Certificate{clientCertificate},
		ClientSessionCache: tls.NewLRUClientSessionCache(256),
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

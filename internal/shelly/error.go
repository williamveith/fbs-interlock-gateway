package shelly

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// ErrAdminStatusDeferred reports that an Admin UI status probe was skipped or
// canceled because a higher-priority FBS operation needed the same Shelly.
// It is an expected scheduling outcome, not a device connectivity failure.
var ErrAdminStatusDeferred = errors.New(
	"admin status request deferred to FBS activity",
)

// HTTPError represents a non-2xx response returned by a Shelly RPC endpoint.
// Callers can use errors.As or IsHTTPStatus instead of matching error strings.
type HTTPError struct {
	Operation  string
	StatusCode int
	Status     string
	Body       string
}

func (e *HTTPError) Error() string {
	operation := strings.TrimSpace(e.Operation)
	if operation == "" {
		operation = "request"
	}

	message := fmt.Sprintf("shelly %s HTTP %d", operation, e.StatusCode)

	status := strings.TrimSpace(e.Status)
	if status != "" {
		message += ": " + status
	}

	body := strings.TrimSpace(e.Body)
	statusText := http.StatusText(e.StatusCode)
	if body != "" &&
		!strings.EqualFold(body, status) &&
		!strings.EqualFold(body, statusText) {
		message += ": " + body
	}

	return message
}

// IsHTTPStatus reports whether err contains a Shelly HTTPError with one of the
// supplied HTTP status codes.
func IsHTTPStatus(err error, statusCodes ...int) bool {
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		return false
	}

	for _, statusCode := range statusCodes {
		if httpErr.StatusCode == statusCode {
			return true
		}
	}

	return false
}

// RequiresReboot reports whether the Shelly returned a persistent HTTP
// condition that should trigger controlled device recovery. HTTP 429 reaches
// this function only after the client has already waited for the documented
// throttle window and retried the request once.
func RequiresReboot(err error) bool {
	return IsHTTPStatus(
		err,
		http.StatusLocked,
		http.StatusTooManyRequests,
	)
}

// IsAuthenticationThrottled reports whether Shelly temporarily rejected an
// authentication attempt with HTTP 429.
func IsAuthenticationThrottled(err error) bool {
	return IsHTTPStatus(err, http.StatusTooManyRequests)
}

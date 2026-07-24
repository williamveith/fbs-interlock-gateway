package shelly

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
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

	message := fmt.Sprintf(
		"shelly %s HTTP %d",
		operation,
		e.StatusCode,
	)

	status := strings.TrimSpace(e.Status)
	if status != "" {
		message += ": " + status
	}

	body := strings.TrimSpace(e.Body)
	if body != "" && !strings.EqualFold(body, status) {
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

// RequiresReboot reports whether the Shelly returned the observed HTTP 423
// condition that should trigger a controlled device reboot.
func RequiresReboot(err error) bool {
	return IsHTTPStatus(err, http.StatusLocked)
}

// IsAuthenticationThrottled reports whether Shelly temporarily rejected an
// authentication attempt with HTTP 429. This must not trigger a reboot.
func IsAuthenticationThrottled(err error) bool {
	return IsHTTPStatus(err, http.StatusTooManyRequests)
}

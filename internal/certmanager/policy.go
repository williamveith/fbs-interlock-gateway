// Package certmanager prepares certificate renewals without contacting Shelly
// devices. It deliberately has no gateway, network, database, or PKCS#11
// dependencies. Disruptive installation must be implemented separately.
package certmanager

import (
	"errors"
	"time"
)

const DefaultRenewBefore = 90 * 24 * time.Hour

type RenewalStatus string

const (
	StatusUnknown     RenewalStatus = "unknown"
	StatusNotYetValid RenewalStatus = "not_yet_valid"
	StatusHealthy     RenewalStatus = "healthy"
	StatusDue         RenewalStatus = "due"
	StatusExpired     RenewalStatus = "expired"
)

// Observation describes a peer certificate previously obtained from a normal,
// successfully verified HTTPS request to the physical Shelly. Missing
// observations do not trigger a new request or an issuance attempt.
type Observation struct {
	Host         string
	SerialNumber string
	NotBefore    time.Time
	NotAfter     time.Time
}

type Decision struct {
	Status    RenewalStatus
	Remaining time.Duration
	Due       bool
}

// Evaluate has no side effects and accepts the current time explicitly so all
// tests and renewal decisions are independent of host scheduling and clocks.
// An expired or not-yet-valid certificate requires operator recovery rather
// than an unattended attempt to connect over HTTPS.
func Evaluate(now time.Time, observed Observation, renewBefore time.Duration) (Decision, error) {
	if renewBefore <= 0 {
		return Decision{}, errors.New("renewal threshold must be positive")
	}
	if observed.NotBefore.IsZero() || observed.NotAfter.IsZero() {
		return Decision{Status: StatusUnknown}, nil
	}
	if !observed.NotAfter.After(observed.NotBefore) {
		return Decision{}, errors.New("observed certificate validity period is invalid")
	}
	remaining := observed.NotAfter.Sub(now)
	switch {
	case now.Before(observed.NotBefore):
		return Decision{Status: StatusNotYetValid, Remaining: remaining}, nil
	case !now.Before(observed.NotAfter):
		return Decision{Status: StatusExpired, Remaining: remaining}, nil
	case remaining <= renewBefore:
		return Decision{Status: StatusDue, Remaining: remaining, Due: true}, nil
	default:
		return Decision{Status: StatusHealthy, Remaining: remaining}, nil
	}
}

// PrepareIfDue performs no signing when the observed certificate is unknown,
// healthy, expired, or not yet valid. Run it only on a separate certificate
// issuance worker, never in an FBS request handler.
func PrepareIfDue(now time.Time, observed Observation, renewBefore time.Duration, issuer *Issuer) (*Bundle, Decision, error) {
	decision, err := Evaluate(now, observed, renewBefore)
	if err != nil || !decision.Due {
		return nil, decision, err
	}
	if issuer == nil {
		return nil, decision, errors.New("certificate issuer is unavailable")
	}
	bundle, err := issuer.Issue(observed.Host, now)
	if err != nil {
		return nil, decision, err
	}
	return bundle, decision, nil
}

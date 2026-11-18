package admin

import (
	"testing"
	"time"

	"github.com/williamveith/fbs-interlock-gateway/internal/shelly"
	"github.com/williamveith/fbs-interlock-gateway/internal/status"
)

type fakeCertificateClient map[string]shelly.ServerCertificateInfo

func (c fakeCertificateClient) GetServerCertificate(host string) (shelly.ServerCertificateInfo, bool) {
	info, ok := c[host]
	return info, ok
}

func TestCertificateStatusIsPassiveAndDoesNotConfuseUnknownWithAbsent(t *testing.T) {
	now := time.Date(2026, 10, 16, 21, 0, 0, 0, time.UTC)
	row := status.ToolStatus{IP: "unreachable.example.test", Protocol: "https", Connected: false}
	got := certificateState(row, fakeCertificateClient{}, now)
	if got.State != "unknown" || got.NotAfter != nil || got.DaysRemaining != nil {
		t.Fatalf("unobserved/unreachable HTTPS cert must remain unknown: %+v", got)
	}
	row.Protocol = "http"
	if got := certificateState(row, fakeCertificateClient{}, now); got.State != "not_applicable" {
		t.Fatalf("plain HTTP should be not applicable: %+v", got)
	}
}

func TestCertificateStatusUsesObservedCertificateAfterConnectionFailure(t *testing.T) {
	now := time.Date(2026, 10, 16, 21, 0, 0, 0, time.UTC)
	observed := now.Add(-time.Hour)
	row := status.ToolStatus{IP: "shelly.example.test", Protocol: "https", Connected: false, Error: "unreachable"}
	cert := shelly.ServerCertificateInfo{
		Subject: "CN=shelly.example.test", Issuer: "CN=CA", SerialNumber: "123",
		NotBefore: now.Add(-30 * 24 * time.Hour), NotAfter: now.Add(30 * 24 * time.Hour),
		ObservedAt: observed,
	}
	got := certificateState(row, fakeCertificateClient{row.IP: cert}, now)
	if got.State != "expiring_soon" || got.DaysRemaining == nil || *got.DaysRemaining != 30 {
		t.Fatalf("unexpected cached result: %+v", got)
	}
	if got.ObservedAt == nil || !got.ObservedAt.Equal(observed) || got.Subject != cert.Subject {
		t.Fatalf("metadata missing from cached result: %+v", got)
	}
}

func TestCertificateStatusThresholdAndExpiry(t *testing.T) {
	now := time.Date(2026, 10, 16, 21, 0, 0, 0, time.UTC)
	row := status.ToolStatus{IP: "shelly.example.test", Protocol: "https"}
	for _, tc := range []struct {
		name  string
		until time.Duration
		state string
		days  int
	}{
		{"more_than_90_days", 91 * 24 * time.Hour, "valid", 91},
		{"exactly_90_days", 90 * 24 * time.Hour, "expiring_soon", 90},
		{"one_hour_remaining", time.Hour, "expiring_soon", 1},
		{"expired", -time.Hour, "expired", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cert := shelly.ServerCertificateInfo{NotBefore: now.Add(-time.Hour), NotAfter: now.Add(tc.until)}
			got := certificateState(row, fakeCertificateClient{row.IP: cert}, now)
			if got.State != tc.state || got.DaysRemaining == nil || *got.DaysRemaining != tc.days {
				t.Fatalf("got %+v, want state=%s days=%d", got, tc.state, tc.days)
			}
		})
	}
}

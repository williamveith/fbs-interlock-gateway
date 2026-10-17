package certmanager

import (
	"testing"
	"time"
)

func TestEvaluateExpirationPolicy(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		obs  Observation
		want RenewalStatus
		due  bool
	}{
		{"unobserved", Observation{}, StatusUnknown, false},
		{"healthy", Observation{NotBefore: now.Add(-time.Hour), NotAfter: now.Add(91 * 24 * time.Hour)}, StatusHealthy, false},
		{"threshold exact", Observation{NotBefore: now.Add(-time.Hour), NotAfter: now.Add(90 * 24 * time.Hour)}, StatusDue, true},
		{"within threshold", Observation{NotBefore: now.Add(-time.Hour), NotAfter: now.Add(30 * 24 * time.Hour)}, StatusDue, true},
		{"expires now", Observation{NotBefore: now.Add(-time.Hour), NotAfter: now}, StatusExpired, false},
		{"already expired", Observation{NotBefore: now.Add(-48 * time.Hour), NotAfter: now.Add(-time.Hour)}, StatusExpired, false},
		{"clock invalid", Observation{NotBefore: now.Add(time.Hour), NotAfter: now.Add(365 * 24 * time.Hour)}, StatusNotYetValid, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Evaluate(now, tc.obs, DefaultRenewBefore)
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != tc.want || got.Due != tc.due {
				t.Fatalf("Evaluate() = %+v, want status=%s due=%t", got, tc.want, tc.due)
			}
		})
	}
}

func TestEvaluateRejectsInvalidPolicies(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	if _, err := Evaluate(now, Observation{}, 0); err == nil {
		t.Fatal("want invalid threshold error")
	}
	if _, err := Evaluate(now, Observation{NotBefore: now, NotAfter: now}, DefaultRenewBefore); err == nil {
		t.Fatal("want invalid validity error")
	}
}

func TestPrepareWithoutIssuerDoesNotSignIfNotDue(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	healthy := Observation{Host: "shelly.example.test", NotBefore: now.Add(-time.Hour), NotAfter: now.Add(100 * 24 * time.Hour)}
	bundle, decision, err := PrepareIfDue(now, healthy, DefaultRenewBefore, nil)
	if err != nil || bundle != nil || decision.Due {
		t.Fatalf("healthy certificate must not sign: bundle=%v decision=%+v err=%v", bundle, decision, err)
	}
	healthy.NotAfter = now.Add(20 * 24 * time.Hour)
	bundle, decision, err = PrepareIfDue(now, healthy, DefaultRenewBefore, nil)
	if err == nil || bundle != nil || !decision.Due {
		t.Fatalf("due certificate must require issuer: %+v, %v", decision, err)
	}
}

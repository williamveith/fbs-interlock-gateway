package process

import "testing"

func TestKillPortRejectsInvalidPorts(t *testing.T) {
	for _, port := range []int{-1, 0, 65536} {
		if err := KillPort(port); err == nil {
			t.Fatalf("KillPort(%d) returned nil error", port)
		}
	}
}

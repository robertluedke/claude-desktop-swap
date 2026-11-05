package cmd

import (
	"testing"
	"time"

	"github.com/FranCalveyra/claude-desktop-swap/internal/profile"
)

func TestHealthLabelDistinguishesAllStates(t *testing.T) {
	for _, health := range []profile.Health{profile.HealthUsable, profile.HealthExpired, profile.HealthMissing, profile.HealthUnknown} {
		if got := healthLabel(health); got != string(health) {
			t.Fatalf("healthLabel(%q) = %q", health, got)
		}
	}
}

func TestUsageLabel(t *testing.T) {
	now := time.Date(2026, 11, 5, 1, 30, 0, 0, time.Local)
	tests := []struct {
		name string
		u    profile.UsageWindow
		want string
	}{
		{"upcoming", profile.UsageWindow{Percent: 37.4, ResetsAt: now.Add(90 * time.Minute)}, "03:00 (37%)"},
		{"no window", profile.UsageWindow{}, "-"},
		{"already reset", profile.UsageWindow{Percent: 100, ResetsAt: now.Add(-time.Minute)}, "-"},
	}
	for _, tt := range tests {
		if got := usageLabel(tt.u, now, "15:04"); got != tt.want {
			t.Errorf("%s: usageLabel = %q, want %q", tt.name, got, tt.want)
		}
	}
}

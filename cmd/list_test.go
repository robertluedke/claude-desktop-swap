package cmd

import (
	"reflect"
	"testing"
	"time"

	"github.com/robertluedke/claude-desktop-swap/internal/profile"
)

func TestHealthLabelDistinguishesAllStates(t *testing.T) {
	for _, health := range []profile.Health{profile.HealthUsable, profile.HealthExpired, profile.HealthMissing, profile.HealthUnknown} {
		if got := healthLabel(health); got != string(health) {
			t.Fatalf("healthLabel(%q) = %q", health, got)
		}
	}
}

func TestGroupLinkedBracketsLinkedProfilesTogether(t *testing.T) {
	profiles := []profile.Meta{{Name: "alpha"}, {Name: "personal"}, {Name: "beta"}, {Name: "work"}, {Name: "zeta"}}
	links := profile.SessionLinks{Primary: "work", Links: []profile.SessionLink{{Profile: "personal"}, {Profile: "zeta"}}}

	ordered, labels := groupLinked(profiles, links)
	var names []string
	for _, p := range ordered {
		names = append(names, p.Name)
	}
	if want := []string{"alpha", "work", "personal", "zeta", "beta"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("order = %v, want %v", names, want)
	}
	if want := []string{"", "┐ primary", "┤", "┘", ""}; !reflect.DeepEqual(labels, want) {
		t.Fatalf("labels = %q, want %q", labels, want)
	}

	_, labels = groupLinked(profiles, profile.SessionLinks{})
	if !reflect.DeepEqual(labels, make([]string, len(profiles))) {
		t.Fatalf("unlinked labels = %q", labels)
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

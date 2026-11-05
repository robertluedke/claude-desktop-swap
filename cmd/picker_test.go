package cmd

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/robertluedke/claude-desktop-swap/internal/profile"
)

func TestPickerShowsHealthWithoutSecretData(t *testing.T) {
	m := newPickerModel([]profile.Meta{{Name: "work", ObservedHealth: profile.HealthExpired}}, "work")
	view := m.View()
	if !strings.Contains(view, "expired") {
		t.Fatalf("view = %q", view)
	}
	if strings.Contains(view, "sessionKey") || strings.Contains(view, "secret") {
		t.Fatalf("view exposed session data: %q", view)
	}
}

func TestPickerBlocksUnusableSelection(t *testing.T) {
	for _, health := range []profile.Health{profile.HealthExpired, profile.HealthMissing, profile.HealthUnknown} {
		m := newPickerModel([]profile.Meta{{Name: "work", ObservedHealth: health}}, "")
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		got := updated.(pickerModel)
		if got.chosen != "" {
			t.Fatalf("health %s selected %q", health, got.chosen)
		}
	}
}

func TestPickerAllowsUsableSelection(t *testing.T) {
	m := newPickerModel([]profile.Meta{{Name: "work", ObservedHealth: profile.HealthUsable}}, "")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := updated.(pickerModel).chosen; got != "work" {
		t.Fatalf("chosen = %q", got)
	}
}

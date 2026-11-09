package cmd

import (
	"slices"
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

func TestChoicePickerShowsHelpFirstAndReturnsChoice(t *testing.T) {
	m := choiceModel{title: "Move", options: []string{"All", "Choose"}, chosen: -1}
	if first := strings.SplitN(strings.TrimSpace(m.View()), "\n", 2)[0]; !strings.Contains(first, "Enter") {
		t.Fatalf("first line is not the help: %q", first)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := updated.(choiceModel).chosen; got != 1 {
		t.Fatalf("chosen = %d", got)
	}
}

func TestSessionPickerShowsOneHeaderPerSection(t *testing.T) {
	m := sessionPickerModel{title: "Sessions", selected: map[int]bool{}, sessions: []profile.SessionInfo{
		{File: "local_a.json", Title: "A", Section: "PaxBuddy"},
		{File: "local_b.json", Title: "B", Section: "PaxBuddy"},
		{File: "local_c.json", Title: "C", Section: "Sessions"},
	}}
	view := m.View()
	if strings.Count(view, "PaxBuddy (2)") != 1 || strings.Count(view, "Sessions (1)") != 1 {
		t.Fatalf("section headers missing or repeated:\n%s", view)
	}
	if strings.Index(view, "PaxBuddy (2)") > strings.Index(view, "  A\n") || strings.Index(view, "Sessions (1)") > strings.Index(view, "  C\n") {
		t.Fatalf("header not above its sessions:\n%s", view)
	}

	m.height = 9
	var updated tea.Model = m
	for range 2 {
		updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	if view := updated.View(); !strings.Contains(view, "  C\n") || !strings.Contains(view, "Sessions (1)") {
		t.Fatalf("cursor row or its header scrolled out of view:\n%s", view)
	}
}

func TestSessionPickerSelectsWithSpaceAndConfirmsWithEnter(t *testing.T) {
	m := sessionPickerModel{title: "Sessions", selected: map[int]bool{}, sessions: []profile.SessionInfo{
		{File: "local_a.json", Title: "A"}, {File: "local_b.json", Title: "B"},
	}}
	if first := strings.SplitN(strings.TrimSpace(m.View()), "\n", 2)[0]; !strings.Contains(first, "Space") {
		t.Fatalf("first line is not the help: %q", first)
	}
	var updated tea.Model = m
	updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if updated.(sessionPickerModel).confirmed {
		t.Fatal("enter without a selection must not confirm")
	}
	updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyDown})
	updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeySpace})
	updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(sessionPickerModel)
	if !got.confirmed || !slices.Equal(got.selectedFiles(), []string{"local_b.json"}) {
		t.Fatalf("confirmed = %v, files = %v", got.confirmed, got.selectedFiles())
	}
}

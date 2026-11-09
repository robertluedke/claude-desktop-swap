package cmd

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/robertluedke/claude-desktop-swap/internal/profile"
)

var (
	pickerTitleStyle   = lipgloss.NewStyle().Bold(true)
	pickerHeaderStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	pickerHelpStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	pickerBadgeStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	pickerSectionStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
)

type pickerModel struct {
	profiles  []profile.Meta
	current   string
	cursor    int
	chosen    string
	cancelled bool
	typed     string
	now       time.Time
}

func runPicker(profiles []profile.Meta, current string) (string, error) {
	if len(profiles) == 0 {
		return "", nil
	}

	final, err := tea.NewProgram(newPickerModel(profiles, current)).Run()
	if err != nil {
		return "", err
	}

	m, ok := final.(pickerModel)
	if !ok || m.cancelled {
		return "", nil
	}
	return m.chosen, nil
}

func newPickerModel(profiles []profile.Meta, current string) pickerModel {
	m := pickerModel{profiles: profiles, current: current, now: time.Now()}
	for i, p := range profiles {
		if p.Name == current {
			m.cursor = i
			break
		}
	}
	return m
}

func (m pickerModel) Init() tea.Cmd {
	return nil
}

func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			m.typed = ""
			m.cancelled = true
			return m, tea.Quit
		case "up", "k":
			m.typed = ""
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			m.typed = ""
			if m.cursor < len(m.profiles)-1 {
				m.cursor++
			}
		case "enter":
			m.typed = ""
			if m.profiles[m.cursor].ObservedHealth != profile.HealthUsable {
				return m, nil
			}
			m.chosen = m.profiles[m.cursor].Name
			return m, tea.Quit
		case "backspace":
			if m.typed != "" {
				m.typed = m.typed[:len(m.typed)-1]
				m.jumpToTypedRow()
			}
		case "0", "1", "2", "3", "4", "5", "6", "7", "8", "9":
			m.typeRowNumber(msg.String())
		}
	}
	return m, nil
}

func (m *pickerModel) typeRowNumber(digit string) {
	if digit == "0" && m.typed == "" {
		return
	}

	maxDigits := len(strconv.Itoa(len(m.profiles)))
	if len(m.typed) >= maxDigits {
		m.typed = ""
	}

	m.typed += digit
	m.jumpToTypedRow()
}

func (m *pickerModel) jumpToTypedRow() {
	if m.typed == "" {
		return
	}

	row, err := strconv.Atoi(m.typed)
	if err != nil || row < 1 || row > len(m.profiles) {
		return
	}
	m.cursor = row - 1
}

func (m pickerModel) View() string {
	var b strings.Builder
	accountWidth, planWidth, healthWidth, expiresWidth, lastWidth := m.columnWidths()

	b.WriteString("\n  ")
	b.WriteString(pickerTitleStyle.Render("Select account to activate:"))
	b.WriteString("\n\n")
	b.WriteString(pickerHeaderStyle.Render(fmt.Sprintf(
		"       %s  %s  %s  %s  %s",
		padRight("ACCOUNT", accountWidth),
		padRight("PLAN", planWidth),
		padRight("HEALTH", healthWidth),
		padRight("LOGIN EXPIRES", expiresWidth),
		padRight("LAST", lastWidth),
	)))
	b.WriteString("\n")

	for i, p := range m.profiles {
		cursor := " "
		if i == m.cursor {
			cursor = ">"
		}

		badge := ""
		if p.Name == m.current {
			badge = "  " + pickerBadgeStyle.Render("[ACTIVE]")
		}

		fmt.Fprintf(&b,
			"  %s %02d %s  %s  %s  %s  %s%s\n",
			cursor,
			i+1,
			padRight(accountLabel(p), accountWidth),
			padRight(planLabel(p), planWidth),
			padRight(healthLabel(p.ObservedHealth), healthWidth),
			padRight(metaExpiryLabel(p, m.now), expiresWidth),
			padRight(relativeLastUsed(p.LastUsed), lastWidth),
			badge,
		)
	}

	b.WriteString("\n  ")
	b.WriteString(pickerHelpStyle.Render("Keys: ↑/↓ or j/k, Enter select, 1-9 type, Backspace edit, Esc or q quit"))
	b.WriteString("\n")
	return b.String()
}

func (m pickerModel) columnWidths() (accountWidth, planWidth, healthWidth, expiresWidth, lastWidth int) {
	accountWidth = lipgloss.Width("ACCOUNT")
	planWidth = lipgloss.Width("PLAN")
	healthWidth = lipgloss.Width("HEALTH")
	expiresWidth = lipgloss.Width("LOGIN EXPIRES")
	lastWidth = lipgloss.Width("LAST")

	for _, p := range m.profiles {
		accountWidth = max(accountWidth, lipgloss.Width(accountLabel(p)))
		planWidth = max(planWidth, lipgloss.Width(planLabel(p)))
		healthWidth = max(healthWidth, lipgloss.Width(healthLabel(p.ObservedHealth)))
		expiresWidth = max(expiresWidth, lipgloss.Width(metaExpiryLabel(p, m.now)))
		lastWidth = max(lastWidth, lipgloss.Width(relativeLastUsed(p.LastUsed)))
	}

	planWidth = max(planWidth, 8)
	lastWidth = max(lastWidth, 6)
	return accountWidth, planWidth, healthWidth, expiresWidth, lastWidth
}

func accountLabel(p profile.Meta) string {
	if p.Email != "" {
		return p.Email
	}
	return p.Name
}

func planLabel(p profile.Meta) string {
	if p.Plan == "" {
		return "-"
	}
	return p.Plan
}

func relativeLastUsed(t time.Time) string {
	if t.IsZero() {
		return "Never"
	}

	elapsed := time.Since(t)
	if elapsed < time.Minute {
		return "Now"
	}
	if elapsed < time.Hour {
		return fmt.Sprintf("%dm ago", int(elapsed.Minutes()))
	}
	if elapsed < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(elapsed.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(elapsed.Hours()/24))
}

type choiceModel struct {
	title   string
	options []string
	cursor  int
	chosen  int
	done    bool
}

func runChoice(title string, options []string) (int, error) {
	final, err := tea.NewProgram(choiceModel{title: title, options: options, chosen: -1}).Run()
	if err != nil {
		return -1, err
	}
	return final.(choiceModel).chosen, nil
}

func (m choiceModel) Init() tea.Cmd { return nil }

func (m choiceModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+c", "esc", "q":
			m.done = true
			return m, tea.Quit
		case "up", "k":
			m.cursor = max(m.cursor-1, 0)
		case "down", "j":
			m.cursor = min(m.cursor+1, len(m.options)-1)
		case "enter":
			m.chosen, m.done = m.cursor, true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m choiceModel) View() string {
	if m.done {
		return ""
	}
	var b strings.Builder
	b.WriteString("  " + pickerHelpStyle.Render("↑/↓ choose · Enter confirm · Esc cancel") + "\n\n")
	b.WriteString("  " + pickerTitleStyle.Render(m.title) + "\n\n")
	for i, o := range m.options {
		cursor := " "
		if i == m.cursor {
			cursor = ">"
		}
		fmt.Fprintf(&b, "  %s %s\n", cursor, o)
	}
	return b.String()
}

type sessionPickerModel struct {
	title     string
	sessions  []profile.SessionInfo
	selected  map[int]bool
	cursor    int
	offset    int
	height    int
	confirmed bool
	done      bool
}

func runSessionPicker(title string, sessions []profile.SessionInfo) ([]string, error) {
	final, err := tea.NewProgram(sessionPickerModel{title: title, sessions: sessions, selected: map[int]bool{}}).Run()
	if err != nil {
		return nil, err
	}
	m := final.(sessionPickerModel)
	if !m.confirmed {
		return nil, nil
	}
	return m.selectedFiles(), nil
}

func (m sessionPickerModel) Init() tea.Cmd { return nil }

func (m sessionPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			m.done = true
			return m, tea.Quit
		case "up", "k":
			m.cursor = max(m.cursor-1, 0)
		case "down", "j":
			m.cursor = min(m.cursor+1, len(m.sessions)-1)
		case " ", "space":
			m.selected[m.cursor] = !m.selected[m.cursor]
		case "enter":
			if len(m.selectedFiles()) == 0 {
				return m, nil
			}
			m.confirmed, m.done = true, true
			return m, tea.Quit
		}
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	for m.offset < m.cursor && m.linesFor(m.offset, m.cursor) > m.visibleRows() {
		m.offset++
	}
	return m, nil
}

func (m sessionPickerModel) sectionStarts(i, from int) bool {
	return i == from || m.sessions[i].Section != m.sessions[i-1].Section
}

// linesFor counts the screen lines rows from..to take, including the header
// shown above the first row of each section.
func (m sessionPickerModel) linesFor(from, to int) int {
	n := 0
	for i := from; i <= to; i++ {
		if m.sectionStarts(i, from) {
			n++
		}
		n++
	}
	return n
}

func (m sessionPickerModel) selectedFiles() []string {
	var files []string
	for i, s := range m.sessions {
		if m.selected[i] {
			files = append(files, s.File)
		}
	}
	return files
}

// visibleRows leaves room for the help, title and blank lines around the list.
func (m sessionPickerModel) visibleRows() int {
	if m.height == 0 {
		return 2 * len(m.sessions)
	}
	return max(m.height-6, 3)
}

func (m sessionPickerModel) View() string {
	if m.done {
		return ""
	}
	var b strings.Builder
	b.WriteString("  " + pickerHelpStyle.Render("↑/↓ move · Space select · Enter move selected · Esc cancel") + "\n\n")
	b.WriteString("  " + pickerTitleStyle.Render(fmt.Sprintf("%s (%d selected)", m.title, len(m.selectedFiles()))) + "\n\n")
	counts := map[string]int{}
	for _, s := range m.sessions {
		counts[s.Section]++
	}
	end := m.offset
	for end < len(m.sessions) && m.linesFor(m.offset, end) <= m.visibleRows() {
		end++
	}
	for i := m.offset; i < end; i++ {
		s := m.sessions[i]
		if m.sectionStarts(i, m.offset) {
			b.WriteString("  " + pickerSectionStyle.Render(fmt.Sprintf("%s (%d)", s.Section, counts[s.Section])) + "\n")
		}
		cursor, box := " ", "[ ]"
		if i == m.cursor {
			cursor = ">"
		}
		if m.selected[i] {
			box = "[x]"
		}
		title := s.Title
		if title == "" {
			title = s.File
		}
		folder := ""
		if s.Cwd != "" && filepath.Base(s.Cwd) != s.Section {
			folder = "  " + pickerHeaderStyle.Render(filepath.Base(s.Cwd))
		}
		fmt.Fprintf(&b, "  %s %s %s  %s%s\n", cursor, box, s.LastActivity.Format("2006-01-02 15:04"), title, folder)
	}
	if end < len(m.sessions) {
		b.WriteString("  " + pickerHelpStyle.Render(fmt.Sprintf("… %d more", len(m.sessions)-end)) + "\n")
	}
	return b.String()
}

func padRight(s string, width int) string {
	padding := width - lipgloss.Width(s)
	if padding <= 0 {
		return s
	}
	return s + strings.Repeat(" ", padding)
}

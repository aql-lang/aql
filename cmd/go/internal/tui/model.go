// model.go is the bubbletea Model: state, update, view. The TUI
// shows a table of services, polls the api every refreshInterval,
// and dispatches state-transition actions on key press.
//
// Keys:
//
//	↑/↓ or k/j   move cursor
//	p            pause selected service
//	r            resume selected service
//	x            stop selected service
//	enter        no-op (placeholder for future detail view)
//	q or ctrl-c  quit

package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// refreshInterval is how often we re-poll /v1/services.
const refreshInterval = 500 * time.Millisecond

// styles for the TUI.
var (
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	cursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	pausedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	runStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	stopStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	helpStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

// model is the bubbletea Model for the tui.
type model struct {
	client   *apiClient
	services []serviceEntity
	server   *serverInfo
	cursor   int
	status   string // last action/error line
	err      string // last poll error, sticky
}

// newModel constructs the initial model.
func newModel(cli *apiClient) *model {
	return &model{client: cli}
}

// Init kicks off the first poll.
func (m *model) Init() tea.Cmd {
	return tea.Batch(m.pollCmd(), m.tickCmd())
}

// --- messages ---

type servicesMsg struct {
	services []serviceEntity
	server   *serverInfo
}

type errMsg struct{ err error }

type actionResultMsg struct {
	name   string
	action string
	err    error
}

type tickMsg time.Time

// pollCmd refreshes services and supervisor info in parallel.
func (m *model) pollCmd() tea.Cmd {
	return func() tea.Msg {
		svcs, err := m.client.listServices()
		if err != nil {
			return errMsg{err}
		}
		info, _ := m.client.getServer() // info is optional
		return servicesMsg{services: svcs, server: info}
	}
}

func (m *model) tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *model) actionCmd(name, action string) tea.Cmd {
	return func() tea.Msg {
		err := m.client.applyAction(name, action)
		return actionResultMsg{name: name, action: action, err: err}
	}
}

// Update handles one message and returns the next model + command.
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	case servicesMsg:
		m.services = msg.services
		m.server = msg.server
		m.err = ""
		// Keep cursor in range as services come and go.
		if m.cursor >= len(m.services) {
			m.cursor = max0(len(m.services) - 1)
		}
		return m, nil
	case errMsg:
		m.err = msg.err.Error()
		return m, nil
	case actionResultMsg:
		if msg.err != nil {
			m.status = fmt.Sprintf("%s %s: %s", msg.action, msg.name, msg.err)
		} else {
			m.status = fmt.Sprintf("%s %s: ok", msg.action, msg.name)
		}
		// Re-poll immediately so state change shows up without
		// waiting for the next tick.
		return m, m.pollCmd()
	case tickMsg:
		return m, tea.Batch(m.pollCmd(), m.tickCmd())
	}
	return m, nil
}

// handleKey routes one keypress.
func (m *model) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.services)-1 {
			m.cursor++
		}
	case "p":
		if svc, ok := m.selected(); ok {
			return m, m.actionCmd(svc.Name, "pause")
		}
	case "r":
		if svc, ok := m.selected(); ok {
			return m, m.actionCmd(svc.Name, "resume")
		}
	case "x":
		if svc, ok := m.selected(); ok {
			return m, m.actionCmd(svc.Name, "stop")
		}
	}
	return m, nil
}

// selected returns the service at the cursor, if any.
func (m *model) selected() (serviceEntity, bool) {
	if m.cursor < 0 || m.cursor >= len(m.services) {
		return serviceEntity{}, false
	}
	return m.services[m.cursor], true
}

// View renders the current model state to a string.
func (m *model) View() string {
	var b strings.Builder

	// Header.
	b.WriteString(headerStyle.Render("aql tui"))
	if m.server != nil {
		fmt.Fprintf(&b, "  pid=%d  version=%s  uptime=%.0fs  services=%d",
			m.server.PID, m.server.Version, m.server.UptimeSeconds, m.server.ServiceCount)
	}
	b.WriteString("\n\n")

	// Table.
	if len(m.services) == 0 {
		b.WriteString(helpStyle.Render("  (no services)\n"))
	} else {
		b.WriteString(headerStyle.Render(fmt.Sprintf("  %-12s %-10s %s", "NAME", "STATE", "METADATA")))
		b.WriteString("\n")
		for i, svc := range m.services {
			cursor := "  "
			if i == m.cursor {
				cursor = cursorStyle.Render("▸ ")
			}
			var stateStr string
			switch svc.State {
			case "running":
				stateStr = runStyle.Render("running ")
			case "paused":
				stateStr = pausedStyle.Render("paused  ")
			case "stopped":
				stateStr = stopStyle.Render("stopped ")
			default:
				stateStr = stopStyle.Render(fmt.Sprintf("%-8s", svc.State))
			}
			fmt.Fprintf(&b, "%s%-12s %s %s\n", cursor, svc.Name, stateStr, formatMetadata(svc.Metadata))
		}
	}

	b.WriteString("\n")

	// Status / error line.
	if m.err != "" {
		b.WriteString(errStyle.Render("  ! " + m.err))
		b.WriteString("\n")
	} else if m.status != "" {
		b.WriteString(helpStyle.Render("  " + m.status))
		b.WriteString("\n")
	} else {
		b.WriteString("\n")
	}

	// Help.
	b.WriteString(helpStyle.Render("  ↑/↓ move   p pause   r resume   x stop   q quit"))
	b.WriteString("\n")
	return b.String()
}

func formatMetadata(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	parts := make([]string, 0, len(m))
	for k, v := range m {
		parts = append(parts, k+"="+v)
	}
	return helpStyle.Render(strings.Join(parts, " "))
}

func max0(v int) int {
	if v < 0 {
		return 0
	}
	return v
}

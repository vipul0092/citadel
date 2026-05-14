package dashboard

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/vipul0092/citadel/internal/client"
	"github.com/vipul0092/citadel/internal/control"
	"github.com/vipul0092/citadel/internal/server"
)

// --- styles ---

var (
	titleStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	headerStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	selectedStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	normalStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	serverRoleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	clientRoleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("75"))
	dimStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	footerStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	errorStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	modalBoxStyle   = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(1, 3)
	labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
)

const dashboardArt = "" +
	"   _     _     _   \n" +
	"  | |   | |   | |  \n" +
	"  | |___| |___| |  \n" +
	"  |      ⚔      |  \n" +
	"  |_____________|  "

// DashboardModel is the top-level BubbleTea model for the dashboard.
type DashboardModel struct {
	instances  []Instance
	selected   int
	modal      modalState
	inputs     [3]textinput.Model
	inputIdx   int
	inputCount int                      // how many inputs the current modal uses
	inputErr   string                   // validation error for current modal
	eventsCh   chan instanceEvent
	killing    map[string]bool          // sockPath → kill in flight
	restarts   map[string]pendingRestart // sockPath → restart pending after instance disappears
	active     tea.Model                // non-nil when drilled into an instance
	version    string
	width      int
	height     int
}

// New creates a ready DashboardModel.
func New(version string) DashboardModel {
	inputs := [3]textinput.Model{textinput.New(), textinput.New(), textinput.New()}
	return DashboardModel{
		eventsCh: make(chan instanceEvent, 64),
		inputs:   inputs,
		killing:  make(map[string]bool),
		restarts: make(map[string]pendingRestart),
		version:  version,
		width:    80,
		height:   24,
	}
}

func (m DashboardModel) Init() tea.Cmd {
	return tea.Batch(doScan(), waitForEvent(m.eventsCh))
}

func (m DashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Dashboard-level messages always processed, even in drill-in mode.
	switch msg := msg.(type) {
	case scanResultMsg:
		m = m.reconcile([]control.SentinelInfo(msg))
		if m.active != nil {
			return m, scanTick()
		}
		return m, scanTick()

	case instanceEvent:
		m = m.applyEvent(msg)
		return m, waitForEvent(m.eventsCh)

	case killTimerMsg:
		if m.isAlive(msg.sockPath) {
			for _, inst := range m.instances {
				if inst.Info.SockPath == msg.sockPath {
					_ = sendSigterm(inst.Info.Pid)
					break
				}
			}
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.active != nil {
			newActive, cmd := m.active.Update(msg)
			m.active = newActive
			return m, cmd
		}
		return m, nil
	}

	// Drill-in mode: forward all other messages to the active sub-TUI.
	if m.active != nil {
		// Sub-TUI /quit exits drill-in instead of the whole program.
		if _, ok := msg.(server.DrillInExitMsg); ok {
			m.active = nil
			return m, nil
		}
		if _, ok := msg.(client.DrillInExitMsg); ok {
			m.active = nil
			return m, nil
		}
		if km, ok := msg.(tea.KeyMsg); ok && km.String() == "esc" {
			m.active = nil
			return m, nil
		}
		newActive, cmd := m.active.Update(msg)
		m.active = newActive
		return m, cmd
	}

	// Normal table view.
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m DashboardModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.modal == modalNone {
		return m.handleTableKey(msg)
	}
	if m.modal == modalKillConfirm {
		return m.handleKillConfirmKey(msg)
	}
	return m.handleModalKey(msg)
}

func (m DashboardModel) handleTableKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		for _, inst := range m.instances {
			if inst.conn != nil {
				inst.conn.Close()
			}
		}
		return m, tea.Quit

	case "up":
		if m.selected > 0 {
			m.selected--
		}

	case "down":
		if m.selected < len(m.instances)-1 {
			m.selected++
		}

	case "h":
		m = m.openModal(modalHost, 3,
			"Lobby name", "Your name", fmt.Sprintf("%d", 7777))

	case "c":
		m = m.openModal(modalConnect, 2,
			"Server address", "Your name", "")

	case "s":
		m = m.openModal(modalServerOnly, 2,
			"Server name", "Port", fmt.Sprintf("%d", 7777))

	case "k":
		if len(m.instances) > 0 {
			m.modal = modalKillConfirm
		}

	case "r":
		if len(m.instances) > 0 && m.selected < len(m.instances) {
			var cmd tea.Cmd
			m, cmd = doRestart(m)
			return m, cmd
		}

	case "enter":
		if len(m.instances) == 0 || m.selected >= len(m.instances) {
			return m, nil
		}
		return m.drillIn()
	}
	return m, nil
}

func (m DashboardModel) handleKillConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		m.modal = modalNone
		var cmd tea.Cmd
		m, cmd = startKill(m)
		return m, cmd
	default:
		m.modal = modalNone
	}
	return m, nil
}

func (m DashboardModel) handleModalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.modal = modalNone
		m.inputErr = ""
		return m, nil

	case "tab":
		m.inputs[m.inputIdx].Blur()
		m.inputIdx = (m.inputIdx + 1) % m.inputCount
		m.inputs[m.inputIdx].Focus()
		return m, nil

	case "enter":
		if m.inputIdx == m.inputCount-1 {
			var cmd tea.Cmd
			m, cmd = submitModal(m)
			return m, cmd
		}
		// advance to next input on Enter
		m.inputs[m.inputIdx].Blur()
		m.inputIdx++
		m.inputs[m.inputIdx].Focus()
		return m, nil
	}

	var cmd tea.Cmd
	m.inputs[m.inputIdx], cmd = m.inputs[m.inputIdx].Update(msg)
	m.inputErr = ""
	return m, cmd
}

// --- View ---

func (m DashboardModel) View() string {
	if m.active != nil {
		return m.active.View()
	}
	if m.width < 50 {
		return "Terminal too narrow for dashboard."
	}

	var sb strings.Builder

	// Header
	sep := strings.Repeat("─", m.width-2)
	if m.width >= 55 {
		sb.WriteString(lipgloss.NewStyle().Width(m.width).Align(lipgloss.Center).Render(dashboardArt))
		sb.WriteString("\n")
	}
	sb.WriteString(titleStyle.Render("Citadel Dashboard"))
	sb.WriteString(dimStyle.Render("  " + m.version))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render(sep))
	sb.WriteString("\n")

	if len(m.instances) == 0 {
		sb.WriteString(dimStyle.Render("  No running citadel processes."))
		sb.WriteString("\n")
	} else {
		// Column headers
		sb.WriteString(headerStyle.Render(
			fmt.Sprintf("  %-7s %-15s %-22s %-10s %-8s",
				"ROLE", "NAME", "ADDR", "PEERS", "UPTIME")))
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render("  " + strings.Repeat("─", m.width-4)))
		sb.WriteString("\n")

		for i, inst := range m.instances {
			selector := "  "
			if i == m.selected {
				selector = "▶ "
			}

			role := inst.Info.Role
			var roleStyle lipgloss.Style
			if role == "client" {
				roleStyle = clientRoleStyle
			} else {
				roleStyle = serverRoleStyle
			}

			rolePadded := fmt.Sprintf("%-7s", role)
			name := truncate(inst.Info.Name, 15)
			addr := truncate(inst.Info.Addr, 22)
			peers := formatPeers(inst)
			uptime := formatUptime(inst.Info.Started)
			star := "  "
			if inst.starred {
				star = " ★"
			}

			if i == m.selected {
				line := fmt.Sprintf("%s%-7s %-15s %-22s %-10s %-8s%s",
					selector, role, name, addr, peers, uptime, star)
				sb.WriteString(selectedStyle.Render(line))
			} else {
				rest := fmt.Sprintf(" %-15s %-22s %-10s %-8s%s", name, addr, peers, uptime, star)
				sb.WriteString(selector + roleStyle.Render(rolePadded) + normalStyle.Render(rest))
			}
			sb.WriteString("\n")
		}
	}

	sb.WriteString(dimStyle.Render(sep))
	sb.WriteString("\n")

	// Footer
	footer := footerStyle.Render("[h] Host  [c] Connect  [s] Server-only  [k] Kill  [r] Restart  [q] Quit")
	sb.WriteString(footer)
	sb.WriteString("\n")

	view := sb.String()

	// Modal overlay
	if m.modal != modalNone {
		view = lipgloss.Place(m.width, m.height,
			lipgloss.Center, lipgloss.Center,
			m.renderModal(),
			lipgloss.WithWhitespaceChars(" "),
		)
	}

	return view
}

func (m DashboardModel) renderModal() string {
	switch m.modal {
	case modalKillConfirm:
		if len(m.instances) == 0 {
			return ""
		}
		inst := m.instances[m.selected]
		title := fmt.Sprintf("Kill %s (%s, pid %d)?", inst.Info.Name, inst.Info.Role, inst.Info.Pid)
		body := title + "\n\n" + labelStyle.Render("[y] Yes    [n] No")
		return modalBoxStyle.Render(body)

	case modalHost:
		return m.renderInputModal("Host a lobby",
			[]string{"Lobby name", "Your name", "Port"},
			m.inputCount)

	case modalConnect:
		return m.renderInputModal("Connect to a server",
			[]string{"Server address (host:port or lobby name)", "Your name"},
			m.inputCount)

	case modalServerOnly:
		return m.renderInputModal("Start server-only",
			[]string{"Server name", "Port"},
			m.inputCount)
	}
	return ""
}

func (m DashboardModel) renderInputModal(title string, labels []string, count int) string {
	var sb strings.Builder
	sb.WriteString(titleStyle.Render(title))
	sb.WriteString("\n\n")
	for i := 0; i < count; i++ {
		sb.WriteString(labelStyle.Render(labels[i] + ": "))
		sb.WriteString(m.inputs[i].View())
		sb.WriteString("\n")
	}
	if m.inputErr != "" {
		sb.WriteString("\n")
		sb.WriteString(errorStyle.Render(m.inputErr))
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
	sb.WriteString(footerStyle.Render("[Enter] Submit    [Esc] Cancel"))
	return modalBoxStyle.Render(sb.String())
}

// --- modal helpers ---

func (m DashboardModel) openModal(state modalState, count int, placeholders ...string) DashboardModel {
	m.modal = state
	m.inputCount = count
	m.inputIdx = 0
	m.inputErr = ""
	for i := 0; i < 3; i++ {
		m.inputs[i] = textinput.New()
		m.inputs[i].Width = 30
		if i < len(placeholders) {
			m.inputs[i].Placeholder = placeholders[i]
			if placeholders[i] == fmt.Sprintf("%d", 7777) {
				m.inputs[i].SetValue(placeholders[i])
			}
		}
	}
	m.inputs[0].Focus()
	return m
}

// --- rendering helpers ---

func formatPeers(inst Instance) string {
	if inst.Info.Role == "client" {
		return fmt.Sprintf("%-10d", inst.peerCount)
	}
	return fmt.Sprintf("%-10d", inst.peerCount)
}

func formatUptime(started string) string {
	t, err := time.Parse(time.RFC3339, started)
	if err != nil {
		return "?"
	}
	d := time.Since(t)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

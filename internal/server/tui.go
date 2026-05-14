package server

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vipul0092/citadel/internal/completer"
	citadelstyle "github.com/vipul0092/citadel/internal/style"
)

// --- styles ---

var (
	headerStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	labelStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("240"))
	peerStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	logChatStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	logEvStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true)
	serverSayStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	errorStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	borderStyle    = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62"))
	inputStyle     = lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("62")).Padding(0, 1)
)

var (
	serverCommands = []string{"/kick", "/motd", "/quit", "/say"}
	serverNameCmds = []string{"/kick"}
)

// DrillInExitMsg is sent by the server TUI when /quit is used while embedded in
// the dashboard drill-in. The dashboard intercepts it to exit drill-in mode
// instead of quitting the whole program.
type DrillInExitMsg struct{}

// --- tea messages ---

type hubEventMsg HubEvent

// hubSourceClosedMsg is sent when the HubEventSource channel is closed (e.g. the
// remote server shut down while the dashboard was drilled in).
type hubSourceClosedMsg struct{}

func waitForHubEvent(ch <-chan HubEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return hubSourceClosedMsg{}
		}
		return hubEventMsg(ev)
	}
}

type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// --- server TUI model ---

// TUI is the Bubble Tea model for the server admin interface.
type TUI struct {
	source     HubEventSource
	serverName string
	listenAddr string
	version    string

	cmdInput textinput.Model
	logVP    viewport.Model

	peers    []PeerEntry
	suggIdx  int
	logLines []string

	// layout dimensions — set in relayout(), used read-only in View()
	peerPanelW int // outer width of the peer list pane
	logPanelW  int // outer width of the activity log pane
	bodyH      int // total height of the body area (including borders)

	width    int
	height   int
	ready    bool
	quitting bool
	drillIn  bool // true when embedded in the dashboard; /quit exits drill-in, not the whole program
	readOnly bool // true when spectating from the dashboard; input is disabled
}

// SetDrillIn marks this TUI as embedded in the dashboard. When set, /quit sends
// DrillInExitMsg instead of tea.Quit.
func (m *TUI) SetDrillIn(v bool) { m.drillIn = v }

// SetReadOnly puts the TUI in spectator mode: the command input is disabled and
// replaced with a read-only indicator.
func (m *TUI) SetReadOnly(v bool) {
	m.readOnly = v
	if v {
		m.cmdInput.Blur()
	}
}

// NewTUI creates the server admin TUI backed by an in-process Hub.
func NewTUI(hub *Hub, serverName, listenAddr, version string) *TUI {
	return NewTUIFromSource(NewInProcessSource(hub), serverName, listenAddr, version)
}

// NewTUIFromSource creates the server admin TUI backed by any HubEventSource.
// Used by the dashboard for remote drill-in.
func NewTUIFromSource(source HubEventSource, serverName, listenAddr, version string) *TUI {
	ti := textinput.New()
	ti.Placeholder = "type a command (/kick, /say, /motd, /quit)…"
	ti.Focus()
	return &TUI{
		source:     source,
		serverName: serverName,
		listenAddr: listenAddr,
		version:    version,
		cmdInput:   ti,
	}
}

func (m *TUI) Init() tea.Cmd {
	return tea.Batch(
		waitForHubEvent(m.source.Events()),
		tickCmd(),
		textinput.Blink,
	)
}

func (m *TUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.relayout()
		m.ready = true

	case tickMsg:
		cmds = append(cmds, tickCmd())

	case hubSourceClosedMsg:
		// Remote server shut down; exit drill-in or quit.
		if m.drillIn {
			return m, func() tea.Msg { return DrillInExitMsg{} }
		}
		m.quitting = true
		return m, tea.Quit

	case hubEventMsg:
		m.applyHubEvent(HubEvent(msg))
		cmds = append(cmds, waitForHubEvent(m.source.Events()))

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			m.quitting = true
			return m, tea.Quit

		case tea.KeyEnter:
			if !m.readOnly {
				m.suggIdx = 0
				line := strings.TrimSpace(m.cmdInput.Value())
				m.cmdInput.SetValue("")
				if line != "" {
					cmds = append(cmds, m.handleAdminCmd(line))
				}
			}

		case tea.KeyTab:
			if !m.readOnly {
				val := m.cmdInput.Value()
				suggestions := completer.Compute(val, serverCommands, serverNameCmds, m.peerNames())
				if len(suggestions) > 0 {
					m.cmdInput.SetValue(completer.Complete(val, suggestions, m.suggIdx))
					m.cmdInput.CursorEnd()
					m.suggIdx = 0
				}
			}

		case tea.KeyUp:
			if !m.readOnly {
				suggestions := completer.Compute(m.cmdInput.Value(), serverCommands, serverNameCmds, m.peerNames())
				if len(suggestions) > 0 {
					m.suggIdx--
					if m.suggIdx < 0 {
						m.suggIdx = len(suggestions) - 1
					}
					break
				}
			}
			m.logVP.ScrollUp(1)

		case tea.KeyDown:
			if !m.readOnly {
				suggestions := completer.Compute(m.cmdInput.Value(), serverCommands, serverNameCmds, m.peerNames())
				if len(suggestions) > 0 {
					m.suggIdx++
					if m.suggIdx >= len(suggestions) {
						m.suggIdx = 0
					}
					break
				}
			}
			m.logVP.ScrollDown(1)

		case tea.KeyPgUp:
			m.logVP.ScrollUp(5)
		case tea.KeyPgDown:
			m.logVP.ScrollDown(5)

		default:
			m.suggIdx = 0
		}
	}

	var vpCmd, tiCmd tea.Cmd
	m.logVP, vpCmd = m.logVP.Update(msg)
	m.cmdInput, tiCmd = m.cmdInput.Update(msg)
	cmds = append(cmds, vpCmd, tiCmd)

	return m, tea.Batch(cmds...)
}

func (m *TUI) View() string {
	if !m.ready {
		return "Initialising…\n"
	}
	if m.quitting {
		return ""
	}

	header := headerStyle.Render(fmt.Sprintf(
		" ⚔  Citadel Server: %s  │  %s  │  %d connected  │  %s ",
		m.serverName, m.listenAddr, len(m.peers), m.version,
	))

	// peer list pane — content only, no model mutations
	peerLines := []string{labelStyle.Render("CLIENTS")}
	if len(m.peers) == 0 {
		peerLines = append(peerLines, logEvStyle.Render("  (none)"))
	}
	for _, p := range m.peers {
		age := time.Since(p.Connected).Round(time.Second)
		nameCol := citadelstyle.NameStyle(p.Name).Render(fmt.Sprintf("%-20s", p.Name))
		peerLines = append(peerLines, "  "+nameCol+" "+peerStyle.Render(fmt.Sprintf("%-15s %s", p.RemoteIP, age)))
	}
	suggestions := completer.Compute(m.cmdInput.Value(), serverCommands, serverNameCmds, m.peerNames())

	innerW := m.peerPanelW - 2
	innerH := m.bodyH - 2
	if innerH < 1 {
		innerH = 1
	}
	peerBlock := borderStyle.Width(innerW).Height(innerH).Render(strings.Join(peerLines, "\n"))

	logInnerW := m.logPanelW - 2
	m.logVP.Height = innerH
	scrollbar := citadelstyle.Scrollbar(innerH, m.logVP.TotalLineCount(), m.logVP.ScrollPercent())
	logContent := lipgloss.JoinHorizontal(lipgloss.Top, m.logVP.View(), scrollbar)
	logBlock := borderStyle.Width(logInnerW).Height(innerH).Render(logContent)

	body := lipgloss.JoinHorizontal(lipgloss.Top, peerBlock, " ", logBlock)

	// Input box: command input (normal) or read-only spectator indicator (dashboard drill-in).
	var inputBox string
	if m.readOnly {
		inputBox = inputStyle.Width(m.width - 4).Render(
			"\n" + logEvStyle.Render("  spectator mode  ·  read-only  ·  Esc to exit"))
	} else {
		suggBar := completer.Bar(suggestions, m.suggIdx, m.width-6)
		inputLines := []string{suggBar, m.cmdInput.View()}
		inputBox = inputStyle.Width(m.width - 4).Render(strings.Join(inputLines, "\n"))
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, body, inputBox)
}

// relayout recomputes pane dimensions and resizes the viewport.
// Must be the only place that sets m.logVP.Width / m.logVP.Height.
func (m *TUI) relayout() {
	// Fixed heights: header=1, input box=4 (border+suggbar+input+border), separators=2
	m.bodyH = m.height - 1 - 4 - 2
	if m.bodyH < 3 {
		m.bodyH = 3
	}

	m.peerPanelW = m.width / 3
	m.logPanelW = m.width - m.peerPanelW - 1 // 1-char gap between panes

	// Viewport content fits inside the rounded border (1 char on each edge),
	// minus 1 for the scrollbar gutter.
	m.logVP.Width = max(1, m.logPanelW-3)
	m.logVP.Height = max(1, m.bodyH-2)
	m.logVP.SetContent(strings.Join(m.logLines, "\n"))
	m.logVP.GotoBottom()
}

// applyHubEvent updates model state from a hub notification.
// Called from Update(), so model mutations are safe here.
func (m *TUI) applyHubEvent(ev HubEvent) {
	ts := time.Now().Format("15:04:05")
	named := func(n string) string { return citadelstyle.NameStyle(n).Render(n) }

	switch ev.Kind {
	case EvJoin:
		m.peers = ev.Peers
		m.appendLog(logEvStyle.Render(fmt.Sprintf("[%s] *", ts)) + " " + named(ev.Name) + logEvStyle.Render(" joined"))
	case EvLeave:
		m.peers = ev.Peers
		m.appendLog(logEvStyle.Render(fmt.Sprintf("[%s] *", ts)) + " " + named(ev.Name) + logEvStyle.Render(" left"))
	case EvPeers:
		m.peers = ev.Peers // initial or refreshed peer list from remote source; no log entry

	case EvKick:
		m.peers = ev.Peers
		m.appendLog(logEvStyle.Render(fmt.Sprintf("[%s] *", ts)) + " " + named(ev.Name) + logEvStyle.Render(fmt.Sprintf(" was kicked (%s)", ev.Text)))
	case EvChat:
		m.appendLog(logEvStyle.Render(fmt.Sprintf("[%s]", ts)) + " " + named(ev.Name) + logChatStyle.Render(": "+ev.Text))
	case EvDirect:
		m.appendLog(logEvStyle.Render(fmt.Sprintf("[%s] →", ts)) + " " + named(ev.Name) + logEvStyle.Render(" (to ") + named(ev.Target) + logEvStyle.Render("): ") + logChatStyle.Render(ev.Text))
	case EvSay:
		m.appendLog(logEvStyle.Render(fmt.Sprintf("[%s]", ts)) + " " + serverSayStyle.Render("⚔ "+m.serverName) + logChatStyle.Render(": "+ev.Text))
	case EvMotd:
		m.appendLog(logEvStyle.Render(fmt.Sprintf("[%s] * motd updated: %s", ts, ev.Text)))
	}
}

func (m *TUI) handleAdminCmd(line string) tea.Cmd {
	parts := strings.SplitN(line, " ", 2)
	cmd := strings.ToLower(parts[0])
	arg := ""
	if len(parts) > 1 {
		arg = parts[1]
	}
	switch cmd {
	case "/kick":
		if arg == "" {
			m.addLog("usage: /kick <name>")
		} else if !m.source.Kick(arg, "kicked by admin") {
			m.addLog(errorStyle.Render("client not found: " + arg))
		}
	case "/say":
		if arg == "" {
			m.addLog("usage: /say <text>")
		} else {
			m.source.Say(arg)
		}
	case "/motd":
		m.source.SetMotd(arg)
	case "/quit":
		if m.drillIn {
			return func() tea.Msg { return DrillInExitMsg{} }
		}
		m.quitting = true
		return tea.Quit
	default:
		m.addLog(errorStyle.Render("unknown command: " + cmd))
	}
	return nil
}

// addLog appends a timestamped line and scrolls to the bottom.
func (m *TUI) addLog(line string) {
	ts := time.Now().Format("15:04:05")
	m.appendLog(fmt.Sprintf("[%s] %s", ts, line))
}

func (m *TUI) peerNames() []string {
	names := make([]string, len(m.peers))
	for i, p := range m.peers {
		names[i] = p.Name
	}
	return names
}

// appendLog appends a pre-formatted line to the log and updates the viewport.
func (m *TUI) appendLog(line string) {
	m.logLines = append(m.logLines, line)
	m.logVP.SetContent(strings.Join(m.logLines, "\n"))
	m.logVP.GotoBottom()
}

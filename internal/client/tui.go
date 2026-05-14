package client

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vipul0092/citadel/internal/completer"
	"github.com/vipul0092/citadel/internal/discovery"
	"github.com/vipul0092/citadel/internal/proto"
	citadelstyle "github.com/vipul0092/citadel/internal/style"
)

// --- styles ---

var (
	titleStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	statusStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	selectedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("86")).Bold(true)
	normalStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	errorMsgStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	systemStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true)
	directStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	serverSayStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("86")).Bold(true)
	gamePaneStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Italic(true)
	inputBorder    = lipgloss.NewStyle().BorderStyle(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("62")).Padding(0, 1)
	chatBorder     = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62"))
)

var (
	clientCommands = []string{"/help", "/msg", "/quit", "/who"}
	clientNameCmds = []string{"/msg"}
)

// DrillInExitMsg is sent by the client TUI when /quit is used while embedded in
// the dashboard drill-in. The dashboard intercepts it to exit drill-in mode
// instead of quitting the whole program.
type DrillInExitMsg struct{}

// --- view states ---

type viewState int

const (
	viewDiscovery viewState = iota
	viewNaming
	viewChat
	viewDisconnected
)

// --- tea messages ---

type serverFoundMsg discovery.ServerInfo
type scanDoneMsg struct{}
type autoConnectTickMsg struct{}
type scanTimeoutMsg struct{}
type localProbeResultMsg struct {
	conn *Conn
	err  error
}
type dialResultMsg struct {
	conn *Conn
	err  error
}
type handshakeResultMsg struct {
	err error
}
type netEnvMsg proto.Envelope
type netErrMsg struct{ err error }

func autoConnectTick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return autoConnectTickMsg{} })
}

func probeLocalhostCmd() tea.Cmd {
	return func() tea.Msg {
		conn, err := ProbeLocalhost()
		return localProbeResultMsg{conn: conn, err: err}
	}
}

func scanTimeoutCmd() tea.Cmd {
	return tea.Tick(10*time.Second, func(time.Time) tea.Msg { return scanTimeoutMsg{} })
}

// --- TUI model ---

// TUI is the Bubble Tea model for the client.
type TUI struct {
	state viewState

	// discovery
	scanCh           <-chan discovery.ServerInfo
	servers          []discovery.ServerInfo
	serverIdx        int
	manualAddr       string
	autoConnectSecs  int  // countdown to auto-select the sole discovered server; 0 = off
	scanTimedOut     bool // true after 5s with no servers found via mDNS

	// naming
	nameInput     textinput.Model
	nameErr       string
	pendingServer discovery.ServerInfo

	// chat
	conn       *Conn          // used for pre-handshake lifecycle only
	ctrl       ConnController // used for all post-handshake operations
	serverName string         // set from ctrl.ServerName() at handshake
	chatVP     viewport.Model
	chatInput  textinput.Model
	messages   []string
	peers      []string
	suggIdx    int
	version    string

	width    int
	height   int
	ready    bool
	quitting bool
	lastErr  string
	drillIn  bool // true when embedded in the dashboard; /quit exits drill-in, not the whole program
	readOnly bool // true when spectating from the dashboard; input is disabled
}

// SetDrillIn marks this TUI as embedded in the dashboard. When set, /quit sends
// DrillInExitMsg instead of tea.Quit.
func (m *TUI) SetDrillIn(v bool) { m.drillIn = v }

// SetReadOnly puts the TUI in spectator mode: chat input is disabled and the
// input box is replaced with a read-only indicator.
func (m *TUI) SetReadOnly(v bool) {
	m.readOnly = v
	if v {
		m.chatInput.Blur()
	}
}

// NewTUI creates the client TUI.
// manualAddr: if non-empty, discovery is skipped and we dial this address.
// preFillName: if non-empty, the name prompt is pre-filled.
// scanCh: discovery channel; nil when manualAddr is provided.
// version: build version string shown in the header.
func NewTUI(manualAddr, preFillName string, scanCh <-chan discovery.ServerInfo, version string) *TUI {
	ni := textinput.New()
	ni.Placeholder = "your name (A-Z a-z 0-9 _ -)"
	ni.CharLimit = proto.MaxNameLen
	if preFillName != "" {
		ni.SetValue(preFillName)
	}

	ci := textinput.New()
	ci.Placeholder = "message or /help"

	return &TUI{
		manualAddr: manualAddr,
		scanCh:     scanCh,
		nameInput:  ni,
		chatInput:  ci,
		version:    version,
	}
}

func (m *TUI) Init() tea.Cmd {
	// Drill-in entry: skip discovery entirely, start receiving immediately.
	if m.state == viewChat && m.ctrl != nil {
		return tea.Batch(textinput.Blink, waitForNet(m.ctrl))
	}
	cmds := []tea.Cmd{textinput.Blink}
	if m.manualAddr != "" {
		m.state = viewNaming
		m.pendingServer = discovery.ServerInfo{Addr: m.manualAddr, Name: m.manualAddr}
		m.nameInput.Focus()
	} else if m.scanCh != nil {
		// Probe localhost:7777 immediately (fast-path for same-machine use).
		// Also start mDNS scan in parallel for cross-machine discovery.
		cmds = append(cmds, probeLocalhostCmd(), waitForServer(m.scanCh), scanTimeoutCmd())
	}
	return tea.Batch(cmds...)
}

func (m *TUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.relayout()
		m.ready = true

	case serverFoundMsg:
		// only update discovery state while still in the discovery view
		if m.state != viewDiscovery {
			if m.scanCh != nil {
				cmds = append(cmds, waitForServer(m.scanCh))
			}
			break
		}
		si := discovery.ServerInfo(msg)
		prevCount := len(m.servers)
		found := false
		for i, s := range m.servers {
			if s.Name == si.Name {
				m.servers[i] = si
				found = true
				break
			}
		}
		if !found {
			m.servers = append(m.servers, si)
		}
		switch {
		case prevCount == 0 && len(m.servers) == 1:
			m.autoConnectSecs = 3
			cmds = append(cmds, autoConnectTick())
		case len(m.servers) > 1:
			m.autoConnectSecs = 0
		}
		if m.scanCh != nil {
			cmds = append(cmds, waitForServer(m.scanCh))
		}

	case localProbeResultMsg:
		if msg.err == nil && m.state == viewDiscovery {
			m.conn = msg.conn
			m.pendingServer = discovery.ServerInfo{Name: "localhost", Addr: "localhost:7777"}
			m.autoConnectSecs = 0
			m.state = viewNaming
			m.nameErr = ""
			m.nameInput.Focus()
		}

	case scanDoneMsg:
		// scan channel closed; nothing to do

	case scanTimeoutMsg:
		// Still in discovery with no results — show a hint but keep scanning.
		// Don't give up: broadcast packets may still arrive.
		if m.state == viewDiscovery && len(m.servers) == 0 {
			m.scanTimedOut = true
		}

	case autoConnectTickMsg:
		if m.state == viewDiscovery && m.autoConnectSecs > 0 && len(m.servers) == 1 {
			m.autoConnectSecs--
			if m.autoConnectSecs == 0 {
				m.pendingServer = m.servers[0]
				m.state = viewNaming
				m.nameErr = ""
				m.nameInput.Focus()
			} else {
				cmds = append(cmds, autoConnectTick())
			}
		}

	case dialResultMsg:
		// ignore stale dial results if we've already moved past discovery/naming
		if m.state != viewDiscovery && m.state != viewNaming {
			if msg.conn != nil {
				msg.conn.Close()
			}
			break
		}
		if msg.err != nil {
			// dial failure — show error, stay in discovery and keep scanning
			m.lastErr = fmt.Sprintf("connect to %s failed: %v", m.pendingServer.Addr, msg.err)
			m.state = viewDiscovery
		} else {
			m.conn = msg.conn
			if m.state == viewNaming {
				// normal path: user already typed a name
				name := strings.TrimSpace(m.nameInput.Value())
				cmds = append(cmds, doHandshake(m.conn, name, m.version))
			} else {
				// fallback path: dial succeeded, ask for name
				m.state = viewNaming
				m.nameErr = ""
				m.nameInput.Focus()
			}
		}

	case handshakeResultMsg:
		if msg.err != nil {
			m.nameErr = msg.err.Error()
			m.conn = nil
		} else {
			m.ctrl = NewInProcessController(m.conn)
			m.serverName = m.ctrl.ServerName()
			m.state = viewChat
			m.peers = m.ctrl.Peers()
			motd := m.ctrl.Motd()
			if motd != "" {
				m.addMessage(systemStyle.Render("📋 " + motd))
			}
			m.chatInput.Focus()
			cmds = append(cmds, waitForNet(m.ctrl))
		}

	case netEnvMsg:
		env := proto.Envelope(msg)
		m.handleNetMsg(&env)
		if m.ctrl != nil {
			cmds = append(cmds, waitForNet(m.ctrl))
		}

	case netErrMsg:
		m.state = viewDisconnected
		m.lastErr = msg.err.Error()
		m.conn = nil
		m.ctrl = nil

	case tea.KeyMsg:
		cmds = append(cmds, m.handleKey(msg)...)
	}

	// update sub-models
	var vpCmd, niCmd, ciCmd tea.Cmd
	m.chatVP, vpCmd = m.chatVP.Update(msg)
	m.nameInput, niCmd = m.nameInput.Update(msg)
	m.chatInput, ciCmd = m.chatInput.Update(msg)
	cmds = append(cmds, vpCmd, niCmd, ciCmd)

	return m, tea.Batch(cmds...)
}

func (m *TUI) handleKey(msg tea.KeyMsg) []tea.Cmd {
	var cmds []tea.Cmd

	switch m.state {
	case viewDiscovery:
		switch msg.Type {
		case tea.KeyCtrlC:
			m.quitting = true
			cmds = append(cmds, tea.Quit)
		case tea.KeyUp:
			m.autoConnectSecs = 0
			if m.serverIdx > 0 {
				m.serverIdx--
			}
		case tea.KeyDown:
			m.autoConnectSecs = 0
			if m.serverIdx < len(m.servers)-1 {
				m.serverIdx++
			}
		case tea.KeyEnter:
			m.autoConnectSecs = 0
			if len(m.servers) > 0 {
				m.pendingServer = m.servers[m.serverIdx]
				m.state = viewNaming
				m.nameErr = ""
				m.nameInput.SetValue("")
				m.nameInput.Focus()
			}
		case tea.KeyRunes:
			m.autoConnectSecs = 0
			if msg.String() == "q" {
				m.quitting = true
				cmds = append(cmds, tea.Quit)
			}
		case tea.KeyEsc:
			m.autoConnectSecs = 0
		}

	case viewNaming:
		switch msg.Type {
		case tea.KeyCtrlC:
			m.quitting = true
			cmds = append(cmds, tea.Quit)
		case tea.KeyEsc:
			if m.manualAddr == "" {
				m.state = viewDiscovery
			}
		case tea.KeyEnter:
			name := strings.TrimSpace(m.nameInput.Value())
			if err := proto.ValidateName(name); err != nil {
				m.nameErr = err.Error()
				break
			}
			m.nameErr = ""
			if m.conn != nil {
				// already have a connection (localhost probe) — handshake directly
				cmds = append(cmds, doHandshake(m.conn, name, m.version))
			} else {
				cmds = append(cmds, doDial(m.pendingServer.Addr))
			}
		}

	case viewChat:
		switch msg.Type {
		case tea.KeyCtrlC:
			if m.ctrl != nil {
				m.ctrl.Close()
			}
			m.quitting = true
			cmds = append(cmds, tea.Quit)
		case tea.KeyEnter:
			if !m.readOnly {
				m.suggIdx = 0
				line := strings.TrimSpace(m.chatInput.Value())
				m.chatInput.SetValue("")
				if line != "" {
					cmds = append(cmds, m.handleChatInput(line)...)
				}
			}
		case tea.KeyTab:
			if !m.readOnly {
				val := m.chatInput.Value()
				suggestions := completer.Compute(val, clientCommands, clientNameCmds, m.peers)
				if len(suggestions) > 0 {
					m.chatInput.SetValue(completer.Complete(val, suggestions, m.suggIdx))
					m.chatInput.CursorEnd()
					m.suggIdx = 0
				}
			}
		case tea.KeyUp:
			if !m.readOnly {
				suggestions := completer.Compute(m.chatInput.Value(), clientCommands, clientNameCmds, m.peers)
				if len(suggestions) > 0 {
					m.suggIdx--
					if m.suggIdx < 0 {
						m.suggIdx = len(suggestions) - 1
					}
					break
				}
			}
			m.chatVP.ScrollUp(1)
		case tea.KeyDown:
			if !m.readOnly {
				suggestions := completer.Compute(m.chatInput.Value(), clientCommands, clientNameCmds, m.peers)
				if len(suggestions) > 0 {
					m.suggIdx++
					if m.suggIdx >= len(suggestions) {
						m.suggIdx = 0
					}
					break
				}
			}
			m.chatVP.ScrollDown(1)
		case tea.KeyPgUp:
			m.chatVP.ScrollUp(5)
		case tea.KeyPgDown:
			m.chatVP.ScrollDown(5)
		default:
			m.suggIdx = 0
		}

	case viewDisconnected:
		m.quitting = true
		return []tea.Cmd{tea.Quit}
	}

	return cmds
}

func (m *TUI) handleChatInput(line string) []tea.Cmd {
	cmd := Parse(line)
	switch cmd.Kind {
	case CmdQuit:
		if m.drillIn {
			if m.ctrl != nil {
				m.ctrl.Close()
			}
			return []tea.Cmd{func() tea.Msg { return DrillInExitMsg{} }}
		}
		if m.ctrl != nil {
			m.ctrl.Close()
		}
		m.quitting = true
		return []tea.Cmd{tea.Quit}

	case CmdHelp:
		m.addMessage(systemStyle.Render(HelpText))

	case CmdWho:
		if len(m.peers) == 0 {
			m.addMessage(systemStyle.Render("(no other peers connected)"))
		} else {
			m.addMessage(systemStyle.Render("Connected: " + strings.Join(m.peers, ", ")))
		}

	case CmdMsg:
		if m.ctrl != nil {
			_ = m.ctrl.Send(proto.TypeChat, cmd.Target, proto.ChatPayload{Text: cmd.Text, To: cmd.Target})
			ts := time.Now().Format("15:04:05")
			m.addMessage(directStyle.Render(fmt.Sprintf("[%s] → %s (private): %s", ts, cmd.Target, cmd.Text)))
		}

	default:
		if cmd.Text != "" && !strings.HasPrefix(cmd.Text, "unknown command") {
			// plain chat
			if m.ctrl != nil {
				_ = m.ctrl.Send(proto.TypeChat, "", proto.ChatPayload{Text: cmd.Text})
			}
		} else if strings.HasPrefix(cmd.Text, "unknown command") || strings.HasPrefix(cmd.Text, "usage") {
			m.addMessage(errorMsgStyle.Render(cmd.Text))
		}
	}
	return nil
}

func (m *TUI) handleNetMsg(env *proto.Envelope) {
	ts := time.Now().Format("15:04:05")
	switch env.Type {
	case proto.TypeChat:
		var p proto.ChatPayload
		if err := proto.UnmarshalPayload(env, &p); err != nil {
			return
		}
		if env.From == "server" {
			m.addMessage(fmt.Sprintf("[%s] ", ts) + serverSayStyle.Render("⚔ "+m.serverName+":") + " " + normalStyle.Render(p.Text))
		} else if env.To != "" {
			// direct message — color the sender name
			sender := citadelstyle.NameStyle(env.From).Render(env.From)
			m.addMessage(fmt.Sprintf("[%s] → %s (private): %s", ts, sender, p.Text))
		} else {
			// broadcast — name in their color, message text in neutral white
			sender := citadelstyle.NameStyle(env.From).Render(env.From)
			m.addMessage(fmt.Sprintf("[%s] %s: %s", ts, sender, p.Text))
		}

	case proto.TypeSystem:
		var p proto.SystemPayload
		if err := proto.UnmarshalPayload(env, &p); err != nil {
			return
		}
		switch p.Event {
		case proto.EvJoin:
			m.peers = append(m.peers, p.Name)
			m.addMessage(systemStyle.Render(fmt.Sprintf("[%s] * %s joined", ts, p.Name)))
		case proto.EvLeave:
			m.removePeer(p.Name)
			m.addMessage(systemStyle.Render(fmt.Sprintf("[%s] * %s left", ts, p.Name)))
		case proto.EvKick:
			m.removePeer(p.Name)
			msg := fmt.Sprintf("[%s] * %s was kicked", ts, p.Name)
			if p.Message != "" {
				msg += " (" + p.Message + ")"
			}
			m.addMessage(systemStyle.Render(msg))
		case proto.EvMotd:
			m.addMessage(systemStyle.Render(fmt.Sprintf("[%s] 📋 %s", ts, p.Message)))
		case proto.EvTooLong:
			m.addMessage(errorMsgStyle.Render("[" + ts + "] message too long (max 2 KB)"))
		}

	case proto.TypeKick:
		var p proto.KickPayload
		_ = proto.UnmarshalPayload(env, &p)
		m.state = viewDisconnected
		m.lastErr = "you were kicked: " + p.Reason
		m.conn = nil
		m.ctrl = nil

	case proto.TypePong:
		// heartbeat; no display needed

	case proto.TypeGame:
		// placeholder until Phase 6
	}
}

func (m *TUI) View() string {
	if !m.ready {
		return "Initialising…\n"
	}
	if m.quitting {
		return ""
	}

	switch m.state {
	case viewDiscovery:
		return m.viewDiscovery()
	case viewNaming:
		return m.viewNaming()
	case viewChat:
		return m.viewChat()
	case viewDisconnected:
		return m.viewDisconnected()
	}
	return ""
}

func (m *TUI) viewDiscovery() string {
	title := titleStyle.Render(" ⚔  Citadel  —  Select a server")

	var hint string
	if m.autoConnectSecs > 0 && len(m.servers) == 1 {
		hint = selectedStyle.Render(fmt.Sprintf(
			"  Auto-connecting to %s in %ds…  press any key to choose manually",
			m.servers[0].Name, m.autoConnectSecs,
		))
	} else {
		hint = statusStyle.Render("  ↑/↓ navigate   Enter connect   q quit")
	}

	var rows []string
	if len(m.servers) == 0 {
		rows = append(rows, statusStyle.Render("  Scanning for servers…"))
		if m.scanTimedOut {
			rows = append(rows, errorMsgStyle.Render("  Nothing found yet. Is the server on the same network?"))
			rows = append(rows, statusStyle.Render("  Tip: use --server <host:port> if auto-discovery doesn't work"))
		}
	} else {
		for i, s := range m.servers {
			line := fmt.Sprintf("  %s  %s", s.Name, statusStyle.Render(s.Addr))
			if i == m.serverIdx {
				line = selectedStyle.Render("▶ " + strings.TrimPrefix(line, "  "))
			} else {
				line = normalStyle.Render(line)
			}
			rows = append(rows, line)
		}
	}
	if m.lastErr != "" {
		rows = append(rows, errorMsgStyle.Render("  Error: "+m.lastErr))
	}
	body := chatBorder.Width(m.width - 4).Render(strings.Join(rows, "\n"))
	return lipgloss.JoinVertical(lipgloss.Left, title, hint, body)
}

func (m *TUI) viewNaming() string {
	title := titleStyle.Render(fmt.Sprintf(" ⚔  Citadel  —  connecting to %s", m.pendingServer.Name))
	addrLine := statusStyle.Render(fmt.Sprintf("  address: %s", m.pendingServer.Addr))
	prompt := normalStyle.Render("  Enter your name: ") + m.nameInput.View()
	hint := statusStyle.Render("  1–24 chars  A-Z a-z 0-9 _ -   Enter connect   Esc back")
	parts := []string{title, addrLine, prompt, hint}
	if m.nameErr != "" {
		parts = append(parts, errorMsgStyle.Render("  "+m.nameErr))
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m *TUI) viewChat() string {
	myName := ""
	if m.ctrl != nil {
		myName = m.ctrl.Name()
	}
	serverName := m.serverName

	// Header: server name + client's own name in their assigned color + version
	myNameStyled := citadelstyle.NameStyle(myName).Render(myName)
	header := titleStyle.Render(" ⚔  Citadel") +
		statusStyle.Render("  connected to: ") +
		titleStyle.Render(serverName) +
		statusStyle.Render("  │  you are ") +
		myNameStyled +
		statusStyle.Render(fmt.Sprintf("  │  %d peers  │  %s", len(m.peers), m.version))

	// inputBoxH is always fixed: 2 content lines + top/bottom border
	const inputBoxH = 4

	// Input box: either a live input (normal) or a read-only spectator indicator (dashboard drill-in).
	var inputBox string
	if m.readOnly {
		inputBox = inputBorder.Width(m.width - 4).Render(
			"\n" + statusStyle.Render("  spectator mode  ·  read-only  ·  Esc to exit"))
	} else {
		suggestions := completer.Compute(m.chatInput.Value(), clientCommands, clientNameCmds, m.peers)
		suggBar := completer.Bar(suggestions, m.suggIdx, m.width-6)
		inputLine := citadelstyle.NameStyle(myName).Render(myName) + statusStyle.Render(" > ") + m.chatInput.View()
		inputLines := []string{suggBar, inputLine}
		inputBox = inputBorder.Width(m.width - 4).Render(strings.Join(inputLines, "\n"))
	}

	// Fixed heights: header=1, game placeholder=1, separators=2
	const gamePaneH = 1
	chatH := m.height - 1 - inputBoxH - gamePaneH - 3
	if chatH < 1 {
		chatH = 1
	}

	m.chatVP.Width = m.width - 5
	m.chatVP.Height = chatH
	m.chatVP.SetContent(strings.Join(m.messages, "\n"))

	scrollbar := citadelstyle.Scrollbar(chatH, m.chatVP.TotalLineCount(), m.chatVP.ScrollPercent())
	chatContent := lipgloss.JoinHorizontal(lipgloss.Top, m.chatVP.View(), scrollbar)
	chatPane := chatBorder.Width(m.width - 4).Height(chatH).Render(chatContent)
	gamePaneLine := gamePaneStyle.Render("  [game: not connected]")

	return lipgloss.JoinVertical(lipgloss.Left, header, chatPane, gamePaneLine, inputBox)
}

func (m *TUI) viewDisconnected() string {
	msg := errorMsgStyle.Render("  Disconnected: " + m.lastErr)
	hint := statusStyle.Render("  Press any key to exit")
	return lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render(" ⚔  Citadel"),
		msg, hint,
	)
}

// --- helpers ---

func (m *TUI) relayout() {
	inputH := 4 // border + suggestion bar + input + border
	chatH := m.height - 4 - inputH
	if chatH < 1 {
		chatH = 1
	}
	m.chatVP = viewport.New(m.width-5, chatH)
	m.chatVP.SetContent(strings.Join(m.messages, "\n"))
	m.chatVP.GotoBottom()
}

func (m *TUI) addMessage(line string) {
	m.messages = append(m.messages, line)
	m.chatVP.SetContent(strings.Join(m.messages, "\n"))
	m.chatVP.GotoBottom()
}

func (m *TUI) removePeer(name string) {
	updated := m.peers[:0]
	for _, p := range m.peers {
		if p != name {
			updated = append(updated, p)
		}
	}
	m.peers = updated
}

// --- tea.Cmd factories ---

func waitForServer(ch <-chan discovery.ServerInfo) tea.Cmd {
	return func() tea.Msg {
		si, ok := <-ch
		if !ok {
			return scanDoneMsg{}
		}
		return serverFoundMsg(si)
	}
}

func doDial(addr string) tea.Cmd {
	return func() tea.Msg {
		conn, err := Dial(addr)
		return dialResultMsg{conn: conn, err: err}
	}
}

func doHandshake(conn *Conn, name, version string) tea.Cmd {
	return func() tea.Msg {
		err := conn.Handshake(name, version)
		return handshakeResultMsg{err: err}
	}
}

func waitForNet(ctrl ConnController) tea.Cmd {
	return func() tea.Msg {
		env, err := ctrl.Recv()
		if err != nil {
			return netErrMsg{err: err}
		}
		return netEnvMsg(*env)
	}
}

// NewTUIFromController creates the client chat TUI backed by a ConnController.
// Used by the dashboard for remote drill-in (starts directly in viewChat).
func NewTUIFromController(ctrl ConnController, displayServerName, myName, version string) *TUI {
	ci := textinput.New()
	ci.Placeholder = "message or /help"
	ci.Focus()
	return &TUI{
		state:      viewChat,
		ctrl:       ctrl,
		serverName: displayServerName,
		peers:      ctrl.Peers(),
		version:    version,
		chatInput:  ci,
	}
}


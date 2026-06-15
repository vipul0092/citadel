package dashboard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vipul0092/citadel/internal/client"
	"github.com/vipul0092/citadel/internal/control"
	ctrlclient "github.com/vipul0092/citadel/internal/control/client"
	"github.com/vipul0092/citadel/internal/server"
)

// --- message types ---

type modalState int

const (
	modalNone        modalState = iota
	modalHost                   // [h] — lobby name, your name, port
	modalConnect                // [c] — server addr, your name
	modalServerOnly             // [s] — server name, port
	modalKillConfirm            // [k] — confirm kill
)

type instanceEvent struct {
	sockPath string
	ev       control.Event
}

type killTimerMsg struct{ sockPath string }
type scanResultMsg []control.SentinelInfo

type pendingRestart struct{ args []string }

// --- core types ---

// Instance represents one running citadel process tracked by the dashboard.
type Instance struct {
	Info      control.SentinelInfo
	conn      *ctrlclient.Subscriber
	peerCount int
	starred   bool
	// live is set true when KindLive is received, marking the end of ring-buffer
	// replay. peer-join/peer-leave increments are only applied after that point;
	// replayed events are ignored so they don't overcount the KindPeers snapshot.
	live bool
}

// --- scan and reconcile ---

func doScan() tea.Cmd {
	return func() tea.Msg {
		results, _ := control.ScanSentinels()
		return scanResultMsg(results)
	}
}

func scanTick() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return doScan()()
	})
}

func waitForEvent(ch chan instanceEvent) tea.Cmd {
	return func() tea.Msg {
		return <-ch
	}
}

func (m DashboardModel) reconcile(fresh []control.SentinelInfo) DashboardModel {
	starred := starredPids()

	// Index new sentinels by sockPath.
	freshMap := make(map[string]control.SentinelInfo, len(fresh))
	for _, s := range fresh {
		freshMap[s.SockPath] = s
	}

	// Index existing instances by sockPath.
	existing := make(map[string]Instance, len(m.instances))
	for _, inst := range m.instances {
		existing[inst.Info.SockPath] = inst
	}

	// Close connections for gone instances; fire pending restarts.
	for sp, inst := range existing {
		if _, ok := freshMap[sp]; !ok {
			if inst.conn != nil {
				inst.conn.Close()
			}
			// Check if a restart is pending for this instance.
			if pr, ok := m.restarts[sp]; ok {
				delete(m.restarts, sp)
				_ = spawnDetached(pr.args...)
			}
		}
	}

	// Build new instances list.
	result := make([]Instance, 0, len(fresh))
	for _, s := range fresh {
		if inst, ok := existing[s.SockPath]; ok {
			// Existing: update info + ★
			inst.Info = s
			inst.starred = starred[s.Pid]
			result = append(result, inst)
		} else {
			// New: open subscription.
			inst := Instance{Info: s, starred: starred[s.Pid]}
			inst.conn = openSubscription(s.SockPath, m.eventsCh)
			result = append(result, inst)
		}
	}

	m.instances = result
	if m.selected >= len(m.instances) {
		m.selected = max(0, len(m.instances)-1)
	}
	return m
}

func (m DashboardModel) applyEvent(ev instanceEvent) DashboardModel {
	instances := make([]Instance, len(m.instances))
	copy(instances, m.instances)
	for i := range instances {
		if instances[i].Info.SockPath != ev.sockPath {
			continue
		}
		switch ev.ev.Kind {
		case control.KindLive:
			instances[i].live = true
		case control.KindPeers:
			// Authoritative snapshot from list-peers — always trust it.
			instances[i].peerCount = len(ev.ev.Peers)
		case control.KindPeerJoin:
			// Only apply incremental changes once we're past the ring-buffer replay.
			if instances[i].live {
				instances[i].peerCount++
			}
		case control.KindPeerLeave, control.KindKick:
			if instances[i].live && instances[i].peerCount > 0 {
				instances[i].peerCount--
			}
		}
		break
	}
	m.instances = instances
	return m
}

func (m DashboardModel) drillIn() (DashboardModel, tea.Cmd) {
	inst := m.instances[m.selected]
	sizeMsg := tea.WindowSizeMsg{Width: m.width, Height: m.height}

	switch inst.Info.Role {
	case "server":
		src, err := server.NewRemoteSource(inst.Info.SockPath, inst.Info.Name, inst.Info.Addr)
		if err != nil {
			return m, nil
		}
		sub := server.NewTUIFromSource(src, inst.Info.Name, inst.Info.Addr, m.version)
		sub.SetDrillIn(true)
		sub.SetReadOnly(true)
		m.active = sub
		m.active, _ = m.active.Update(sizeMsg)
		return m, sub.Init()

	case "client":
		serverDisplay := inst.Info.ServerName
		if serverDisplay == "" {
			serverDisplay = inst.Info.Addr
		}
		ctrl, err := client.NewRemoteController(inst.Info.SockPath, inst.Info.Name, serverDisplay)
		if err != nil {
			return m, nil
		}
		sub := client.NewTUIFromController(ctrl, serverDisplay, inst.Info.Name, m.version)
		sub.SetDrillIn(true)
		sub.SetReadOnly(true)
		m.active = sub
		m.active, _ = m.active.Update(sizeMsg)
		return m, sub.Init()
	}
	return m, nil
}

func (m DashboardModel) isAlive(sockPath string) bool {
	for _, inst := range m.instances {
		if inst.Info.SockPath == sockPath {
			return true
		}
	}
	return false
}

func starredPids() map[int]bool {
	pids := make(map[int]bool)
	home, err := os.UserHomeDir()
	if err != nil {
		return pids
	}
	// host/current.json
	if data, err := os.ReadFile(filepath.Join(home, ".citadel", "host", "current.json")); err == nil {
		var m struct {
			ServerPid int `json:"server_pid"`
			ClientPid int `json:"client_pid"`
		}
		if json.Unmarshal(data, &m) == nil {
			if m.ServerPid > 0 {
				pids[m.ServerPid] = true
			}
			if m.ClientPid > 0 {
				pids[m.ClientPid] = true
			}
		}
	}
	// client/current.json
	if data, err := os.ReadFile(filepath.Join(home, ".citadel", "client", "current.json")); err == nil {
		var m struct {
			ClientPid int `json:"client_pid"`
		}
		if json.Unmarshal(data, &m) == nil && m.ClientPid > 0 {
			pids[m.ClientPid] = true
		}
	}
	return pids
}

func openSubscription(sockPath string, eventsCh chan instanceEvent) *ctrlclient.Subscriber {
	sub, err := ctrlclient.DialAndSubscribe(sockPath, "summary", 0)
	if err != nil {
		return nil
	}
	// Request current peer list after subscribing.
	_ = sub.Send(map[string]any{"op": "list-peers"})

	go func() {
		for ev := range sub.Events() {
			eventsCh <- instanceEvent{sockPath: sockPath, ev: ev}
		}
	}()
	return sub
}


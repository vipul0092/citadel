package dashboard

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// spawnDetached starts the citadel binary with the given args in a new process group.
func spawnDetached(args ...string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stderr = os.Stderr
	return cmd.Start()
}

// sendSigterm sends SIGTERM to the process with the given pid.
func sendSigterm(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Signal(syscall.SIGTERM)
}

// submitModal validates inputs and spawns the appropriate process.
func submitModal(m DashboardModel) (DashboardModel, tea.Cmd) {
	switch m.modal {
	case modalHost:
		lobbyName := m.inputs[0].Value()
		myName := m.inputs[1].Value()
		port := m.inputs[2].Value()
		if lobbyName == "" || myName == "" {
			m.inputErr = "lobby name and your name are required"
			return m, nil
		}
		if port == "" {
			port = "7777"
		}
		m.modal = modalNone
		args := []string{"host", "--name", lobbyName, "--my-name", myName, "--port", port}
		_ = spawnDetached(args...)

	case modalConnect:
		serverAddr := m.inputs[0].Value()
		myName := m.inputs[1].Value()
		if serverAddr == "" || myName == "" {
			m.inputErr = "server address and your name are required"
			return m, nil
		}
		m.modal = modalNone
		args := []string{"connect", "--headless", "--server", serverAddr, "--name", myName}
		_ = spawnDetached(args...)

	case modalServerOnly:
		serverName := m.inputs[0].Value()
		port := m.inputs[1].Value()
		if serverName == "" {
			m.inputErr = "server name is required"
			return m, nil
		}
		if port == "" {
			port = "7777"
		}
		m.modal = modalNone
		args := []string{"server", "--headless", "--name", serverName, "--port", port}
		_ = spawnDetached(args...)
	}
	return m, nil
}

// startKill sends the shutdown op and queues a kill timer.
func startKill(m DashboardModel) (DashboardModel, tea.Cmd) {
	if len(m.instances) == 0 || m.selected >= len(m.instances) {
		return m, nil
	}
	inst := m.instances[m.selected]
	sockPath := inst.Info.SockPath

	if inst.conn != nil {
		_ = inst.conn.Send(map[string]any{"op": "shutdown"})
	}
	m.killing[sockPath] = true

	return m, tea.Tick(3*time.Second, func(time.Time) tea.Msg {
		return killTimerMsg{sockPath: sockPath}
	})
}

// doRestart marks the selected instance for restart after it exits.
func doRestart(m DashboardModel) (DashboardModel, tea.Cmd) {
	if len(m.instances) == 0 || m.selected >= len(m.instances) {
		return m, nil
	}
	inst := m.instances[m.selected]
	if len(inst.Info.Args) == 0 {
		return m, nil
	}

	m.restarts[inst.Info.SockPath] = pendingRestart{args: inst.Info.Args}
	return startKill(m)
}

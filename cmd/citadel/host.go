package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/vipul0092/citadel/internal/control"
)

func runHost(args []string) {
	fs := flag.NewFlagSet("host", flag.ExitOnError)
	name := fs.String("name", "", "lobby/server name (required)")
	myName := fs.String("my-name", "", "captain's client name (required)")
	port := fs.Int("port", 7777, "TCP listen port")
	motd := fs.String("motd", "", "message of the day")
	_ = fs.Parse(args)

	if *name == "" {
		fmt.Fprintln(os.Stderr, "error: --name is required")
		os.Exit(1)
	}
	if *myName == "" {
		fmt.Fprintln(os.Stderr, "error: --my-name is required")
		os.Exit(1)
	}

	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: resolve executable: %v\n", err)
		os.Exit(1)
	}

	portStr := fmt.Sprintf("%d", *port)

	// --- Spawn server ---
	srvArgs := []string{"server", "--headless", "--name", *name, "--port", portStr}
	if *motd != "" {
		srvArgs = append(srvArgs, "--motd", *motd)
	}
	srvCmd := exec.Command(exe, srvArgs...)
	srvCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	srvCmd.Stderr = os.Stderr
	if err := srvCmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "error: start server: %v\n", err)
		os.Exit(1)
	}
	slog.Info("server started", "pid", srvCmd.Process.Pid, "name", *name)

	// Wait for the server sentinel to appear (up to 10 s).
	srvCtx, srvCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer srvCancel()
	serverInfo, err := control.WaitForSentinel(srvCtx, "server", *name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: server sentinel not found: %v\n", err)
		_ = srvCmd.Process.Signal(syscall.SIGTERM)
		os.Exit(1)
	}
	slog.Info("server sentinel found", "sock", serverInfo.SockPath)

	// --- Spawn client ---
	cliCmd := exec.Command(exe,
		"connect", "--headless",
		"--server", "localhost:"+portStr,
		"--name", *myName,
	)
	cliCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cliCmd.Stderr = os.Stderr
	if err := cliCmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "error: start client: %v\n", err)
		_ = srvCmd.Process.Signal(syscall.SIGTERM)
		os.Exit(1)
	}
	slog.Info("client started", "pid", cliCmd.Process.Pid, "name", *myName)

	// Wait for the client sentinel to appear (up to 10 s).
	cliCtx, cliCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cliCancel()
	clientInfo, err := control.WaitForSentinel(cliCtx, "client", *myName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: client sentinel not found: %v\n", err)
		_ = cliCmd.Process.Signal(syscall.SIGTERM)
		_ = srvCmd.Process.Signal(syscall.SIGTERM)
		os.Exit(1)
	}
	slog.Info("client sentinel found", "sock", clientInfo.SockPath)

	// Write host pointer file.
	ptr, err := control.WriteHostPointer(serverInfo, clientInfo, *myName)
	if err != nil {
		slog.Warn("host pointer write failed", "err", err)
	}
	defer ptr.Unlink()

	slog.Info("host session ready",
		"lobby", *name,
		"captain", *myName,
		"server_sock", serverInfo.SockPath,
		"client_sock", clientInfo.SockPath,
	)

	// Monitor both children; exit if either dies or we receive a signal.
	serverDone := make(chan error, 1)
	clientDone := make(chan error, 1)
	go func() { serverDone <- srvCmd.Wait() }()
	go func() { clientDone <- cliCmd.Wait() }()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverDone:
		if err != nil {
			slog.Warn("server exited unexpectedly", "err", err)
		}
		slog.Info("server exited; terminating client")
		_ = cliCmd.Process.Signal(syscall.SIGTERM)
		<-clientDone

	case err := <-clientDone:
		if err != nil {
			slog.Warn("client exited unexpectedly", "err", err)
		}
		slog.Info("client exited; terminating server")
		_ = srvCmd.Process.Signal(syscall.SIGTERM)
		<-serverDone

	case sig := <-sigCh:
		slog.Info("received signal; shutting down", "signal", sig)
		_ = srvCmd.Process.Signal(syscall.SIGTERM)
		_ = cliCmd.Process.Signal(syscall.SIGTERM)
		<-serverDone
		<-clientDone
	}

	slog.Info("host session ended")
}

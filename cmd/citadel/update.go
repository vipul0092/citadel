package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

func runUpdate() {
	brewPath, err := exec.LookPath("brew")
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: brew not found — citadel update requires Homebrew (https://brew.sh)")
		os.Exit(1)
	}

	fmt.Println("Updating Homebrew tap...")
	update := exec.Command(brewPath, "update")
	update.Stdout = os.Stdout
	update.Stderr = os.Stderr
	if err := update.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: brew update failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Upgrading citadel...")
	upgrade := exec.Command(brewPath, "upgrade", "citadel")
	var upgradeBuf strings.Builder
	upgrade.Stdout = io.MultiWriter(os.Stdout, &upgradeBuf)
	upgrade.Stderr = io.MultiWriter(os.Stderr, &upgradeBuf)
	if err := upgrade.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: brew upgrade failed: %v\n", err)
		os.Exit(1)
	}

	ver := installedVersion(brewPath)
	if strings.Contains(upgradeBuf.String(), "already installed") {
		fmt.Printf("Already up to date! Current version: %s\n", ver)
	} else {
		fmt.Printf("Done! Updated to version: %s\n", ver)
	}
}

func installedVersion(brewPath string) string {
	out, err := exec.Command(brewPath, "info", "--cask", "citadel").Output()
	if err != nil {
		return "unknown"
	}
	// First line: "==> citadel (citadel): 0.0.6"
	line := strings.SplitN(string(out), "\n", 2)[0]
	if i := strings.LastIndex(line, ": "); i >= 0 {
		return strings.TrimSpace(line[i+2:])
	}
	return "unknown"
}

package main

import (
	"bytes"
	"fmt"
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

	fmt.Println("Checking for updates...")
	var updateOut bytes.Buffer
	update := exec.Command(brewPath, "update")
	update.Stdout = &updateOut
	update.Stderr = &updateOut
	if err := update.Run(); err != nil {
		fmt.Fprint(os.Stderr, updateOut.String())
		fmt.Fprintf(os.Stderr, "error: brew update failed: %v\n", err)
		os.Exit(1)
	}

	var upgradeOut bytes.Buffer
	upgrade := exec.Command(brewPath, "upgrade", "citadel")
	upgrade.Stdout = &upgradeOut
	upgrade.Stderr = &upgradeOut
	if err := upgrade.Run(); err != nil {
		fmt.Fprint(os.Stderr, upgradeOut.String())
		fmt.Fprintf(os.Stderr, "error: brew upgrade failed: %v\n", err)
		os.Exit(1)
	}

	ver := installedVersion(brewPath)
	if strings.Contains(upgradeOut.String(), "already installed") {
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

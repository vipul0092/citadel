package main

import (
	"fmt"
	"os"
)

const version = "0.1.1"

// versionString returns the full version label shown in TUI headers and `citadel version`.
func versionString() string {
	return "v" + version
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}
	switch os.Args[1] {
	case "server":
		runServer(os.Args[2:])
	case "connect":
		runClient(os.Args[2:])
	case "host":
		runHost(os.Args[2:])
	case "dashboard":
		runDashboard(os.Args[2:])
	case "test":
		runTest(os.Args[2:])
	case "update":
		runUpdate()
	case "version", "--version", "-v":
		fmt.Println(versionString())
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage: citadel <subcommand> [flags]

Subcommands:
  server   Start a Citadel server
  connect  Connect to a Citadel server
  host      Spawn a matched server+client pair (captain mode)
  dashboard Show all running citadel processes, launch/kill/restart
  test      Drive a running session from the terminal (dev tool)
  update   Update citadel to the latest version (requires Homebrew)
  version  Print version

Server flags:
  --name         Server display name (required)
  --port         TCP listen port (default 7777)
  --motd         Message of the day shown to clients on join
  --max-clients  Max simultaneous clients (default 16)
  --log-file     Activity log path (default ~/.citadel/<name>/activity.log)
                 Set to "" to disable
  --headless     Run without TUI; logs to stderr (control plane stays active)

Connect flags:
  --server    Skip mDNS discovery; connect directly to host:port
  --name      Pre-fill the name prompt
  --headless  Connect without TUI; --server and --name required;
              writes ~/.citadel/client/current.json

Host flags:
  --name      Lobby/server name (required)
  --my-name   Captain's client name (required)
  --port      TCP listen port (default 7777)
  --motd      Message of the day (optional)
`)
}

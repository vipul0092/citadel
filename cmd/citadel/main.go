package main

import (
	"fmt"
	"os"
)

const version = "0.0.2"

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
  version  Print version and build timestamp

Server flags:
  --name         Server display name (required)
  --port         TCP listen port (default 7777)
  --motd         Message of the day shown to clients on join
  --max-clients  Max simultaneous clients (default 16)
  --log-file     Activity log path (default ~/.citadel/<name>/activity.log)
                 Set to "" to disable

Connect flags:
  --server  Skip mDNS discovery; connect directly to host:port
  --name    Pre-fill the name prompt
`)
}

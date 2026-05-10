package client

import (
	"strings"
)

// CommandKind identifies a parsed slash command.
type CommandKind int

const (
	CmdNone CommandKind = iota
	CmdWho
	CmdMsg
	CmdHelp
	CmdQuit
)

// Command is the result of parsing a slash command.
type Command struct {
	Kind   CommandKind
	Target string // /msg target
	Text   string // /msg text or plain chat text
	Raw    string // original input
}

// Parse parses one line of user input. If the input starts with '/' it is
// treated as a command; otherwise it is a plain chat message.
func Parse(input string) Command {
	input = strings.TrimSpace(input)
	if input == "" {
		return Command{Kind: CmdNone, Raw: input}
	}
	if !strings.HasPrefix(input, "/") {
		return Command{Kind: CmdNone, Text: input, Raw: input}
	}

	parts := strings.SplitN(input, " ", 3)
	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "/who":
		return Command{Kind: CmdWho, Raw: input}
	case "/quit":
		return Command{Kind: CmdQuit, Raw: input}
	case "/help":
		return Command{Kind: CmdHelp, Raw: input}
	case "/msg":
		if len(parts) < 3 {
			return Command{Kind: CmdNone, Text: "usage: /msg <name> <text>", Raw: input}
		}
		return Command{Kind: CmdMsg, Target: parts[1], Text: parts[2], Raw: input}
	default:
		return Command{Kind: CmdNone, Text: "unknown command: " + cmd + " (try /help)", Raw: input}
	}
}

// HelpText returns the client help string.
const HelpText = `/who        — list connected peers
/msg <name> <text>  — send a direct message
/help       — show this help
/quit       — disconnect and exit`

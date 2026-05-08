package tui

import (
	"fmt"
	"os/exec"
	"runtime"

	tea "github.com/charmbracelet/bubbletea"
)

type browserOpenedMsg struct {
	URL string
}

type browserOpenFailedMsg struct {
	URL string
	Err error
}

func openBrowserCommand(rawURL string) tea.Cmd {
	return func() tea.Msg {
		if err := openBrowserURL(rawURL); err != nil {
			return browserOpenFailedMsg{URL: rawURL, Err: err}
		}
		return browserOpenedMsg{URL: rawURL}
	}
}

func openBrowserURL(rawURL string) error {
	for _, command := range browserCommands(rawURL) {
		if err := exec.Command(command.name, command.args...).Start(); err == nil {
			return nil
		}
	}
	return fmt.Errorf("no browser launcher found for %s", rawURL)
}

type browserCommand struct {
	name string
	args []string
}

func browserCommands(rawURL string) []browserCommand {
	switch runtime.GOOS {
	case "darwin":
		return []browserCommand{{name: "open", args: []string{rawURL}}}
	case "windows":
		return []browserCommand{{name: "rundll32", args: []string{"url.dll,FileProtocolHandler", rawURL}}}
	default:
		return []browserCommand{
			{name: "xdg-open", args: []string{rawURL}},
			{name: "wslview", args: []string{rawURL}},
			{name: "rundll32.exe", args: []string{"url.dll,FileProtocolHandler", rawURL}},
			{name: "sensible-browser", args: []string{rawURL}},
		}
	}
}

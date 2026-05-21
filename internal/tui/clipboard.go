package tui

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// copyToClipboard copies text to the system clipboard. It runs the OS-native
// command (pbcopy / clip / wl-copy / xclip / clip.exe) AND emits the OSC 52
// escape sequence: the two backends cover different setups (a local desktop vs.
// an SSH session vs. WSL), and a terminal silently swallows an OSC 52 sequence
// it does not understand, so emitting it can never be trusted on its own. The
// copy succeeds when either backend succeeds.
func copyToClipboard(text string) error {
	platformErr := copyPlatform(text)
	osc52Err := copyOSC52(text)
	if platformErr == nil || osc52Err == nil {
		return nil
	}
	return platformErr
}

// copyOSC52 writes the OSC 52 escape sequence to stdout, asking the terminal
// emulator to set the clipboard. Returns nil if the write succeeded (most
// terminals silently swallow unsupported sequences, so this is best-effort).
func copyOSC52(text string) error {
	encoded := base64.StdEncoding.EncodeToString([]byte(text))
	seq := fmt.Sprintf("\x1b]52;c;%s\x07", encoded)
	_, err := os.Stdout.WriteString(seq)
	return err
}

// copyPlatform shells out to the OS-native clipboard command.
func copyPlatform(text string) error {
	cmd := clipboardCommand()
	if cmd == nil {
		return fmt.Errorf("clipboard: no supported clipboard command found")
	}
	c := exec.Command(cmd[0], cmd[1:]...)
	c.Stdin = strings.NewReader(text)
	return c.Run()
}

// clipboardCommand returns the OS-native clipboard command and args.
func clipboardCommand() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"pbcopy"}
	case "windows":
		return []string{"clip"}
	default:
		// Linux / WSL: try wl-copy (Wayland) then xclip (X11) then clip.exe (WSL)
		if _, err := exec.LookPath("wl-copy"); err == nil {
			return []string{"wl-copy"}
		}
		if _, err := exec.LookPath("xclip"); err == nil {
			return []string{"xclip", "-selection", "clipboard"}
		}
		if _, err := exec.LookPath("clip.exe"); err == nil {
			return []string{"clip.exe"}
		}
		return nil
	}
}

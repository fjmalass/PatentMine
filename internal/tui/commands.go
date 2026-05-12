package tui

import "strings"

type Command struct {
	Name string
	Args []string
	Raw  string
}

func ParseCommand(input string) Command {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return Command{}
	}
	if strings.HasPrefix(raw, "/") {
		return Command{Name: commandSearch, Args: []string{strings.TrimSpace(strings.TrimPrefix(raw, keySearch))}, Raw: raw}
	}
	if strings.HasPrefix(raw, keyCommand) {
		raw = strings.TrimSpace(strings.TrimPrefix(raw, keyCommand))
	}
	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return Command{}
	}
	return Command{Name: strings.ToLower(parts[0]), Args: parts[1:], Raw: input}
}

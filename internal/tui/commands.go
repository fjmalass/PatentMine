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
	name := strings.ToLower(parts[0])
	args := parts[1:]

	// Short aliases
	switch name {
	case "ta":
		name = commandTag
		args = append([]string{tagSubAdd}, args...)
	case "tl":
		name = commandTag
		args = append([]string{tagSubList}, args...)
	case "td":
		name = commandTag
		args = append([]string{tagSubDelete}, args...)
	case "tr":
		name = commandTag
		args = append([]string{tagSubRename}, args...)
	case "tc":
		name = commandTag
		args = append([]string{tagSubColor}, args...)
	case "tf":
		name = commandTag
		args = append([]string{tagSubFilter}, args...)
	case "th":
		name = commandTag
		args = append([]string{tagSubHelp}, args...)
	case "co":
		name = commandCountry
	}

	return Command{Name: name, Args: args, Raw: input}
}

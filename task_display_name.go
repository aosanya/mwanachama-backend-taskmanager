package mwanachamataskmanager

import "strings"

const displayNameLimit = 40

func displayName(code, title, id string) string {
	if code != "" {
		return code
	}
	name := strings.TrimSpace(title)
	if name == "" {
		return id
	}
	if runes := []rune(name); len(runes) > displayNameLimit {
		name = strings.TrimSpace(string(runes[:displayNameLimit]))
	}
	return name
}

func taskDisplayName(task Task) string {
	return displayName(task.Code, task.Title, task.ID)
}

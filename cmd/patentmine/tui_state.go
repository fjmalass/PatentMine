package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"patentmine/internal/domain"
)

const lastProjectFileName = "last-project.txt"

func loadLastProject(home string) (domain.ProjectID, error) {
	body, err := os.ReadFile(filepath.Join(home, lastProjectFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read last project: %w", err)
	}
	return domain.ProjectID(strings.TrimSpace(string(body))), nil
}

func saveLastProject(home string, id domain.ProjectID) error {
	path := filepath.Join(home, lastProjectFileName)
	if err := os.WriteFile(path, []byte(string(id)+"\n"), 0o644); err != nil {
		return fmt.Errorf("write last project: %w", err)
	}
	return nil
}

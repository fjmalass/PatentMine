package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"patentmine/internal/config"
	"patentmine/internal/domain"
)

const lastProjectFileName = "last-project.txt"

func loadLastProject(home config.Path) (domain.ProjectID, error) {
	body, err := os.ReadFile(filepath.Join(string(home), lastProjectFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read last project: %w", err)
	}
	return domain.ProjectID(strings.TrimSpace(string(body))), nil
}

func saveLastProject(home config.Path, id domain.ProjectID) error {
	path := filepath.Join(string(home), lastProjectFileName)
	if err := os.WriteFile(path, []byte(string(id)+"\n"), 0o644); err != nil {
		return fmt.Errorf("write last project: %w", err)
	}
	return nil
}

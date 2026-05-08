package logging

import (
	"log/slog"
	"os"
)

func Open(path string) (*slog.Logger, func(), error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return slog.Default(), func() {}, err
	}
	logger := slog.New(slog.NewTextHandler(file, &slog.HandlerOptions{Level: slog.LevelInfo}))
	closeFn := func() {
		_ = file.Close()
	}
	return logger, closeFn, nil
}

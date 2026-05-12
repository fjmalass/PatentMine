package version

import (
	"os/exec"
	"runtime/debug"
	"strings"
	"sync"
)

var (
	GitVersion = ""
	GitCommit  = ""

	resolvedOnce sync.Once
	resolved     string
)

func Current() string {
	resolvedOnce.Do(func() {
		resolved = resolve()
	})
	return resolved
}

func resolve() string {
	if GitVersion != "" {
		return GitVersion
	}

	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			return info.Main.Version
		}
		revision := buildSetting(info, "vcs.revision")
		if revision != "" {
			short := revision
			if len(short) > 12 {
				short = short[:12]
			}
			if buildSetting(info, "vcs.modified") == "true" {
				return short + "-dirty"
			}
			return short
		}
	}

	if out, err := exec.Command("git", "describe", "--tags", "--always", "--dirty").Output(); err == nil {
		if s := strings.TrimSpace(string(out)); s != "" {
			return s
		}
	}

	return "dev"
}

func buildSetting(info *debug.BuildInfo, key string) string {
	for _, setting := range info.Settings {
		if setting.Key == key {
			return setting.Value
		}
	}
	return ""
}

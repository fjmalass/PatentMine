package engine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// gitMirrorIdentity is the committer stamped on the response mirror's commits.
// It is supplied per-command so the mirror works even when the host has no
// global git user configured.
const (
	gitMirrorName  = "PatentMine"
	gitMirrorEmail = "patentmine@localhost"
)

// gitCommitDir commits every change under dir to a per-matter git repository,
// initializing the repository on first use, and returns the resulting commit
// hash. It is best-effort and never the reason a snapshot fails: if the git
// binary is absent, or any git step errors, it returns an empty hash and a nil
// error, so the structured draft_revision history (the source of truth) is
// recorded regardless. The repository is the human-readable backup that gives
// `git log` / `git diff` over new_claims.md and response.md across versions.
func gitCommitDir(ctx context.Context, dir, message string) string {
	gitBin, err := exec.LookPath("git")
	if err != nil {
		return "" // git not installed — the mirror is simply skipped
	}
	run := func(args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, gitBin, append([]string{"-C", dir}, args...)...)
		// Keep the mirror from inheriting a surprising global identity or hooks.
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME="+gitMirrorName, "GIT_AUTHOR_EMAIL="+gitMirrorEmail,
			"GIT_COMMITTER_NAME="+gitMirrorName, "GIT_COMMITTER_EMAIL="+gitMirrorEmail,
		)
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}

	if _, err := os.Stat(filepath.Join(dir, ".git")); os.IsNotExist(err) {
		if _, err := run("init"); err != nil {
			return ""
		}
	}
	if _, err := run("add", "-A"); err != nil {
		return ""
	}
	// commit fails when there is nothing staged (an unchanged re-snapshot); that
	// is not an error for us — we still return the current HEAD below.
	_, _ = run("commit", "-m", message)
	hash, err := run("rev-parse", "HEAD")
	if err != nil {
		return ""
	}
	return hash
}

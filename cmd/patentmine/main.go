package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	appconfig "patentmine/internal/config"
	"patentmine/internal/importer"
	"patentmine/internal/logging"
	sqliterepo "patentmine/internal/storage/sqlite"
	"patentmine/internal/tui"
)

func main() {
	ctx := context.Background()
	config, err := parseCLI(os.Args[1:])
	if err != nil {
		exit(err)
	}
	if config.Help {
		showCLIHelp()
		return
	}

	// Ensure directories exist
	for _, dir := range []string{tui.DefaultDBDir, tui.DefaultLogDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			exit(fmt.Errorf("failed to create %s directory: %w", dir, err))
		}
	}

	logger, closeLog, logPath, err := logging.Open(config.LogFile, config.MaxLogs)
	if err != nil {
		exit(err)
	}
	defer closeLog()
	slog.SetDefault(logger)
	logger.Info("starting patentmine", "log_file", logPath, "max_logs", config.MaxLogs)

	activityLog, closeActivity, err := logging.OpenActivity(tui.DefaultActivityPath)
	if err != nil {
		logger.Error("activity log open failed", "error", err)
		activityLog = nil
	}
	if closeActivity != nil {
		defer closeActivity()
	}
	repo, err := sqliterepo.Open(tui.DefaultDBPath, logger)
	if err != nil {
		logger.Error("open sqlite failed", "error", err)
		exit(err)
	}
	defer repo.Close()
	if err := repo.Setup(ctx); err != nil {
		logger.Error("sqlite setup failed", "error", err)
		exit(err)
	}
	if err := seedFixture(ctx, repo); err != nil {
		logger.Error("fixture seed failed", "error", err)
		exit(err)
	}
	cfg := appconfig.Load()
	appconfig.ApplyCLI(&cfg, config.ImportSource, config.USPTOAPIKey, config.USPTOAPIKeyFile)

	keyStatus := "not found"
	if cfg.USPTO.APIKey != "" {
		keyStatus = "found"
	}
	logger.Info("configuration loaded", "import_source", cfg.ImportSource, "uspto_key", keyStatus)

	model := tui.New(ctx, repo, logger, activityLog, cfg)
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		logger.Error("tui failed", "error", err)
		exit(err)
	}
	logger.Info("stopped patentmine")
}

type cliConfig struct {
	LogFile         string
	MaxLogs         int
	Help            bool
	ImportSource    string
	USPTOAPIKey     string
	USPTOAPIKeyFile string
}

func parseCLI(args []string) (cliConfig, error) {
	config := cliConfig{}
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	flags := flag.NewFlagSet("patentmine", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&config.LogFile, "log-file", tui.DefaultLogPath, "base log file path; the current date is appended before the extension")
	flags.IntVar(&config.MaxLogs, "max-logs", 5, "maximum dated log files to keep; set to 0 to disable pruning")
	flags.BoolVar(&config.Help, "help", false, "show CLI help")
	flags.BoolVar(&config.Help, "h", false, "show CLI help")
	flags.StringVar(&config.ImportSource, "import-source", "", "patent data source: google (default) or uspto")
	flags.StringVar(&config.USPTOAPIKey, "uspto-api-key", "", "USPTO Open Data Portal API key")
	flags.StringVar(&config.USPTOAPIKeyFile, "uspto-api-key-file", "", "path to file containing USPTO ODP API key (e.g. ~/.ssh/uspto_odp_key)")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			config.Help = true
			return config, nil
		}
		return config, err
	}
	return config, nil
}

func showCLIHelp() {
	if isTerminal(os.Stdout) && isTerminal(os.Stdin) && canOpenTTY() {
		if _, err := tea.NewProgram(cliHelpModel{}, tea.WithAltScreen()).Run(); err == nil {
			return
		}
	}
	fmt.Print(cliHelpText())
}

func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func canOpenTTY() bool {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return false
	}
	_ = tty.Close()
	return true
}

type cliHelpModel struct{}

func (cliHelpModel) Init() tea.Cmd {
	return nil
}

func (m cliHelpModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(tea.KeyMsg); ok {
		return m, tea.Quit
	}
	return m, nil
}

func (cliHelpModel) View() string {
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(tui.ColorTheme)).Render("PatentMine CLI")
	hint := lipgloss.NewStyle().Foreground(lipgloss.Color(tui.ColorSubtle)).Render("Press any key to close.")
	return "\n" + title + "\n\n" + cliHelpText() + "\n" + hint + "\n"
}

func cliHelpText() string {
	var b strings.Builder
	b.WriteString("Usage:\n")
	b.WriteString("  patentmine [flags]\n\n")
	b.WriteString("Flags:\n")
	b.WriteString("  --log-file PATH   Base log path. The app writes to PATH with YYYY-MM-DD before the extension.\n")
	b.WriteString("                    Default: logs/patentmine.log -> logs/patentmine-2026-05-08.log\n")
	b.WriteString("  --max-logs N      Maximum dated log files to keep. Use 0 to disable pruning. Default: 5\n")
	b.WriteString("  --import-source SOURCE  Patent data source: \"google\" (default) or \"uspto\".\n")
	b.WriteString("  --uspto-api-key KEY     USPTO Open Data Portal API key.\n")
	b.WriteString("  --uspto-api-key-file PATH  Path to file containing USPTO ODP API key (e.g. ~/.ssh/uspto_odp_key).\n")
	b.WriteString("  --help, -h        Show this help.\n")
	return b.String()
}

func seedFixture(ctx context.Context, repo *sqliterepo.Repository) error {
	bundle, err := importer.LoadFixture("fixtures/US11611785B2.json")
	if err != nil {
		return err
	}
	bundle.Patent.ImportSource = "google"
	// Note: We use a placeholder project ID for seeding
	existing, err := repo.GetPatent(ctx, "default", "US11611785B2")
	if err == nil && existing.ExpirationDate != "" && existing.ExpirationEstimated == bundle.Patent.ExpirationEstimated {
		return nil
	}
	return repo.UpsertPatentBundle(ctx, "default", bundle)
}

func exit(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

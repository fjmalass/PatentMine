package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"patentmine/internal/config"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
)

const draftUsage = `usage:
  patentmine draft <subcommand> [options]

subcommands:
  new        create a draft        --project <id> --kind provisional|nonprovisional|oa-response [--title T] [--oa <officeActionID>]
  list       list a project's drafts   --project <id>
  show       print a draft as JSON      --id <draftID>
  export     render a draft to .docx    --id <draftID>          (prints the written path)
  ai         AI-draft one section       --id <draftID> --section <ordinal> [--instruction T] [--note T] [--pin "verbatim"]...
  oa-import  import an office action     --project <id> [--file path.pdf] [--type non_final] [--examiner N] [--art-unit N] [--app N] [--mail-date YYYY-MM-DD] [--text T]
  oa-list    list a project's office actions  --project <id>

Drafting runs in the engine daemon; start it first with: patentmine serve
`

// stringList collects repeated --pin flags.
type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ", ") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

func runDraft(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, draftUsage)
		return 2
	}
	sub := args[0]
	rest := args[1:]
	switch sub {
	case "new":
		return draftNew(rest)
	case "list":
		return draftList(rest)
	case "show":
		return draftShow(rest)
	case "export":
		return draftExport(rest)
	case "ai":
		return draftAI(rest)
	case "oa-import":
		return draftOAImport(rest)
	case "oa-list":
		return draftOAList(rest)
	case "help", "-h", "--help":
		fmt.Print(draftUsage)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "patentmine draft: unknown subcommand %q\n\n%s", sub, draftUsage)
		return 2
	}
}

// dialDaemon connects to the running engine; drafting is a daemon operation.
func dialDaemon() (*rpc.Client, func(), int) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, fail(err)
	}
	client, err := rpc.Dial(string(cfg.SocketPath))
	if err != nil {
		fmt.Fprintln(os.Stderr, "patentmine draft: engine daemon is not running. Start it with: patentmine serve")
		return nil, nil, 1
	}
	return client, func() { _ = client.Close() }, 0
}

func draftNew(args []string) int {
	fs := flag.NewFlagSet("draft new", flag.ContinueOnError)
	project := fs.String("project", "", "project id")
	kind := fs.String("kind", "", "provisional | nonprovisional | oa-response")
	title := fs.String("title", "", "invention title (optional)")
	oa := fs.String("oa", "", "office action id (for an oa-response)")
	if code, ok := parseFlags(fs, args); !ok {
		return code
	}
	dk, err := domain.ParseDraftKind(strings.ReplaceAll(*kind, "-", "_"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "patentmine draft new: %s\n", err)
		return 2
	}
	if *project == "" {
		fmt.Fprintln(os.Stderr, "patentmine draft new: --project is required")
		return 2
	}
	client, done, code := dialDaemon()
	if client == nil {
		return code
	}
	defer done()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var res proto.DraftResult
	if err := client.Call(ctx, proto.MethodDraftCreate, proto.DraftCreateParams{
		Project: domain.ProjectID(*project), Kind: dk, Title: *title,
	}, &res); err != nil {
		return failCmd("draft new", err)
	}
	d := res.Draft
	if *oa != "" {
		d.OfficeActionID = *oa
		if err := client.Call(ctx, proto.MethodDraftSave, proto.DraftSaveParams{Draft: d}, &res); err != nil {
			return failCmd("draft new (link oa)", err)
		}
		d = res.Draft
	}
	fmt.Printf("created draft %s (%s) with %d section(s)\n", d.ID, d.Kind, len(d.Sections))
	for _, s := range d.Sections {
		fmt.Printf("  [%d] %s\n", s.Ordinal, s.Heading)
	}
	return 0
}

func draftList(args []string) int {
	fs := flag.NewFlagSet("draft list", flag.ContinueOnError)
	project := fs.String("project", "", "project id")
	if code, ok := parseFlags(fs, args); !ok {
		return code
	}
	if *project == "" {
		fmt.Fprintln(os.Stderr, "patentmine draft list: --project is required")
		return 2
	}
	client, done, code := dialDaemon()
	if client == nil {
		return code
	}
	defer done()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var res proto.DraftListResult
	if err := client.Call(ctx, proto.MethodDraftList, proto.DraftListParams{Project: domain.ProjectID(*project)}, &res); err != nil {
		return failCmd("draft list", err)
	}
	if len(res.Drafts) == 0 {
		fmt.Println("(no drafts)")
		return 0
	}
	for _, d := range res.Drafts {
		title := d.Title
		if title == "" {
			title = "(untitled)"
		}
		fmt.Printf("%s  %-15s %-7s  %s\n", d.ID, d.Kind, d.Status, title)
	}
	return 0
}

func draftShow(args []string) int {
	fs := flag.NewFlagSet("draft show", flag.ContinueOnError)
	id := fs.String("id", "", "draft id")
	if code, ok := parseFlags(fs, args); !ok {
		return code
	}
	if *id == "" {
		fmt.Fprintln(os.Stderr, "patentmine draft show: --id is required")
		return 2
	}
	client, done, code := dialDaemon()
	if client == nil {
		return code
	}
	defer done()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var res proto.DraftResult
	if err := client.Call(ctx, proto.MethodDraftGet, proto.DraftIDParams{ID: domain.DraftID(*id)}, &res); err != nil {
		return failCmd("draft show", err)
	}
	out, _ := json.MarshalIndent(res.Draft, "", "  ")
	fmt.Println(string(out))
	return 0
}

func draftExport(args []string) int {
	fs := flag.NewFlagSet("draft export", flag.ContinueOnError)
	id := fs.String("id", "", "draft id")
	if code, ok := parseFlags(fs, args); !ok {
		return code
	}
	if *id == "" {
		fmt.Fprintln(os.Stderr, "patentmine draft export: --id is required")
		return 2
	}
	client, done, code := dialDaemon()
	if client == nil {
		return code
	}
	defer done()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var res proto.DraftExportResult
	if err := client.Call(ctx, proto.MethodDraftExportDocx, proto.DraftIDParams{ID: domain.DraftID(*id)}, &res); err != nil {
		return failCmd("draft export", err)
	}
	fmt.Println(res.Path)
	return 0
}

func draftAI(args []string) int {
	fs := flag.NewFlagSet("draft ai", flag.ContinueOnError)
	id := fs.String("id", "", "draft id")
	section := fs.Int("section", 0, "section ordinal")
	instruction := fs.String("instruction", "", "specific drafting instruction")
	note := fs.String("note", "", "attorney notes / focus")
	var pins stringList
	fs.Var(&pins, "pin", "a span that must appear verbatim (repeatable)")
	if code, ok := parseFlags(fs, args); !ok {
		return code
	}
	if *id == "" {
		fmt.Fprintln(os.Stderr, "patentmine draft ai: --id is required")
		return 2
	}
	client, done, code := dialDaemon()
	if client == nil {
		return code
	}
	defer done()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	var res proto.DraftSectionAIResult
	if err := client.Call(ctx, proto.MethodDraftSectionAI, proto.DraftSectionAIParams{
		Draft:          domain.DraftID(*id),
		SectionOrdinal: *section,
		Instruction:    *instruction,
		Notes:          *note,
		Pinned:         pins,
	}, &res); err != nil {
		return failCmd("draft ai", err)
	}
	fmt.Printf("--- drafted by %s/%s ---\n%s\n", res.Provider, res.Model, res.Text)
	if len(res.Problems) > 0 {
		fmt.Fprintln(os.Stderr, "\nReview required before accepting:")
		for _, p := range res.Problems {
			fmt.Fprintf(os.Stderr, "  - %s\n", p)
		}
	}
	return 0
}

func draftOAImport(args []string) int {
	fs := flag.NewFlagSet("draft oa-import", flag.ContinueOnError)
	project := fs.String("project", "", "project id")
	file := fs.String("file", "", "path to the office action PDF/text to import")
	oaType := fs.String("type", "", "non_final | final | restriction | advisory | notice_of_allowance")
	examiner := fs.String("examiner", "", "examiner name")
	artUnit := fs.String("art-unit", "", "art unit")
	app := fs.String("app", "", "application number")
	mailDate := fs.String("mail-date", "", "office action mail date (YYYY-MM-DD)")
	text := fs.String("text", "", "pre-extracted office action text (optional)")
	if code, ok := parseFlags(fs, args); !ok {
		return code
	}
	if *project == "" {
		fmt.Fprintln(os.Stderr, "patentmine draft oa-import: --project is required")
		return 2
	}
	client, done, code := dialDaemon()
	if client == nil {
		return code
	}
	defer done()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var res proto.OfficeActionResult
	if err := client.Call(ctx, proto.MethodOfficeActionImport, proto.OfficeActionImportParams{
		Project:           domain.ProjectID(*project),
		ApplicationNumber: *app,
		MailDate:          *mailDate,
		Type:              domain.OAType(*oaType),
		Examiner:          *examiner,
		ArtUnit:           *artUnit,
		SourcePath:        *file,
		ExtractedText:     *text,
	}, &res); err != nil {
		return failCmd("draft oa-import", err)
	}
	fmt.Printf("imported office action %s", res.OfficeAction.ID)
	if res.OfficeAction.BlobPath != "" {
		fmt.Printf(" (stored %s)", res.OfficeAction.BlobPath)
	}
	fmt.Println()
	return 0
}

func draftOAList(args []string) int {
	fs := flag.NewFlagSet("draft oa-list", flag.ContinueOnError)
	project := fs.String("project", "", "project id")
	if code, ok := parseFlags(fs, args); !ok {
		return code
	}
	if *project == "" {
		fmt.Fprintln(os.Stderr, "patentmine draft oa-list: --project is required")
		return 2
	}
	client, done, code := dialDaemon()
	if client == nil {
		return code
	}
	defer done()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var res proto.OfficeActionListResult
	if err := client.Call(ctx, proto.MethodOfficeActionList, proto.OfficeActionListParams{Project: domain.ProjectID(*project)}, &res); err != nil {
		return failCmd("draft oa-list", err)
	}
	if len(res.OfficeActions) == 0 {
		fmt.Println("(no office actions)")
		return 0
	}
	for _, oa := range res.OfficeActions {
		date := ""
		if !oa.MailDate.IsZero() {
			date = oa.MailDate.Format(domain.DateLayout)
		}
		fmt.Printf("%s  %-18s %-10s %s\n", oa.ID, oa.Type, date, oa.Examiner)
	}
	return 0
}

// parseFlags parses fs, mapping flag.ErrHelp to a clean exit. The bool reports
// whether parsing succeeded; the int is the exit code to use when it did not.
func parseFlags(fs *flag.FlagSet, args []string) (int, bool) {
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0, false
		}
		return 2, false
	}
	return 0, true
}

func failCmd(cmd string, err error) int {
	fmt.Fprintf(os.Stderr, "patentmine %s: %s\n", cmd, err)
	return 1
}

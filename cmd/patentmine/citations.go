package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"patentmine/internal/config"
	"patentmine/internal/domain"
	"patentmine/internal/engine"
	"patentmine/internal/observability"
	"patentmine/internal/proto"
	"patentmine/internal/store/sqlite"
	"patentmine/internal/uspto"
	appversion "patentmine/internal/version"
)

const citationsUsage = `usage:
  patentmine uspto-citations [options] <xml-file-or-number>

Parses the references-cited block of a USPTO grant/pgpub XML file and saves the
citations (and, by default, citation-graph edges). The argument is either a
path to an XML file or a patent/application number resolved against the patents
cache directory.

options:
  -graph   also write citation-graph edges and cited-patent stubs (default true)
  -quiet   suppress the metrics summary
`

func runCitations(args []string) int {
	fs := flag.NewFlagSet("uspto-citations", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(fs.Output(), citationsUsage) }

	var graph, quiet bool
	fs.BoolVar(&graph, "graph", true, "also write citation-graph edges and cited-patent stubs")
	fs.BoolVar(&quiet, "quiet", false, "suppress the metrics summary")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		return 1
	}

	xmlPath, err := resolveCitationFile(fs.Arg(0), string(cfg.PatentsDir))
	if err != nil {
		fmt.Fprintf(os.Stderr, "patentmine uspto-citations: %v\n", err)
		return 1
	}

	// Observability runtime owns the metrics registry the parser writes to
	// (wired into the engine below) plus the activity log.
	rt, err := observability.Open(string(cfg.LogsDir), "uspto", appversion.String())
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open observability runtime: %v\n", err)
		return 1
	}
	defer rt.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	repo, err := sqlite.Open(ctx, string(cfg.DBPath))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open database: %v\n", err)
		return 1
	}
	defer repo.Close()

	// engine.New wires uspto.Metrics to this metrics registry, so ParseCitations
	// records its parse duration and counts here.
	eng := engine.New(ctx, repo, nil,
		engine.WithMetrics(rt.Metrics),
		engine.WithLogger(rt.Logger),
	)
	defer eng.Close()

	f, err := os.Open(xmlPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "patentmine uspto-citations: open %s: %v\n", xmlPath, err)
		return 1
	}
	defer f.Close()

	res, err := uspto.ParseCitations(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "patentmine uspto-citations: parse %s: %v\n", xmlPath, err)
		return 1
	}

	record, ok := res.Record()
	if !ok {
		fmt.Fprintf(os.Stderr, "patentmine uspto-citations: %s has no usable patent number\n", xmlPath)
		return 1
	}
	kind := res.Kind
	if kind == "" {
		kind = string(proto.USPTOXMLKindGrant)
	}

	saveStart := time.Now()
	rows, saveErr := repo.SaveUSPTOCitations(ctx, res.ApplicationNumber, record, kind, res.Citations)
	rt.Metrics.ObserveDuration(uspto.MetricCitationSaveDuration, time.Since(saveStart), saveErr != nil)
	rt.Metrics.IncCounter(uspto.MetricCitationRows, int64(rows))
	if saveErr != nil {
		recordCitationActivity(ctx, rt, record, res, rows, 0, saveErr)
		fmt.Fprintf(os.Stderr, "patentmine uspto-citations: save: %v\n", saveErr)
		return 1
	}

	edges := 0
	if graph {
		var graphErr error
		edges, graphErr = eng.SaveCitationGraph(ctx, record, res.Citations)
		if graphErr != nil {
			recordCitationActivity(ctx, rt, record, res, rows, edges, graphErr)
			fmt.Fprintf(os.Stderr, "patentmine uspto-citations: save citation graph: %v\n", graphErr)
			return 1
		}
		rt.Metrics.IncCounter(uspto.MetricCitationGraphEdges, int64(edges))
	}

	recordCitationActivity(ctx, rt, record, res, rows, edges, nil)
	if !quiet {
		printCitationSummary(xmlPath, record, res, rows, edges, rt.Metrics.Snapshot())
	}
	return 0
}

// resolveCitationFile accepts an explicit XML path or a bare patent/application
// number, resolving the latter against the patents cache directory whose files
// are named "<appnum>_<patentnum>.xml".
func resolveCitationFile(arg, patentsDir string) (string, error) {
	if st, err := os.Stat(arg); err == nil && !st.IsDir() {
		return arg, nil
	}
	matches, err := filepath.Glob(filepath.Join(patentsDir, "*"+arg+"*.xml"))
	if err != nil {
		return "", fmt.Errorf("search patents dir: %w", err)
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no XML file matching %q in %s", arg, patentsDir)
	}
	return matches[0], nil
}

func recordCitationActivity(ctx context.Context, rt *observability.Runtime, record domain.PatentNumber, res uspto.CitationParseResult, rows, edges int, err error) {
	status := "ok"
	attrs := map[string]any{
		"application_number": res.ApplicationNumber,
		"kind":               res.Kind,
		"parsed":             len(res.Citations),
		"patent":             res.PatentCount,
		"npl":                res.NPLCount,
		"examiner":           res.ExaminerCount,
		"applicant":          res.ApplicantCount,
		"other":              res.OtherCount,
		"rows":               rows,
		"graph_edges":        edges,
	}
	if err != nil {
		status = "error"
		attrs["error"] = err.Error()
	}
	_ = rt.Activity.Record(ctx, observability.Record{
		Component:  "uspto",
		Action:     "parse-citations",
		Entity:     "patent",
		EntityID:   record.String(),
		Status:     status,
		Attributes: attrs,
	})
}

func printCitationSummary(xmlPath string, record domain.PatentNumber, res uspto.CitationParseResult, rows, edges int, snap observability.Snapshot) {
	fmt.Println("\n=======================================================")
	fmt.Println("            USPTO Citation Load Summary                 ")
	fmt.Println("=======================================================")
	fmt.Printf("File:                          %s\n", xmlPath)
	fmt.Printf("Patent Number:                 %s\n", record.String())
	fmt.Printf("Application Number:            %s\n", res.ApplicationNumber)
	fmt.Printf("Kind:                          %s\n", res.Kind)
	fmt.Println("\n--- Parsed ---")
	fmt.Printf("Citations:                     %d\n", len(res.Citations))
	fmt.Printf("  Patent / NPL:                %d / %d\n", res.PatentCount, res.NPLCount)
	fmt.Printf("  Examiner / Applicant / Other:%d / %d / %d\n", res.ExaminerCount, res.ApplicantCount, res.OtherCount)
	fmt.Println("\n--- Saved ---")
	fmt.Printf("Citation rows written:         %d\n", rows)
	fmt.Printf("Citation graph edges:          %d\n", edges)

	parseMs := timingMillis(snap, uspto.MetricCitationParseDuration)
	saveMs := timingMillis(snap, uspto.MetricCitationSaveDuration)
	fmt.Println("\n--- Timing ---")
	fmt.Printf("Parse:                         %.2f ms\n", parseMs)
	fmt.Printf("Save:                          %.2f ms\n", saveMs)
	fmt.Println("=======================================================")
}

func timingMillis(snap observability.Snapshot, name string) float64 {
	t, ok := snap.Timings[name]
	if !ok || t.Count == 0 {
		return 0
	}
	return float64(t.LastNanos) / 1e6
}

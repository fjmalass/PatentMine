package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"patentmine/internal/config"
)

const checkUsage = `usage:
  patentmine check-connectivity        check all external service connectivity
  patentmine check-connectivity uspto  check USPTO ODP API key and connectivity
  patentmine check-connectivity b2     check Backblaze B2 access through rclone
  patentmine check-connectivity b2-api check Backblaze B2 native API key and bucket access

flags:
  -v, --verbose  show the equivalent curl command for each check
`

type checkState string

const (
	checkGreen  checkState = "green"
	checkYellow checkState = "yellow"
	checkRed    checkState = "red"
)

type checkResult struct {
	Name    string
	State   checkState
	Message string
}

func runCheck(args []string) int {
	fs := flag.NewFlagSet("check-connectivity", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	verbose := fs.Bool("v", false, "show equivalent curl command")
	fs.BoolVar(verbose, "verbose", false, "show equivalent curl command")
	fs.Usage = func() { fmt.Fprint(fs.Output(), checkUsage) }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	cfg, err := config.Load()
	if err != nil {
		return fail(err)
	}

	target := "all"
	if fs.NArg() > 0 {
		target = fs.Arg(0)
	}
	if fs.NArg() > 1 {
		fs.Usage()
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	var results []checkResult
	switch target {
	case "all":
		results = append(results, checkUSPTO(ctx, cfg, *verbose), checkRcloneB2(ctx, cfg, *verbose))
	case "uspto":
		results = append(results, checkUSPTO(ctx, cfg, *verbose))
	case "b2", "backblaze":
		results = append(results, checkRcloneB2(ctx, cfg, *verbose))
	case "b2-api", "backblaze-api":
		results = append(results, checkB2(ctx, cfg, *verbose))
	case "rclone-b2", "rclone":
		results = append(results, checkRcloneB2(ctx, cfg, *verbose))
	case "help", "-h", "--help":
		fmt.Print(checkUsage)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "patentmine check-connectivity: unknown target %q\n\n%s", target, checkUsage)
		return 2
	}

	printCheckReport(results)
	if checksHealthy(results) {
		return 0
	}
	return 1
}

func printCheckReport(results []checkResult) {
	fmt.Println("PatentMine Connectivity Report")
	fmt.Println(strings.Repeat("=", 55))
	nameWidth := 0
	for _, result := range results {
		if len(result.Name) > nameWidth {
			nameWidth = len(result.Name)
		}
	}
	for _, result := range results {
		fmt.Printf("  %s:%*s %s - %s\n", result.Name, nameWidth-len(result.Name), "", checkStateEmoji(result.State), result.Message)
	}
	fmt.Println(strings.Repeat("=", 55))
}

func checkStateEmoji(state checkState) string {
	switch state {
	case checkGreen:
		return "🟢"
	case checkYellow:
		return "🟡"
	case checkRed:
		return "🔴"
	default:
		return "⚪"
	}
}

func checksHealthy(results []checkResult) bool {
	for _, result := range results {
		if result.State != checkGreen {
			return false
		}
	}
	return true
}

func checkUSPTO(ctx context.Context, cfg config.Config, verbose bool) checkResult {
	if strings.TrimSpace(cfg.USPTOAPIKey) == "" {
		return checkResult{Name: "USPTO", State: checkRed, Message: "missing PATENTMINE_USPTO_API_KEY"}
	}

	apiKey, err := config.ResolveAPIKey(cfg.USPTOAPIKey)
	if err != nil {
		return checkResult{Name: "USPTO", State: checkRed, Message: err.Error()}
	}

	if verbose {
		displayKey := apiKey
		if len(displayKey) > 8 {
			displayKey = displayKey[:4] + "..." + displayKey[len(displayKey)-4:]
		}
		url := fmt.Sprintf("https://api.uspto.gov/api/v1/patent/applications/search?q=applicationNumberText:%s", "16123456")
		fmt.Fprintf(os.Stderr, "[verbose] curl -L -X GET -H 'x-api-key: %s' -H 'Accept: application/json' '%s'\n", displayKey, url)
	}

	result := config.CheckUSPTOConnection(ctx, apiKey)

	if result.Connected {
		msg := result.Message
		if !result.AssignmentsActivated {
			msg += " (Warning: Patent Assignment Search API is not active)"
		}
		return checkResult{Name: "USPTO", State: checkGreen, Message: msg}
	}
	return checkResult{Name: "USPTO", State: checkYellow, Message: result.Message}
}

type b2AuthorizeResponse struct {
	AccountID          string `json:"accountId"`
	APIURL             string `json:"apiUrl"`
	AuthorizationToken string `json:"authorizationToken"`
}

type b2ListBucketsResponse struct {
	Buckets []struct {
		BucketName string `json:"bucketName"`
	} `json:"buckets"`
}

func checkB2(ctx context.Context, cfg config.Config, verbose bool) checkResult {
	missing := missingB2Config(cfg)
	if len(missing) > 0 {
		return checkResult{Name: "B2", State: checkRed, Message: "missing " + strings.Join(missing, ", ")}
	}
	if cfg.BackupProvider != "" && !strings.EqualFold(cfg.BackupProvider, config.BackupProviderB2) {
		return checkResult{Name: "B2", State: checkYellow, Message: "backup provider is " + cfg.BackupProvider + ", not " + config.BackupProviderB2}
	}

	client := &http.Client{Timeout: 15 * time.Second}
	auth, err := b2Authorize(ctx, client, cfg)
	if err != nil {
		return checkResult{Name: "B2", State: checkYellow, Message: "key loaded, authorization failed: " + err.Error()}
	}
	found, err := b2BucketExists(ctx, client, auth, cfg.BackupBucket)
	if err != nil {
		return checkResult{Name: "B2", State: checkYellow, Message: "authorized, bucket check failed: " + err.Error()}
	}
	if !found {
		return checkResult{Name: "B2", State: checkYellow, Message: "authorized, bucket not found: " + cfg.BackupBucket}
	}
	return checkResult{Name: "B2", State: checkGreen, Message: "key loaded, authorized, and bucket reachable: " + cfg.BackupBucket}
}

func checkRcloneB2(ctx context.Context, cfg config.Config, verbose bool) checkResult {
	if strings.TrimSpace(cfg.BackupBucket) == "" {
		return checkResult{Name: "Rclone B2", State: checkRed, Message: "missing PATENTMINE_BACKUP_BUCKET"}
	}
	if _, err := exec.LookPath(config.RcloneExecutable); err != nil {
		return checkResult{Name: "Rclone B2", State: checkYellow, Message: "rclone is not installed or not on PATH"}
	}

	remote := strings.TrimSpace(cfg.BackupRcloneRemote)
	if remote == "" {
		remote = config.DefaultBackupRcloneRemote
	}
	target := remote + ":" + cfg.BackupBucket
	out, err := runRcloneList(ctx, target, os.Environ())
	if ctx.Err() != nil {
		return checkResult{Name: "Rclone B2", State: checkYellow, Message: "rclone check timed out"}
	}
	if err == nil {
		return checkResult{Name: "Rclone B2", State: checkGreen, Message: "rclone can list " + target}
	}

	if strings.TrimSpace(cfg.BackupB2KeyID) != "" && strings.TrimSpace(cfg.BackupB2Secret) != "" {
		injectedOut, injectedErr := runRcloneList(ctx, target, rcloneB2Env(os.Environ(), remote, cfg))
		if ctx.Err() != nil {
			return checkResult{Name: "Rclone B2", State: checkYellow, Message: "rclone check timed out"}
		}
		if injectedErr == nil {
			return checkResult{Name: "Rclone B2", State: checkGreen, Message: "rclone can list " + target + " using loaded B2 keys"}
		}
		out = injectedOut
		err = injectedErr
	}

	msg := compactBody(out)
	if msg == "" {
		msg = err.Error()
	}
	return checkResult{Name: "Rclone B2", State: checkYellow, Message: "rclone cannot list " + target + ": " + msg}
}

func runRcloneList(ctx context.Context, target string, env []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, config.RcloneExecutable, config.RcloneListDirCommand, target)
	cmd.Env = env
	return cmd.CombinedOutput()
}

func rcloneB2Env(base []string, remote string, cfg config.Config) []string {
	if strings.TrimSpace(cfg.BackupB2KeyID) == "" || strings.TrimSpace(cfg.BackupB2Secret) == "" {
		return base
	}
	name := rcloneConfigEnvName(remote)
	return append(base,
		"RCLONE_CONFIG_"+name+"_TYPE="+config.BackupProviderB2,
		"RCLONE_CONFIG_"+name+"_ACCOUNT="+cfg.BackupB2KeyID,
		"RCLONE_CONFIG_"+name+"_KEY="+cfg.BackupB2Secret,
	)
}

func rcloneConfigEnvName(remote string) string {
	var b strings.Builder
	for _, r := range remote {
		if r >= 'a' && r <= 'z' {
			b.WriteRune(r - 'a' + 'A')
			continue
		}
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	return b.String()
}

func missingB2Config(cfg config.Config) []string {
	var missing []string
	if strings.TrimSpace(cfg.BackupBucket) == "" {
		missing = append(missing, "PATENTMINE_BACKUP_BUCKET")
	}
	if strings.TrimSpace(cfg.BackupB2KeyID) == "" {
		missing = append(missing, "PATENTMINE_BACKUP_B2_KEY_ID")
	}
	if strings.TrimSpace(cfg.BackupB2Secret) == "" {
		missing = append(missing, "PATENTMINE_BACKUP_B2_SECRET")
	}
	return missing
}

func b2Authorize(ctx context.Context, client *http.Client, cfg config.Config) (b2AuthorizeResponse, error) {
	base := strings.TrimRight(cfg.BackupB2APIURL, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/b2api/v2/b2_authorize_account", nil)
	if err != nil {
		return b2AuthorizeResponse{}, err
	}
	token := base64.StdEncoding.EncodeToString([]byte(cfg.BackupB2KeyID + ":" + cfg.BackupB2Secret))
	req.Header.Set("Authorization", "Basic "+token)

	resp, err := client.Do(req)
	if err != nil {
		return b2AuthorizeResponse{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return b2AuthorizeResponse{}, fmt.Errorf("HTTP %d: %s", resp.StatusCode, compactBody(body))
	}

	var auth b2AuthorizeResponse
	if err := json.Unmarshal(body, &auth); err != nil {
		return b2AuthorizeResponse{}, err
	}
	if auth.AccountID == "" || auth.APIURL == "" || auth.AuthorizationToken == "" {
		return b2AuthorizeResponse{}, fmt.Errorf("authorization response missing required fields")
	}
	return auth, nil
}

func b2BucketExists(ctx context.Context, client *http.Client, auth b2AuthorizeResponse, bucket string) (bool, error) {
	body, err := json.Marshal(map[string]string{
		"accountId":  auth.AccountID,
		"bucketName": bucket,
	})
	if err != nil {
		return false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(auth.APIURL, "/")+"/b2api/v2/b2_list_buckets", bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", auth.AuthorizationToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Errorf("HTTP %d: %s", resp.StatusCode, compactBody(respBody))
	}

	var listed b2ListBucketsResponse
	if err := json.Unmarshal(respBody, &listed); err != nil {
		return false, err
	}
	for _, bucketInfo := range listed.Buckets {
		if bucketInfo.BucketName == bucket {
			return true, nil
		}
	}
	return false, nil
}

func compactBody(body []byte) string {
	compact := strings.Join(strings.Fields(string(body)), " ")
	if len(compact) > 240 {
		return compact[:240] + "..."
	}
	return compact
}

package app

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	commandName          = "actions-watcher"
	apiBaseURL           = "https://api.github.com"
	apiVersion           = "2026-03-10"
	defaultInterval      = 2 * time.Second
	detailsAPIRunVersion = "application/vnd.github+json"
)

var version = "dev"

var badConclusions = map[string]struct{}{
	"action_required": {},
	"cancelled":       {},
	"failure":         {},
	"stale":           {},
	"startup_failure": {},
	"timed_out":       {},
}

var runSuccessConclusions = map[string]struct{}{
	"success": {},
	"neutral": {},
	"skipped": {},
}

type runState struct {
	ID         int    `json:"id"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	HTMLURL    string `json:"html_url"`
}

type stepState struct {
	Number     int    `json:"number"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
}

type jobState struct {
	ID         int         `json:"id"`
	Name       string      `json:"name"`
	Status     string      `json:"status"`
	Conclusion string      `json:"conclusion"`
	HTMLURL    string      `json:"html_url"`
	Steps      []stepState `json:"steps"`
}

type runJobsResponse struct {
	Jobs []jobState `json:"jobs"`
}

type githubAPI interface {
	GetRun(repo string, runID int) (runState, error)
	GetJobs(repo string, runID int) ([]jobState, error)
}

type apiClient struct {
	baseURL string
	token   string
	client  *http.Client
}

func newAPIClient(baseURL, token string) *apiClient {
	return &apiClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (a *apiClient) doJSON(path string, out any) error {
	url := a.baseURL + path
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", detailsAPIRunVersion)
	req.Header.Set("Authorization", "Bearer "+a.token)
	req.Header.Set("X-GitHub-Api-Version", apiVersion)

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("request %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
		return fmt.Errorf("GitHub API %d for %s: %s", resp.StatusCode, path, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response for %s: %w", path, err)
	}
	return nil
}

func (a *apiClient) GetRun(repo string, runID int) (runState, error) {
	var run runState
	path := fmt.Sprintf("/repos/%s/actions/runs/%d", repo, runID)
	if err := a.doJSON(path, &run); err != nil {
		return runState{}, err
	}
	return run, nil
}

func (a *apiClient) GetJobs(repo string, runID int) ([]jobState, error) {
	jobs := make([]jobState, 0, 32)
	for page := 1; ; page++ {
		var payload runJobsResponse
		path := fmt.Sprintf("/repos/%s/actions/runs/%d/jobs?per_page=100&page=%d", repo, runID, page)
		if err := a.doJSON(path, &payload); err != nil {
			return nil, err
		}
		jobs = append(jobs, payload.Jobs...)
		if len(payload.Jobs) < 100 {
			break
		}
	}
	return jobs, nil
}

func Run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || isHelpArg(args[0]) {
		printRootHelp(stdout)
		return nil
	}

	switch args[0] {
	case "version":
		_, _ = fmt.Fprintf(stdout, "%s %s\n", commandName, version)
		return nil
	case "watch":
		return runWatch(args[1:], stdout, stderr)
	default:
		if isLikelyRunIDs(args) {
			return runWatch(args, stdout, stderr)
		}
		return fmt.Errorf("unknown command %q (run `%s --help`)", args[0], commandName)
	}
}

func isLikelyRunIDs(args []string) bool {
	if len(args) == 0 {
		return false
	}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			return true
		}
		if _, err := strconv.Atoi(arg); err != nil {
			return false
		}
	}
	return true
}

func runWatch(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("watch", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	repo := strings.TrimSpace(os.Getenv("GITHUB_REPOSITORY"))
	interval := defaultInterval.Seconds()
	token := ""

	fs.StringVar(&repo, "repo", repo, "owner/repo slug (defaults to GITHUB_REPOSITORY or git origin)")
	fs.Float64Var(&interval, "interval", interval, "poll interval in seconds")
	fs.StringVar(&token, "token", "", "GitHub token (default: GH_TOKEN, GITHUB_TOKEN, or `gh auth token`)")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("invalid args: %w", err)
	}
	if interval <= 0 {
		return errors.New("--interval must be positive")
	}

	runIDs, err := parseRunIDs(fs.Args())
	if err != nil {
		return err
	}

	repo = strings.TrimSpace(repo)
	if repo == "" {
		repo, err = resolveRepoSlug()
		if err != nil {
			return err
		}
	}

	if token == "" {
		token, err = resolveGitHubToken()
		if err != nil {
			return err
		}
	}

	code, err := watchRuns(repo, runIDs, time.Duration(interval*float64(time.Second)), stdout, stderr, newAPIClient(apiBaseURL, token), time.Sleep)
	if err != nil {
		return err
	}
	if code != 0 {
		return exitCodeError{code: code}
	}
	return nil
}

func parseRunIDs(args []string) ([]int, error) {
	if len(args) == 0 {
		return nil, errors.New("watch requires one or more RUN_ID values")
	}

	runIDs := make([]int, 0, len(args))
	for _, raw := range args {
		id, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("invalid RUN_ID %q", raw)
		}
		runIDs = append(runIDs, id)
	}

	return runIDs, nil
}

func watchRuns(repo string, runIDs []int, interval time.Duration, stdout, stderr io.Writer, api githubAPI, sleeper func(time.Duration)) (int, error) {
	seenJobs := map[string]string{}
	seenSteps := map[string]string{}

	sortedRunIDs := append([]int(nil), runIDs...)
	sort.Ints(sortedRunIDs)

	for {
		runs := make(map[int]runState, len(sortedRunIDs))
		for _, runID := range sortedRunIDs {
			run, err := api.GetRun(repo, runID)
			if err != nil {
				return 0, err
			}
			jobs, err := api.GetJobs(repo, runID)
			if err != nil {
				return 0, err
			}

			printTransitions(runID, jobs, seenJobs, seenSteps, stdout)

			if failure := firstFailure(runID, jobs); failure != "" {
				_, _ = fmt.Fprintf(stderr, "[actions-watcher] %s\n", failure)
				return 1, nil
			}

			runs[runID] = run
		}

		if allRunsSuccessful(runs) {
			for _, runID := range sortedRunIDs {
				run := runs[runID]
				_, _ = fmt.Fprintf(
					stdout,
					"[actions-watcher] run %d completed with conclusion=%s %s\n",
					runID,
					run.Conclusion,
					strings.TrimSpace(run.HTMLURL),
				)
			}
			return 0, nil
		}

		sleeper(interval)
	}
}

func printTransitions(runID int, jobs []jobState, seenJobs, seenSteps map[string]string, out io.Writer) {
	for _, job := range jobs {
		jobKey := fmt.Sprintf("%d:%d", runID, job.ID)
		jobState := fmt.Sprintf("%s|%s", job.Status, job.Conclusion)
		if seenJobs[jobKey] != jobState {
			_, _ = fmt.Fprintf(out, "[actions-watcher] run %d: %s\n", runID, formatJob(job))
			seenJobs[jobKey] = jobState
		}

		for _, step := range job.Steps {
			stepKey := fmt.Sprintf("%d:%d:%d", runID, job.ID, step.Number)
			stepState := fmt.Sprintf("%s|%s", step.Status, step.Conclusion)
			if seenSteps[stepKey] != stepState {
				_, _ = fmt.Fprintf(out, "[actions-watcher] run %d: job `%s` %s\n", runID, defaultStr(job.Name, "<unknown job>"), formatStep(step))
				seenSteps[stepKey] = stepState
			}
		}
	}
}

func firstFailure(runID int, jobs []jobState) string {
	for _, job := range jobs {
		if isFailureConclusion(job.Conclusion) {
			return fmt.Sprintf("run %d failed: %s", runID, formatJob(job))
		}
		failedSteps := collectFailedSteps(job)
		if len(failedSteps) == 0 {
			continue
		}

		parts := make([]string, 0, len(failedSteps))
		for _, step := range failedSteps {
			parts = append(parts, formatStep(step))
		}
		return fmt.Sprintf("run %d failed: %s :: %s", runID, formatJob(job), strings.Join(parts, "; "))
	}
	return ""
}

func allRunsSuccessful(runs map[int]runState) bool {
	if len(runs) == 0 {
		return false
	}
	for _, run := range runs {
		if strings.ToLower(strings.TrimSpace(run.Status)) != "completed" {
			return false
		}
		if _, ok := runSuccessConclusions[strings.ToLower(strings.TrimSpace(run.Conclusion))]; !ok {
			return false
		}
	}
	return true
}

func collectFailedSteps(job jobState) []stepState {
	failed := make([]stepState, 0)
	for _, step := range job.Steps {
		if isFailureConclusion(step.Conclusion) {
			failed = append(failed, step)
		}
	}
	return failed
}

func isFailureConclusion(conclusion string) bool {
	_, ok := badConclusions[strings.ToLower(strings.TrimSpace(conclusion))]
	return ok
}

func formatJob(job jobState) string {
	return strings.TrimSpace(fmt.Sprintf(
		"job `%s` id=%d status=%s conclusion=%s %s",
		defaultStr(job.Name, "<unknown job>"),
		job.ID,
		job.Status,
		job.Conclusion,
		job.HTMLURL,
	))
}

func formatStep(step stepState) string {
	return fmt.Sprintf(
		"step %d `%s` status=%s conclusion=%s",
		step.Number,
		defaultStr(step.Name, "<unknown step>"),
		step.Status,
		step.Conclusion,
	)
}

func defaultStr(v string, fallback string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return fallback
	}
	return v
}

func resolveRepoSlug() (string, error) {
	if repo := strings.TrimSpace(os.Getenv("GITHUB_REPOSITORY")); repo != "" {
		return repo, nil
	}

	cmd := exec.Command("git", "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		return "", errors.New("set --repo or GITHUB_REPOSITORY, or run inside a git repo with origin configured")
	}

	remote := strings.TrimSpace(string(out))
	remote = strings.TrimSuffix(remote, ".git")
	re := regexp.MustCompile(`github\.com[:/]([^/]+)/([^/]+)$`)
	match := re.FindStringSubmatch(remote)
	if len(match) != 3 {
		return "", errors.New("failed to infer owner/repo from git remote origin")
	}

	return match[1] + "/" + match[2], nil
}

func resolveGitHubToken() (string, error) {
	for _, name := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			return v, nil
		}
	}

	cmd := exec.Command("gh", "auth", "token")
	out, err := cmd.Output()
	if err == nil {
		if token := strings.TrimSpace(string(out)); token != "" {
			return token, nil
		}
	}

	return "", errors.New("set GH_TOKEN or GITHUB_TOKEN, or authenticate gh CLI with `gh auth login`")
}

type exitCodeError struct {
	code int
}

func (e exitCodeError) Error() string {
	return fmt.Sprintf("exit status %d", e.code)
}

func isHelpArg(v string) bool {
	switch v {
	case "-h", "--help", "help":
		return true
	default:
		return false
	}
}

func printRootHelp(w io.Writer) {
	writeLines(w,
		"actions-watcher - fail-fast watcher for GitHub Actions runs",
		"",
		"Usage:",
		"  actions-watcher watch [--repo owner/repo] [--interval seconds] [--token token] RUN_ID [RUN_ID ...]",
		"  actions-watcher [--repo owner/repo] [--interval seconds] RUN_ID [RUN_ID ...]",
		"",
		"Commands:",
		"  watch           watch one or more run IDs and fail on first job/step failure",
		"  version         print CLI version",
		"",
		"Options:",
		"  --repo          owner/repo slug (defaults to GITHUB_REPOSITORY or git origin)",
		"  --interval      poll interval in seconds (default: 2)",
		"  --token         GitHub token (default: GH_TOKEN, GITHUB_TOKEN, or gh auth token)",
		"",
		"Examples:",
		"  actions-watcher watch 123456789",
		"  actions-watcher watch --repo amxv/actions-watcher 123456789 123456790",
		"  actions-watcher --interval 1.5 123456789",
	)
}

func writeLines(w io.Writer, lines ...string) {
	for _, line := range lines {
		_, _ = fmt.Fprintln(w, line)
	}
}

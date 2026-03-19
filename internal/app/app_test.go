package app

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeAPI struct {
	runs map[int][]runState
	jobs map[int][][]jobState
	i    map[int]int
}

func newFakeAPI() *fakeAPI {
	return &fakeAPI{
		runs: map[int][]runState{},
		jobs: map[int][][]jobState{},
		i:    map[int]int{},
	}
}

func (f *fakeAPI) GetRun(_ string, runID int) (runState, error) {
	items, ok := f.runs[runID]
	if !ok || len(items) == 0 {
		return runState{}, errors.New("missing run fixture")
	}
	idx := f.i[runID]
	if idx >= len(items) {
		idx = len(items) - 1
	}
	return items[idx], nil
}

func (f *fakeAPI) GetJobs(_ string, runID int) ([]jobState, error) {
	items, ok := f.jobs[runID]
	if !ok || len(items) == 0 {
		return nil, errors.New("missing job fixture")
	}
	idx := f.i[runID]
	if idx >= len(items) {
		idx = len(items) - 1
	}
	f.i[runID] = f.i[runID] + 1
	return items[idx], nil
}

func TestRunRootHelp(t *testing.T) {
	var out bytes.Buffer
	var errBuf bytes.Buffer

	err := Run([]string{"--help"}, &out, &errBuf)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !strings.Contains(out.String(), "gha-watch") {
		t.Fatalf("expected root help output, got: %q", out.String())
	}
}

func TestWatchRunsFailsFastOnStepFailure(t *testing.T) {
	api := newFakeAPI()
	api.runs[123] = []runState{{ID: 123, Status: "in_progress"}}
	api.jobs[123] = [][]jobState{{
		{
			ID:     10,
			Name:   "test",
			Status: "completed",
			Steps: []stepState{
				{Number: 1, Name: "Checkout", Status: "completed", Conclusion: "success"},
				{Number: 2, Name: "Run tests", Status: "completed", Conclusion: "failure"},
			},
		},
	}}

	var out bytes.Buffer
	var errBuf bytes.Buffer

	code, err := watchRuns("amxv/repo", []int{123}, 5*time.Millisecond, &out, &errBuf, api, func(time.Duration) {})
	if err != nil {
		t.Fatalf("watchRuns returned error: %v", err)
	}
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(errBuf.String(), "run 123 failed") {
		t.Fatalf("expected failure in stderr, got: %q", errBuf.String())
	}
}

func TestWatchRunsSucceedsAfterCompletion(t *testing.T) {
	api := newFakeAPI()
	api.runs[456] = []runState{
		{ID: 456, Status: "in_progress", HTMLURL: "https://example.test/runs/456"},
		{ID: 456, Status: "completed", Conclusion: "success", HTMLURL: "https://example.test/runs/456"},
	}
	api.jobs[456] = [][]jobState{
		{{ID: 20, Name: "build", Status: "in_progress", Steps: []stepState{{Number: 1, Name: "Compile", Status: "in_progress"}}}},
		{{ID: 20, Name: "build", Status: "completed", Conclusion: "success", Steps: []stepState{{Number: 1, Name: "Compile", Status: "completed", Conclusion: "success"}}}},
	}

	var out bytes.Buffer
	var errBuf bytes.Buffer
	sleeps := 0

	code, err := watchRuns("amxv/repo", []int{456}, 1*time.Millisecond, &out, &errBuf, api, func(time.Duration) { sleeps++ })
	if err != nil {
		t.Fatalf("watchRuns returned error: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if sleeps == 0 {
		t.Fatalf("expected at least one sleep before completion")
	}
	if !strings.Contains(out.String(), "completed with conclusion=success") {
		t.Fatalf("expected completion output, got: %q", out.String())
	}
}

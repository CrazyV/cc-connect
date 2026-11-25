package claudecode

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeRecentClaudeSession(t *testing.T, home, id, cwd string, modified time.Time) string {
	t.Helper()
	projectKey := strings.ReplaceAll(encodeClaudeProjectKey(cwd), "\\", "-")
	projectDir := filepath.Join(home, "projects", projectKey)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}
	line, err := json.Marshal(map[string]any{
		"type": "attachment",
		"attachment": map[string]any{
			"type": "environment",
			"snapshot": map[string]any{
				"workingDirectory": cwd,
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal environment entry: %v", err)
	}
	path := filepath.Join(projectDir, id+".jsonl")
	if err := os.WriteFile(path, append(line, '\n'), 0o644); err != nil {
		t.Fatalf("write session: %v", err)
	}
	if err := os.Chtimes(path, modified, modified); err != nil {
		t.Fatalf("chtimes session: %v", err)
	}
	return projectDir
}

func TestRecentWorkDirs_ResolvesClaudeProjectPathsFromTranscripts(t *testing.T) {
	home := t.TempDir()
	projectA := filepath.Join(t.TempDir(), "project-a")
	projectB := filepath.Join(t.TempDir(), "project-b")
	for _, dir := range []string{projectA, projectB} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	now := time.Now()
	writeRecentClaudeSession(t, home, "a-old", projectA, now.Add(-2*time.Hour))
	projectADir := writeRecentClaudeSession(t, home, "a-new", projectA, now.Add(-1*time.Hour))
	writeRecentClaudeSession(t, home, "b", projectB, now)
	if files := readClaudeSessionFiles(projectADir); len(files) != 2 {
		t.Fatalf("project A session files = %d, want 2", len(files))
	}
	if cwd := readClaudeSessionWorkingDirectory(filepath.Join(projectADir, "a-new.jsonl")); cwd != projectA {
		t.Fatalf("parsed cwd = %q, want %q", cwd, projectA)
	}

	unknownDir := filepath.Join(home, "projects", "unknown-project")
	if err := os.MkdirAll(unknownDir, 0o755); err != nil {
		t.Fatalf("mkdir unknown project: %v", err)
	}
	unknownFile := filepath.Join(unknownDir, "unknown.jsonl")
	if err := os.WriteFile(unknownFile, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write unknown session: %v", err)
	}
	if err := os.Chtimes(unknownFile, now.Add(time.Hour), now.Add(time.Hour)); err != nil {
		t.Fatalf("chtimes unknown session: %v", err)
	}

	agent := &Agent{configEnv: []string{"CLAUDE_CONFIG_DIR=" + home}}
	if got := agent.configHome(); got != home {
		t.Fatalf("configHome() = %q, want %q", got, home)
	}
	dirs, err := agent.RecentWorkDirs(context.Background(), 10)
	if err != nil {
		t.Fatalf("RecentWorkDirs: %v", err)
	}
	if len(dirs) != 2 {
		t.Fatalf("RecentWorkDirs returned %d dirs, want 2: %+v", len(dirs), dirs)
	}
	if dirs[0].Path != projectB || dirs[0].SessionCount != 1 {
		t.Fatalf("first dir = %+v, want project B with 1 session", dirs[0])
	}
	if dirs[1].Path != projectA || dirs[1].SessionCount != 2 {
		t.Fatalf("second dir = %+v, want project A with 2 sessions", dirs[1])
	}

	limited, err := agent.RecentWorkDirs(context.Background(), 1)
	if err != nil {
		t.Fatalf("RecentWorkDirs limit: %v", err)
	}
	if len(limited) != 1 || limited[0].Path != projectB {
		t.Fatalf("RecentWorkDirs limit = %+v, want only project B", limited)
	}
}

package codex

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func createRecentCodexStateDB(t *testing.T, home string) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(home, "state_5.sqlite")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open state db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	stmts := []string{
		`CREATE TABLE projects (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			metadata TEXT NOT NULL DEFAULT '{}',
			position INTEGER NOT NULL,
			created_at_ms INTEGER NOT NULL,
			updated_at_ms INTEGER NOT NULL
		)`,
		`CREATE TABLE project_roots (
			project_id TEXT NOT NULL,
			position INTEGER NOT NULL,
			path TEXT NOT NULL,
			PRIMARY KEY (project_id, position)
		)`,
		`CREATE TABLE threads (
			id TEXT PRIMARY KEY,
			cwd TEXT NOT NULL,
			archived INTEGER NOT NULL DEFAULT 0,
			updated_at INTEGER NOT NULL,
			updated_at_ms INTEGER
		)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("create schema: %v", err)
		}
	}
	return db
}

func TestRecentWorkDirs_UsesCodexProjectRegistry(t *testing.T) {
	home := t.TempDir()
	db := createRecentCodexStateDB(t, home)
	projectA := filepath.Join(t.TempDir(), "project-a")
	projectB := filepath.Join(t.TempDir(), "project-b")
	missingDir := filepath.Join(t.TempDir(), "missing")
	for _, dir := range []string{projectA, projectB} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	now := time.Now()
	projectAUpdated := now.Add(-2 * time.Hour).UnixMilli()
	projectBUpdated := now.Add(-3 * time.Hour).UnixMilli()
	if _, err := db.Exec(
		`INSERT INTO projects (id,name,position,created_at_ms,updated_at_ms) VALUES
			('p-a','Project A',1,?,?),
			('p-b','Project B',2,?,?),
			('p-missing','Missing Project',3,?,?)`,
		projectAUpdated, projectAUpdated,
		projectBUpdated, projectBUpdated,
		projectAUpdated, projectAUpdated,
	); err != nil {
		t.Fatalf("insert projects: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO project_roots (project_id,position,path) VALUES
			('p-a',0,?),
			('p-b',0,?),
			('p-missing',0,?)`,
		projectA, projectB, missingDir,
	); err != nil {
		t.Fatalf("insert project roots: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO threads (id,cwd,archived,updated_at,updated_at_ms) VALUES
			('a1',?,0,?,?),
			('a2',?,0,?,?),
			('b1',?,0,?,?)`,
		`\\?\`+projectA, now.Add(-2*time.Hour).Unix(), now.Add(-2*time.Hour).UnixMilli(),
		`\\?\`+projectA, now.Add(-1*time.Hour).Unix(), now.Add(-1*time.Hour).UnixMilli(),
		`\\?\`+projectB, now.Add(-90*time.Minute).Unix(), now.Add(-90*time.Minute).UnixMilli(),
	); err != nil {
		t.Fatalf("insert threads: %v", err)
	}

	agent := &Agent{codexHome: home}
	dirs, err := agent.RecentWorkDirs(context.Background(), 10)
	if err != nil {
		t.Fatalf("RecentWorkDirs: %v", err)
	}
	if len(dirs) != 2 {
		t.Fatalf("RecentWorkDirs returned %d dirs, want 2: %+v", len(dirs), dirs)
	}
	if dirs[0].Name != "Project A" || dirs[0].Path != projectA || dirs[0].SessionCount != 2 {
		t.Fatalf("first dir = %+v, want Project A with 2 sessions", dirs[0])
	}
	if dirs[1].Name != "Project B" || dirs[1].Path != projectB || dirs[1].SessionCount != 1 {
		t.Fatalf("second dir = %+v, want Project B with 1 session", dirs[1])
	}

	limited, err := agent.RecentWorkDirs(context.Background(), 1)
	if err != nil {
		t.Fatalf("RecentWorkDirs limit: %v", err)
	}
	if len(limited) != 1 || limited[0].Name != "Project A" {
		t.Fatalf("RecentWorkDirs limit = %+v, want only Project A", limited)
	}
}

func TestRecentWorkDirs_DoesNotUseArbitrarySessionCwds(t *testing.T) {
	home := t.TempDir()
	db := createRecentCodexStateDB(t, home)
	if _, err := db.Exec(`INSERT INTO projects (id,name,position,created_at_ms,updated_at_ms) VALUES ('p-empty','Empty',1,1,1)`); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	projectless := filepath.Join(t.TempDir(), "projectless")
	if err := os.MkdirAll(projectless, 0o755); err != nil {
		t.Fatalf("mkdir projectless: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO threads (id,cwd,archived,updated_at,updated_at_ms) VALUES ('t1',?,0,?,?)`,
		projectless, time.Now().Unix(), time.Now().UnixMilli(),
	); err != nil {
		t.Fatalf("insert thread: %v", err)
	}

	agent := &Agent{codexHome: home}
	dirs, err := agent.RecentWorkDirs(context.Background(), 10)
	if err != nil {
		t.Fatalf("RecentWorkDirs: %v", err)
	}
	if len(dirs) != 0 {
		t.Fatalf("RecentWorkDirs = %+v, want no arbitrary session cwds", dirs)
	}
}

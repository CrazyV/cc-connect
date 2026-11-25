package codex

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/chenhg5/cc-connect/core"
)

type codexProjectRoot struct {
	name         string
	path         string
	updatedAtMS  int64
	sessionCount int
	lastActivity time.Time
}

type codexThreadStats struct {
	count        int
	lastActivity time.Time
}

// RecentWorkDirs reads the same project registry used by the Codex desktop
// project picker. Project roots are matched against Codex thread cwd values for
// session counts and latest activity.
func (a *Agent) RecentWorkDirs(ctx context.Context, limit int) ([]core.RecentWorkDir, error) {
	if limit <= 0 {
		return nil, nil
	}

	a.mu.RLock()
	codexHome := a.codexHome
	a.mu.RUnlock()
	codexHome = resolveCodexHomeDir(codexHome)
	if codexHome == "" {
		return nil, nil
	}
	dbPath := filepath.Join(codexHome, "state_5.sqlite")
	if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?mode=ro")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	roots, err := loadCodexProjectRoots(ctx, db)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return nil, nil
		}
		return nil, err
	}
	if len(roots) == 0 {
		return nil, nil
	}
	stats, err := loadCodexThreadStats(ctx, db)
	if err != nil {
		stats = nil
	}

	result := make([]core.RecentWorkDir, 0, len(roots))
	seen := make(map[string]struct{}, len(roots))
	for _, root := range roots {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		info, err := os.Stat(root.path)
		if err != nil || !info.IsDir() {
			continue
		}
		key := normalizeCodexCwd(root.path)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if stat, ok := stats[key]; ok {
			root.sessionCount = stat.count
			if stat.lastActivity.After(root.lastActivity) {
				root.lastActivity = stat.lastActivity
			}
		}
		result = append(result, core.RecentWorkDir{
			Name:         root.name,
			Path:         root.path,
			LastActivity: root.lastActivity,
			SessionCount: root.sessionCount,
		})
	}

	sort.SliceStable(result, func(i, j int) bool {
		if result[i].LastActivity.Equal(result[j].LastActivity) {
			if result[i].Name == result[j].Name {
				return result[i].Path < result[j].Path
			}
			return result[i].Name < result[j].Name
		}
		return result[i].LastActivity.After(result[j].LastActivity)
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func loadCodexProjectRoots(ctx context.Context, db *sql.DB) ([]codexProjectRoot, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT p.name, p.updated_at_ms, pr.path
		FROM projects p
		JOIN project_roots pr ON pr.project_id = p.id
		ORDER BY p.position, pr.position`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roots []codexProjectRoot
	for rows.Next() {
		var root codexProjectRoot
		if err := rows.Scan(&root.name, &root.updatedAtMS, &root.path); err != nil {
			return nil, err
		}
		root.lastActivity = time.UnixMilli(root.updatedAtMS)
		roots = append(roots, root)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return roots, nil
}

func loadCodexThreadStats(ctx context.Context, db *sql.DB) (map[string]codexThreadStats, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT cwd, COUNT(*), COALESCE(MAX(updated_at_ms), MAX(updated_at) * 1000)
		FROM threads
		WHERE archived = 0
		GROUP BY cwd`)
	if err != nil {
		rows, err = db.QueryContext(ctx, `
			SELECT cwd, COUNT(*), MAX(updated_at) * 1000
			FROM threads
			WHERE archived = 0
			GROUP BY cwd`)
		if err != nil {
			return nil, err
		}
	}
	defer rows.Close()

	stats := make(map[string]codexThreadStats)
	for rows.Next() {
		var cwd string
		var count int
		var lastMS int64
		if err := rows.Scan(&cwd, &count, &lastMS); err != nil {
			return nil, err
		}
		stats[normalizeCodexCwd(cwd)] = codexThreadStats{
			count:        count,
			lastActivity: time.UnixMilli(lastMS),
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return stats, nil
}

func normalizeCodexCwd(path string) string {
	path = strings.TrimPrefix(path, `\\?\`)
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return path
}

var _ core.RecentWorkDirProvider = (*Agent)(nil)

package claudecode

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/chenhg5/cc-connect/core"
)

type claudeSessionFile struct {
	path     string
	modified int64
}

// RecentWorkDirs discovers project paths from Claude Code's project session
// directories. Claude encodes project directory names lossily, so the actual
// working directory is read from the environment snapshot embedded in recent
// session transcripts.
func (a *Agent) RecentWorkDirs(ctx context.Context, limit int) ([]core.RecentWorkDir, error) {
	if limit <= 0 {
		return nil, nil
	}
	configHome := a.configHome()
	if configHome == "" {
		return nil, fmt.Errorf("claudecode: cannot determine config home")
	}
	projectsBase := filepath.Join(configHome, "projects")
	entries, err := os.ReadDir(projectsBase)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("claudecode: read projects: %w", err)
	}

	type aggregate struct {
		path         string
		lastActivity int64
		sessionCount int
	}
	byPath := make(map[string]*aggregate)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		projectDir := filepath.Join(projectsBase, entry.Name())
		sessionFiles := readClaudeSessionFiles(projectDir)
		if len(sessionFiles) == 0 {
			continue
		}

		var cwd string
		probes := len(sessionFiles)
		if probes > 3 {
			probes = 3
		}
		for i := 0; i < probes && cwd == ""; i++ {
			cwd = readClaudeSessionWorkingDirectory(sessionFiles[i].path)
		}
		if cwd == "" {
			continue
		}
		absCwd, err := filepath.Abs(cwd)
		if err != nil {
			continue
		}
		info, err := os.Stat(absCwd)
		if err != nil || !info.IsDir() {
			continue
		}

		key := filepath.Clean(absCwd)
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		agg := byPath[key]
		if agg == nil {
			agg = &aggregate{path: absCwd}
			byPath[key] = agg
		}
		agg.sessionCount += len(sessionFiles)
		if latest := sessionFiles[0].modified; latest > agg.lastActivity {
			agg.lastActivity = latest
		}
	}

	result := make([]core.RecentWorkDir, 0, len(byPath))
	for _, agg := range byPath {
		result = append(result, core.RecentWorkDir{
			Path:         agg.path,
			LastActivity: time.Unix(0, agg.lastActivity),
			SessionCount: agg.sessionCount,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].LastActivity.Equal(result[j].LastActivity) {
			return result[i].Path < result[j].Path
		}
		return result[i].LastActivity.After(result[j].LastActivity)
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func readClaudeSessionFiles(projectDir string) []claudeSessionFile {
	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return nil
	}
	files := make([]claudeSessionFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, claudeSessionFile{
			path:     filepath.Join(projectDir, entry.Name()),
			modified: info.ModTime().UnixNano(),
		})
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].modified > files[j].modified
	})
	return files
}

func readClaudeSessionWorkingDirectory(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 256*1024), maxSessionJSONLLineSize)
	for scanner.Scan() {
		var entry struct {
			Type       string `json:"type"`
			Attachment struct {
				Type     string `json:"type"`
				Snapshot struct {
					WorkingDirectory string `json:"workingDirectory"`
				} `json:"snapshot"`
			} `json:"attachment"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		if entry.Type == "attachment" && entry.Attachment.Type == "environment" {
			if cwd := strings.TrimSpace(entry.Attachment.Snapshot.WorkingDirectory); cwd != "" {
				return cwd
			}
		}
	}
	return ""
}

var _ core.RecentWorkDirProvider = (*Agent)(nil)

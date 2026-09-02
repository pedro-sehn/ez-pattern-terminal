package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const defaultPatternsFolder = ".easy-pattern"

type PatternStore struct {
	WorkspaceDir string
	PatternsDir  string
	GlobalDir    string
}

func NewPatternStore(workspaceDir, patternsDir, globalDir string) *PatternStore {
	if patternsDir == "" {
		patternsDir = defaultPatternsFolder
	}
	return &PatternStore{WorkspaceDir: workspaceDir, PatternsDir: patternsDir, GlobalDir: globalDir}
}

func (s *PatternStore) workspacePath() string {
	if filepath.IsAbs(s.PatternsDir) {
		return s.PatternsDir
	}
	return filepath.Join(s.WorkspaceDir, s.PatternsDir)
}

func (s *PatternStore) Load() ([]LoadedPattern, error) {
	workspace, err := s.readDir(s.workspacePath(), "workspace")
	if err != nil {
		return nil, err
	}
	global, err := s.readDir(s.GlobalDir, "global")
	if err != nil {
		return nil, err
	}
	patterns := append(workspace, global...)
	for i := range patterns {
		for j := i + 1; j < len(patterns); j++ {
			if strings.ToLower(patterns[j].Pattern.Name) < strings.ToLower(patterns[i].Pattern.Name) {
				patterns[i], patterns[j] = patterns[j], patterns[i]
			}
		}
	}
	return patterns, nil
}

func (s *PatternStore) readDir(dir, scope string) ([]LoadedPattern, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	result := []LoadedPattern{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		body, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not read %s: %v\n", path, err)
			continue
		}
		var pattern Pattern
		if err := json.Unmarshal(body, &pattern); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not parse %s: %v\n", path, err)
			continue
		}
		if len(pattern.Files) == 0 {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: pattern must declare a non-empty files array\n", path)
			continue
		}
		if pattern.ID == "" {
			pattern.ID = strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		}
		if pattern.Name == "" {
			pattern.Name = pattern.ID
		}
		result = append(result, LoadedPattern{Pattern: pattern, FilePath: path, Scope: scope, WorkspaceName: filepath.Base(s.WorkspaceDir)})
	}
	return result, nil
}

func (s *PatternStore) Find(query string) (LoadedPattern, error) {
	patterns, err := s.Load()
	if err != nil {
		return LoadedPattern{}, err
	}
	for _, pattern := range patterns {
		if pattern.Pattern.ID == query || strings.EqualFold(pattern.Pattern.Name, query) {
			return pattern, nil
		}
	}
	return LoadedPattern{}, fmt.Errorf("pattern %q not found", query)
}

func (s *PatternStore) Save(pattern Pattern, global bool) (string, error) {
	if strings.TrimSpace(pattern.Name) == "" {
		return "", fmt.Errorf("pattern name is required")
	}
	if len(pattern.Files) == 0 {
		return "", fmt.Errorf("at least one file is required")
	}
	if pattern.ID == "" {
		pattern.ID = Slug(pattern.Name)
	}
	dir := s.workspacePath()
	if global {
		dir = s.GlobalDir
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, pattern.ID+".json")
	body, err := json.MarshalIndent(pattern, "", "  ")
	if err != nil {
		return "", err
	}
	body = append(body, '\n')
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func (s *PatternStore) Delete(loaded LoadedPattern) error { return os.Remove(loaded.FilePath) }

func Slug(input string) string {
	var builder strings.Builder
	lastDash := false
	previousDash := false
	for _, r := range strings.ToLower(input) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			lastDash = false
			previousDash = false
			continue
		}
		if builder.Len() > 0 {
			lastDash = true
		}
		if lastDash && !previousDash {
			builder.WriteByte('-')
			previousDash = true
		}
	}
	result := strings.Trim(builder.String(), "-")
	if result == "" {
		return "pattern"
	}
	return result
}

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func PlanFiles(loaded LoadedPattern, targetDir string, values Values) ([]PlannedFile, error) {
	root := ""
	var err error
	if loaded.Pattern.RootFolder != "" {
		root, err = Render(loaded.Pattern.RootFolder, values)
		if err != nil {
			return nil, err
		}
	}
	planned := make([]PlannedFile, 0, len(loaded.Pattern.Files))
	for _, file := range loaded.Pattern.Files {
		if !EvaluateCondition(file.When, values) {
			continue
		}
		pathTemplate := strings.ReplaceAll(file.Path, "\\", "/")
		pathTemplate = strings.TrimLeft(pathTemplate, "/")
		rendered, err := Render(pathTemplate, values)
		if err != nil {
			return nil, err
		}
		relative := filepath.Clean(filepath.Join(filepath.FromSlash(root), filepath.FromSlash(rendered)))
		if relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) || filepath.IsAbs(relative) {
			return nil, fmt.Errorf("pattern file path escapes the target directory: %q", relative)
		}
		content := ""
		if !file.Directory {
			content, err = Render(file.Content, values)
			if err != nil {
				return nil, err
			}
		}
		absolute := filepath.Join(targetDir, relative)
		_, statErr := os.Stat(absolute)
		exists := statErr == nil
		if statErr != nil && !os.IsNotExist(statErr) {
			return nil, statErr
		}
		planned = append(planned, PlannedFile{Path: absolute, RelativePath: filepath.ToSlash(relative), Content: content, Directory: file.Directory, Exists: exists})
	}
	if len(planned) == 0 {
		return nil, fmt.Errorf("nothing to generate: every file was excluded by its condition")
	}
	return planned, nil
}

func WritePlan(planned []PlannedFile) ([]string, error) {
	written := []string{}
	for _, file := range planned {
		if file.Directory {
			if err := os.MkdirAll(file.Path, 0o755); err != nil {
				return written, err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(file.Path), 0o755); err != nil {
			return written, err
		}
		if err := os.WriteFile(file.Path, []byte(file.Content), 0o644); err != nil {
			return written, err
		}
		written = append(written, file.Path)
	}
	return written, nil
}

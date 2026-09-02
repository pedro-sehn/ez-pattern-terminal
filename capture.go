package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var defaultCaptureIgnore = map[string]bool{
	"node_modules": true, ".git": true, "dist": true, "build": true, "out": true,
	".next": true, "__pycache__": true, ".DS_Store": true,
}

type capturedEntry struct {
	RelativePath, Content string
	Directory             bool
}

func CaptureFolder(folder, token string, ignore map[string]bool) ([]PatternFile, string, error) {
	baseName := filepath.Base(filepath.Clean(folder))
	entries, err := collectFolder(folder, "", ignore)
	if err != nil {
		return nil, "", err
	}
	if len(entries) == 0 {
		return nil, "", fmt.Errorf("that folder has no capturable files")
	}
	replace := func(value string) string { return value }
	if strings.TrimSpace(token) != "" {
		replace = templatizer(strings.TrimSpace(token))
	}
	files := make([]PatternFile, 0, len(entries))
	for _, entry := range entries {
		file := PatternFile{Path: replace(entry.RelativePath), Directory: entry.Directory}
		if !entry.Directory {
			file.Content = replace(entry.Content)
		}
		files = append(files, file)
	}
	root := baseName
	if strings.TrimSpace(token) != "" {
		root = replace(baseName)
	}
	return files, root, nil
}

func collectFolder(root, prefix string, ignore map[string]bool) ([]capturedEntry, error) {
	dir := root
	if prefix != "" {
		dir = filepath.Join(root, filepath.FromSlash(prefix))
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	result := []capturedEntry{}
	for _, entry := range entries {
		if ignore[entry.Name()] {
			continue
		}
		relative := entry.Name()
		if prefix != "" {
			relative = filepath.ToSlash(filepath.Join(prefix, entry.Name()))
		}
		if entry.IsDir() {
			children, err := collectFolder(root, relative, ignore)
			if err != nil {
				return nil, err
			}
			if len(children) == 0 {
				result = append(result, capturedEntry{RelativePath: relative, Directory: true})
			} else {
				result = append(result, children...)
			}
			continue
		}
		if !entry.Type().IsRegular() {
			continue
		}
		bytes, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			return nil, err
		}
		if isBinary(bytes) {
			continue
		}
		result = append(result, capturedEntry{RelativePath: relative, Content: string(bytes)})
	}
	return result, nil
}

func isBinary(bytes []byte) bool {
	limit := len(bytes)
	if limit > 1024 {
		limit = 1024
	}
	for _, b := range bytes[:limit] {
		if b == 0 {
			return true
		}
	}
	return false
}

func templatizer(token string) func(string) string {
	type variant struct{ text, filter string }
	variants := []variant{}
	for _, name := range []string{"pascal", "camel", "kebab", "snake", "constant", "title", "lower", "upper"} {
		produced := filters[name](token)
		seen := false
		for _, item := range variants {
			if item.text == produced {
				seen = true
				break
			}
		}
		if produced != "" && !seen {
			variants = append(variants, variant{produced, name})
		}
	}
	sort.Slice(variants, func(i, j int) bool { return len(variants[i].text) > len(variants[j].text) })
	return func(text string) string {
		var out strings.Builder
		for index := 0; index < len(text); {
			replaced := false
			for _, item := range variants {
				if strings.HasPrefix(text[index:], item.text) {
					out.WriteString("{{name | " + item.filter + "}}")
					index += len(item.text)
					replaced = true
					break
				}
			}
			if !replaced {
				out.WriteByte(text[index])
				index++
			}
		}
		return out.String()
	}
}

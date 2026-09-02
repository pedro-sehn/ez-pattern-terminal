package main

// Pattern is the JSON format shared with easy-pattern.
type Pattern struct {
	ID          string            `json:"id,omitempty"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	RootFolder  string            `json:"rootFolder,omitempty"`
	Variables   []PatternVariable `json:"variables"`
	Files       []PatternFile     `json:"files"`
}

type PatternVariable struct {
	Name         string   `json:"name"`
	Label        string   `json:"label,omitempty"`
	Type         string   `json:"type"`
	Description  string   `json:"description,omitempty"`
	Default      any      `json:"default,omitempty"`
	Options      []string `json:"options,omitempty"`
	Pattern      string   `json:"pattern,omitempty"`
	PatternError string   `json:"patternError,omitempty"`
	Required     *bool    `json:"required,omitempty"`
}

type PatternFile struct {
	Path      string `json:"path"`
	Content   string `json:"content,omitempty"`
	Directory bool   `json:"directory,omitempty"`
	When      string `json:"when,omitempty"`
}

type LoadedPattern struct {
	Pattern       Pattern
	FilePath      string
	Scope         string
	WorkspaceName string
}

type Values map[string]any

type PlannedFile struct {
	Path         string
	RelativePath string
	Content      string
	Directory    bool
	Exists       bool
}

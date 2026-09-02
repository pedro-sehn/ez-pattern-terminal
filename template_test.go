package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestWordsAndFilters(t *testing.T) {
	if got := Words("HTTP user-card"); !reflect.DeepEqual(got, []string{"http", "user", "card"}) {
		t.Fatalf("Words() = %#v", got)
	}
	values := Values{"name": "user card", "enabled": true}
	got, err := Render("{{name|pascal}} {{name|camel}} {{name|constant}} {{#if enabled}}yes{{else}}no{{/if}}", values)
	if err != nil {
		t.Fatal(err)
	}
	if got != "UserCard userCard USER_CARD yes" {
		t.Fatalf("Render() = %q", got)
	}
}

func TestRenderNestedAndUnknownTemplates(t *testing.T) {
	got, err := Render("{{#if outer}}{{#unless hidden}}kept{{/unless}}{{else}}removed{{/if}} {{.Values}} {{missing}}", Values{"outer": true, "hidden": false})
	if err != nil {
		t.Fatal(err)
	}
	if got != "kept {{.Values}} {{missing}}" {
		t.Fatalf("Render() = %q", got)
	}
}

func TestReferencedVariablesIgnoresFilters(t *testing.T) {
	got := ReferencedVariables("{{name | pascal}} {{#if withTest && !internal}}{{db|lower}}{{/if}} {{.GoValue}}")
	if !reflect.DeepEqual(got, []string{"db", "internal", "name", "withTest"}) {
		t.Fatalf("ReferencedVariables() = %#v", got)
	}
}

func TestTemplatizerDoesNotRewriteGeneratedTemplates(t *testing.T) {
	got := templatizer("name")("Name name")
	if got != "{{name | pascal}} {{name | camel}}" {
		t.Fatalf("templatizer() = %q", got)
	}
}

func TestPlanFilesRejectsEscapeAndRendersCondition(t *testing.T) {
	target := t.TempDir()
	loaded := LoadedPattern{Pattern: Pattern{RootFolder: "{{name|kebab}}", Files: []PatternFile{
		{Path: "main.txt", Content: "{{name}}", When: "enabled"},
		{Path: "skip.txt", Content: "no", When: "!enabled"},
	}}}
	planned, err := PlanFiles(loaded, target, Values{"name": "Hello World", "enabled": true})
	if err != nil {
		t.Fatal(err)
	}
	if len(planned) != 1 || planned[0].RelativePath != filepath.ToSlash(filepath.Join("hello-world", "main.txt")) || planned[0].Content != "Hello World" {
		t.Fatalf("unexpected plan: %#v", planned)
	}
	loaded.Pattern.Files = []PatternFile{{Path: "../../outside.txt"}}
	if _, err := PlanFiles(loaded, target, Values{}); err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("expected escape error, got %v", err)
	}
}

func TestCaptureFolder(t *testing.T) {
	root := filepath.Join(t.TempDir(), "UserCard")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "UserCard.tsx"), []byte("export function UserCard() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "node_modules", "ignored.js"), []byte("ignored"), 0o644); err != nil {
		t.Fatal(err)
	}
	files, rootTemplate, err := CaptureFolder(root, "UserCard", defaultCaptureIgnore)
	if err != nil {
		t.Fatal(err)
	}
	if rootTemplate != "{{name | pascal}}" {
		t.Fatalf("root template = %q", rootTemplate)
	}
	if len(files) != 2 {
		t.Fatalf("captured %d entries, want 2", len(files))
	}
	if files[0].Path != "{{name | pascal}}.tsx" || !strings.Contains(files[0].Content, "{{name | pascal}}") {
		t.Fatalf("unexpected captured file: %#v", files[0])
	}
	if !files[1].Directory || files[1].Path != "empty" {
		t.Fatalf("unexpected empty directory: %#v", files[1])
	}
}

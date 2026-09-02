package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string, in io.Reader, out, errOut io.Writer) error {
	if len(args) == 0 {
		printUsage(out)
		return nil
	}
	command := args[0]
	if command == "help" || command == "-h" || command == "--help" {
		printUsage(out)
		return nil
	}
	workspace, _ := os.Getwd()
	global := defaultGlobalDir()
	store := func(patternsDir, globalDir string) *PatternStore {
		return NewPatternStore(workspace, patternsDir, globalDir)
	}
	switch command {
	case "list":
		flags := flag.NewFlagSet("list", flag.ContinueOnError)
		flags.SetOutput(errOut)
		patternsDir := flags.String("patterns-dir", defaultPatternsFolder, "workspace pattern folder")
		globalDir := flags.String("global-dir", global, "global pattern folder")
		if err := flags.Parse(reorderFlags(args[1:], map[string]bool{"patterns-dir": true, "global-dir": true})); err != nil {
			return err
		}
		return listPatterns(store(*patternsDir, *globalDir), out)
	case "generate":
		return generateCommand(args[1:], in, out, errOut, workspace, global)
	case "create":
		return createCommand(args[1:], in, out, errOut, workspace, global)
	case "capture":
		return captureCommand(args[1:], in, out, errOut, workspace, global)
	case "edit":
		return editCommand(args[1:], in, out, errOut, workspace, global)
	case "duplicate":
		return duplicateCommand(args[1:], in, out, errOut, workspace, global)
	case "delete":
		return deleteCommand(args[1:], in, out, errOut, workspace, global)
	default:
		return fmt.Errorf("unknown command %q (try `ezplate help`)", command)
	}
}

func generateCommand(args []string, in io.Reader, out, errOut io.Writer, workspace, global string) error {
	flags := flag.NewFlagSet("generate", flag.ContinueOnError)
	flags.SetOutput(errOut)
	patternsDir := flags.String("patterns-dir", defaultPatternsFolder, "workspace pattern folder")
	globalDir := flags.String("global-dir", global, "global pattern folder")
	targetFlag := flags.String("target", ".", "directory where files are created")
	noConfirm := flags.Bool("no-confirm", false, "overwrite existing files without asking")
	skip := flags.Bool("skip-existing", false, "keep existing files")
	dryRun := flags.Bool("dry-run", false, "show files without writing")
	vars := stringFlags{}
	flags.Var(&vars, "var", "variable value, for example --var name=user-card")
	if err := flags.Parse(reorderFlags(args, map[string]bool{"patterns-dir": true, "global-dir": true, "target": true, "var": true})); err != nil {
		return err
	}
	positionals := flags.Args()
	query := ""
	if len(positionals) > 0 {
		query = positionals[0]
	}
	target := *targetFlag
	if len(positionals) > 1 {
		target = positionals[1]
	}
	if len(positionals) > 2 {
		return fmt.Errorf("generate accepts at most a pattern and target")
	}
	store := NewPatternStore(workspace, *patternsDir, *globalDir)
	patterns, err := store.Load()
	if err != nil {
		return err
	}
	if len(patterns) == 0 {
		return fmt.Errorf("no patterns found; add JSON files to %s or run `ezplate create`", filepath.Join(workspace, *patternsDir))
	}
	var loaded LoadedPattern
	if query == "" {
		loaded, err = choosePattern(patterns, NewPrompter(in, out), "Generate from pattern")
	} else {
		loaded, err = store.Find(query)
	}
	if err != nil {
		return err
	}
	values, err := NewPrompter(in, out).AskVariables(loaded.Pattern.Variables, vars)
	if err != nil {
		return err
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return err
	}
	if stat, statErr := os.Stat(target); statErr != nil || !stat.IsDir() {
		if statErr != nil {
			return fmt.Errorf("target directory: %w", statErr)
		}
		return fmt.Errorf("target is not a directory: %s", target)
	}
	planned, err := PlanFiles(loaded, target, values)
	if err != nil {
		return err
	}
	clashes := []PlannedFile{}
	for _, file := range planned {
		if file.Exists && !file.Directory {
			clashes = append(clashes, file)
		}
	}
	if len(clashes) > 0 && !*noConfirm && !*skip {
		fmt.Fprintf(out, "%d file(s) already exist:\n", len(clashes))
		for _, file := range clashes {
			fmt.Fprintf(out, "  %s\n", file.RelativePath)
		}
		answer, askErr := NewPrompter(in, out).input("Overwrite existing files? (y/n)", "", "n")
		if askErr != nil {
			return askErr
		}
		yes, parseErr := parseYesNo(answer)
		if parseErr != nil || !yes {
			return fmt.Errorf("generation cancelled")
		}
	}
	if *skip {
		filtered := planned[:0]
		for _, file := range planned {
			if !(file.Exists && !file.Directory) {
				filtered = append(filtered, file)
			}
		}
		planned = filtered
		if len(planned) == 0 {
			return fmt.Errorf("nothing left to generate")
		}
	}
	if *dryRun {
		for _, file := range planned {
			fmt.Fprintf(out, "%s%s\n", map[bool]string{true: "mkdir ", false: "write "}[file.Directory], file.RelativePath)
		}
		return nil
	}
	written, err := WritePlan(planned)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "created %d file(s) from %q\n", len(written), loaded.Pattern.Name)
	for _, path := range written {
		fmt.Fprintf(out, "  %s\n", filepath.ToSlash(path))
	}
	return nil
}

func listPatterns(store *PatternStore, out io.Writer) error {
	patterns, err := store.Load()
	if err != nil {
		return err
	}
	if len(patterns) == 0 {
		fmt.Fprintln(out, "No patterns found.")
		return nil
	}
	for _, loaded := range patterns {
		count := len(loaded.Pattern.Files)
		scope := loaded.Scope
		fmt.Fprintf(out, "%s [%s] (%d file", loaded.Pattern.Name, scope, count)
		if count != 1 {
			fmt.Fprint(out, "s")
		}
		fmt.Fprintf(out, ")\n  id: %s\n", loaded.Pattern.ID)
		if loaded.Pattern.Description != "" {
			fmt.Fprintf(out, "  %s\n", loaded.Pattern.Description)
		}
	}
	return nil
}

func createCommand(args []string, in io.Reader, out, errOut io.Writer, workspace, global string) error {
	flags := flag.NewFlagSet("create", flag.ContinueOnError)
	flags.SetOutput(errOut)
	patternsDir := flags.String("patterns-dir", defaultPatternsFolder, "workspace pattern folder")
	globalDir := flags.String("global-dir", global, "global pattern folder")
	useGlobal := flags.Bool("global", false, "save globally")
	if err := flags.Parse(reorderFlags(args, map[string]bool{"patterns-dir": true, "global-dir": true})); err != nil {
		return err
	}
	p := NewPrompter(in, out)
	name, err := p.input("Pattern name", `Examples: "React Component", "NestJS Module"`, "")
	if err != nil || strings.TrimSpace(name) == "" {
		return fmt.Errorf("pattern name is required")
	}
	description, err := p.input("Description (optional)", "Shown when picking a pattern", "")
	if err != nil {
		return err
	}
	root, err := p.input("Root folder template (optional)", "Leave empty to write directly into the target", "{{name | pascal}}")
	if err != nil {
		return err
	}
	filesInput, err := p.input("Files to create", "Comma-separated paths, templates allowed", "{{name | pascal}}.tsx, index.ts")
	if err != nil || strings.TrimSpace(filesInput) == "" {
		return fmt.Errorf("at least one file is required")
	}
	files := []PatternFile{}
	for _, path := range strings.Split(filesInput, ",") {
		if strings.TrimSpace(path) != "" {
			files = append(files, PatternFile{Path: strings.TrimSpace(path)})
		}
	}
	pattern := Pattern{ID: Slug(name), Name: strings.TrimSpace(name), Description: strings.TrimSpace(description), RootFolder: strings.TrimSpace(root), Variables: inferVariables(append([]string{root}, filesPaths(files)...), nil), Files: files}
	path, err := NewPatternStore(workspace, *patternsDir, *globalDir).Save(pattern, *useGlobal)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "created pattern %q at %s\n", pattern.Name, path)
	return openEditor(path, out)
}

func captureCommand(args []string, in io.Reader, out, errOut io.Writer, workspace, global string) error {
	flags := flag.NewFlagSet("capture", flag.ContinueOnError)
	flags.SetOutput(errOut)
	patternsDir := flags.String("patterns-dir", defaultPatternsFolder, "workspace pattern folder")
	globalDir := flags.String("global-dir", global, "global pattern folder")
	useGlobal := flags.Bool("global", false, "save globally")
	tokenFlag := flags.String("token", "", "word to replace with {{name}} (empty keeps files verbatim)")
	nameFlag := flags.String("name", "", "pattern name")
	if err := flags.Parse(reorderFlags(args, map[string]bool{"patterns-dir": true, "global-dir": true, "token": true, "name": true})); err != nil {
		return err
	}
	if flags.NArg() < 1 {
		return fmt.Errorf("capture requires a folder")
	}
	folder, err := filepath.Abs(flags.Arg(0))
	if err != nil {
		return err
	}
	stat, err := os.Stat(folder)
	if err != nil {
		return err
	}
	if !stat.IsDir() {
		return fmt.Errorf("capture requires a folder")
	}
	p := NewPrompter(in, out)
	token := *tokenFlag
	if token == "" && !hasOption(args, "token") {
		token, err = p.input("Which word should become the variable?", "Leave empty to capture files verbatim", filepath.Base(folder))
		if err != nil {
			return err
		}
	}
	name := *nameFlag
	if name == "" && !hasOption(args, "name") {
		name, err = p.input("Pattern name", "", filepath.Base(folder)+" pattern")
		if err != nil {
			return err
		}
	}
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("pattern name is required")
	}
	files, root, err := CaptureFolder(folder, token, defaultCaptureIgnore)
	if err != nil {
		return err
	}
	known := []PatternVariable{}
	if strings.TrimSpace(token) != "" {
		known = append(known, PatternVariable{Name: "name", Label: "Name", Type: "string", Description: "Replaces \"" + strings.TrimSpace(token) + "\""})
	}
	pattern := Pattern{ID: Slug(name), Name: strings.TrimSpace(name), Description: "Captured from " + filepath.Base(folder), RootFolder: root, Variables: inferVariables(append([]string{root}, filesPaths(files)...), known), Files: files}
	path, err := NewPatternStore(workspace, *patternsDir, *globalDir).Save(pattern, *useGlobal)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "captured %d entries into %s\n", len(files), path)
	return openEditor(path, out)
}

func editCommand(args []string, in io.Reader, out, errOut io.Writer, workspace, global string) error {
	store, query, err := commandStore(args, errOut, workspace, global, "edit")
	if err != nil {
		return err
	}
	loaded, err := findOrChoose(store, query, in, out, "Edit pattern")
	if err != nil {
		return err
	}
	return openEditor(loaded.FilePath, out)
}

func duplicateCommand(args []string, in io.Reader, out, errOut io.Writer, workspace, global string) error {
	flags := flag.NewFlagSet("duplicate", flag.ContinueOnError)
	flags.SetOutput(errOut)
	patternsDir := flags.String("patterns-dir", defaultPatternsFolder, "workspace pattern folder")
	globalDir := flags.String("global-dir", global, "global pattern folder")
	useGlobal := flags.Bool("global", false, "save globally")
	nameFlag := flags.String("name", "", "new pattern name")
	if err := flags.Parse(reorderFlags(args, map[string]bool{"patterns-dir": true, "global-dir": true, "name": true})); err != nil {
		return err
	}
	store := NewPatternStore(workspace, *patternsDir, *globalDir)
	loaded, err := findOrChoose(store, flags.Arg(0), in, out, "Duplicate pattern")
	if err != nil {
		return err
	}
	name := *nameFlag
	if name == "" {
		name, err = NewPrompter(in, out).input("Name for the copy", "", loaded.Pattern.Name+" copy")
		if err != nil {
			return err
		}
	}
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("pattern name is required")
	}
	pattern := loaded.Pattern
	pattern.ID, pattern.Name = Slug(name), strings.TrimSpace(name)
	path, err := store.Save(pattern, *useGlobal)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "duplicated pattern to %s\n", path)
	return openEditor(path, out)
}

func deleteCommand(args []string, in io.Reader, out, errOut io.Writer, workspace, global string) error {
	store, query, err := commandStore(args, errOut, workspace, global, "delete")
	if err != nil {
		return err
	}
	loaded, err := findOrChoose(store, query, in, out, "Delete pattern")
	if err != nil {
		return err
	}
	answer, err := NewPrompter(in, out).input(`Delete pattern "`+loaded.Pattern.Name+`"? (y/n)`, loaded.FilePath, "n")
	if err != nil {
		return err
	}
	yes, _ := parseYesNo(answer)
	if !yes {
		return fmt.Errorf("deletion cancelled")
	}
	if err := store.Delete(loaded); err != nil {
		return err
	}
	fmt.Fprintf(out, "deleted %q\n", loaded.Pattern.Name)
	return nil
}

func commandStore(args []string, errOut io.Writer, workspace, global, name string) (*PatternStore, string, error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(errOut)
	patternsDir := flags.String("patterns-dir", defaultPatternsFolder, "workspace pattern folder")
	globalDir := flags.String("global-dir", global, "global pattern folder")
	if err := flags.Parse(reorderFlags(args, map[string]bool{"patterns-dir": true, "global-dir": true})); err != nil {
		return nil, "", err
	}
	return NewPatternStore(workspace, *patternsDir, *globalDir), flags.Arg(0), nil
}

func findOrChoose(store *PatternStore, query string, in io.Reader, out io.Writer, title string) (LoadedPattern, error) {
	patterns, err := store.Load()
	if err != nil {
		return LoadedPattern{}, err
	}
	if query != "" {
		return store.Find(query)
	}
	return choosePattern(patterns, NewPrompter(in, out), title)
}

func choosePattern(patterns []LoadedPattern, p *Prompter, title string) (LoadedPattern, error) {
	if len(patterns) == 0 {
		return LoadedPattern{}, fmt.Errorf("no patterns found")
	}
	fmt.Fprintf(p.Writer, "%s\n", title)
	for i, pattern := range patterns {
		fmt.Fprintf(p.Writer, "  %d) %s", i+1, pattern.Pattern.Name)
		if pattern.Pattern.Description != "" {
			fmt.Fprintf(p.Writer, " - %s", pattern.Pattern.Description)
		}
		fmt.Fprintln(p.Writer)
	}
	answer, err := p.input("Choose", "", "1")
	if err != nil {
		return LoadedPattern{}, err
	}
	index, err := strconv.Atoi(answer)
	if err != nil || index < 1 || index > len(patterns) {
		return LoadedPattern{}, fmt.Errorf("choose a number from 1 to %d", len(patterns))
	}
	return patterns[index-1], nil
}

func inferVariables(templates []string, known []PatternVariable) []PatternVariable {
	result := append([]PatternVariable{}, known...)
	seen := map[string]bool{}
	for _, variable := range result {
		seen[variable.Name] = true
	}
	for _, template := range templates {
		for _, name := range ReferencedVariables(template) {
			if !seen[name] {
				seen[name] = true
				label := strings.Join(Words(name), " ")
				if label != "" {
					label = strings.ToUpper(label[:1]) + label[1:]
				}
				result = append(result, PatternVariable{Name: name, Label: label, Type: "string"})
			}
		}
	}
	return result
}
func filesPaths(files []PatternFile) []string {
	result := make([]string, len(files))
	for i, file := range files {
		result[i] = file.Path
	}
	return result
}
func defaultGlobalDir() string {
	config, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", "terminal-ezplate", "patterns")
	}
	return filepath.Join(config, "terminal-ezplate", "patterns")
}
func openEditor(path string, out io.Writer) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		fmt.Fprintf(out, "edit pattern with $EDITOR: %s\n", path)
		return nil
	}
	command := exec.Command("sh", "-c", editor+" \"$1\"", "editor", path)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	return command.Run()
}
func printUsage(out io.Writer) {
	fmt.Fprintln(out, `ezplate - scaffold files from reusable patterns

Usage:
  ezplate list [--patterns-dir DIR] [--global-dir DIR]
  ezplate generate [PATTERN] [TARGET] [--var name=value] [--dry-run]
  ezplate create [--global]
  ezplate capture FOLDER [--token WORD] [--name NAME] [--global]
  ezplate edit [PATTERN]
  ezplate duplicate [PATTERN] [--name NAME] [--global]
  ezplate delete [PATTERN]

Patterns use the easy-pattern JSON format. Workspace patterns live in
.easy-pattern by default; global patterns live in the OS config directory.`)
}

// reorderFlags lets users put flags after positional arguments, as is common in
// terminal commands even though the standard flag package stops at the first one.
func reorderFlags(args []string, valueFlags map[string]bool) []string {
	options, positionals := []string{}, []string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positionals = append(positionals, arg)
			continue
		}
		options = append(options, arg)
		name := strings.TrimLeft(strings.SplitN(arg, "=", 2)[0], "-")
		if !strings.Contains(arg, "=") && valueFlags[name] && i+1 < len(args) {
			i++
			options = append(options, args[i])
		}
	}
	return append(options, positionals...)
}

func hasOption(args []string, wanted string) bool {
	for _, arg := range args {
		name := strings.TrimLeft(strings.SplitN(arg, "=", 2)[0], "-")
		if name == wanted {
			return true
		}
	}
	return false
}

type stringFlags map[string]string

func (s *stringFlags) String() string { return "" }
func (s *stringFlags) Set(value string) error {
	key, value, ok := strings.Cut(value, "=")
	if !ok || strings.TrimSpace(key) == "" {
		return fmt.Errorf("variable must be name=value")
	}
	if *s == nil {
		*s = stringFlags{}
	}
	(*s)[key] = value
	return nil
}

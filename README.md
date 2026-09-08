# ezplate

Design a file or folder pattern once, then scaffold it anywhere from the terminal.

`ezplate` reads the same JSON format as [easy-pattern](https://github.com/pedro-sehn/easy-pattern), so patterns can move between the VS Code extension and this CLI.

## Quick Start

```bash
go build -o ezplate .
mkdir -p .easy-pattern
cp examples/react-component.json .easy-pattern/
./ezplate list
./ezplate generate react-component ./src/components
```

Generation asks for each declared variable, renders paths and file bodies, creates missing directories, and confirms before overwriting existing files.

Use `--var` to make generation scriptable:

```bash
./ezplate generate react-component ./src/components \
  --var name=user-card --var withTest=true --var withStyles=false
```

Preview a generation without writing anything with `--dry-run`. Use `--skip-existing` or `--no-confirm` for overwrite behavior.

## Commands

| Command | Purpose |
| --- | --- |
| `list` | List workspace and global patterns |
| `generate [PATTERN] [TARGET]` | Render a pattern into a target directory |
| `create` | Interactively create a new pattern definition |
| `capture FOLDER` | Turn an existing folder into a reusable pattern |
| `edit [PATTERN]` | Open a pattern in `$EDITOR` |
| `duplicate [PATTERN]` | Copy a pattern with a new name |
| `delete [PATTERN]` | Delete a pattern after confirmation |

Run `ezplate help` for all flags.

## Pattern Storage

Workspace patterns are JSON files in `.easy-pattern/` by default. They are intended to be committed with a repository. Global patterns are stored under the OS configuration directory in `terminal-ezplate/patterns` and can be selected with `--global` when creating, capturing, or duplicating.

Change either location with `--patterns-dir` and `--global-dir`.

## Pattern Format

```json
{
  "name": "React Component",
  "rootFolder": "{{name | pascal}}",
  "variables": [
    { "name": "name", "label": "Component name", "type": "string" },
    { "name": "withTest", "label": "Add a test?", "type": "boolean", "default": true }
  ],
  "files": [
    { "path": "{{name | pascal}}.tsx", "content": "export default function {{name | pascal}}() {}\n" },
    { "path": "{{name | pascal}}.test.tsx", "when": "withTest", "content": "" }
  ]
}
```

Templates support chained `pascal`, `camel`, `kebab`, `snake`, `constant`, `dot`, `path`, `title`, `lower`, `upper`, `capitalize`, `trim`, `plural`, and `singular` filters. Conditional blocks support `{{#if value}}...{{else}}...{{/if}}` and `{{#unless value}}...{{/unless}}`. Unknown variables are left untouched, allowing captured Go, Vue, or Handlebars templates to survive generation.

`capture` recursively collects text files, skips binary files, preserves empty directories, and replaces casing variants of the selected token with `{{name | filter}}`. It skips `node_modules`, `.git`, build output, and cache directories by default.

## Development

```bash
go test ./...
go build ./...
```

The application uses only the Go standard library.

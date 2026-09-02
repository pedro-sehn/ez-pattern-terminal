package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var (
	camelBoundary      = regexp.MustCompile(`([a-z0-9])([A-Z])`)
	acronymBoundary    = regexp.MustCompile(`([A-Z]+)([A-Z][a-z])`)
	wordSeparator      = regexp.MustCompile(`[^a-zA-Z0-9]+`)
	identifier         = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	variableExpression = regexp.MustCompile(`\{\{([^#/][^}]*)\}\}`)
	blockOpen          = regexp.MustCompile(`\{\{#(if|unless)\s+([^}]+?)\}\}`)
	blockToken         = regexp.MustCompile(`\{\{(#(?:if|unless)\s+[^}]+|/(?:if|unless)|else)\}\}`)
)

// Words splits camelCase, kebab-case, snake_case, and spaced identifiers.
func Words(input string) []string {
	input = camelBoundary.ReplaceAllString(input, `$1 $2`)
	input = acronymBoundary.ReplaceAllString(input, `$1 $2`)
	parts := wordSeparator.Split(input, -1)
	words := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			words = append(words, strings.ToLower(part))
		}
	}
	return words
}

func upperFirst(value string) string {
	if value == "" {
		return value
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

var filters = map[string]func(string) string{
	"pascal": func(v string) string {
		var out string
		for _, word := range Words(v) {
			out += upperFirst(word)
		}
		return out
	},
	"camel": func(v string) string {
		words := Words(v)
		if len(words) == 0 {
			return ""
		}
		out := words[0]
		for _, word := range words[1:] {
			out += upperFirst(word)
		}
		return out
	},
	"kebab":    func(v string) string { return strings.Join(Words(v), "-") },
	"snake":    func(v string) string { return strings.Join(Words(v), "_") },
	"constant": func(v string) string { return strings.ToUpper(strings.Join(Words(v), "_")) },
	"dot":      func(v string) string { return strings.Join(Words(v), ".") },
	"path":     func(v string) string { return strings.Join(Words(v), "/") },
	"title": func(v string) string {
		words := Words(v)
		for i := range words {
			words[i] = upperFirst(words[i])
		}
		return strings.Join(words, " ")
	},
	"lower":      strings.ToLower,
	"upper":      strings.ToUpper,
	"capitalize": upperFirst,
	"trim":       strings.TrimSpace,
	"plural":     plural,
	"singular":   singular,
}

func plural(value string) string {
	if regexp.MustCompile(`(?i)(s|x|z|ch|sh)$`).MatchString(value) {
		return value + "es"
	}
	if regexp.MustCompile(`(?i)[^aeiou]y$`).MatchString(value) {
		return value[:len(value)-1] + "ies"
	}
	return value + "s"
}

func singular(value string) string {
	lower := strings.ToLower(value)
	if strings.HasSuffix(lower, "ies") {
		return value[:len(value)-3] + "y"
	}
	if regexp.MustCompile(`(?i)(ses|xes|zes|ches|shes)$`).MatchString(value) {
		return value[:len(value)-2]
	}
	if strings.HasSuffix(lower, "s") && !strings.HasSuffix(lower, "ss") {
		return value[:len(value)-1]
	}
	return value
}

func truthy(values Values, expression string) bool {
	negated := strings.HasPrefix(expression, "!")
	key := strings.TrimSpace(strings.TrimPrefix(expression, "!"))
	raw, ok := values[key]
	value := false
	if ok {
		switch typed := raw.(type) {
		case bool:
			value = typed
		case string:
			value = strings.TrimSpace(typed) != "" && typed != "false"
		default:
			value = raw != nil
		}
	}
	if negated {
		return !value
	}
	return value
}

// EvaluateCondition supports the easy-pattern condition syntax: !, &&, and ||.
func EvaluateCondition(expression string, values Values) bool {
	if strings.TrimSpace(expression) == "" {
		return true
	}
	for _, orPart := range strings.Split(expression, "||") {
		all := true
		seen := false
		for _, token := range strings.Split(orPart, "&&") {
			token = strings.TrimSpace(token)
			if token == "" {
				continue
			}
			seen = true
			if !truthy(values, token) {
				all = false
				break
			}
		}
		if seen && all {
			return true
		}
	}
	return false
}

func resolveExpression(expression string, values Values, original string) (string, error) {
	parts := strings.Split(expression, "|")
	name := strings.TrimSpace(parts[0])
	raw, ok := values[name]
	if !ok || raw == nil {
		return original, nil
	}
	out := fmt.Sprint(raw)
	for _, step := range parts[1:] {
		name := strings.TrimSpace(step)
		if name == "" {
			continue
		}
		filter, ok := filters[name]
		if !ok {
			return "", fmt.Errorf("unknown filter %q in {{%s}}", name, expression)
		}
		out = filter(out)
	}
	return out, nil
}

// Render renders variables and nested if/unless blocks without executing code.
func Render(template string, values Values) (string, error) {
	withBlocks := template
	for guard := 0; guard < 1000; guard++ {
		match := blockOpen.FindStringSubmatchIndex(withBlocks)
		if match == nil {
			break
		}
		start, end := match[0], match[1]
		kind := withBlocks[match[2]:match[3]]
		condition := withBlocks[match[4]:match[5]]
		closeStart, closeEnd, elseStart, elseEnd, ok := matchingBlock(withBlocks, end)
		if !ok {
			break
		}
		body := withBlocks[end:closeStart]
		trueBody, falseBody := body, ""
		if elseStart >= 0 {
			trueBody = body[:elseStart-end]
			falseBody = body[elseEnd-end:]
		}
		chosen := trueBody
		keep := EvaluateCondition(condition, values)
		if kind == "unless" {
			keep = !keep
		}
		if !keep {
			chosen = falseBody
		}
		rendered, err := Render(chosen, values)
		if err != nil {
			return "", err
		}
		withBlocks = withBlocks[:start] + rendered + withBlocks[closeEnd:]
	}

	var firstErr error
	result := variableExpression.ReplaceAllStringFunc(withBlocks, func(full string) string {
		if firstErr != nil {
			return full
		}
		inside := full[2 : len(full)-2]
		value, err := resolveExpression(inside, values, full)
		if err != nil {
			firstErr = err
			return full
		}
		return value
	})
	return result, firstErr
}

func matchingBlock(input string, bodyStart int) (closeStart, closeEnd, elseStart, elseEnd int, ok bool) {
	depth := 1
	elseStart, elseEnd = -1, -1
	for _, match := range blockToken.FindAllStringIndex(input[bodyStart:], -1) {
		start := bodyStart + match[0]
		end := bodyStart + match[1]
		token := input[start:end]
		switch {
		case strings.HasPrefix(token, "{{#"):
			depth++
		case token == "{{else}}" && depth == 1:
			elseStart, elseEnd = start, end
		case strings.HasPrefix(token, "{{/"):
			depth--
			if depth == 0 {
				return start, end, elseStart, elseEnd, true
			}
		}
	}
	return 0, 0, -1, -1, false
}

// ReferencedVariables returns declared-variable candidates used in templates.
func ReferencedVariables(template string) []string {
	found := map[string]bool{}
	for _, match := range regexp.MustCompile(`\{\{([^}]*)\}\}`).FindAllStringSubmatch(template, -1) {
		body := strings.TrimSpace(match[1])
		if body == "else" || strings.HasPrefix(body, "/") {
			continue
		}
		if strings.HasPrefix(body, "#if ") {
			body = strings.TrimSpace(strings.TrimPrefix(body, "#if "))
		} else if strings.HasPrefix(body, "#unless ") {
			body = strings.TrimSpace(strings.TrimPrefix(body, "#unless "))
		}
		for _, orPart := range strings.Split(body, "||") {
			for _, part := range strings.Split(orPart, "&&") {
				name := strings.TrimSpace(strings.Split(part, "|")[0])
				name = strings.TrimSpace(strings.TrimPrefix(name, "!"))
				if identifier.MatchString(name) {
					found[name] = true
				}
			}
		}
	}
	result := make([]string, 0, len(found))
	for name := range found {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

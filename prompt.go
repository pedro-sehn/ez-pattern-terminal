package main

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

type Prompter struct {
	Reader *bufio.Reader
	Writer io.Writer
}

func NewPrompter(in io.Reader, out io.Writer) *Prompter {
	return &Prompter{Reader: bufio.NewReader(in), Writer: out}
}

func (p *Prompter) input(label, description, defaultValue string) (string, error) {
	if description != "" {
		fmt.Fprintf(p.Writer, "%s\n", description)
	}
	if defaultValue != "" {
		fmt.Fprintf(p.Writer, "%s [%s]: ", label, defaultValue)
	} else {
		fmt.Fprintf(p.Writer, "%s: ", label)
	}
	line, err := p.Reader.ReadString('\n')
	if err != nil && len(line) == 0 {
		return "", fmt.Errorf("input cancelled")
	}
	line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	if line == "" {
		return defaultValue, nil
	}
	return line, nil
}

func (p *Prompter) AskVariables(variables []PatternVariable, supplied map[string]string) (Values, error) {
	values := Values{}
	for _, variable := range variables {
		label := variable.Label
		if label == "" {
			label = variable.Name
		}
		given, provided := supplied[variable.Name]
		switch variable.Type {
		case "boolean":
			value := variable.Default == true || variable.Default == "true"
			if provided {
				parsed, err := strconv.ParseBool(given)
				if err != nil {
					parsed, err = parseYesNo(given)
					if err != nil {
						return nil, fmt.Errorf("%s must be true or false", label)
					}
				}
				value = parsed
			} else {
				answer, err := p.input(label+" (y/n)", variable.Description, map[bool]string{true: "y", false: "n"}[value])
				if err != nil {
					return nil, err
				}
				if answer != "" {
					parsed, err := parseYesNo(answer)
					if err != nil {
						return nil, fmt.Errorf("%s: %v", label, err)
					}
					value = parsed
				}
			}
			values[variable.Name] = value
		case "choice":
			if len(variable.Options) == 0 {
				return nil, fmt.Errorf("variable %q is a choice but has no options", variable.Name)
			}
			choice := ""
			if value, ok := variable.Default.(string); ok {
				choice = value
			}
			if provided {
				choice = given
			} else {
				fmt.Fprintf(p.Writer, "%s\n", label)
				for i, option := range variable.Options {
					marker := " "
					if option == choice {
						marker = "*"
					}
					fmt.Fprintf(p.Writer, "  %s %d) %s\n", marker, i+1, option)
				}
				answer, err := p.input("Choose", variable.Description, choice)
				if err != nil {
					return nil, err
				}
				if answer != choice {
					index, parseErr := strconv.Atoi(answer)
					if parseErr == nil && index >= 1 && index <= len(variable.Options) {
						choice = variable.Options[index-1]
					} else {
						valid := false
						for _, option := range variable.Options {
							if answer == option {
								valid = true
								break
							}
						}
						if !valid {
							return nil, fmt.Errorf("choose a number from 1 to %d", len(variable.Options))
						}
						choice = answer
					}
				}
			}
			valid := false
			for _, option := range variable.Options {
				if option == choice {
					valid = true
				}
			}
			if !valid {
				return nil, fmt.Errorf("%s is not a valid option for %s", choice, label)
			}
			values[variable.Name] = choice
		default:
			defaultValue, _ := variable.Default.(string)
			value := given
			if !provided {
				var err error
				value, err = p.input(label, variable.Description, defaultValue)
				if err != nil {
					return nil, err
				}
			}
			if variable.Required == nil || *variable.Required {
				if strings.TrimSpace(value) == "" {
					return nil, fmt.Errorf("%s is required", label)
				}
			}
			if variable.Pattern != "" && value != "" {
				matched, err := regexp.MatchString(variable.Pattern, value)
				if err != nil {
					return nil, fmt.Errorf("invalid pattern for %s: %v", label, err)
				}
				if !matched {
					message := variable.PatternError
					if message == "" {
						message = "must match " + variable.Pattern
					}
					return nil, fmt.Errorf("%s", message)
				}
			}
			values[variable.Name] = value
		}
	}
	return values, nil
}

func parseYesNo(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "y", "yes", "true":
		return true, nil
	case "n", "no", "false":
		return false, nil
	default:
		return false, fmt.Errorf("answer yes or no")
	}
}

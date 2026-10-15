package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"campfire/internal/shared/model"
)

func splitEggRules(text string) ([]string, error) {
	rules := []string{}

	for text != "" {
		end := strings.IndexByte(text, '|')
		if strings.HasPrefix(text, "regex:") {
			if len(text) < 8 || !strings.ContainsRune("/~#%", rune(text[6])) {
				return nil, errors.New("use a supported delimited regex")
			}

			delimiter := text[6]
			escaped, inClass := false, false
			closing := -1

			for i := 7; i < len(text); i++ {
				ch := text[i]

				if escaped {
					escaped = false
					continue
				}

				if ch == '\\' {
					escaped = true
					continue
				}

				if ch == '[' {
					inClass = true
				}

				if ch == ']' {
					inClass = false
				}

				if ch == delimiter && !inClass {
					closing = i
					break
				}
			}

			if closing < 0 {
				return nil, errors.New("unterminated regex rule")
			}

			end = strings.IndexByte(text[closing+1:], '|')
			if end >= 0 {
				end += closing + 1
			}
		}

		if end < 0 {
			rules = append(rules, text)
			break
		}

		if end == 0 {
			return nil, errors.New("empty validation rule")
		}

		rules = append(rules, text[:end])
		text = text[end+1:]
	}

	return rules, nil
}

func eggRegex(value string) (string, error) {
	if len(value) < 2 || !strings.ContainsRune("/~#%", rune(value[0])) {
		return "", errors.New("unsupported regex delimiter")
	}

	end := strings.LastIndexByte(value, value[0])
	if end < 1 {
		return "", errors.New("unterminated regex")
	}

	flags := ""

	for _, flag := range value[end+1:] {
		if !strings.ContainsRune("ims", flag) {
			return "", errors.New("unsupported PCRE modifier; only i, m and s are supported")
		}

		flags += string(flag)
	}

	pattern := strings.ReplaceAll(value[1:end], "\\"+string(value[0]), string(value[0]))
	if flags != "" {
		pattern = "(?" + flags + ")" + pattern
	}

	if len(pattern) > 512 {
		return "", errors.New("regex exceeds 512 bytes")
	}

	if _, err := regexp.Compile(pattern); err != nil {
		return "", errors.New("regex is outside the supported RE2 subset (no lookarounds or backreferences)")
	}

	return pattern, nil
}

func convertEggRules(raw json.RawMessage) (model.Variable, error) {
	rules := []string{}

	if !emptyEggValue(raw) {
		var text string
		if err := json.Unmarshal(raw, &text); err == nil {
			parsed, err := splitEggRules(text)
			if err != nil {
				return model.Variable{}, err
			}

			rules = parsed
		} else if err := json.Unmarshal(raw, &rules); err != nil {
			return model.Variable{}, errors.New("rules must be a string or string array")
		}
	}

	variable := model.Variable{Validation: &model.VariableValidation{Type: "string"}}
	validation := variable.Validation

	for _, rule := range rules {
		switch rule {
		case "integer", "numeric", "boolean":
			if validation.Type != "string" && validation.Type != rule {
				return variable, errors.New("conflicting variable types")
			}

			validation.Type = rule
		}
	}

	for _, rule := range rules {
		name, arg, hasArg := strings.Cut(rule, ":")
		switch name {
		case "required":
			variable.Required = true
		case "nullable":
			validation.Nullable = true
		case "string", "integer", "numeric", "boolean":
		case "regex":
			if !hasArg || variable.Pattern != "" {
				return variable, errors.New("only one delimited regex rule is supported")
			}

			pattern, err := eggRegex(arg)
			if err != nil {
				return variable, err
			}

			variable.Pattern = pattern
		case "in":
			if !hasArg || len(validation.Choices) > 0 {
				return variable, errors.New("invalid choice rule")
			}

			if strings.ContainsAny(arg, "\"\\") {
				return variable, errors.New("quoted or escaped choice lists are unsupported")
			}

			validation.Choices = strings.Split(arg, ",")
		case "min", "max", "between", "size":
			if !hasArg {
				return variable, errors.New("validation bound requires a value")
			}

			bounds := []string{arg}
			if name == "between" {
				bounds = strings.Split(arg, ",")
				if len(bounds) != 2 {
					return variable, errors.New("between requires two bounds")
				}
			}

			for index, bound := range bounds {
				minimum := name == "min" || name == "between" && index == 0 || name == "size"
				maximum := name == "max" || name == "between" && index == 1 || name == "size"

				switch validation.Type {
				case "integer", "numeric":
					value, err := model.NumericBound(bound)
					if err != nil {
						return variable, errors.New("invalid numeric bound")
					}

					if minimum && (validation.Min == nil || value > *validation.Min) {
						validation.Min = new(value)
					}

					if maximum && (validation.Max == nil || value < *validation.Max) {
						validation.Max = new(value)
					}
				case "string":
					value, err := strconv.Atoi(bound)
					if err != nil {
						return variable, errors.New("string bounds must be character counts")
					}

					if minimum && (validation.MinLength == nil || value > *validation.MinLength) {
						validation.MinLength = new(value)
					}

					if maximum && (validation.MaxLength == nil || value < *validation.MaxLength) {
						validation.MaxLength = new(value)
					}
				default:
					return variable, errors.New("boolean bounds are unsupported")
				}
			}
		default:
			return variable, fmt.Errorf("unsupported validation rule %q", name)
		}

		if hasArg && name != "regex" && name != "in" && name != "min" && name != "max" && name != "between" && name != "size" {
			return variable, errors.New("unexpected validation rule argument")
		}
	}

	if err := validation.Validate(); err != nil {
		return variable, err
	}

	return variable, nil
}

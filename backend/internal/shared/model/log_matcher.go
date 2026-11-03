package model

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

type compiledLogMatch struct {
	literal    string
	pattern    *regexp.Regexp
	ignoreCase bool
}

// LogMatcher compiles bounded literal/RE2 alternatives once for a log consumer.
type LogMatcher struct {
	any []compiledLogMatch
}

// CompileLogMatcher prepares validated matching rules for repeated log input.
func CompileLogMatcher(anyOf []LogMatch) LogMatcher {
	result := LogMatcher{}

	for _, match := range anyOf {
		item := compiledLogMatch{literal: match.Value, ignoreCase: match.IgnoreCase}
		if match.Regex {
			pattern := match.Value
			if match.IgnoreCase {
				pattern = "(?i:" + pattern + ")"
			}

			var err error

			item.pattern, err = regexp.Compile(pattern)
			if err != nil {
				continue
			}
		} else if match.IgnoreCase {
			item.literal = strings.ToLower(item.literal)
		}

		result.any = append(result.any, item)
	}

	return result
}

// Matches evaluates OR alternatives against a single console line.
func (m LogMatcher) Matches(line string) bool {
	line = StripLogANSI(line)

	for _, match := range m.any {
		if match.pattern != nil {
			if match.pattern.MatchString(line) {
				return true
			}
		} else {
			text := line
			if match.ignoreCase {
				text = strings.ToLower(text)
			}

			if strings.Contains(text, match.literal) {
				return true
			}
		}
	}

	return false
}

// ReadinessMatcher preserves native literal and extended readiness semantics.
func (definition RuntimeDefinition) ReadinessMatcher() LogMatcher {
	if definition.Readiness != nil {
		return CompileLogMatcher(definition.Readiness.Any)
	}

	return CompileLogMatcher([]LogMatch{{Value: definition.Ready}})
}

var terminalEscapes = regexp.MustCompile("\x1b(?:\\[[0-?]*[ -/]*[@-~]|\\][^\x07\x1b]*(?:\x07|\x1b\\\\))")

// StripLogANSI removes terminal formatting from matched evidence.
func StripLogANSI(value string) string { return terminalEscapes.ReplaceAllString(value, "") }

// LogEvidence strips terminal controls and bounds a UTF-8 console excerpt.
func LogEvidence(value string) string {
	value = StripLogANSI(strings.ToValidUTF8(value, "\uFFFD"))

	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}

		return r
	}, value)
	if len(value) > 1024 {
		value = value[:1024]
		for !utf8.ValidString(value) {
			value = value[:len(value)-1]
		}
	}

	return value
}

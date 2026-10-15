package model

import (
	"errors"
	"math"
	"math/big"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

var integerValue = regexp.MustCompile(`^[+-]?(0|[1-9][0-9]*)$`)
var numericValue = regexp.MustCompile(`^[+-]?([0-9]+(\.[0-9]*)?|\.[0-9]+)([eE][+-]?[0-9]+)?$`)

func decimalNumber(value string) (*big.Rat, error) {
	if len(value) > 8192 || !numericValue.MatchString(value) {
		return nil, errors.New("value must be a decimal number")
	}

	if _, exponent, ok := strings.Cut(strings.ToLower(value), "e"); ok {
		power, err := strconv.Atoi(exponent)
		if err != nil || power < -10000 || power > 10000 {
			return nil, errors.New("decimal exponent exceeds supported limits")
		}
	}

	number, ok := new(big.Rat).SetString(value)
	if !ok {
		return nil, errors.New("invalid decimal number")
	}

	return number, nil
}

// NumericBound rejects source bounds that would lose precision in the native format.
func NumericBound(value string) (float64, error) {
	exact, err := decimalNumber(value)
	if err != nil {
		return 0, err
	}

	number, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, errors.New("numeric bound must be finite")
	}

	encoded, err := decimalNumber(strconv.FormatFloat(number, 'g', -1, 64))
	if err != nil || exact.Cmp(encoded) != 0 {
		return 0, errors.New("numeric bound cannot be represented without rounding")
	}

	return number, nil
}

// VariableValidation preserves the supported Egg validation rules in native Embers.
type VariableValidation struct {
	Type      string   `json:"type"`
	Nullable  bool     `json:"nullable,omitempty"`
	Min       *float64 `json:"min,omitempty"`
	Max       *float64 `json:"max,omitempty"`
	MinLength *int     `json:"min_length,omitempty"`
	MaxLength *int     `json:"max_length,omitempty"`
	Choices   []string `json:"choices,omitempty"`
}

// Validate checks that constraints are finite, bounded and internally consistent.
func (v VariableValidation) Validate() error {
	if !slices.Contains([]string{"string", "integer", "numeric", "boolean"}, v.Type) {
		return errors.New("unsupported variable type")
	}

	for _, bound := range []*float64{v.Min, v.Max} {
		if bound != nil && (math.IsNaN(*bound) || math.IsInf(*bound, 0)) {
			return errors.New("invalid numeric bound")
		}
	}

	if (v.Min != nil || v.Max != nil) && v.Type != "integer" && v.Type != "numeric" {
		return errors.New("numeric bounds require a numeric variable")
	}

	if v.Min != nil && v.Max != nil && *v.Min > *v.Max {
		return errors.New("minimum exceeds maximum")
	}

	for _, bound := range []*int{v.MinLength, v.MaxLength} {
		if bound != nil && (*bound < 0 || *bound > 8192) {
			return errors.New("invalid variable length")
		}
	}

	if (v.MinLength != nil || v.MaxLength != nil) && v.Type != "string" {
		return errors.New("length bounds require a string variable")
	}

	if v.MinLength != nil && v.MaxLength != nil && *v.MinLength > *v.MaxLength {
		return errors.New("minimum length exceeds maximum")
	}

	if len(v.Choices) > 128 {
		return errors.New("too many variable choices")
	}

	return nil
}

// ValidateValue applies native patterns and imported rules at both panel and Spark boundaries.
func (v Variable) ValidateValue(value string) error {
	if len(value) > 8192 || strings.ContainsRune(value, 0) || !utf8.ValidString(value) || v.Required && (value == "" || v.Validation != nil && strings.Trim(value, " \t\r\n\v") == "") {
		return errors.New("invalid variable value")
	}

	rules := v.Validation
	if rules != nil && rules.Nullable && value == "" && !v.Required {
		return nil
	}

	if v.Pattern != "" {
		pattern, err := regexp.Compile(v.Pattern)
		if err != nil || !pattern.MatchString(value) {
			return errors.New("value does not match its pattern")
		}
	}

	if rules == nil {
		return nil
	}

	if err := rules.Validate(); err != nil {
		return err
	}

	if len(rules.Choices) > 0 && !slices.Contains(rules.Choices, value) {
		return errors.New("value is not an allowed choice")
	}

	length := utf8.RuneCountInString(value)
	if rules.MinLength != nil && length < *rules.MinLength || rules.MaxLength != nil && length > *rules.MaxLength {
		return errors.New("value is outside its length bounds")
	}

	if rules.Type == "boolean" && value != "0" && value != "1" {
		return errors.New("boolean value must be 0 or 1")
	}

	if rules.Type == "integer" || rules.Type == "numeric" {
		var number *big.Rat

		text := strings.Trim(value, " \t\r\n\v\f")

		if rules.Type == "integer" {
			if !integerValue.MatchString(text) {
				return errors.New("value must be an integer")
			}

			integer, err := strconv.ParseInt(text, 10, 64)
			if err != nil {
				return errors.New("value must be an integer")
			}

			number = new(big.Rat).SetInt64(integer)
		} else {
			parsed, err := decimalNumber(text)
			if err != nil {
				return errors.New("value must be numeric")
			}

			number = parsed
		}

		for _, bound := range []struct {
			value   *float64
			minimum bool
		}{{rules.Min, true}, {rules.Max, false}} {
			if bound.value == nil {
				continue
			}

			exact, err := decimalNumber(strconv.FormatFloat(*bound.value, 'g', -1, 64))
			if err != nil {
				return err
			}

			comparison := number.Cmp(exact)
			if bound.minimum && comparison < 0 || !bound.minimum && comparison > 0 {
				return errors.New("value is outside its numeric bounds")
			}
		}
	}

	return nil
}

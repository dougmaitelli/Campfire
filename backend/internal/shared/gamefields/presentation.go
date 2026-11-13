// Package gamefields defines game-state field presentation for all consumers.
// Keep these rules in parity with web/src/utils/gameFieldPresentation.ts.
// Integrations only label custom fields.
package gamefields

import (
	"math"
	"strings"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"golang.org/x/text/number"

	"campfire/internal/shared/model"
)

// Definition describes a built-in metric and its optional combined rendering.
type Definition struct {
	ID               string
	LabelKey         string
	CombineWith      string
	CombinedValueKey string
	Read             func(*model.GameState) *int
}

var builtins = []Definition{
	{
		ID: "players", LabelKey: "gameState.players",
		CombineWith: "capacity", CombinedValueKey: "gameState.playersWithCapacity",
		Read: func(state *model.GameState) *int { return state.Players },
	},
	{
		ID: "capacity", LabelKey: "gameState.capacity",
		Read: func(state *model.GameState) *int { return state.Capacity },
	},
}

// External publications currently use English. A localizer can be supplied to
// Present without changing field definitions or asking integrations for labels.
var englishMessages = map[string]string{
	"gameState.players":             "Players",
	"gameState.capacity":            "Capacity",
	"gameState.playersWithCapacity": "{{players}} / {{capacity}}",
	"gameState.yes":                 "Yes",
	"gameState.no":                  "No",
}

// Builtin is the sole lookup for the built-in/custom distinction.
func Builtin(id string) (Definition, bool) {
	for _, definition := range builtins {
		if definition.ID == id {
			return definition, true
		}
	}

	return Definition{}, false
}

// Translate resolves a catalog message with named interpolation values.
// A nil translator deliberately selects English for external publications.
type Translate func(key string, values map[string]string) string

func english(key string, values map[string]string) string {
	text := englishMessages[key]
	for name, value := range values {
		text = strings.ReplaceAll(text, "{{"+name+"}}", value)
	}

	return text
}

// Field is the presentation shared by card previews and external publications.
type Field struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Value     string `json:"value"`
	Available bool   `json:"available"`
}

// Label resolves built-in translations, integration labels, or an exact raw key.
// Labels and fallback keys are preserved verbatim on every output surface.
func Label(id string, labels model.GameFieldLabels, translate Translate) string {
	if translate == nil {
		translate = english
	}

	if definition, ok := Builtin(id); ok {
		return translate(definition.LabelKey, nil)
	}

	key := strings.TrimPrefix(id, "field.")
	if label, ok := labels[key]; ok {
		return label
	}

	return key
}

// Present formats selected fields. Combination is only permitted when both
// fields were selected and are available: unselected values never enter a card.
// The combined companion is omitted to avoid displaying capacity twice.
func Present(state *model.GameState, ids []string, labels model.GameFieldLabels, translate Translate) []Field {
	if translate == nil {
		translate = english
	}

	values := make(map[string]string, len(ids))
	for _, id := range ids {
		if value, available := valueText(state, id, translate); available {
			values[id] = value
		}
	}

	combined := make(map[string]string)
	omitted := make(map[string]bool)

	for _, definition := range builtins {
		value, available := values[definition.ID]

		companion, companionAvailable := values[definition.CombineWith]
		if definition.CombineWith != "" && available && companionAvailable {
			combined[definition.ID] = translate(definition.CombinedValueKey, map[string]string{definition.ID: value, definition.CombineWith: companion})
			omitted[definition.CombineWith] = true
		}
	}

	fields := make([]Field, 0, len(ids))
	for _, id := range ids {
		if omitted[id] {
			continue
		}

		name := Label(id, labels, translate)

		value, available := values[id]
		if text, ok := combined[id]; ok {
			value = text
		}

		fields = append(fields, Field{ID: id, Name: name, Value: value, Available: available})
	}

	return fields
}

func englishNumber(value any) string {
	return message.NewPrinter(language.English).Sprintf("%v", number.Decimal(value, number.Precision(21), number.MaxFractionDigits(324)))
}

func valueText(state *model.GameState, id string, translate Translate) (string, bool) {
	if state == nil {
		return "", false
	}

	if definition, builtin := Builtin(id); builtin {
		if value := definition.Read(state); value != nil {
			return englishNumber(*value), true
		}

		return "", false
	}

	switch value := state.Fields[strings.TrimPrefix(id, "field.")].(type) {
	case string:
		return value, strings.TrimSpace(value) != ""
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return "", false
		}

		if value == 0 {
			return "0", true
		}

		return englishNumber(value), true
	case int, int64:
		return englishNumber(value), true
	case bool:
		key := "gameState.no"
		if value {
			key = "gameState.yes"
		}

		return translate(key, nil), true
	default:
		return "", false
	}
}

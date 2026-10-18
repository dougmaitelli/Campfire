package service

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"campfire/internal/shared/model"
)

// ImportOptions controls explicit selection and acknowledgement during conversion.
type ImportOptions struct {
	ReplaceID           string `json:"-"`
	StartupCommand      string `json:"startup_command,omitempty"`
	AcknowledgeWarnings bool   `json:"acknowledge_warnings,omitempty"`
	SourceURL           string `json:"source_url,omitempty"`
}

// ImportIssue identifies a field that needs attention before an Egg can be used.
type ImportIssue struct {
	Field         string `json:"field"`
	Message       string `json:"message"`
	Informational bool   `json:"informational,omitempty"`
}

// EmberPreview is a non-persisting conversion result shared by all import sources.
type EmberPreview struct {
	Format                 string        `json:"format"`
	Definition             string        `json:"definition"`
	Ember                  *model.Ember  `json:"ember"`
	Warnings               []ImportIssue `json:"warnings"`
	Errors                 []ImportIssue `json:"errors"`
	StartupCommands        []string      `json:"startup_commands"`
	SelectedStartupCommand string        `json:"selected_startup_command,omitempty"`
}

func (p *EmberPreview) fail(field, message string) {
	p.Errors = append(p.Errors, ImportIssue{Field: field, Message: message})
}
func (p *EmberPreview) warn(field, message string) {
	p.Warnings = append(p.Warnings, ImportIssue{Field: field, Message: message})
}

func (p *EmberPreview) note(field, message string) {
	p.Warnings = append(p.Warnings, ImportIssue{Field: field, Message: message, Informational: true})
}

// PreviewEmber auto-detects native Embers and supported Egg exports without executing scripts.
func PreviewEmber(data []byte, options ImportOptions) EmberPreview {
	report := EmberPreview{Format: "unknown", Definition: string(data), Warnings: []ImportIssue{}, Errors: []ImportIssue{}, StartupCommands: []string{}}

	normalized, err := definitionDocument(data)
	if err != nil {
		report.fail("definition", err.Error())
		return report
	}

	var marker struct {
		Schema *int `json:"schema"`
		Meta   struct {
			Version string `json:"version"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(normalized, &marker); err != nil {
		report.fail("definition", "Invalid format marker.")
		return report
	}

	var ember model.Ember

	if marker.Schema != nil {
		report.Format = "ember"
		if marker.Meta.Version != "" {
			report.fail("meta.version", "Do not mix Ember and Egg schemas.")
			return report
		}

		if err := strictDefinition(normalized, &ember); err != nil {
			report.fail("definition", err.Error())
			return report
		}

		if options.StartupCommand != "" {
			report.fail("startup_command", "Startup selection is only available for Pelican v3 exports.")
		}
	} else {
		report.Format = marker.Meta.Version
		switch report.Format {
		case "PTDL_v1", "PTDL_v2", "PLCN_v1", "PLCN_v2", "PLCN_v3":
			ember = convertEgg(normalized, options, &report)
		default:
			report.fail("meta.version", "Supported Egg versions are PTDL_v1/v2 and PLCN_v1/v2/v3.")
			return report
		}
	}

	ember.ID = ""
	if ember.UpdateURL == "" && ember.Origin != nil {
		ember.UpdateURL = ember.Origin.UpdateURL
	}

	if ember.UpdateURL == "" {
		ember.UpdateURL = options.SourceURL
	}

	if len(report.Errors) == 0 {
		if err := validateEmber(&ember); err != nil {
			report.fail("ember", err.Error())
		}
	}

	if len(report.Errors) == 0 {
		report.Ember = &ember
	}

	return report
}

// ImportDefinition rechecks the original document and stores only a validated native revision.
func (s *EmberService) ImportDefinition(ctx context.Context, data []byte, options ImportOptions) (model.Ember, error) {
	report := PreviewEmber(data, options)
	if len(report.Errors) > 0 {
		return model.Ember{}, fmt.Errorf("%w: %s: %s", ErrInvalidEmber, report.Errors[0].Field, report.Errors[0].Message)
	}

	if slices.ContainsFunc(report.Warnings, func(issue ImportIssue) bool { return !issue.Informational }) && !options.AcknowledgeWarnings {
		return model.Ember{}, fmt.Errorf("%w: review and acknowledge compatibility warnings before importing", ErrInvalidEmber)
	}

	if options.ReplaceID != "" {
		return s.Replace(ctx, options.ReplaceID, *report.Ember)
	}

	return s.Import(ctx, *report.Ember)
}

package statuspage

import (
	"fmt"
	"strings"
	"testing"
)

func TestPublicationValidationBoundsAndNames(t *testing.T) {
	for _, slug := range []string{"abc", "my-community", "123"} {
		if !ValidSlug(slug) {
			t.Fatal(slug)
		}
	}

	for _, slug := range []string{"ab", "UPPER", "-abc", "abc-", "a/b", "a.b", strings.Repeat("a", 65)} {
		if ValidSlug(slug) {
			t.Fatal(slug)
		}
	}

	input := Input{Title: " Community ", Slug: "community", Entries: []Selection{{ServerID: "first", Name: " Public game "}}}
	if err := input.Normalize(); err != nil || input.Title != "Community" || input.Entries[0].Name != "Public game" {
		t.Fatal(input, err)
	}

	for _, name := range []string{"", strings.Repeat("界", 101), "line\nbreak", string([]byte{0xff})} {
		invalid := Input{Title: "Community", Slug: "community", Entries: []Selection{{ServerID: "first", Name: name}}}
		if err := invalid.Normalize(); err == nil {
			t.Fatal("accepted invalid name")
		}
	}

	for i := range MaxEntries + 1 {
		input.Entries = append(input.Entries, Selection{ServerID: fmt.Sprintf("server-%d", i), Name: "Public"})
	}

	if err := input.Normalize(); err == nil {
		t.Fatal("accepted oversized publication")
	}
}

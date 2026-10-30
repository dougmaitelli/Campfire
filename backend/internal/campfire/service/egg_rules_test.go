package service

import (
	"encoding/json"
	"testing"
)

func TestEggRulesRetainConstraints(t *testing.T) {
	for _, test := range []struct {
		rules     any
		good, bad []string
	}{
		{"required|integer|min:1|max:128", []string{"1", "128", " +20 "}, []string{"", "0", "129", "1.2", "NaN", "01", "1_0"}},
		{[]string{"required", "numeric", "between:0.5,2.5"}, []string{"0.5", "2.5", "1e0"}, []string{"0.4", "2.6", "Inf", "0x1p0", "1_0"}},
		{"required|string|between:2,4", []string{"ab", "🎮é"}, []string{"", "a", "abcde", "  "}},
		{"nullable|string|min:2|max:4", []string{"", "ab"}, []string{"a", "abcde"}},
		{"required|string|in:latest,stable", []string{"latest", "stable"}, []string{"", "unknown", "LATEST"}},
		{"required|string|regex:/^(alpha|beta)$/i", []string{"alpha", "BETA"}, []string{"gamma", "alpha-extra"}},
		{"required|string|size:3", []string{"abc"}, []string{"ab", "abcd"}},
		{"required|boolean", []string{"0", "1"}, []string{"true", "false", "2", ""}},
		{"required|url", []string{"https://thunderstore.io/api/experimental/package/denikson/BepInExPack_Valheim/", "http://localhost:8080/package", "ftp://example.com/file", "https://[::1]/file"}, []string{"", "not a url", "//example.com/path", "/relative", "https:///missing-host", "javascript:alert(1)", "https://example.com/a b", "https://example.com\\path", "https://example.com\n"}},
		{[]string{"nullable", "url"}, []string{"", "https://example.com"}, []string{"example.com"}},
		{"url", []string{"", "https://example.com"}, []string{"example.com"}},
		{"required|numeric|min:0.3|max:1", []string{"0.3", "1"}, []string{"0.29999999999999999999", "1.00000000000000000001", "1e1000000000"}},
	} {
		raw, err := json.Marshal(test.rules)
		if err != nil {
			t.Fatal(err)
		}

		variable, err := convertEggRules(raw)
		if err != nil {
			t.Fatal(test.rules, err)
		}

		for _, value := range test.good {
			if err := variable.ValidateValue(value); err != nil {
				t.Errorf("%v rejected %q: %v", test.rules, value, err)
			}
		}

		for _, value := range test.bad {
			if err := variable.ValidateValue(value); err == nil {
				t.Errorf("%v accepted %q", test.rules, value)
			}
		}
	}

	for _, rule := range []string{"url:http,https", "integer|url", "url|boolean", "regex:/^(?=secret).*$/", "regex:/^(x)\\1$/", "regex:/x/g", "regex:/unterminated", "regex:x", "integer|min:5|max:1", "string|min:-1", "required:foo", "required|regex:/a/|regex:/b/", "numeric|min:NaN", "integer|boolean"} {
		raw, _ := json.Marshal(rule)
		if _, err := convertEggRules(raw); err == nil {
			t.Errorf("unsupported rule accepted: %s", rule)
		}
	}
}

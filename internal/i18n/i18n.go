// Package i18n holds the user-facing strings, shared by Go and the UI, in
// messages.json. The language is process wide (settings `language`).
package i18n

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
)

const Default = "fr-fr"

// Languages lists the supported languages, default first.
var Languages = []string{"fr-fr", "en-us"}

//go:embed messages.json
var raw []byte

var catalog = func() map[string]map[string]string {
	messages := map[string]map[string]string{}
	if err := json.Unmarshal(raw, &messages); err != nil {
		panic(err)
	}
	return messages
}()

var current atomic.Value

// Normalize returns a supported language, Default for empty or unknown ones.
func Normalize(language string) string {
	language = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(language, "_", "-")))
	if _, ok := catalog[language]; ok {
		return language
	}
	return Default
}

// Supported reports whether language is in the catalog.
func Supported(language string) bool {
	_, ok := catalog[strings.ToLower(strings.TrimSpace(language))]
	return ok
}

// Set selects the language of T and Errorf.
func Set(language string) { current.Store(Normalize(language)) }

// Current is the selected language.
func Current() string {
	if language, ok := current.Load().(string); ok {
		return language
	}
	return Default
}

// Messages returns the catalog of language.
func Messages(language string) map[string]string { return catalog[Normalize(language)] }

// T formats the message key in the current language; unknown keys fall back
// to the default language, then to the key itself.
func T(key string, args ...any) string {
	message, ok := catalog[Current()][key]
	if !ok {
		if message, ok = catalog[Default][key]; !ok {
			message = key
		}
	}
	if len(args) == 0 {
		return message
	}
	return fmt.Sprintf(message, args...)
}

// Errorf is fmt.Errorf with a translated format, %w included.
func Errorf(key string, args ...any) error {
	message, ok := catalog[Current()][key]
	if !ok {
		message = catalog[Default][key]
	}
	return fmt.Errorf(message, args...)
}

package ui

import (
	"regexp"
	"strings"
	"testing"
)

func TestPage(t *testing.T) {
	page := Page("fr-fr")
	if strings.Contains(page, `href="style.css"`) || strings.Contains(page, `src="app.js"`) || !strings.Contains(page, "window.totpDone") {
		t.Fatal("assets not inlined")
	}
	if !strings.Contains(page, `<html lang="fr">`) || !strings.Contains(page, "PUT EVERYTHING HERE") {
		t.Fatal("french page")
	}
}

func TestEveryPlaceholderTranslated(t *testing.T) {
	for _, language := range []string{"fr-fr", "en-us"} {
		page := Page(language)
		if leftover := regexp.MustCompile(`\{\{[^}]*\}\}`).FindString(page); leftover != "" {
			t.Fatalf("%s: %s left", language, leftover)
		}
	}
	if page := Page("en-us"); !strings.Contains(page, `<html lang="en">`) || !strings.Contains(page, "Search (/)") || !strings.Contains(page, `"language":"en-us"`) {
		t.Fatal("english page")
	}
}

func TestEveryScriptKeyExists(t *testing.T) {
	page := Page("en-us")
	script, _ := assets.ReadFile("app.js")
	for _, match := range regexp.MustCompile(`\bt\('([^']+)'`).FindAllStringSubmatch(string(script), -1) {
		if !strings.Contains(page, `"`+match[1]+`":`) {
			t.Errorf("app.js uses unknown message %s", match[1])
		}
	}
}

// Package ui holds the window document: index.html with style.css and app.js.
// {{key}} placeholders of index.html are replaced by the i18n messages, which
// app.js also receives as window.I18N.
package ui

import (
	"embed"
	"encoding/json"
	"html"
	"regexp"
	"strings"

	"github.com/131/ssh-agent-totp/internal/i18n"
)

var placeholder = regexp.MustCompile(`\{\{([a-z0-9.]+)\}\}`)

//go:embed index.html style.css app.js
var assets embed.FS

// Page returns the UI as a single document in language, with style and
// script inlined.
func Page(language string) string {
	read := func(name string) string {
		body, err := assets.ReadFile(name)
		if err != nil {
			panic(err)
		}
		return string(body)
	}
	messages := i18n.Messages(language)
	page := placeholder.ReplaceAllStringFunc(read("index.html"), func(match string) string {
		return html.EscapeString(messages[match[2:len(match)-2]])
	})
	state, _ := json.Marshal(map[string]any{"language": i18n.Normalize(language), "languages": i18n.Languages, "messages": messages})
	page = strings.Replace(page, `<link rel="stylesheet" href="style.css">`, "<style>"+read("style.css")+"</style>", 1)
	page = strings.Replace(page, `<script src="app.js"></script>`, "<script>window.I18N="+string(state)+";</script><script>"+read("app.js")+"</script>", 1)
	return page
}

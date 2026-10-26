package i18n

import (
	"errors"
	"testing"
)

func TestCatalogsMatch(t *testing.T) {
	for _, language := range Languages {
		for key := range catalog[Default] {
			if catalog[language][key] == "" {
				t.Errorf("%s: %s missing", language, key)
			}
		}
		if len(catalog[language]) != len(catalog[Default]) {
			t.Errorf("%s: %d messages, %d in %s", language, len(catalog[language]), len(catalog[Default]), Default)
		}
	}
}

func TestLanguage(t *testing.T) {
	defer Set(Default)
	if Normalize("EN_us") != "en-us" || Normalize("de-de") != Default || Normalize("") != Default {
		t.Fatal("normalize")
	}
	Set("en-us")
	if T("tray.quit") != "Quit" || T("err.totp.digits", 9) != "9 digits: 6 to 8 expected" || T("nope") != "nope" {
		t.Fatal(T("tray.quit"))
	}
	cause := errors.New("boom")
	if err := Errorf("err.manifest", cause); !errors.Is(err, cause) || err.Error() != "unreadable manifest.json: boom" {
		t.Fatal(err)
	}
}

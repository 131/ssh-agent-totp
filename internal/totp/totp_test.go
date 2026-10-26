package totp

import (
	"encoding/base32"
	"encoding/base64"
	"net/url"
	"strings"
	"testing"
	"time"
)

func secret(ascii string) string {
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte(ascii))
}

// RFC 6238 appendix B.
func TestRFC6238(t *testing.T) {
	seeds := map[string]string{
		"SHA1":   "12345678901234567890",
		"SHA256": "12345678901234567890123456789012",
		"SHA512": "1234567890123456789012345678901234567890123456789012345678901234",
	}
	for _, test := range []struct {
		time      int64
		algorithm string
		code      string
	}{
		{59, "SHA1", "94287082"}, {59, "SHA256", "46119246"}, {59, "SHA512", "90693936"},
		{1111111109, "SHA1", "07081804"}, {1111111111, "SHA256", "67062674"}, {1234567890, "SHA512", "93441116"},
		{2000000000, "SHA1", "69279037"}, {20000000000, "SHA512", "47863826"},
	} {
		token := Token{Secret: secret(seeds[test.algorithm]), Algorithm: test.algorithm, Digits: 8}
		code, expires, err := token.Code(time.Unix(test.time, 0))
		if err != nil || code != test.code {
			t.Errorf("%d %s: %s %v", test.time, test.algorithm, code, err)
		}
		if expires.Unix()%30 != 0 || expires.Unix() <= test.time || expires.Unix()-test.time > 30 {
			t.Errorf("expires %v", expires)
		}
	}
}

func TestNormalize(t *testing.T) {
	token := Token{Secret: "jbsw y3dp-ehpk 3pxp====", Issuer: " GitHub "}
	if err := token.Normalize(); err != nil || token.Secret != "JBSWY3DPEHPK3PXP" || token.Algorithm != "SHA1" || token.Digits != 6 || token.Period != 30 || token.Issuer != "GitHub" {
		t.Fatalf("%+v %v", token, err)
	}
	for _, bad := range []Token{{Secret: "not base32!"}, {Secret: "JBSWY3DPEHPK3PXP", Algorithm: "MD5"}, {Secret: "JBSWY3DPEHPK3PXP", Digits: 9}, {Secret: "JBSWY3DPEHPK3PXP", Period: -1}} {
		if bad.Normalize() == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
}

func TestParse(t *testing.T) {
	text := strings.Join([]string{
		"# comment",
		"otpauth://totp/GitHub:fake-user?secret=JBSWY3DPEHPK3PXP&issuer=GitHub",
		`see "otpauth://totp/Azure%2Ffleurent(perso)?secret=GEZDGNBVGY3TQOJQ&digits=8&period=60&algorithm=SHA256" here`,
		"Azure/fleurent(admin) JBSW Y3DP EHPK 3PXA",
		"github 131: GEZDGNBVGY3TQOJQGEZDGNBV",
		"otpauth://hotp/Old?secret=JBSWY3DPEHPK3PXP&counter=1",
		"AWS ROOT: JBSW Y3DP EHPK 3PXP",
		"Perso/mail jbswy3dpehpk3pxp",
		"Azure:fleurent JBSW Y3DP EHPK 3PXP",
		"GitHub glued:JBSWY3DPEHPK3PXP",
		"just some words",
		"",
	}, "\r\n")
	tokens, skipped := Parse(text)
	if len(tokens) != 8 || len(skipped) != 2 {
		t.Fatalf("%+v %+v", tokens, skipped)
	}
	if tokens[0].Issuer != "GitHub" || tokens[0].Account != "fake-user" || tokens[0].Secret != "JBSWY3DPEHPK3PXP" {
		t.Errorf("%+v", tokens[0])
	}
	if tokens[1].Issuer != "" || tokens[1].Account != "Azure/fleurent(perso)" || tokens[1].Digits != 8 || tokens[1].Period != 60 || tokens[1].Algorithm != "SHA256" {
		t.Errorf("%+v", tokens[1])
	}
	if tokens[2].Issuer != "Azure/fleurent(admin)" || tokens[2].Secret != "JBSWY3DPEHPK3PXA" {
		t.Errorf("%+v", tokens[2])
	}
	if tokens[3].Issuer != "github 131" || tokens[3].Account != "" {
		t.Errorf("%+v", tokens[3])
	}
	if tokens[4].Issuer != "AWS ROOT" || tokens[4].Secret != "JBSWY3DPEHPK3PXP" || tokens[5].Issuer != "Perso/mail" {
		t.Errorf("%+v %+v", tokens[4], tokens[5])
	}
	if tokens[6].Issuer != "Azure" || tokens[6].Account != "fleurent" || tokens[7].Issuer != "GitHub glued" || tokens[7].Secret != "JBSWY3DPEHPK3PXP" {
		t.Errorf("%+v %+v", tokens[6], tokens[7])
	}
	if skipped[0].Line != "otpauth://hotp/Old?secret=JBSWY3DPEHPK3PXP&counter=1" || skipped[1].Line != "just some words" {
		t.Errorf("%+v", skipped)
	}
}

func TestURIRoundTrip(t *testing.T) {
	for _, token := range []Token{
		{Issuer: "Azure/fleurent", Account: "perso", Secret: "JBSWY3DPEHPK3PXP"},
		{Account: "a b&c", Secret: "GEZDGNBVGY3TQOJQ", Algorithm: "SHA512", Digits: 8, Period: 15},
	} {
		token.Normalize()
		found, err := ParseURI(token.URI())
		if err != nil || len(found) != 1 || found[0] != token {
			t.Errorf("%s: %+v %v", token.URI(), found, err)
		}
	}
}

// protobuf helpers for a fake Google Authenticator export.
func field(number int, value []byte) []byte {
	out := []byte{byte(number<<3 | 2)}
	for size := len(value); ; size >>= 7 {
		if size < 0x80 {
			out = append(out, byte(size))
			break
		}
		out = append(out, byte(size)|0x80)
	}
	return append(out, value...)
}

func number(field int, value byte) []byte { return []byte{byte(field << 3), value} }

func entry(secret, name, issuer string, digits, kind byte) []byte {
	var message []byte
	message = append(message, field(1, []byte(secret))...)
	message = append(message, field(2, []byte(name))...)
	message = append(message, field(3, []byte(issuer))...)
	message = append(message, number(4, 1)...)
	message = append(message, number(5, digits)...)
	return append(message, number(6, kind)...)
}

func TestMigration(t *testing.T) {
	var payload []byte
	payload = append(payload, field(1, entry("12345678901234567890", "GitHub:fake-user", "GitHub", 1, 2))...)
	payload = append(payload, field(1, entry("abcdefghij", "fake@example.com", "Azure/fleurent", 2, 2))...)
	payload = append(payload, field(1, entry("counter-based", "old", "Legacy", 1, 1))...)
	payload = append(payload, number(2, 1)...) // version
	link := "otpauth-migration://offline?data=" + url.QueryEscape(base64.StdEncoding.EncodeToString(payload))
	tokens, skipped := Parse(link)
	if len(tokens) != 2 || len(skipped) != 1 || !strings.Contains(skipped[0].Reason, "Legacy") {
		t.Fatalf("%+v %+v", tokens, skipped)
	}
	if tokens[0].Issuer != "GitHub" || tokens[0].Account != "fake-user" || tokens[0].Secret != secret("12345678901234567890") || tokens[0].Digits != 6 {
		t.Errorf("%+v", tokens[0])
	}
	if tokens[1].Issuer != "Azure/fleurent" || tokens[1].Digits != 8 {
		t.Errorf("%+v", tokens[1])
	}
	if _, skipped := Parse("otpauth-migration://offline?data=%%%"); len(skipped) != 1 {
		t.Error("broken export accepted")
	}
}

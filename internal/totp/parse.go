package totp

import (
	"encoding/base32"
	"encoding/base64"
	"errors"
	"github.com/131/ssh-agent-totp/internal/i18n"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Skipped is an input line the import could not use, with the reason.
type Skipped struct {
	Line   string `json:"line"`
	Reason string `json:"reason"`
}

var uriPattern = regexp.MustCompile(`otpauth(?:-migration)?://[^\s"'<>]+`)

// Parse reads every token of text: otpauth:// URIs, Google Authenticator
// otpauth-migration:// exports and bare base32 secrets, one per line,
// optionally preceded by a label ("GitHub JBSW Y3DP EHPK 3PXP" is fine too
// when the last words form the secret). Anything else is reported.
func Parse(text string) ([]Token, []Skipped) {
	var tokens []Token
	var skipped []Skipped
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		uris := uriPattern.FindAllString(line, -1)
		if len(uris) == 0 {
			token, err := parseBare(line)
			if err != nil {
				skipped = append(skipped, Skipped{Line: line, Reason: err.Error()})
				continue
			}

			tokens = append(tokens, token)
			continue
		}

		for _, uri := range uris {
			found, err := ParseURI(uri)
			if err != nil {
				skipped = append(skipped, Skipped{Line: uri, Reason: err.Error()})
			}
			tokens = append(tokens, found...)
		}
	}

	return tokens, skipped
}

// ParseURI reads an otpauth:// or otpauth-migration:// URI.
func ParseURI(uri string) ([]Token, error) {
	link, err := url.Parse(uri)
	if err != nil {
		return nil, i18n.Errorf("err.totp.uri")
	}

	switch link.Scheme {

	case "otpauth":
		token, err := parseOTPAuth(link)
		if err != nil {
			return nil, err
		}

		return []Token{token}, nil
	case "otpauth-migration":
		return parseMigration(link)
	}
	return nil, i18n.Errorf("err.totp.uri")
}

func parseOTPAuth(link *url.URL) (Token, error) {
	if strings.ToLower(link.Host) == "hotp" {
		return Token{}, i18n.Errorf("err.totp.hotp")
	}

	if strings.ToLower(link.Host) != "totp" {
		return Token{}, i18n.Errorf("err.totp.uri")
	}

	query := link.Query()
	label := strings.TrimPrefix(link.Path, "/")
	token := Token{
		Pinned:    query.Get("pinned") == "true",
		Secret:    query.Get("secret"),
		Issuer:    query.Get("issuer"),
		Algorithm: query.Get("algorithm"),
		Account:   label,
	}
	// The label is "issuer:account"; the issuer parameter wins, and may
	// itself hold a colon.
	if rest, found := strings.CutPrefix(label, token.Issuer+":"); token.Issuer != "" && found {
		token.Account = strings.TrimSpace(rest)
	} else if issuer, account, found := strings.Cut(label, ":"); found {
		token.Account = strings.TrimSpace(account)
		if token.Issuer == "" {
			token.Issuer = issuer
		}
	}
	for name, target := range map[string]*int{"digits": &token.Digits, "period": &token.Period} {
		if value := query.Get(name); value != "" {
			number, err := strconv.Atoi(value)
			if err != nil {
				return Token{}, i18n.Errorf("err.totp.number", name)
			}

			*target = number
		}
	}

	return token, token.Normalize()
}

// parseBare reads "[label] SECRET". The secret is the last word (16
// characters or more) or the trailing groups of 4 characters ("JBSW Y3DP
// EHPK 3PXP"), at least 16 characters in all. A colon right before the secret
// ends the label explicitly ("AWS ROOT: JBSW…"); "issuer:account SECRET" sets
// both.
func parseBare(line string) (Token, error) {
	words := strings.Fields(line)
	start := secretStart(words)
	if start < 0 {
		return Token{}, i18n.Errorf("err.totp.unknown")
	}

	label := strings.Join(words[:start], " ")
	if before, after, found := strings.Cut(words[start], ":"); found && after != "" && secretStart([]string{after}) == 0 {
		// "label:SECRET" glued together.
		label, words[start] = strings.TrimSpace(label+" "+before), after
	}
	token := Token{Secret: strings.Join(words[start:], ""), Issuer: strings.TrimSpace(strings.TrimSuffix(label, ":"))}
	if issuer, account, found := strings.Cut(token.Issuer, ":"); found {
		token.Issuer, token.Account = strings.TrimSpace(issuer), strings.TrimSpace(account)
	}
	return token, token.Normalize()
}

// secretStart is the index of the first word of the secret, -1 for none.
func secretStart(words []string) int {
	start := len(words) - 1
	if start < 0 {
		return -1
	}

	last := words[start]
	if index := strings.LastIndex(last, ":"); index >= 0 {
		last = last[index+1:]
	}
	if len(last) <= 4 {
		for start > 0 && base32Word.MatchString(words[start-1]) {
			start--
		}
	}
	secret := strings.Join(append([]string{last}, words[start+1:]...), "")
	if start < len(words)-1 {
		secret = strings.Join(words[start:], "")
	}
	if len(NormalizeSecret(secret)) < 16 || !base32Text.MatchString(secret) {
		return -1
	}

	if _, err := Key(secret); err != nil {
		return -1
	}

	return start
}

var base32Text = regexp.MustCompile(`^[A-Za-z2-7]+=*$`)

var base32Word = regexp.MustCompile(`^[A-Za-z2-7]{4}$`)

// Google Authenticator export: otpauth-migration://offline?data=<base64
// protobuf MigrationPayload>.
func parseMigration(link *url.URL) ([]Token, error) {
	data := strings.NewReplacer("-", "+", "_", "/").Replace(link.Query().Get("data"))
	payload, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		payload, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(data, "="))
	}
	if err != nil || len(payload) == 0 {
		return nil, i18n.Errorf("err.totp.migration")
	}

	var tokens []Token
	var problems []string
	err = protoFields(payload, func(field int, value []byte, _ uint64) error {
		if field != 1 || value == nil {
			return nil
		}

		token, hotp, err := migrationEntry(value)
		switch {
		case err != nil:
			return err
		case hotp:
			problems = append(problems, i18n.T("err.totp.hotp.entry", token.Label()))
		case token.Normalize() != nil:
			problems = append(problems, i18n.T("err.totp.entry", token.Label()))
		default:
			tokens = append(tokens, token)
		}
		return nil
	})
	if err != nil {
		return nil, i18n.Errorf("err.totp.migration")
	}

	if len(problems) > 0 {
		return tokens, errors.New(strings.Join(problems, "; "))
	}

	return tokens, nil
}

func migrationEntry(message []byte) (token Token, hotp bool, err error) {
	err = protoFields(message, func(field int, value []byte, number uint64) error {
		switch field {
		case 1:
			token.Secret = base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(value)
		case 2:
			token.Account = string(value)
		case 3:
			token.Issuer = string(value)
		case 4:
			token.Algorithm = map[uint64]string{2: "SHA256", 3: "SHA512", 4: "MD5"}[number]
		case 5:
			if number == 2 {
				token.Digits = 8
			}
		case 6:
			hotp = number == 1
		}
		return nil
	})
	if issuer, account, found := strings.Cut(token.Account, ":"); found && (token.Issuer == "" || token.Issuer == issuer) {
		token.Issuer, token.Account = issuer, strings.TrimSpace(account)
	}
	return token, hotp, err
}

// protoFields walks a protobuf message: varints come as number, length
// delimited fields as value.
func protoFields(message []byte, visit func(field int, value []byte, number uint64) error) error {
	for len(message) > 0 {
		tag, n := varint(message)
		if n == 0 {
			return errors.New("protobuf")
		}

		message = message[n:]
		field := int(tag >> 3)
		switch tag & 7 {

		case 0:
			number, n := varint(message)
			if n == 0 {
				return errors.New("protobuf")
			}

			message = message[n:]
			if err := visit(field, nil, number); err != nil {
				return err
			}

		case 2:
			size, n := varint(message)
			if n == 0 || uint64(len(message)-n) < size {
				return errors.New("protobuf")
			}

			value := message[n : n+int(size)]
			message = message[n+int(size):]
			if err := visit(field, value, 0); err != nil {
				return err
			}

		case 1, 5:
			width := map[uint64]int{1: 8, 5: 4}[tag&7]
			if len(message) < width {
				return errors.New("protobuf")
			}

			message = message[width:]
		default:
			return errors.New("protobuf")
		}
	}

	return nil
}

func varint(buffer []byte) (uint64, int) {
	var value uint64
	for i := 0; i < len(buffer) && i < 10; i++ {
		value |= uint64(buffer[i]&0x7f) << (7 * i)
		if buffer[i] < 0x80 {
			return value, i + 1
		}
	}

	return 0, 0
}

// URI is the otpauth:// form of t: a line of tokens.crypt and of the export.
func (t Token) URI() string {
	// No issuer: a leading colon keeps a colon of the account in it.
	label := t.Account
	if t.Issuer != "" || strings.Contains(t.Account, ":") {
		label = t.Issuer + ":" + t.Account
	}
	query := url.Values{"secret": {t.Secret}}
	if t.Pinned {
		query.Set("pinned", "true")
	}
	if t.Issuer != "" {
		query.Set("issuer", t.Issuer)
	}
	if t.Algorithm != "" && t.Algorithm != DefaultAlgorithm {
		query.Set("algorithm", t.Algorithm)
	}
	if t.Digits != 0 && t.Digits != DefaultDigits {
		query.Set("digits", strconv.Itoa(t.Digits))
	}
	if t.Period != 0 && t.Period != DefaultPeriod {
		query.Set("period", strconv.Itoa(t.Period))
	}
	return (&url.URL{Scheme: "otpauth", Host: "totp", Path: "/" + label, RawQuery: query.Encode()}).String()
}

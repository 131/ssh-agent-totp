// Package totp computes RFC 6238 codes and reads the token formats accepted
// by the import box (specs/TOKENS.md#import).
package totp

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"github.com/131/ssh-agent-totp/internal/i18n"
	"hash"
	"strings"
	"time"
)

const (
	DefaultAlgorithm = "SHA1"
	DefaultDigits    = 6
	DefaultPeriod    = 30
)

// Token is one TOTP account. Secret is the normalized base32 secret
// (upper case, no padding nor spaces).
type Token struct {
	Pinned    bool
	Issuer    string
	Account   string
	Secret    string
	Algorithm string
	Digits    int
	Period    int
}

// NormalizeSecret upper-cases a base32 secret and drops spaces, dashes and
// padding.
func NormalizeSecret(secret string) string {
	return strings.TrimRight(strings.Map(func(r rune) rune {
		switch {
		case r == ' ' || r == '-' || r == '\t':
			return -1
		case r >= 'a' && r <= 'z':
			return r - 'a' + 'A'
		}
		return r
	}, secret), "=")
}

// Key decodes a normalized base32 secret.
func Key(secret string) ([]byte, error) {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(NormalizeSecret(secret))
	if err != nil || len(key) == 0 {
		return nil, i18n.Errorf("err.totp.secret")
	}
	return key, nil
}

// Normalize fills defaults and checks the token.
func (t *Token) Normalize() error {
	t.Issuer, t.Account = strings.TrimSpace(t.Issuer), strings.TrimSpace(t.Account)
	t.Secret = NormalizeSecret(t.Secret)
	t.Algorithm = strings.ToUpper(strings.TrimSpace(t.Algorithm))
	if t.Algorithm == "" {
		t.Algorithm = DefaultAlgorithm
	}
	if t.Digits == 0 {
		t.Digits = DefaultDigits
	}
	if t.Period == 0 {
		t.Period = DefaultPeriod
	}
	if _, err := Key(t.Secret); err != nil {
		return err
	}
	if newHash(t.Algorithm) == nil {
		return i18n.Errorf("err.totp.algorithm", t.Algorithm)
	}
	if t.Digits < 6 || t.Digits > 8 {
		return i18n.Errorf("err.totp.digits", t.Digits)
	}
	if t.Period < 1 || t.Period > 3600 {
		return i18n.Errorf("err.totp.period", t.Period)
	}
	return nil
}

func newHash(algorithm string) func() hash.Hash {
	switch algorithm {
	case "SHA1":
		return sha1.New
	case "SHA256":
		return sha256.New
	case "SHA512":
		return sha512.New
	}
	return nil
}

// Code returns the code of t at now and the end of its validity.
func (t Token) Code(now time.Time) (string, time.Time, error) {
	if err := t.Normalize(); err != nil {
		return "", time.Time{}, err
	}
	key, _ := Key(t.Secret)
	period := int64(t.Period)
	counter := now.Unix() / period
	mac := hmac.New(newHash(t.Algorithm), key)
	binary.Write(mac, binary.BigEndian, uint64(counter))
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	modulo := uint32(1)
	for range t.Digits {
		modulo *= 10
	}
	return fmt.Sprintf("%0*d", t.Digits, value%modulo), time.Unix((counter+1)*period, 0), nil
}

// Label is "Issuer (account)", or whichever is set.
func (t Token) Label() string {
	switch {
	case t.Issuer != "" && t.Account != "":
		return t.Issuer + " (" + t.Account + ")"
	case t.Issuer != "":
		return t.Issuer
	}
	return t.Account
}

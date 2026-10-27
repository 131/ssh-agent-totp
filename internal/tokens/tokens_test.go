package tokens

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/131/ssh-agent-totp/internal/totp"
)

// fake seals with base64 and the key name: ssh-agent-crypt:v3:<key>.<base64>.
func fake(fail *bool) Crypt {
	return Crypt{
		Encrypt: func(plain []byte, key string) (string, error) {
			if *fail {
				return "", errors.New("agent down")
			}
			return sealed + "v3:" + key + "." + base64.StdEncoding.EncodeToString(plain) + "\n", nil
		},
		Decrypt: func(armored []byte) ([]byte, error) {
			if *fail {
				return nil, errors.New("agent down")
			}
			_, payload, _ := strings.Cut(strings.TrimSpace(string(armored)), ".")
			return base64.StdEncoding.DecodeString(payload)
		},
	}
}

func store(t *testing.T, body string) (*Store, *bool) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tokens.crypt")
	if body != "" {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	fail := false
	return &Store{Path: path, Key: "k1", Crypt: fake(&fail), Now: func() time.Time { return time.Unix(59, 0) }}, &fail
}

// plain unseals the file written by the fake crypt.
func plain(t *testing.T, s *Store) string {
	t.Helper()
	body, _ := os.ReadFile(s.Path)
	if !strings.HasPrefix(string(body), sealed) {
		t.Fatalf("not sealed: %s", body)
	}
	var fail bool
	clear, err := fake(&fail).Decrypt(body)
	if err != nil {
		t.Fatal(err)
	}
	return string(clear)
}

func TestStrictFile(t *testing.T) {
	good := "otpauth://totp/a?secret=JBSWY3DPEHPK3PXP\n"
	for name, body := range map[string]string{
		"bare secret": good + "a JBSWY3DPEHPK3PXP\n",
		"yaml":        good + "version: 1\n",
		"hotp":        good + "otpauth://hotp/a?secret=JBSWY3DPEHPK3PXP&counter=1\n",
		"digits":      good + "otpauth://totp/a?secret=JBSWY3DPEHPK3PXP&digits=12\n",
		"no secret":   good + "otpauth://totp/a\n",
		"migration":   good + "otpauth-migration://offline?data=CgA%3D\n",
	} {
		for _, seal := range []bool{false, true} {
			file := body
			if seal {
				var fail bool
				file, _ = fake(&fail).Encrypt([]byte(body), "k1")
			}
			s, _ := store(t, file)
			err := s.Load()
			if err == nil {
				t.Errorf("%s (sealed %v) accepted", name, seal)
			} else if !strings.Contains(err.Error(), "2") || strings.Contains(err.Error(), "JBSW") {
				t.Errorf("%s: %v (line number expected, no secret)", name, err)
			}
			if after, _ := os.ReadFile(s.Path); string(after) != file {
				t.Errorf("%s: file rewritten", name)
			}
		}
	}
}

func TestURIRoundTrip(t *testing.T) {
	tokens := []totp.Token{
		{Issuer: "Azure/fleurent(perso)", Account: "a b@c.d", Secret: "JBSWY3DPEHPK3PXP", Algorithm: "SHA1", Digits: 6, Period: 30},
		{Issuer: "odd: issuer?#&", Account: "x:y", Secret: "GEZDGNBVGY3TQOJQ", Algorithm: "SHA512", Digits: 8, Period: 60},
		{Account: "no:issuer", Secret: "JBSWY3DPEHPK3PXA", Algorithm: "SHA256", Digits: 7, Period: 45},
		{Issuer: "only issuer", Secret: "JBSWY3DPEHPK3PXB", Algorithm: "SHA1", Digits: 6, Period: 30},
	}
	got, err := Parse(Marshal(tokens))
	if err != nil || len(got) != len(tokens) {
		t.Fatal(got, err)
	}
	for index := range tokens {
		if got[index] != tokens[index] {
			t.Errorf("%+v became %+v", tokens[index], got[index])
		}
	}
}

func TestMissingFileIsEmpty(t *testing.T) {
	s, _ := store(t, "")
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
	if codes, err := s.Codes(); err != nil || len(codes) != 0 {
		t.Fatal(codes, err)
	}
	if _, err := os.Stat(s.Path); err == nil {
		t.Fatal("empty file written")
	}
}

func TestClearFileIsSealed(t *testing.T) {
	s, _ := store(t, "# by hand\n\notpauth://totp/Azure%2Ffleurent(perso):?secret=gezdgnbvgy3tqojqgezdgnbvgy3tqojq&digits=8\n")
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(s.Path)
	if strings.Contains(string(body), "gezd") || !strings.HasPrefix(string(body), "ssh-agent-crypt:v3:k1.") {
		t.Fatal(string(body))
	}
	if _, err := os.Stat(s.Path + ".bak"); err == nil {
		t.Fatal("clear file kept as .bak")
	}
	clear := plain(t, s)
	if !strings.HasPrefix(clear, header) || !strings.Contains(clear, "otpauth://totp/Azure/fleurent%28perso%29:?digits=8&issuer=Azure%2Ffleurent%28perso%29&secret=GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ\n") {
		t.Fatal(clear)
	}
	codes, err := s.Codes()
	if err != nil || len(codes) != 1 || codes[0].Code != "94287082" || codes[0].Issuer != "Azure/fleurent(perso)" || codes[0].Expires != 60000 {
		t.Fatal(codes, err)
	}
}

func TestAgentDownLocksEverything(t *testing.T) {
	s, fail := store(t, "")
	if _, err := s.Import("A: JBSWY3DPEHPK3PXP\nB: GEZDGNBVGY3TQOJQ"); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(s.Path)
	*fail = true
	if err := s.Load(); err == nil {
		t.Fatal("unsealed without agent")
	}
	if _, err := s.Import("C: JBSWY3DPEHPK3PXA"); err == nil {
		t.Fatal("sealed without agent")
	}
	if after, _ := os.ReadFile(s.Path); string(after) != string(before) {
		t.Fatal("file changed")
	}
}

func TestImportEditExport(t *testing.T) {
	s, _ := store(t, "")
	result, err := s.Import("otpauth://totp/GitHub:fake?secret=JBSWY3DPEHPK3PXP&issuer=GitHub\nAzure/fleurent(admin): GEZDGNBVGY3TQOJQ\nnope\notpauth://totp/GitHub:fake?secret=JBSWY3DPEHPK3PXP&issuer=GitHub")
	if err != nil || result.Added != 2 || result.Duplicates != 1 || len(result.Skipped) != 1 {
		t.Fatalf("%+v %v", result, err)
	}
	again, _ := s.Import("otpauth://totp/github:FAKE?secret=jbswy3dpehpk3pxp")
	if again.Added != 0 || again.Duplicates != 1 {
		t.Fatalf("%+v", again)
	}
	codes, _ := s.Codes()
	if codes[0].Issuer != "Azure/fleurent(admin)" || codes[1].Issuer != "GitHub" {
		t.Fatal("not sorted", codes)
	}
	if err := s.Rename(codes[1].ID, "Work/GitHub", "fake"); err != nil {
		t.Fatal(err)
	}
	if err := s.Rename(codes[1].ID, " ", ""); err == nil {
		t.Fatal("empty label")
	}
	codes, _ = s.Codes()
	if codes[0].Issuer != "Azure/fleurent(admin)" || codes[1].Issuer != "Work/GitHub" {
		t.Fatal(codes)
	}
	if _, err := os.Stat(s.Path + ".bak"); err != nil {
		t.Fatal("no .bak")
	}
	export, _ := s.Export()
	if strings.Count(export, "otpauth://totp/") != 2 || strings.Contains(export, "ssh-agent-crypt") {
		t.Fatal(export)
	}
	if again, _ := s.Import(export); again.Added != 0 || again.Duplicates != 2 {
		t.Fatalf("export does not import back: %+v\n%s", again, export)
	}
	if err := s.Delete(codes[0].ID); err != nil {
		t.Fatal(err)
	}
	if codes, _ = s.Codes(); len(codes) != 1 {
		t.Fatal(codes)
	}
	if err := s.Delete(999); err == nil {
		t.Fatal("unknown id")
	}
}

func TestAlphabeticalOrder(t *testing.T) {
	s, _ := store(t, "otpauth://totp/github 131:?secret=JBSWY3DPEHPK3PXP\notpauth://totp/x?issuer=Azure/fleurent(perso)&secret=JBSWY3DPEHPK3PXA\notpauth://totp/azure:b?secret=JBSWY3DPEHPK3PXB\notpauth://totp/Azure/fleurent(admin):?secret=JBSWY3DPEHPK3PXC\notpauth://totp/azure:A?secret=JBSWY3DPEHPK3PXD\n")
	codes, err := s.Codes()
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, code := range codes {
		got = append(got, code.Issuer+"|"+code.Account)
	}
	if strings.Join(got, ",") != "azure|A,azure|b,Azure/fleurent(admin)|,Azure/fleurent(perso)|x,github 131|" {
		t.Fatal(got)
	}
	if clear := plain(t, s); strings.Index(clear, "admin") > strings.Index(clear, "perso") {
		t.Fatal("file not sorted", clear)
	}
}

func TestReloadOnChangeAndReseal(t *testing.T) {
	s, _ := store(t, "")
	s.Import("A: JBSWY3DPEHPK3PXP")
	// Edited by hand: ssh-agent-crypt -decrypt, edit, ssh-agent-crypt.
	var fail bool
	edited, _ := fake(&fail).Encrypt([]byte("otpauth://totp/A:?secret=JBSWY3DPEHPK3PXP\notpauth://totp/B:added%20by%20hand?secret=GEZDGNBVGY3TQOJQ\n"), "k1")
	os.WriteFile(s.Path, []byte(edited), 0o600)
	codes, err := s.Codes()
	if err != nil || len(codes) != 2 || codes[1].Account != "added by hand" {
		t.Fatal(codes, err)
	}
	count, err := s.Reseal("k2")
	if err != nil || count != 2 || s.Key != "k2" {
		t.Fatal(count, err)
	}
	if body, _ := os.ReadFile(s.Path); !strings.HasPrefix(string(body), "ssh-agent-crypt:v3:k2.") {
		t.Fatal(string(body))
	}
}

func TestAddOne(t *testing.T) {
	s, _ := store(t, "")
	if err := s.Add(totp.Token{Issuer: " Azure/fake ", Account: "admin", Secret: "jbsw y3dp ehpk 3pxp", Digits: 8, Period: 60, Algorithm: "sha256"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(totp.Token{Issuer: "azure/FAKE", Account: "ADMIN", Secret: "JBSWY3DPEHPK3PXP"}); err == nil {
		t.Fatal("duplicate accepted")
	}
	for _, bad := range []totp.Token{{Secret: "JBSWY3DPEHPK3PXA"}, {Issuer: "x", Secret: "!!"}, {Issuer: "x", Secret: "JBSWY3DPEHPK3PXA", Digits: 9}} {
		if err := s.Add(bad); err == nil {
			t.Fatal("accepted", bad)
		}
	}
	if clear := plain(t, s); !strings.Contains(clear, "otpauth://totp/Azure/fake:admin?algorithm=SHA256&digits=8&issuer=Azure%2Ffake&period=60&secret=JBSWY3DPEHPK3PXP\n") {
		t.Fatal(clear)
	}
}

func TestRenameFolder(t *testing.T) {
	s, _ := store(t, "otpauth://totp/Azure/fleurent(perso):?secret=JBSWY3DPEHPK3PXA\notpauth://totp/x?issuer=Azure%20/%20sub/x&secret=JBSWY3DPEHPK3PXB\notpauth://totp/Azure:?secret=JBSWY3DPEHPK3PXC\notpauth://totp/Azurex/y:?secret=JBSWY3DPEHPK3PXD\n")
	// "Azure" itself (a token named like the folder) follows the folder.
	if renamed, err := s.RenameFolder("Azure", " Cloud/MS "); err != nil || renamed != 3 {
		t.Fatal(renamed, err)
	}
	if _, err := s.RenameFolder("Azure", "x"); err == nil {
		t.Fatal("empty folder renamed")
	}
	if renamed, err := s.RenameFolder("Cloud/MS/sub", "Cloud/MS"); err != nil || renamed != 1 {
		t.Fatal(renamed, err)
	}
	codes, _ := s.Codes()
	got := []string{}
	for _, code := range codes {
		got = append(got, code.Issuer)
	}
	if strings.Join(got, ",") != "Azurex/y,Cloud/MS,Cloud/MS/fleurent(perso),Cloud/MS/x" {
		t.Fatal(got)
	}
	// Out of every folder, the folder's own token keeps its last part.
	if renamed, err := s.RenameFolder("Cloud/MS", ""); err != nil || renamed != 3 {
		t.Fatal(renamed, err)
	}
	codes, _ = s.Codes()
	got = got[:0]
	for _, code := range codes {
		got = append(got, code.Issuer)
	}
	if strings.Join(got, ",") != "Azurex/y,fleurent(perso),MS,x" {
		t.Fatal(got)
	}
}

func TestPinnedShortcuts(t *testing.T) {
	s, fail := store(t, "")
	for _, issuer := range []string{"z/b/c", "a / b/c", "Hidden"} {
		if err := s.Add(totp.Token{Issuer: issuer, Secret: "JBSWY3DPEHPK3PXP"}); err != nil {
			t.Fatal(err)
		}
	}
	codes, _ := s.Codes()
	yes, no := true, false
	for _, code := range codes {
		if code.Issuer == "Hidden" {
			continue
		}
		if err := s.Edit(code.ID, code.Issuer, "", &yes); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
	pins := s.Pinned()
	if len(pins) != 2 || pins[0].Label != "a / b / c" || pins[1].Label != "z / b / c" {
		t.Fatalf("pins: %+v", pins)
	}
	before, err := s.PinnedCode(pins[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	s.Now = func() time.Time { return time.Unix(90, 0) }
	after, err := s.PinnedCode(pins[0].ID)
	if err != nil || before == after {
		t.Fatalf("code not recomputed: %v", err)
	}
	*fail = true
	if err := s.Edit(pins[0].ID, "Changed", "", &no); err == nil {
		t.Fatal("expected failed save")
	}
	if got := s.Pinned(); len(got) != 2 || got[0].Label != pins[0].Label {
		t.Fatal("failed save changed pins")
	}
	*fail = false
	if err := s.Rename(pins[0].ID, "new/path", "account"); err != nil {
		t.Fatal(err)
	}
	if s.Pinned()[0].Label != "new / path / account" {
		t.Fatal("rename lost pin")
	}
	if err := s.Edit(pins[0].ID, "new/path", "account", &no); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PinnedCode(pins[0].ID); err == nil {
		t.Fatal("unpinned code accepted")
	}
	if err := s.Delete(pins[1].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PinnedCode(pins[1].ID); err == nil {
		t.Fatal("deleted code accepted")
	}
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
	if len(s.Pinned()) != 0 {
		t.Fatal("unpin not persisted")
	}
}

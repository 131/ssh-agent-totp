// Package tokens keeps the token file, tokens.crypt: one otpauth:// URI per
// line, the whole text sealed with ssh-agent-crypt by a key of the SSH agent.
// It is unsealed once, kept in memory, and codes are computed here. See
// specs/TOKENS.md.
package tokens

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/131/ssh-agent-totp/internal/i18n"
	"github.com/131/ssh-agent-totp/internal/totp"
)

const (
	header = "# ssh-agent-totp tokens: https://github.com/131/ssh-agent-totp/blob/master/specs/TOKENS.md\n"
	// sealed starts a file encrypted by ssh-agent-crypt.
	sealed = "ssh-agent-crypt:"
)

// Crypt seals and unseals the file; key is the agent key selector (empty: the
// first agent key).
type Crypt struct {
	Encrypt func(plain []byte, key string) (string, error)
	Decrypt func(armored []byte) ([]byte, error)
}

// Code is one row of the list shown by the UI.
type Code struct {
	Pinned  bool   `json:"pinned"`
	ID      int    `json:"id"`
	Issuer  string `json:"issuer"`
	Account string `json:"account"`
	Code    string `json:"code,omitempty"`
	Period  int    `json:"period"`
	// Expires is the end of the code validity, in Unix milliseconds.
	Expires int64  `json:"expires,omitempty"`
	Error   string `json:"error,omitempty"`
}

type item struct {
	id    int
	token totp.Token
}

// Store is the token file, unsealed in memory.
type Store struct {
	Path  string
	Key   string
	Crypt Crypt
	Now   func() time.Time

	mu       sync.Mutex
	items    []item
	modified time.Time
	size     int64
	nextID   int
	loaded   bool
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}

	return time.Now()
}

func (s *Store) stat() (time.Time, int64) {
	info, err := os.Stat(s.Path)
	if err != nil {
		return time.Time{}, -1
	}

	return info.ModTime(), info.Size()
}

// Parse reads the clear text strictly: one otpauth:// URI per line, blank
// lines and "#" comments ignored. Any other line makes the whole file
// unreadable (the error gives its number, not its content: it holds a
// secret).
func Parse(body []byte) ([]totp.Token, error) {
	result := []totp.Token{}
	for index, line := range strings.Split(strings.ReplaceAll(string(body), "\r", ""), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		found, err := totp.ParseURI(line)
		if err == nil && !strings.HasPrefix(strings.ToLower(line), "otpauth://") {
			err = i18n.Errorf("err.totp.uri")
		}
		if err != nil {
			return nil, i18n.Errorf("err.tokens.line", index+1, err)
		}

		result = append(result, found...)
	}

	return result, nil
}

// Marshal is the clear text of tokens: the header comment, then one
// otpauth:// URI per line.
func Marshal(tokens []totp.Token) []byte {
	var body strings.Builder
	body.WriteString(header)
	for _, token := range tokens {
		body.WriteString(token.URI() + "\n")
	}

	return []byte(body.String())
}

// Load reads and unseals the file. A file written in clear is sealed and
// rewritten at once. Any error (agent, key, line) leaves the file untouched.
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.load()
}

func (s *Store) load() error {
	body, err := os.ReadFile(s.Path)
	clear := false
	switch {
	case errors.Is(err, os.ErrNotExist):
		body, err = nil, nil
	case err != nil:
		return i18n.Errorf("err.tokens.read", s.Path, err)
	case strings.HasPrefix(strings.TrimSpace(string(body)), sealed):
		if body, err = s.Crypt.Decrypt(bytes.TrimSpace(body)); err != nil {
			return i18n.Errorf("err.tokens.unseal", s.Path, err)
		}
	default:
		clear = len(bytes.TrimSpace(body)) > 0
	}
	found, err := Parse(body)
	if err != nil {
		return i18n.Errorf("err.tokens.read", s.Path, err)
	}

	s.items, s.loaded = nil, true
	for _, token := range found {
		s.nextID++
		s.items = append(s.items, item{id: s.nextID, token: token})
	}

	sortItems(s.items)
	if clear {
		return s.save(false)
	}

	s.modified, s.size = s.stat()
	return nil
}

// fresh reloads the file when it changed on disk since the last load or save.
func (s *Store) fresh() error {
	if modified, size := s.stat(); !s.loaded || !modified.Equal(s.modified) || size != s.size {
		return s.load()
	}

	return nil
}

func (s *Store) tokens() []totp.Token {
	tokens := []totp.Token{}
	for _, current := range s.items {
		tokens = append(tokens, current.token)
	}

	return tokens
}

// save seals the whole text with Key and writes it atomically. The
// previous sealed version is kept as tokens.crypt.bak (backup false: the
// previous file was in clear, no copy of it is made).
func (s *Store) save(backup bool) error {
	armored, err := s.Crypt.Encrypt(Marshal(s.tokens()), s.Key)
	if err != nil {
		return i18n.Errorf("err.tokens.seal", err)
	}

	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return err
	}

	if previous, err := os.ReadFile(s.Path); backup && err == nil {
		os.WriteFile(s.Path+".bak", previous, 0o600)
	}
	temporary := s.Path + ".tmp"
	if err := os.WriteFile(temporary, []byte(strings.TrimSpace(armored)+"\n"), 0o600); err != nil {
		return err
	}

	if err := os.Rename(temporary, s.Path); err != nil {
		return err
	}

	s.modified, s.size = s.stat()
	return nil
}

// sortItems is the one and only order: alphabetical, case-insensitive, on
// issuer then account. Names sharing a "issuer/" prefix end up together,
// which the UI shows as folders.
func sortItems(items []item) {
	slices.SortStableFunc(items, func(a, b item) int {
		return strings.Compare(sortKey(a.token), sortKey(b.token))
	})
}

func sortKey(token totp.Token) string {
	return strings.ToLower(token.Issuer) + "\x00" + strings.ToLower(token.Account)
}

// Codes returns the current code of every token, in alphabetical order.
func (s *Store) Codes() ([]Code, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.fresh(); err != nil {
		return nil, err
	}

	now := s.now()
	codes := []Code{}
	for _, current := range s.items {
		code := Code{
			Pinned:  current.token.Pinned,
			ID:      current.id,
			Issuer:  current.token.Issuer,
			Account: current.token.Account,
			Period:  current.token.Period,
		}
		if value, expires, err := current.token.Code(now); err != nil {
			code.Error = err.Error()
		} else {
			code.Code, code.Expires = value, expires.UnixMilli()
		}
		codes = append(codes, code)
	}

	return codes, nil
}

// ImportResult reports an import.
type ImportResult struct {
	Added      int            `json:"added"`
	Duplicates int            `json:"duplicates"`
	Skipped    []totp.Skipped `json:"skipped"`
}

func same(a, b totp.Token) bool {
	return a.Secret == b.Secret && strings.EqualFold(a.Issuer, b.Issuer) && strings.EqualFold(a.Account, b.Account)
}

// Import adds every token found in text (see totp.Parse), sealed with Key.
// Tokens already present are skipped.
func (s *Store) Import(text string) (ImportResult, error) {
	found, skipped := totp.Parse(text)
	result := ImportResult{Skipped: skipped}
	if result.Skipped == nil {
		result.Skipped = []totp.Skipped{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.fresh(); err != nil {
		return result, err
	}

	updated, added, duplicates := s.with(found)
	result.Added, result.Duplicates = added, duplicates
	if added == 0 {
		return result, nil
	}

	if err := s.commit(updated); err != nil {
		result.Added = 0
		return result, err
	}

	return result, nil
}

// with returns the current items plus the tokens not already present.
func (s *Store) with(found []totp.Token) (updated []item, added, duplicates int) {
	updated = slices.Clone(s.items)
	for _, token := range found {
		if slices.ContainsFunc(updated, func(current item) bool { return same(current.token, token) }) {
			duplicates++
			continue
		}

		s.nextID++
		updated = append(updated, item{id: s.nextID, token: token})
		added++
	}

	return updated, added, duplicates
}

// Add adds one token typed in the form.
func (s *Store) Add(token totp.Token) error {
	if token.Issuer = strings.TrimSpace(token.Issuer); token.Issuer == "" && strings.TrimSpace(token.Account) == "" {
		return i18n.Errorf("err.tokens.label")
	}

	if err := token.Normalize(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.fresh(); err != nil {
		return err
	}

	updated, added, _ := s.with([]totp.Token{token})
	if added == 0 {
		return i18n.Errorf("err.tokens.duplicate")
	}

	return s.commit(updated)
}

func (s *Store) find(id int) (int, error) {
	if err := s.fresh(); err != nil {
		return -1, err
	}

	index := slices.IndexFunc(s.items, func(current item) bool { return current.id == id })
	if index < 0 {
		return -1, i18n.Errorf("err.tokens.unknown")
	}

	return index, nil
}

// Rename changes the issuer and account of a token.
func (s *Store) Rename(id int, issuer, account string) error {
	return s.Edit(id, issuer, account, nil)
}

// Pin changes only the tray shortcut, keeping the latest token name.
func (s *Store) Pin(id int, pinned bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	index, err := s.find(id)
	if err != nil {
		return err
	}

	updated := slices.Clone(s.items)
	updated[index].token.Pinned = pinned
	return s.commit(updated)
}

// Edit saves the name and optional pin state atomically.
func (s *Store) Edit(id int, issuer, account string, pinned *bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	index, err := s.find(id)
	if err != nil {
		return err
	}

	issuer, account = strings.TrimSpace(issuer), strings.TrimSpace(account)
	if issuer == "" && account == "" {
		return i18n.Errorf("err.tokens.label")
	}

	updated := slices.Clone(s.items)
	updated[index].token.Issuer, updated[index].token.Account = issuer, account
	if pinned != nil {
		updated[index].token.Pinned = *pinned
	}
	return s.commit(updated)
}

// RenameFolder renames the folder "from" (a "/" separated path, see
// specs/TOKENS.md) to "to" in the issuer of every token inside it, the tokens
// whose issuer is "from" itself included (accounts of an issuer, token named
// like the folder). An empty "to" moves them to the top level; an issuer that
// is "from" itself keeps its last part. It returns the number of tokens renamed.
func (s *Store) RenameFolder(from, to string) (int, error) {
	clean := func(path string) string {
		parts := []string{}
		for _, part := range strings.Split(path, "/") {
			if part = strings.TrimSpace(part); part != "" {
				parts = append(parts, part)
			}
		}

		return strings.Join(parts, "/")
	}
	from, to = clean(from), clean(to)
	if from == "" {
		return 0, i18n.Errorf("err.tokens.unknown")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.fresh(); err != nil {
		return 0, err
	}

	updated, renamed := slices.Clone(s.items), 0
	for index, current := range updated {
		issuer := clean(current.token.Issuer)
		rest, inside := strings.CutPrefix(issuer, from+"/")
		switch {
		case inside && to != "":
			rest = to + "/" + rest
		case inside:
		case issuer == from && to != "":
			rest = to
		case issuer == from:
			rest = from[strings.LastIndex(from, "/")+1:]
		default:
			continue
		}
		updated[index].token.Issuer = rest
		renamed++
	}

	if renamed == 0 {
		return 0, i18n.Errorf("err.tokens.unknown")
	}

	return renamed, s.commit(updated)
}

// Delete removes a token.
func (s *Store) Delete(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	index, err := s.find(id)
	if err != nil {
		return err
	}

	return s.commit(slices.Delete(slices.Clone(s.items), index, index+1))
}

// Export returns the otpauth:// URI of every token, one per line.
func (s *Store) Export() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.fresh(); err != nil {
		return "", err
	}

	lines := []string{}
	for _, current := range s.items {
		lines = append(lines, current.token.URI())
	}

	return strings.Join(lines, "\n"), nil
}

// commit saves updated, keeping the former list when the file cannot be
// written.
func (s *Store) commit(updated []item) error {
	sortItems(updated)
	previous := s.items
	s.items = updated
	if err := s.save(true); err != nil {
		s.items = previous
		return err
	}

	return nil
}

// Reseal seals the file again with key, which becomes Key. It returns the
// number of tokens.
func (s *Store) Reseal(key string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.fresh(); err != nil {
		return 0, err
	}

	former := s.Key
	s.Key = key
	if err := s.save(true); err != nil {
		s.Key = former
		return 0, err
	}

	return len(s.items), nil
}

// PinnedItem identifies a tray shortcut without exposing its secret or code.
type PinnedItem struct {
	ID    int
	Label string
}

// Pinned returns the unlocked pins, sorted by their full displayed path.
func (s *Store) Pinned() []PinnedItem {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := []PinnedItem{}
	for _, current := range s.items {
		if !current.token.Pinned {
			continue
		}

		parts := []string{}
		for _, part := range strings.Split(current.token.Issuer, "/") {
			if part = strings.TrimSpace(part); part != "" {
				parts = append(parts, part)
			}
		}

		if current.token.Account != "" {
			parts = append(parts, current.token.Account)
		}
		result = append(result, PinnedItem{current.id, strings.Join(parts, " / ")})
	}

	slices.SortStableFunc(result, func(a, b PinnedItem) int { return strings.Compare(strings.ToLower(a.Label), strings.ToLower(b.Label)) })
	return result
}

// PinnedCode computes at click time and rejects removed or stale shortcuts.
func (s *Store) PinnedCode(id int) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	index, err := s.find(id)
	if err != nil {
		return "", err
	}

	if !s.items[index].token.Pinned {
		return "", i18n.Errorf("err.tokens.unknown")
	}

	value, _, err := s.items[index].token.Code(s.now())
	return value, err
}

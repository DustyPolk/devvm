package agentlogin

import (
	"cmp"
	"errors"
	"maps"
	"slices"
	"strings"
)

// KeyProviders are the providers whose API keys devvm agent login saves
// for kind, sorted: Codex's one, OpenAI's, which needs no name, and the
// opencode providers whose keys the proxy puts in (opencodeAPI). Any
// other's key would reach the VM as it is, so it's never saved this way.
func KeyProviders(kind string) []string {
	if kind == Opencode {
		return slices.Sorted(maps.Keys(opencodeAPI))
	}
	return nil
}

// errKeyProvider: an API key for a provider devvm doesn't put in.
var errKeyProvider = errors.New("no API key devvm puts in for that provider")

// SetKey saves key as the agent's API key in f, the store's login file,
// under its lock, in the agent's own format: Codex's auth.json becomes an
// API-key login, as codex login --with-api-key makes it, its ChatGPT login
// gone, and opencode's gets provider's entry, of KeyProviders, its other
// providers kept.
func (f File) SetKey(provider, key string) error {
	if strings.TrimSpace(key) == "" || strings.ContainsAny(key, "\r\n") {
		return errors.New("the API key is empty or more than one line")
	}
	switch {
	case f.Kind == Codex && provider == "":
	case f.Kind == Opencode && slices.Contains(KeyProviders(Opencode), provider):
	default:
		return errKeyProvider
	}
	unlock, err := f.lockStore()
	if err != nil {
		return err
	}
	defer unlock()
	m, err := readLogin(f.Path)
	if err != nil {
		return err
	}
	if f.Kind == Codex {
		m = map[string]any{"auth_mode": "apikey", "OPENAI_API_KEY": key}
	} else {
		m[provider] = map[string]any{"type": "api", "key": key}
	}
	return writeRefreshed(f.Path, m)
}

// ChatGPT is the ChatGPT login the store's login file f holds: Codex's,
// or opencode's OpenAI login, with the account's email when its tokens
// name one; ok is false when it holds none.
func (f File) ChatGPT() (email string, ok bool, err error) {
	m, err := readLogin(f.Path)
	if err != nil {
		return "", false, err
	}
	var tokens []string
	switch f.Kind {
	case Codex:
		t := obj(m, "tokens")
		tokens = []string{str(t, "id_token"), str(t, "access_token")}
	case Opencode:
		if e := obj(m, "openai"); str(e, "type") == "oauth" {
			tokens = []string{str(e, "access")}
		}
	}
	if len(tokens) == 0 || tokens[len(tokens)-1] == "" {
		return "", false, nil
	}
	for _, tok := range tokens {
		c := jwtClaims(tok)
		if e := cmp.Or(str(c, "email"), str(obj(c, "https://api.openai.com/profile"), "email")); e != "" {
			return e, true, nil
		}
	}
	return "", true, nil
}

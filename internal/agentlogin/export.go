package agentlogin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
)

// OpencodeSkipped is an opencode 2 login that OpencodeFromExport leaves out: its provider
// (integrationID) and opencode's label for it.
type OpencodeSkipped struct {
	Provider, Label string
}

// opencodeExportEntry is one credential as opencode 2's auth export prints it.
type opencodeExportEntry struct {
	IntegrationID string         `json:"integrationID"`
	Label         string         `json:"label"`
	Active        bool           `json:"active"`
	Value         map[string]any `json:"value"`
}

// OpencodeFromExport maps what opencode 2's auth export prints to the store's opencode login, in
// opencode 1's auth.json format: each provider's active credential, an API key as
// {"type":"api","key"} and an OpenAI login from a Codex method as {"type":"oauth","access",
// "refresh","expires","accountId"}. Any other login, such as Sign in with ChatGPT or another
// provider's OAuth, is left out and returned in skipped. login is nil when nothing is left.
func OpencodeFromExport(data []byte) (login []byte, skipped []OpencodeSkipped, err error) {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	var entries []opencodeExportEntry
	if err := d.Decode(&entries); err != nil {
		return nil, nil, fmt.Errorf("reading opencode auth export: %w", err)
	}
	m := map[string]any{}
	for _, e := range entries {
		if !e.Active || e.IntegrationID == "" || m[e.IntegrationID] != nil {
			continue // another of the provider's, which opencode doesn't use
		}
		if v := opencodeStoreEntry(e); v != nil {
			m[e.IntegrationID] = v
			continue
		}
		skipped = append(skipped, OpencodeSkipped{Provider: e.IntegrationID, Label: e.Label})
	}
	if len(m) == 0 {
		return nil, skipped, nil
	}
	login, err = json.MarshalIndent(m, "", "  ")
	return login, skipped, err
}

// opencodeStoreEntry is e in the store's format, nil when the store can't keep it.
func opencodeStoreEntry(e opencodeExportEntry) map[string]any {
	v := e.Value
	switch str(v, "type") {
	case "key":
		if str(v, "key") == "" {
			return nil
		}
		return map[string]any{"type": "api", "key": str(v, "key")}
	case "oauth":
		// Only opencode 2's Codex logins, whose tokens come from Codex's endpoint as opencode 1's
		// did ("" for a login it imported from auth.json); its Sign in with ChatGPT
		// (chatgpt-token-sharing) is a client of its own, whose tokens the store can't refresh.
		codex := []string{"", "chatgpt-browser", "chatgpt-headless"}
		if e.IntegrationID != "openai" || !slices.Contains(codex, str(v, "methodID")) ||
			str(v, "access") == "" || str(v, "refresh") == "" {
			return nil
		}
		out := map[string]any{"type": "oauth", "access": str(v, "access"), "refresh": str(v, "refresh"), "expires": v["expires"]}
		if a := str(obj(v, "metadata"), "accountID"); a != "" {
			out["accountId"] = a
		}
		return out
	}
	return nil
}

package agentlogin

import (
	"bytes"
	"encoding/json"
	"slices"
	"testing"
)

// opencode 2's auth export, mapped to the store's opencode login: each provider's active key and
// the OpenAI login from a Codex method, in auth.json's format; Sign in with ChatGPT, another
// provider's OAuth and an unknown type are skipped, by provider and label; an inactive credential
// is ignored.
func TestOpencodeFromExport(t *testing.T) {
	export := `[
	 {"id":"cred_1","integrationID":"openrouter","label":"old","active":false,"value":{"type":"key","key":"sk-old"}},
	 {"id":"cred_2","integrationID":"openrouter","label":"API key","active":true,"value":{"type":"key","key":"sk-or"}},
	 {"id":"cred_3","integrationID":"llama-cpp","label":"API key","active":true,"value":{"type":"key","key":"sk-ll"}},
	 {"id":"cred_4","integrationID":"openai","label":"OpenAI","active":true,"value":{"type":"oauth","methodID":"chatgpt-headless",
	   "access":"at","refresh":"rt","expires":1792771159402,"metadata":{"accountID":"acct-1"}}},
	 {"id":"cred_5","integrationID":"github-copilot","label":"Copilot","active":true,"value":{"type":"oauth","access":"a","refresh":"r","expires":1}},
	 {"id":"cred_6","integrationID":"anthropic","label":"Mystery","active":true,"value":{"type":"wellknown","token":"x"}}
	]`
	login, skipped, err := OpencodeFromExport([]byte(export))
	must(t, err)
	var m map[string]any
	d := json.NewDecoder(bytes.NewReader(login))
	d.UseNumber()
	must(t, d.Decode(&m))
	if e := obj(m, "openrouter"); len(e) != 2 || str(e, "type") != "api" || str(e, "key") != "sk-or" {
		t.Errorf("openrouter: %v", e)
	}
	if e := obj(m, "llama-cpp"); str(e, "key") != "sk-ll" {
		t.Errorf("llama-cpp: %v", e)
	}
	e := obj(m, "openai")
	if str(e, "type") != "oauth" || str(e, "access") != "at" || str(e, "refresh") != "rt" || str(e, "accountId") != "acct-1" ||
		e["expires"] != json.Number("1792771159402") || len(e) != 5 {
		t.Errorf("openai: %v", e)
	}
	if len(m) != 3 {
		t.Errorf("login: %v", m)
	}
	want := []OpencodeSkipped{{"github-copilot", "Copilot"}, {"anthropic", "Mystery"}}
	if !slices.Equal(skipped, want) {
		t.Errorf("skipped = %v, want %v", skipped, want)
	}

	sharing := `[{"integrationID":"openai","label":"ChatGPT","active":true,"value":{"type":"oauth","methodID":"chatgpt-token-sharing","access":"a","refresh":"r","expires":1}}]`
	login, skipped, err = OpencodeFromExport([]byte(sharing))
	must(t, err)
	if login != nil || !slices.Equal(skipped, []OpencodeSkipped{{"openai", "ChatGPT"}}) {
		t.Errorf("Sign in with ChatGPT: %s, %v", login, skipped)
	}
	if login, skipped, err = OpencodeFromExport([]byte(`[]`)); err != nil || login != nil || skipped != nil {
		t.Errorf("nothing: %s, %v, %v", login, skipped, err)
	}
	if _, _, err = OpencodeFromExport([]byte(`not json`)); err == nil {
		t.Error("not JSON: no error")
	}
}

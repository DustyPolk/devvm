package proxy

import (
	"bytes"
	"strings"
	"testing"
)

func redacted(t *testing.T, format, in string) (string, string) {
	t.Helper()
	var out, names bytes.Buffer
	if err := redactCmd(format, strings.NewReader(in), &out, &names); err != nil {
		t.Fatal(err)
	}
	return out.String(), names.String()
}

// Codex's config.toml with secrets in headers and env, and settings that only name them.
func TestRedactTOML(t *testing.T) {
	in := `model = "o4"
preferred_auth_method = "apikey"
mcp_servers.u.env.SECRET_X = 'DUMMY-SQ'

[model_providers.x]
env_key = "OPENAI_API_KEY"
http_headers = { "Authorization" = "Bearer DUMMY-CODEX", "X-Org" = "acme" }

[mcp_servers.s]
command = "npx"
args = ["srv"]
env = { API_KEY = "DUMMY-ENV", MODE = "fast" }

[mcp_servers.k]
args = ["-y", "srv-k", "--api-key", "DUMMY-ARG"]

[mcp_servers.t.env]
GITHUB_TOKEN = "DUMMY-GH" # a comment
`
	out, names := redacted(t, "toml", in)
	for _, s := range []string{"DUMMY-CODEX", "DUMMY-ENV", "DUMMY-GH", "DUMMY-SQ", "DUMMY-ARG"} {
		if strings.Contains(out, s) {
			t.Errorf("copy holds %q:\n%s", s, out)
		}
	}
	for _, s := range []string{
		`env_key = "OPENAI_API_KEY"`, `"X-Org" = "acme"`, `MODE = "fast"`, `preferred_auth_method = "apikey"`,
		`GITHUB_TOKEN = "devvm-redacted" # a comment`, `args = ["srv"]`,
	} {
		if !strings.Contains(out, s) {
			t.Errorf("copy lost %q:\n%s", s, out)
		}
	}
	want := "mcp_servers.k.args\nmcp_servers.s.env.API_KEY\nmcp_servers.t.env.GITHUB_TOKEN\nmcp_servers.u.env.SECRET_X\nmodel_providers.x.http_headers.Authorization\n"
	if names != want {
		t.Errorf("names = %q, want %q", names, want)
	}
	if _, err := redactTOML("x = \"\"\"\nsecret\n\"\"\"\n", new([]string)); err == nil {
		t.Error("a multi-line string was copied unread")
	}
}

// opencode.json (JSONC) and Claude Code's settings.json.
func TestRedactJSON(t *testing.T) {
	in := `{
  // a comment with "quotes"
  "provider": {"anthropic": {"options": {"apiKey": "DUMMY-OC", "baseURL": "https://x"}}},
  "other": {"options": {"apiKey": "{env:OTHER_KEY}"}},
  "mcp": {"m": {"headers": {"Authorization": "Bearer DUMMY-H"}, "command": ["a", "b"]}},
  "env": {"ANTHROPIC_API_KEY": "DUMMY-CL", "ANTHROPIC_BASE_URL": "https://gw", "N": 1},
  "apiKeyHelper": "/usr/local/bin/key-helper", /* trailing */
}`
	out, names := redacted(t, "json", in)
	for _, s := range []string{"DUMMY-OC", "DUMMY-H", "DUMMY-CL"} {
		if strings.Contains(out, s) {
			t.Errorf("copy holds %q:\n%s", s, out)
		}
	}
	for _, s := range []string{"{env:OTHER_KEY}", "https://gw", "/usr/local/bin/key-helper", `"baseURL": "https://x"`} {
		if !strings.Contains(out, s) {
			t.Errorf("copy lost %q:\n%s", s, out)
		}
	}
	if want := "env.ANTHROPIC_API_KEY\nmcp.m.headers.Authorization\nprovider.anthropic.options.apiKey\n"; names != want {
		t.Errorf("names = %q, want %q", names, want)
	}
}

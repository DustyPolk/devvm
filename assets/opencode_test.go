package assets

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeOpencode stands in for opencode 2 in guest/opencode.sh's test: its
// credentials are $HOME/creds.json, auth export prints them, auth import
// adds the entries whose IDs it lacks (refusing one for the provider
// "refused"), auth logout removes one, and each call is logged to
// $HOME/calls.
const fakeOpencode = `#!/bin/bash
set -eu
c=$HOME/creds.json
[ -f "$c" ] || echo '[]' > "$c"
echo "$*" >> "$HOME/calls"
case "$1 $2" in
"auth export") cat "$c" ;;
"auth import")
  in=$(cat)
  if jq -e 'any(.[]; .integrationID == "refused")' <<<"$in" >/dev/null; then echo "Missing key" >&2; exit 1; fi
  jq --argjson in "$in" '. as $have | . + [$in[] | select(.id as $i | all($have[]; .id != $i))]' "$c" > "$c.new" && mv "$c.new" "$c" ;;
"auth logout")
  jq --arg id "$4" '[.[] | select(.id != $id)]' "$c" > "$c.new" && mv "$c.new" "$c" ;;
"auth switch") ;;
*) exit 1 ;;
esac
`

// opencode.sh imports devvm's entries once opencode has a database, and
// again whenever the input changed or the database lost one of devvm's
// IDs (made again, or replaced); with no database it records nothing; a
// refused entry names its provider, fails, and records nothing.
func TestOpencodeScript(t *testing.T) {
	for _, c := range []string{"bash", "jq"} {
		if _, err := exec.LookPath(c); err != nil {
			t.Skipf("no %s: %v", c, err)
		}
	}
	script, err := fs.ReadFile(FS, "guest/opencode.sh")
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	bin := filepath.Join(home, "bin")
	must(t, os.MkdirAll(bin, 0o700))
	must(t, os.WriteFile(filepath.Join(bin, "opencode"), []byte(fakeOpencode), 0o700)) //nolint:gosec // the fake runs: its owner's alone
	db := filepath.Join(home, ".local", "share", "opencode", "opencode.db")
	const record = ".local/state/devvm/opencode-import.json"
	files := os.DirFS(home)
	read := func(name string) string { b, _ := fs.ReadFile(files, name); return string(b) }
	exists := func(name string) bool { _, err := fs.Stat(files, name); return err == nil }
	run := func(input string) (string, int) {
		cmd := exec.CommandContext(t.Context(), "bash", "-c", string(script), "opencode.sh") //nolint:gosec // devvm's own embedded script

		cmd.Env = []string{"HOME=" + home, "PATH=" + bin + ":" + os.Getenv("PATH")}
		cmd.Stdin = strings.NewReader(input)
		var out bytes.Buffer
		cmd.Stdout = &out
		err := cmd.Run()
		code := 0
		var e *exec.ExitError
		if errors.As(err, &e) {
			code = e.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		return out.String(), code
	}
	imports := func() int { return strings.Count(read("calls"), "auth import") }
	creds := func() string { return read("creds.json") }
	in := `[{"active":true,"id":"cred_devvm_openrouter","integrationID":"openrouter","label":"devvm","value":{"key":"PH","type":"key"}}]`

	if _, code := run(in); code != 0 || imports() != 0 || exists(record) {
		t.Fatalf("no database: code %d, %d imports, record %v", code, imports(), exists(record))
	}
	must(t, os.MkdirAll(filepath.Dir(db), 0o700))
	must(t, os.WriteFile(db, nil, 0o600))
	if _, code := run(in); code != 0 || imports() != 1 || !strings.Contains(creds(), "cred_devvm_openrouter") || !exists(record) {
		t.Fatalf("a database: code %d, %d imports, creds %s", code, imports(), creds())
	}
	if _, code := run(in); code != 0 || imports() != 1 {
		t.Fatalf("the same input again: code %d, %d imports", code, imports())
	}
	// The database made again: devvm's entry is gone, so it's imported again.
	must(t, os.Remove(filepath.Join(home, "creds.json")))
	if _, code := run(in); code != 0 || imports() != 2 || !strings.Contains(creds(), "cred_devvm_openrouter") {
		t.Fatalf("a new database: code %d, %d imports, creds %s", code, imports(), creds())
	}
	refused := `[{"active":true,"id":"cred_devvm_refused","integrationID":"refused","label":"devvm","value":{"key":"PH","type":"key"}}]`
	before := read(record)
	if out, code := run(refused); code != 1 || out != "devvm-opencode-failed refused\n" {
		t.Fatalf("a refused entry: code %d, output %q", code, out)
	}
	if read(record) != before {
		t.Error("a refused entry changed the record")
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

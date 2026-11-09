package assets

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// bashes are the shells guest/transparent.sh's test runs it with: bash on
// the PATH, and macOS's /bin/bash, which is 3.2, as in-VM scripts must
// run on.
func bashes(t *testing.T) []string {
	t.Helper()
	var out []string
	if p, err := exec.LookPath("bash"); err == nil {
		out = append(out, p)
	}
	if _, err := os.Stat("/bin/bash"); err == nil && !slices.Contains(out, "/bin/bash") {
		out = append(out, "/bin/bash")
	}
	if len(out) == 0 {
		t.Skip("no bash")
	}
	return out
}

// transparentScript writes guest/transparent.sh into a folder of the
// test's own, and returns its path and the folder, its files' folder.
func transparentScript(t *testing.T) (string, string) {
	t.Helper()
	b, err := fs.ReadFile(FS, "guest/transparent.sh")
	must(t, err)
	dir := t.TempDir()
	f := filepath.Join(dir, "transparent.sh")
	must(t, os.WriteFile(f, b, 0o600))
	return f, dir
}

// runTransparent runs the script with bash and args, its files in dir,
// and returns its output and whether it succeeded.
func runTransparent(t *testing.T, bash, script, dir string, args ...string) (string, bool) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), bash, append([]string{script}, args...)...) //nolint:gosec // G204: devvm's own embedded script, with the test's arguments
	cmd.Env = append(os.Environ(), "DEVVM_TRANSPARENT_DIR="+dir, "DEVVM_TRANSPARENT_RESOLVERS=192.168.5.2 10.0.2.3")
	out, err := cmd.Output()
	return string(out), err == nil
}

// The ruleset: each redirected port to its listener, every other port of
// the stand-in address to the dead port (so Docker's published ports never
// take it), DNS to public servers and 127.0.0.1 to dnsmasq, private ones
// left alone, and nftables takes it where it can be asked.
func TestTransparentRuleset(t *testing.T) {
	script, dir := transparentScript(t)
	for _, bash := range bashes(t) {
		out, ok := runTransparent(t, bash, script, dir, "render", "80=61080", "443=61443", "8443=61100")
		if !ok {
			t.Fatalf("%s: render failed:\n%s", bash, out)
		}
		for _, want := range []string{
			"ip daddr 198.18.0.1 tcp dport 80 dnat ip to 198.18.0.1:61080",
			"ip daddr 198.18.0.1 tcp dport 8443 dnat ip to 198.18.0.1:61100",
			"meta l4proto tcp dnat ip to 198.18.0.1:61079",
			"meta l4proto udp dnat ip to 198.18.0.1:61079",
			"udp dport 53 dnat ip to 198.18.0.1:53",
			"ip daddr 127.0.0.1 tcp dport 53 dnat ip to 198.18.0.1:53",
			"ip daddr != @kept udp dport 53 dnat ip to 198.18.0.1:53",
			"ip daddr { 192.168.5.2, 10.0.2.3 } tcp dport 53 dnat ip to 198.18.0.1:53",
			"10.0.0.0/8, 100.64.0.0/10, 127.0.0.0/8, 169.254.0.0/16, 172.16.0.0/12, 192.168.0.0/16",
			"type nat hook output priority -110;",
			"type nat hook prerouting priority -110;",
			`iifname "br-*" jump from_containers`,
			"type filter hook prerouting priority -150;",
			"type filter hook output priority -150;",
			`ip daddr 198.18.0.1 iifname != "lo" iifname != "docker0" iifname != "br-*" drop`,
			"ip daddr 198.18.0.1 meta l4proto != { tcp, udp, icmp } drop",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("%s: the ruleset lacks %q:\n%s", bash, want, out)
			}
		}
		if strings.Count(out, "{") != strings.Count(out, "}") || strings.Count(out, "jump sentinel") != 2 {
			t.Errorf("%s: unbalanced, or a chain without the stand-in address's ports:\n%s", bash, out)
		}
		if n, err := exec.LookPath("nft"); err == nil {
			f := filepath.Join(dir, "rules.nft")
			must(t, os.WriteFile(f, []byte(out), 0o600))
			if b, err := exec.CommandContext(t.Context(), n, "-c", "-f", f).CombinedOutput(); err != nil { //nolint:gosec // G204: nft from the PATH, checking the test's ruleset
				if strings.Contains(string(b), "Operation not permitted") {
					t.Log("nft -c can't run here")
				} else {
					t.Errorf("nft -c refused it: %v\n%s", err, b)
				}
			}
		}
	}
}

// Each port past 80 and 443 keeps the listener it had while it stays, a
// port that's gone frees its own, and a new one takes the lowest free.
func TestTransparentSlots(t *testing.T) {
	script, dir := transparentScript(t)
	must(t, os.WriteFile(filepath.Join(dir, "slots"), []byte("8080 61101\n9000 61100\n"), 0o600))
	for _, bash := range bashes(t) {
		out, ok := runTransparent(t, bash, script, dir, "slots", "80", "443", "9000", "8443", "7000")
		if want := "80 61080\n443 61443\n9000 61100\n8443 61101\n7000 61102\n"; !ok || out != want {
			t.Errorf("%s: slots:\n%s\nwant:\n%s", bash, out, want)
		}
	}
}

// The proxy's port list goes into the ruleset only when it's 2 to 34
// ports, each once, with 80 and 443, never 22 or 53.
func TestTransparentPortList(t *testing.T) {
	script, dir := transparentScript(t)
	many := "80\n443\n"
	for p := 9000; p < 9033; p++ {
		many += strconv.Itoa(p) + "\n"
	}
	for _, bash := range bashes(t) {
		for _, c := range []struct {
			list string
			ok   bool
		}{
			{"80\n443\n", true},
			{"80\n443\n8443\n", true},
			{"443\n80\n", true},
			{"80\n", false},
			{"80\n8443\n", false},
			{"80\n443\n22\n", false},
			{"80\n443\n53\n", false},
			{"80\n443\n8443\n8443\n", false},
			{"80\n443\n65536\n", false},
			{"80\n443\n08443\n", false},
			{"80\n443\n84 43\n", false},
			{"80\n443\n$(id)\n", false},
			{many, false},
		} {
			f := filepath.Join(dir, "ports-list")
			must(t, os.WriteFile(f, []byte(c.list), 0o600))
			if _, ok := runTransparent(t, bash, script, dir, "valid", f); ok != c.ok {
				t.Errorf("%s: %q: %v, want %v", bash, c.list, ok, c.ok)
			}
		}
	}
}

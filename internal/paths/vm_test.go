package paths

import (
	"path/filepath"
	"strings"
	"testing"
)

// Every file of a VM's is in its folder, each in its own place, none of
// them where another is.
func TestVMFiles(t *testing.T) {
	v := Dirs{Data: "/d"}.VM("devvm-p-0123456789abcd")
	want := map[string]string{
		v.State():                     "state",
		v.Policy():                    "policy",
		v.Backend():                   "backend",
		v.Run():                       "run",
		v.Logs():                      "logs",
		v.Proxy(".pid"):               "run/proxy.pid",
		v.Proxy(".auth"):              "run/proxy.auth",
		v.Profile():                   "run/sandbox.sb",
		v.BuildProxy():                "run/build-proxy",
		v.Started():                   "run/started",
		v.Secrets():                   "run/secrets.json",
		v.SecretsPolicy():             "run/secrets.policy",
		v.LinksPID():                  "run/links.pid",
		v.LinkList():                  "run/links.json",
		v.EventLog():                  "logs/network.jsonl",
		v.ProxyLog():                  "logs/proxy.log",
		v.LinksLog():                  "logs/links.log",
		v.Record():                    "backend/record.json",
		v.Key():                       "backend/key",
		v.KnownHosts():                "backend/known_hosts",
		v.ProxyURL():                  "backend/proxy",
		v.Ports():                     "backend/ports",
		v.Forwarder(".log"):           "backend/ports.log",
		v.Forwarder(".known_hosts"):   "backend/ports.known_hosts",
		v.Forwarder(".lock"):          "backend/ports.lock",
		filepath.Join(v.Dir, "other"): "other",
	}
	if len(want) != 26 {
		t.Fatalf("%d paths, want 26: two are the same", len(want))
	}
	for got, rel := range want {
		if got != filepath.Join("/d/vms/0123456789abcd", rel) {
			t.Errorf("got %s, want %s in the VM's folder", got, rel)
		}
		if strings.HasSuffix(got, ".sock") || strings.HasSuffix(got, ".ctl") {
			t.Errorf("%s: a socket in a VM's folder", got)
		}
	}
}

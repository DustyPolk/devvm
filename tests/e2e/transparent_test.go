//go:build e2e

package e2e

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/devvmhq/devvm/internal/backend"
	"github.com/devvmhq/devvm/internal/proxy"
)

// Transparent mode, for tools that ignore the proxy settings: on a confined
// VM every name resolves to the stand-in address, whose ports 80, 443 and
// those the rules name reach the VM's proxy, which judges each by the name
// in the connection.

// noProxyEnv runs a command without the proxy settings, as a tool that
// ignores them would.
const noProxyEnv = "env -u HTTPS_PROXY -u HTTP_PROXY -u https_proxy -u http_proxy -u ALL_PROXY -u all_proxy "

// transparentReady is a script that's true once transparent mode runs in
// the VM: its supervisor has started what it runs.
const transparentReady = `for i in $(seq 50); do sudo test -e /run/devvm-transparent/ready && exit 0; sleep 0.2; done; exit 1`

// withoutTransparent runs fn with vm's transparent mode stopped, as root in
// the VM could stop it: its service stopped, which its supervisor doesn't
// start again, and its table, address and dnsmasq gone, all checked before
// fn runs, so fn sees the VM as the host's confinement leaves it. Then it
// starts the service again and waits until it's ready.
func withoutTransparent(s *S, vm string, fn func() error) error {
	if err := inVM(s, vm, `{ sudo systemctl stop devvm-transparent 2>/dev/null || sudo rc-service devvm-transparent stop >/dev/null; } &&
! sudo nft list table inet devvm_transparent >/dev/null 2>&1 && ! ip -4 addr show dev lo | grep -q ' 198\.18\.0\.1/' && ! pgrep -x dnsmasq >/dev/null`); err != nil {
		return failed("transparent mode didn't stop", err)
	}
	err := fn()
	if err2 := inVM(s, vm, `{ sudo systemctl start devvm-transparent 2>/dev/null || sudo rc-service devvm-transparent start >/dev/null; } && `+transparentReady); err == nil && err2 != nil {
		err = failed("transparent mode didn't start again", err2)
	}
	return err
}

// dnsAnswers asks each server, over UDP and TCP, for example.com's A
// record, as root in the VM could: a refusal or no answer passes, and so
// does the stand-in address with TTL 0 when sentinel; any other answer,
// a real one, fails.
func dnsAnswers(sentinel bool, servers ...string) string {
	ok := "False"
	if sentinel {
		ok = "True"
	}
	return `python3 - ` + strings.Join(servers, " ") + ` <<'PY'
import socket, struct, sys
q = bytes.fromhex("123401000001000000000000076578616d706c6503636f6d0000010001")
def answer(d, h, how):
    rcode, n = d[3] & 15, int.from_bytes(d[6:8], "big")
    print(h, how, "rcode", rcode, "answers", n)
    if rcode != 0 or n == 0:
        return
    # The one answer after the question: name (a pointer), type, class, TTL, length, address.
    t, ttl, ln = struct.unpack(">H2xIH", d[len(q) + 2:len(q) + 12])
    a = socket.inet_ntoa(d[len(q) + 12:len(q) + 12 + ln]) if ln == 4 else "?"
    print(" ", a, "ttl", ttl)
    if not (` + ok + ` and n == 1 and t == 1 and a == "` + proxy.SentinelAddr + `" and ttl == 0):
        sys.exit("example.com answered by " + h + " over " + how + ": " + a)
for h in sys.argv[1:]:
    for how in ("udp", "tcp"):
        s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM if how == "udp" else socket.SOCK_STREAM)
        s.settimeout(3)
        try:
            if how == "udp":
                s.sendto(q, (h, 53))
                d = s.recv(512)
            else:
                s.connect((h, 53))
                s.sendall(len(q).to_bytes(2, "big") + q)
                d = s.recv(514)[2:]
        except OSError as e:
            print(h, how, e)
            continue
        if len(d) >= 12:
            answer(d, h, how)
PY`
}

// transparentRuns checks transparent mode in vm, a confined VM, on any
// base (systemd or OpenRC, glibc or musl): its service and what it runs,
// on the stand-in address alone; names; a tool without proxy settings
// reaching an allowed host by name, and a blocked one's 403 over TLS; and
// another port refused at once. dir is the VM's folder, title its kind.
func transparentRuns(s *S, vm, dir, title string) {
	s.Check(title+": transparent mode runs: its service, the stand-in address on lo, its table, and dnsmasq and socat on that address alone", func() error {
		return inVM(s, vm, transparentReady+` && sudo devvm-transparent status | head -1 | grep -q '^Transparent mode: on, for ports 80 443' &&
ip -4 addr show dev lo | grep -q ' 198\.18\.0\.1/32 ' && sudo nft list table inet devvm_transparent >/dev/null &&
l=$(if command -v ss >/dev/null; then sudo ss -Hltnup; else sudo netstat -ltnup; fi) &&
echo "$l" | grep -q '198\.18\.0\.1:53 .*dnsmasq' && echo "$l" | grep -q '198\.18\.0\.1:61443 .*socat' && echo "$l" | grep -q '198\.18\.0\.1:61080 .*socat' &&
! echo "$l" | grep -E 'dnsmasq|socat' | grep -vE '198\.18\.0\.1:(53|610[78][0-9]|61443|611[0-3][0-9]) |127\.0\.77\.1:3128 '`)
	})
	s.Check(title+": every name resolves to the stand-in address, but one without a dot, IPv6 has none at once, and /etc/hosts' names keep theirs", func() error {
		// A name without a dot gets the VM's search domain, which Lima takes from this machine's
		// network, and neither resolves.
		return inVM(s, vm, `ip=$(curl -s -m 10 -o /dev/null -w '%{remote_ip}' --noproxy '*' http://example.org/); [ "$ip" = 198.18.0.1 ] || { echo "example.org: $ip"; exit 1; }
ip=$(curl -s -m 5 -o /dev/null -w '%{remote_ip}' --noproxy '*' http://devvm-e2e-single-label/); [ -z "$ip" ] || { echo "a single-label name resolved to $ip"; exit 1; }
s=$(date +%s%N); if curl -6 -s -m 3 -o /dev/null --noproxy '*' http://example.org/; then exit 1; fi; e=$((($(date +%s%N) - s) / 1000000)); [ "$e" -lt 1000 ] || { echo "IPv6 took $e ms"; exit 1; }
if getent ahostsv4 localhost >/dev/null 2>&1; then
  getent hosts localhost | grep -Eq '^(127\.0\.0\.1|::1) ' && getent ahostsv4 "$(hostname)" | grep -q '^127\.' &&
    getent ahostsv4 host.lima.internal | grep -q '^192\.168\.5\.' || { echo "/etc/hosts' names"; exit 1; }
  ! getent hosts devvm-e2e-single-label || { echo "a single-label name that isn't in /etc/hosts resolved"; exit 1; }
fi`)
	})
	s.Check(title+": a tool without proxy settings reaches an allowed host by name, its own certificate and all, and devvm network log says so", func() error {
		r, err := s.InVM(vm, noProxyEnv+`curl -fsS -m 20 -v -o /dev/null -w '%{http_code} %{remote_ip}\n' https://github.com 2>&1`)
		switch {
		case err != nil:
			return err
		case !strings.Contains(r.Stdout, "200 198.18.0.1") || !strings.Contains(r.Stdout, "issuer:") || strings.Contains(r.Stdout, "devvm proxy CA"):
			return errors.New("github.com wasn't reached through the stand-in address with its own certificate")
		}
		seen(s, title+": curl without proxy settings", LastLine(r.Stdout))
		return s.Until(10*time.Second, func() error {
			l, _ := s.Devvm(dir, "network", "log", "-n", "200")
			if !lineWith(l.Stdout, "  allowed  github.com:443  ", "without proxy settings") {
				return errors.New("devvm network log has no line for it without proxy settings")
			}
			seen(s, title+": devvm network log", linesWith(l.Stdout, "without proxy settings"))
			return nil
		})
	})
	s.Check(title+": a blocked host gets the proxy's 403 over TLS, signed by the VM's CA, with the command that allows it, and the log has it", func() error {
		r, err := s.InVM(vm, noProxyEnv+`curl -sS -m 20 -v -D - -o /tmp/tp-body https://example.net/ 2>&1; cat /tmp/tp-body`)
		switch {
		case err != nil:
			return err
		case !Matches(r.Stdout, `^HTTP/[0-9.]+ 403`) || !Matches(r.Stdout, `(?i)^x-devvm-blocked: example\.net`) ||
			!strings.Contains(r.Stdout, "devvm network allow example.net") || !strings.Contains(r.Stdout, "devvm proxy CA"):
			return errors.New("example.net didn't get the proxy's 403 over the VM's CA")
		}
		b, _ := s.Devvm(dir, "network", "log", "--blocked")
		l, _ := s.Devvm(dir, "network", "log", "-n", "200")
		if !Matches(b.Stdout, `example\.net:443 +devvm network allow example\.net$`) || !lineWith(l.Stdout, "  BLOCKED  example.net:443  ", "without proxy settings") {
			return errors.New("devvm network log doesn't show example.net blocked without proxy settings")
		}
		seen(s, title+": the blocked host's response", linesWith(r.Stdout, "HTTP/", "x-devvm-blocked", "issuer:", "devvm: "))
		return nil
	})
	s.Check(title+": another port of a name is refused at once, and devvm-why names it", func() error {
		if err := inVM(s, vm, `s=$(date +%s%N); if curl --noproxy '*' -sS -m 5 -o /dev/null http://example.org:5432/; then exit 1; fi; e=$((($(date +%s%N) - s) / 1000000))
[ "$e" -lt 1000 ] || { echo "it took $e ms"; exit 1; }
sleep 1; devvm-why | sed -n '/^Refused lately/,/^Names/p' | grep -q '5432'`); err != nil {
			return err
		}
		seen(s, title+": devvm-why", vmOut(s, vm, "devvm-why"))
		return nil
	})
}

// seen records what a check saw in the run's output, a line each, for a
// report: the run prints only checks' results otherwise.
func seen(s *S, what, text string) {
	for l := range strings.SplitSeq(strings.TrimSpace(text), "\n") {
		s.r.out.line("  e2e: %s | %s", what, l)
	}
}

// linesWith is text's lines that hold one of subs, letters' case aside.
func linesWith(text string, subs ...string) string {
	var out []string
	for l := range strings.SplitSeq(text, "\n") {
		for _, sub := range subs {
			if strings.Contains(strings.ToLower(l), strings.ToLower(sub)) {
				out = append(out, l)
				break
			}
		}
	}
	return strings.Join(out, "\n")
}

// netTransparent: the network section's VM, confined, in transparent mode:
// what transparentRuns checks, then a client with a resolver of its own;
// a port that a rule names, redirected while the rule is there; the
// user's own servers on 80 and 443; the service coming back when it's
// killed or its table flushed; and devvm network transparent off and on.
func netTransparent(n *netRun) {
	s, vm := n.s, n.vm
	transparentRuns(s, vm, n.n, "allowlist")
	s.Check("a static Go client with its own resolver and no proxy fetches an allowed host by name, with its real certificate", func() error {
		if err := noProxyClient(s, vm); err != nil {
			return err
		}
		out := vmOut(s, vm, noProxyEnv+"/tmp/noproxy https://github.com/")
		if !strings.HasPrefix(out, "200 ") || strings.Contains(out, "devvm") {
			return errors.New("the Go client got: " + out)
		}
		seen(s, "the Go client", out)
		return nil
	})
	s.Check("without the proxy settings, devvm.proxy answers /blocked alone, and refuses the VM's CA", func() error {
		return inVM(s, vm, noProxyEnv+`curl -sS -m 10 http://devvm.proxy/blocked | grep -q 'example.net' &&
[ "$(`+noProxyEnv+`curl -s -m 10 -o /dev/null -w '%{http_code}' http://devvm.proxy/ca.pem)" = 403 ]`)
	})
	s.Check("a port a rule names is redirected while the rule is there, and refused at once after devvm network rm", func() error { return transparentPort(n) })
	s.Check("the user's own servers on 0.0.0.0:443 and :80 bind and answer while transparent mode runs", func() error {
		return inVM(s, vm, `sudo sh -c 'cd /tmp && (python3 -m http.server --bind 0.0.0.0 443 >/dev/null 2>&1 & echo $! > /tmp/tp443; python3 -m http.server --bind 0.0.0.0 80 >/dev/null 2>&1 & echo $! > /tmp/tp80)'
sleep 2; ok=0; [ "$(curl -s -m 5 -o /dev/null -w '%{http_code}' --noproxy '*' http://127.0.0.1:443/)" = 200 ] &&
[ "$(curl -s -m 5 -o /dev/null -w '%{http_code}' --noproxy '*' http://127.0.0.1:80/)" = 200 ] && ok=1
sudo kill "$(cat /tmp/tp443)" "$(cat /tmp/tp80)"; [ $ok = 1 ]`)
	})
	s.Check("killed, the service is back within seconds; its table flushed, it's loaded again within 15 s", func() error {
		return inVM(s, vm, `sudo systemctl kill -s KILL devvm-transparent && sleep 3 && `+transparentReady+` &&
sudo nft flush ruleset && for i in $(seq 15); do sudo nft list table inet devvm_transparent >/dev/null 2>&1 && exit 0; sleep 1; done; exit 1`)
	})
	s.Check("devvm network transparent off: from the next launch, no service, table or address, and names fail at once; on: back", func() error {
		off, err := s.Devvm(n.n, "network", "transparent", "off")
		if err != nil {
			return err
		}
		seen(s, "devvm network transparent off", off.Out)
		if _, err := n.dv("exit\n", n.home(fakeToken)); err != nil {
			return err
		}
		if err := inVM(s, vm, `! systemctl is-active -q devvm-transparent && [ ! -e /usr/local/sbin/devvm-transparent ] &&
! sudo nft list table inet devvm_transparent >/dev/null 2>&1 && ! ip -4 addr show dev lo | grep -q 198.18.0.1 &&
timeout 3 getent hosts example.com; [ $? -eq 2 ]`); err != nil {
			return failed("transparent mode is still there", err)
		}
		if r, _ := s.Devvm(n.n, "network", "transparent"); strings.TrimSpace(r.Stdout) != "off # devvm network transparent" {
			return errors.New("devvm network transparent shows " + r.Stdout)
		}
		seen(s, "devvm network transparent", "off # devvm network transparent")
		seen(s, "a lookup with it off", vmOut(s, vm, `curl --noproxy '*' -sS -m 5 -o /dev/null https://github.com 2>&1`))
		if _, err := s.Devvm(n.n, "network", "transparent", "on"); err != nil {
			return err
		}
		if _, err := n.dv("exit\n", n.home(fakeToken)); err != nil {
			return err
		}
		return inVM(s, vm, transparentReady+` && [ "$(curl -s -m 10 -o /dev/null -w '%{remote_ip}' --noproxy '*' http://example.org/)" = 198.18.0.1 ]`)
	})
}

// transparentPort: a TLS server on this machine's 127.0.0.1, on a port of
// its own, and a rule for it by name and address: within 15 s the VM's
// table redirects that port, and a tool without proxy settings reaches the
// server by the name localhost (--connect-to stands in for the lookup:
// localhost is /etc/hosts'); after devvm network rm, within 15 s it's
// refused at once.
func transparentPort(n *netRun) error {
	s, vm := n.s, n.vm
	ln, err := tlsListener(s)
	if err != nil {
		return err
	}
	defer func() { _ = ln.Close() }()
	go func() {
		_ = (&http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "e2e-port")
		})}).Serve(ln)
	}()
	p := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
	rules := []string{"localhost:" + p, "127.0.0.1:" + p}
	if _, err := s.Devvm(n.n, append([]string{"network", "allow"}, rules...)...); err != nil {
		return err
	}
	curl := noProxyEnv + `curl -sk -m 5 --connect-to localhost:` + p + `:198.18.0.1:` + p + ` https://localhost:` + p + `/`
	if err := s.Until(20*time.Second, func() error {
		if vmOut(s, vm, curl) != "e2e-port" {
			return errors.New("port " + p + " isn't redirected to the server")
		}
		return nil
	}); err != nil {
		return err
	}
	if _, err := s.Devvm(n.n, append([]string{"network", "rm"}, rules...)...); err != nil {
		return err
	}
	return s.Until(20*time.Second, func() error {
		return inVM(s, vm, `s=$(date +%s%N); if `+curl+`; then exit 1; fi; [ $((($(date +%s%N) - s) / 1000000)) -lt 1000 ]`)
	})
}

// tlsListener is a TLS listener on this machine's 127.0.0.1, on a port of
// its own, with a certificate for localhost that nothing trusts.
func tlsListener(s *S) (net.Listener, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "localhost"}, DNSNames: []string{"localhost"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	ln, err := new(net.ListenConfig).Listen(s.r.window().ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	return tls.NewListener(ln, &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}), nil
}

// noProxyClient builds testdata/noproxy for the VM, once a run, and puts it
// in vm as /tmp/noproxy.
func noProxyClient(s *S, vm string) error {
	bin := filepath.Join(s.sb.work, "noproxy-"+runtime.GOARCH)
	if _, err := os.Stat(bin); err != nil {
		args := []string{"go", "build", "-trimpath", "-o", bin, "./tests/e2e/testdata/noproxy"}
		if _, err := exec.LookPath("go"); err != nil {
			args = append([]string{"mise", "exec", "--"}, args...)
		}
		if _, err := s.Run(Cmd{Args: args, Dir: s.sb.root, Env: []string{"CGO_ENABLED=0", "GOOS=linux", "GOARCH=" + runtime.GOARCH}}); err != nil {
			return err
		}
	}
	b, err := os.ReadFile(bin)
	if err != nil {
		return err
	}
	_, err = s.Run(Cmd{Args: []string{"limactl", "shell", "--workdir", "/", vm, "sh", "-c", "cat > /tmp/noproxy && chmod 755 /tmp/noproxy"}, Stdin: string(b)})
	return err
}

// curlImage is the container the docker step runs curl in, pinned by
// digest: curlimages/curl, Alpine's busybox included.
const curlImage = "curlimages/curl:8.23.0@sha256:3fb7e13e5d179a845c97d78204f3f1064d1907037f26a3ebf91710571151fd93"

// noProxyContainer is docker run's options for a container without the
// proxy settings devvm gives every container.
const noProxyContainer = "-e HTTPS_PROXY= -e HTTP_PROXY= -e https_proxy= -e http_proxy= -e NO_PROXY= -e no_proxy= -e ALL_PROXY= "

// netTransparentDocker: a VM of the docker layer on the allowlist tier,
// whose containers skip the proxy settings: on the default bridge and on
// a network of their own they reach an allowed host by name and get a
// blocked one's block (a certificate error, as they don't trust the VM's
// CA); they reach no other port of the stand-in address, nor its
// listeners directly; a container's published 5432 isn't reached through
// the stand-in address from the VM or from another container, while its
// published 443 doesn't take the VM's connections to the stand-in
// address; and a container on an --internal network reaches nothing.
func netTransparentDocker(n *netRun) {
	s := n.s.Needing(backend.FeatureDockerLayer)
	d, _ := s.NewProject("netdocker")
	var vm string
	docker := func(args string) string { return "sudo docker run --rm " + noProxyContainer + args }
	s.Check("docker: containers that skip the proxy reach an allowed host by name, on the default bridge and on a network of their own", func() error {
		vm, _, _ = launchWith(s, d, n.home(fakeToken), "--network", "allowlist", "--no-git-protect", "--layers", "docker", "--agents", "none")
		if vm == "" || s.Status(vm) != "Running" {
			return errors.New("no docker VM")
		}
		err := inVM(s, vm, `sudo docker pull -q `+curlImage+` >/dev/null && sudo docker network create tpnet >/dev/null &&
[ "$(`+docker(curlImage+` -sS -m 20 -o /dev/null -w '%{http_code} %{remote_ip}' https://github.com`)+`)" = "200 198.18.0.1" ] &&
[ "$(`+docker(`--network tpnet `+curlImage+` -sS -m 20 -o /dev/null -w '%{http_code} %{remote_ip}' https://github.com`)+`)" = "200 198.18.0.1" ]`)
		if err != nil {
			// What decides a container's names: its resolv.conf on each network, the file Docker
			// copies, and the queries dnsmasq answered.
			diag := vmOut(s, vm, `for n in bridge tpnet; do echo "== $n"; sudo docker run --rm --network $n --entrypoint cat `+curlImage+` /etc/resolv.conf; done
echo "== the VM's"; cat /run/systemd/resolve/resolv.conf; echo "== rules"; sudo nft list table inet devvm_transparent | grep 'dport 53'
echo "== status"; sudo devvm-transparent status`)
			return fmt.Errorf("%w\n%s", err, diag)
		}
		return nil
	})
	s.Check("docker: a blocked host is logged, its 403 shown with -k, a certificate error without", func() error {
		if err := inVM(s, vm, `! `+docker(curlImage+` -sS -m 20 -o /dev/null https://example.net/`)+` &&
`+docker(curlImage+` -sk -m 20 https://example.net/`)+` | grep -q 'devvm network allow example.net'`); err != nil {
			return err
		}
		l, _ := s.Devvm(d, "network", "log", "-n", "200")
		if !lineWith(l.Stdout, "  BLOCKED  example.net:443  ", "without proxy settings") {
			return errors.New("devvm network log doesn't show example.net blocked")
		}
		return nil
	})
	s.Check("docker: a container reaches no other port of the stand-in address, nor its listeners or the local proxy directly, and the bridge keeps route_localnet off", func() error {
		info := vmOut(s, vm, `echo "docker $(sudo docker info --format '{{.ServerVersion}}'), route_localnet $(cat /proc/sys/net/ipv4/conf/docker0/route_localnet)"`)
		s.r.out.line("  e2e: %s", info)
		if !strings.HasSuffix(info, "route_localnet 0") {
			return errors.New("docker0's route_localnet isn't 0: " + info)
		}
		return inVM(s, vm, docker(`--entrypoint sh `+curlImage+` -c '! curl -sS -m 5 -o /dev/null http://198.18.0.1:61443/ && ! curl -sS -m 5 -o /dev/null http://198.18.0.1:22/ &&
! curl -sS -m 5 -o /dev/null http://127.0.77.1:3128/'`))
	})
	s.Check("docker: a container's published 5432 isn't reached through the stand-in address, from the VM or a container; its published 443 doesn't take the VM's", func() error {
		defer func() { _ = inVM(s, vm, `sudo docker rm -f tp5432 tp443 >/dev/null 2>&1; true`) }()
		return inVM(s, vm, `sudo docker run -d --name tp5432 --user 0 -p 5432:5432 --entrypoint sh `+curlImage+` -c 'while :; do echo c5432 | nc -l -p 5432; done' >/dev/null &&
sudo docker run -d --name tp443 --user 0 -p 443:443 --entrypoint sh `+curlImage+` -c 'while :; do echo c443 | nc -l -p 443; done' >/dev/null && sleep 2 &&
[ "$(timeout 5 bash -c 'exec 3<>/dev/tcp/127.0.0.1/5432; head -1 <&3')" = c5432 ] &&
[ "$(timeout 5 bash -c 'exec 3<>/dev/tcp/127.0.0.1/443; head -1 <&3')" = c443 ] &&
! timeout 5 bash -c 'exec 3<>/dev/tcp/db.example/5432' &&
! `+docker(`--network tpnet `+curlImage+` -sS -m 5 -o /dev/null telnet://db.example:5432 </dev/null`)+` &&
[ "$(`+noProxyEnv+`curl -sS -m 20 -o /dev/null -w '%{http_code}' https://github.com)" = 200 ]`)
	})
	s.Check("docker: a container on an --internal network reaches nothing, the stand-in address included", func() error {
		defer func() { _ = inVM(s, vm, `sudo docker network rm tpin tpnet >/dev/null 2>&1; true`) }()
		return inVM(s, vm, `sudo docker network create --internal tpin >/dev/null &&
! `+docker(`--network tpin `+curlImage+` -sS -m 5 -o /dev/null https://github.com`)+` &&
! `+docker(`--network tpin `+curlImage+` -sS -m 5 -o /dev/null http://198.18.0.1:443/`))
	})
	if vm != "" {
		_, _ = s.Devvm(d, "rm", "--force", "--no-export")
	}
}

// fromTransparent moves vm, a --from VM in dir run open with env, to the
// allowlist tier for one start, checks transparent mode there as
// transparentRuns does, and puts it back on open after.
func fromTransparent(s *S, vm, dir, title string, env []string) {
	s.Check(title+": devvm network allowlist and devvm restart give it transparent mode", func() error {
		if _, err := s.Run(Cmd{Args: []string{s.sb.devvm, "network", "allowlist"}, Dir: dir, Env: env}); err != nil {
			return err
		}
		if _, err := s.Run(Cmd{Args: []string{s.sb.devvm, "restart"}, Dir: dir, Env: env, Stdin: "exit\n"}); err != nil {
			return err
		}
		if s.Paths.State(vm)["net.made.check"] != "ok" {
			return errors.New("its self-check didn't pass")
		}
		return nil
	})
	transparentRuns(s, vm, dir, title)
	// Alpine has no python3: busybox's nslookup asks each server itself.
	asked := `if command -v python3 >/dev/null; then
` + dnsAnswers(false, "192.168.5.3", "192.168.5.2", "1.1.1.1") + `
else for h in 192.168.5.3 192.168.5.2 1.1.1.1; do if timeout 8 nslookup example.com "$h" >/dev/null 2>&1; then echo "$h answered"; exit 1; fi; done; fi`
	s.Check(title+": with its service stopped, Lima's DNS and a public server answer nothing", func() error {
		return withoutTransparent(s, vm, func() error {
			// How long a lookup takes to fail with nothing on 127.0.0.1:53, for the record: musl
			// may wait for its timeout.
			s.r.out.line("  e2e: %s: a lookup with transparent mode stopped failed after %s ms", title,
				vmOut(s, vm, `s=$(date +%s%N); curl --noproxy '*' -s -m 20 -o /dev/null http://example.org/; echo $((($(date +%s%N) - s) / 1000000))`))
			return inVM(s, vm, asked)
		})
	})
}

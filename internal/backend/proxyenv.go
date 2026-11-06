package backend

import (
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/devvmhq/devvm/internal/proxy"
)

// ProxyUser is the user in the address of every proxy of devvm's, a VM's
// or a build's, which the proxy checks (proxy.ProxyUser); the password is
// the VM's or the build's own.
const ProxyUser = proxy.ProxyUser

// ProxyURL is a proxy of devvm's at host and port as the guest reaches it,
// with ProxyUser and password.
func ProxyURL(host string, port int, password string) string {
	u := url.URL{Scheme: "http", Host: net.JoinHostPort(host, strconv.Itoa(port)), User: url.UserPassword(ProxyUser, password)}
	return u.String()
}

// NewProxyPassword is a new password for a proxy of devvm's: 32 random
// bytes, as 64 hex digits, which go into ProxyURL as they are.
func NewProxyPassword() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// GuestProxyVars are the variables that name devvm's proxy in a guest
// (ProxyURL), in capitals and lowercase: tools read one or the other.
func GuestProxyVars() []string {
	return []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"}
}

// ClientProxyVars are every proxy variable a backend's own client
// (limactl) reads from its environment and passes on to a VM, in capitals
// and lowercase: devvm runs the client without them or sets them empty, so
// the host's proxy, password and all, stays out of VMs devvm doesn't mean
// to give it.
func ClientProxyVars() []string {
	var out []string
	for _, k := range []string{"HTTP_PROXY", "HTTPS_PROXY", "FTP_PROXY", "NO_PROXY", "ALL_PROXY"} {
		out = append(out, k, strings.ToLower(k))
	}
	return out
}

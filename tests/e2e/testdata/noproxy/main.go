// noproxy fetches a URL as a tool that ignores the proxy settings does: Go's own resolver, which
// reads /etc/resolv.conf itself, no proxy whatever the environment says, and the system's roots. It
// prints the status and the issuer of the server's certificate. e2e builds it for the VM
// (CGO_ENABLED=0) and runs it in a confined VM, where transparent mode sends it to the proxy.
package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: noproxy URL")
		os.Exit(2)
	}
	net.DefaultResolver.PreferGo = true
	c := &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{Proxy: nil}}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, os.Args[1], nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	resp, err := c.Do(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_ = resp.Body.Close()
	issuer := ""
	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		issuer = resp.TLS.PeerCertificates[0].Issuer.String()
	}
	fmt.Println(resp.StatusCode, issuer)
}

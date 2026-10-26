package proxy

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"time"
)

var errStop = errors.New("stop after ClientHello")

// sniOf returns the server name in a TLS ClientHello record ("" if none, or it doesn't parse). It
// lets crypto/tls parse the record and stops the handshake at the ClientHello callback.
func sniOf(rec []byte) string {
	var name string
	c := tls.Server(helloConn{bytes.NewReader(rec)}, &tls.Config{
		GetConfigForClient: func(h *tls.ClientHelloInfo) (*tls.Config, error) {
			name = h.ServerName
			return nil, errStop
		},
	})
	_ = c.HandshakeContext(context.Background()) //nolint:forbidigo // reads a record in memory: nothing to wait for or cancel
	return name
}

// helloConn: a net.Conn that reads the recorded bytes and discards writes.
type helloConn struct{ r io.Reader }

func (c helloConn) Read(b []byte) (int, error)       { return c.r.Read(b) }
func (c helloConn) Write(b []byte) (int, error)      { return len(b), nil }
func (c helloConn) Close() error                     { return nil }
func (c helloConn) LocalAddr() net.Addr              { return &net.TCPAddr{} }
func (c helloConn) RemoteAddr() net.Addr             { return &net.TCPAddr{} }
func (c helloConn) SetDeadline(time.Time) error      { return nil }
func (c helloConn) SetReadDeadline(time.Time) error  { return nil }
func (c helloConn) SetWriteDeadline(time.Time) error { return nil }

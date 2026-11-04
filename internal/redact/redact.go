// Package redact keeps passwords out of logs: a proxy key of devvm's in a proxy URL, and the
// password in any URL's user info, such as a company proxy's in the shell's HTTPS_PROXY. String
// takes them out of a line on its way to devvm's run log; File takes them out of a log another
// program keeps writing (Lima's hostagent's).
package redact

import (
	"bytes"
	"errors"
	"io"
	"os"
	"regexp"
)

// proxyKey is a VM's proxy key (32 hex digits) or the build proxy's password (64) in a proxy
// URL, which lets a process act as that VM or build.
var proxyKey = regexp.MustCompile(`devvm:([0-9a-f]{32,64})@`)

// urlPassword is the password in a URL's user info (scheme://user:password@).
var urlPassword = regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*://[^\s/:@"'` + "`" + `]*):([^\s@/?#]+)@`)

// keyStandIn is what String writes for a proxy key.
const keyStandIn = "<key>"

// String is s with each proxy key written as devvm:<key>@ and each other URL password as
// user:<password>@.
func String(s string) string {
	s = proxyKey.ReplaceAllString(s, "devvm:"+keyStandIn+"@")
	return urlPassword.ReplaceAllStringFunc(s, func(m string) string {
		sub := urlPassword.FindStringSubmatch(m)
		if sub[2] == keyStandIn {
			return m
		}
		return sub[1] + ":<password>@"
	})
}

// fileMax is as much of a log as File reads: a hostagent's log right after its start is far
// smaller.
const fileMax = 64 << 20

// File overwrites each proxy key and URL password in the log at path with as many "*", in place:
// the file keeps its length and every other byte, so a program that is still writing to it at
// its own offset (Lima's hostagent, which opened it without O_APPEND) goes on where it was. A
// missing file has nothing to redact.
func File(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0) //nolint:gosec // a log in devvm's own folders
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	b, err := io.ReadAll(io.LimitReader(f, fileMax))
	if err == nil {
		err = overwrite(f, spans(b))
	}
	return errors.Join(err, f.Close())
}

// spans are the offsets of each secret in b, start and end.
func spans(b []byte) [][2]int {
	var out [][2]int
	for _, re := range []*regexp.Regexp{proxyKey, urlPassword} {
		for _, m := range re.FindAllSubmatchIndex(b, -1) {
			// The secret is the last group: the key, or the password.
			start, end := m[len(m)-2], m[len(m)-1]
			out = append(out, [2]int{start, end})
		}
	}
	return out
}

// overwrite writes "*" over each span of f.
func overwrite(f io.WriterAt, spans [][2]int) error {
	for _, s := range spans {
		if _, err := f.WriteAt(bytes.Repeat([]byte{'*'}, s[1]-s[0]), int64(s[0])); err != nil {
			return err
		}
	}
	return nil
}

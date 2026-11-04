package settings

import (
	"net/url"
	"strings"
	"unicode"

	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
)

// KeyUbuntuMirror is config.toml's Ubuntu mirror, which base's first build
// tries before Ubuntu's own archive.
const KeyUbuntuMirror = "ubuntu-mirror"

// UbuntuMirror is config.toml's ubuntu-mirror, "" when it sets none: the
// address of a mirror of Ubuntu's archive for this machine's architecture,
// which base's first build gets its packages from first. A mirror only
// serves files: apt still checks every package against Ubuntu's archive
// key, so it can't give base a package Ubuntu didn't publish; it can
// serve older lists, which apt accepts.
func UbuntuMirror(d paths.Dirs) (string, error) {
	cf, err := decodeConfig(d.ConfigFile())
	if err != nil {
		return "", err
	}
	return cf.UbuntuMirror.v, nil
}

type ubuntuMirrorKind struct{}

func (ubuntuMirrorKind) parse(v any) (string, error) { return checked(text(v))(checkUbuntuMirror) }

// checkUbuntuMirror refuses a mirror that isn't an http:// or https://
// address with a host, or that holds a user, a query, a fragment, a space
// or a control character: it goes onto a line of the build's own list of
// mirrors, which apt reads.
func checkUbuntuMirror(s string) error {
	u, err := url.Parse(s)
	bad := err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" ||
		strings.ContainsFunc(s, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) })
	if bad {
		return msg.UbuntuMirrorInvalid(KeyUbuntuMirror)
	}
	return nil
}

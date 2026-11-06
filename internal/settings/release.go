package settings

import (
	"github.com/w3cj/devvm/internal/paths"
)

// KeyReleaseCheck is config.toml's switch for the daily check for a newer
// devvm release.
const KeyReleaseCheck = "release-check"

// ReleaseCheck reports whether a launch checks for a newer devvm release
// once a day: unless config.toml's release-check is false, or
// DEVVM_NO_RELEASE_CHECK is set in getenv's environment, as CI sets it.
func ReleaseCheck(d paths.Dirs, getenv func(string) string) (bool, error) {
	if getenv("DEVVM_NO_RELEASE_CHECK") != "" {
		return false, nil
	}
	cf, err := decodeConfig(d.ConfigFile())
	if err != nil {
		return false, err
	}
	return cf.ReleaseCheck.v != "false", nil
}

type switchKind struct{}

func (switchKind) parse(v any) (string, error) {
	b, ok := v.(bool)
	if !ok {
		return "", errText("not true or false")
	}
	if b {
		return "true", nil
	}
	return "false", nil
}

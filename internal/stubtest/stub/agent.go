package main

import (
	"regexp"
	"slices"
	"strings"

	"github.com/devvmhq/devvm/internal/stubtest/stubstate"
)

// sshAgent stands in for ssh-agent. It logs every call to $STUB_LOG, and
// with -a SOCKET "runs" an agent there, with no keys: the state's agent.
func sshAgent(c *call) int {
	c.log()
	if c.arg(0) != "-a" || c.arg(1) == "" {
		return 1
	}
	return c.status(c.update(func(st *stubstate.State) error {
		st.Agent = &stubstate.Agent{Socket: c.arg(1)}
		return nil
	}))
}

// spaces are runs of spaces, which a logged key's line has as one.
var spaces = regexp.MustCompile(` {2,}`)

// sshAdd stands in for ssh-add. It logs every call to $STUB_LOG with the
// agent it was given (SOCK=$SSH_AUTH_SOCK), and only the ssh-agent
// stub's agent runs (exit 2 for any other). A key file's contents stand
// for the key: added, it is logged again as key:<contents>, and held
// (once) as fingerprint SHA256:fp-<contents> with that comment. -l lists
// the keys held, -L their contents as the public keys, and -d FILE
// deletes the key FILE holds.
func sshAdd(c *call) int {
	c.log("SOCK=" + c.agent())
	st, err := c.state()
	if err != nil {
		return c.status(err)
	}
	if st.Agent == nil || c.agent() != st.Agent.Socket {
		return 2
	}
	switch c.arg(0) {
	case "-l", "-L":
		if len(st.Agent.Keys) == 0 {
			c.print("The agent has no identities.\n")
			return 1
		}
		for _, k := range st.Agent.Keys {
			if c.arg(0) == "-L" {
				k = strings.Fields(k)[2]
			}
			c.print(k + "\n")
		}
		return 0
	}
	key, err := readFile(c.last())
	if err != nil {
		return c.status(err)
	}
	content := strings.TrimRight(string(key), "\n")
	words := slices.Concat([]string{"ssh-add"}, c.args[:max(0, len(c.args)-1)], []string{"key:" + content})
	c.logLine(c.getenv("STUB_LOG"), spaces.ReplaceAllString(strings.Join(words, " "), " "))
	return c.status(c.update(func(st *stubstate.State) error {
		if st.Agent == nil {
			return errStatus(2)
		}
		fp := " SHA256:fp-" + content + " "
		st.Agent.Keys = slices.DeleteFunc(st.Agent.Keys, func(k string) bool { return strings.Contains(k, fp) })
		if c.arg(0) != "-d" {
			st.Agent.Keys = append(st.Agent.Keys, "256 SHA256:fp-"+content+" "+content+" (ED25519)")
		}
		return nil
	}))
}

// sshKeygen stands in for ssh-keygen: -lf FILE prints a fingerprint for a
// key file's contents (SHA256:fp-<contents>, as the ssh-add stub holds
// it), and fails for an empty one; anything else fails.
func sshKeygen(c *call) int {
	key, err := readFile(c.arg(1))
	if c.arg(0) != "-lf" || err != nil || len(key) == 0 {
		return 1
	}
	c.print("256 SHA256:fp-" + strings.TrimRight(string(key), "\n") + " c (ED25519)\n")
	return 0
}

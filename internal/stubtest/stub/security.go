package main

import (
	"bufio"
	"regexp"

	"github.com/w3cj/devvm/internal/stubtest/stubstate"
)

// notFound is security's exit status for an item that isn't there
// (errSecItemNotFound).
const notFound = 44

// addRE is a line security -i reads: add-generic-password … -a "NAME" …
// -w "VALUE".
var addRE = regexp.MustCompile(`^add-generic-password .*-a "([^"]*)" .*-w "([^"]*)"$`)

// security stands in for macOS's security, so tests never touch the real
// Keychain: an item is the state's, by its account. It logs every call
// to $STUB_LOG, but not its input, as a value there is in no process's
// arguments. find-generic-password -a NAME … -w prints the item,
// delete-generic-password -a NAME deletes it, each failing as security
// does for an item that isn't there (44); security -i reads
// add-generic-password … -a NAME … -w "VALUE" lines from its input and
// saves them. Anything else fails.
func security(c *call) int {
	c.log()
	account := c.after("-a")
	switch c.arg(0) {
	case "find-generic-password":
		st, err := c.state()
		if err != nil {
			return c.status(err)
		}
		v, ok := st.Keychain[account]
		if !ok || account == "" {
			return notFound
		}
		c.print(v + "\n")
		return 0
	case "delete-generic-password":
		return c.status(c.update(func(st *stubstate.State) error {
			if _, ok := st.Keychain[account]; !ok || account == "" {
				return errStatus(notFound)
			}
			delete(st.Keychain, account)
			return nil
		}))
	case "-i":
		return c.status(c.update(func(st *stubstate.State) error {
			lines := bufio.NewScanner(c.stdin)
			for lines.Scan() {
				m := addRE.FindStringSubmatch(lines.Text())
				if m == nil {
					return errStatus(1)
				}
				st.Keychain[m[1]] = m[2]
			}
			return lines.Err()
		}))
	}
	return 1
}

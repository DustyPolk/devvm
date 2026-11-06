package main

// opencode stands in for this machine's opencode, which devvm agent
// import runs: --version prints the switch opencode-version, and auth
// export prints the switch opencode-export, or fails with the switch
// opencode-export-fails. Without opencode-version it fails as a missing
// command does (127), so devvm reads no version. Anything else fails.
func opencode(c *call) int {
	c.log()
	v, ok := c.text("opencode-version")
	switch {
	case !ok:
		c.say("opencode: command not found")
		return 127
	case c.arg(0) == "--version":
		c.print(v + "\n")
		return 0
	case c.joined() == "auth export" && !c.on("opencode-export-fails"):
		c.print(string(c.raw("opencode-export", "[]")))
		return 0
	}
	return 1
}

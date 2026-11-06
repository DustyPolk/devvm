package main

import (
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/devvmhq/devvm/internal/stubtest/stubstate"
)

// controlUsage is the stub command's usage.
const controlUsage = `usage: stub COMMAND
  names                          every stub's name, one a line
  vm NAME STATUS                 Lima lists NAME in STATUS (a new one with Lima's default size)
  vms                            Lima's instances, "NAME STATUS" a line, sorted
  size NAME CPUS MEMORY DISK     an instance's size, MEMORY and DISK in bytes
  keychain ACCOUNT               the Keychain's item for ACCOUNT; fails without one
  keychain-set ACCOUNT           the item, from the input
  agent                          the SSH agent's keys, as ssh-add -l lists them; fails without an agent
  agent-stop                     the SSH agent stops`

// control is the stub command: what scripts and tests read and change in
// the stubs' state. It logs nothing.
func control(c *call) int {
	n := map[string]int{"names": 1, "vm": 3, "vms": 1, "size": 5, "keychain": 2, "keychain-set": 2, "agent": 1, "agent-stop": 1}
	if want, ok := n[c.arg(0)]; !ok || len(c.args) != want {
		c.say(controlUsage)
		return 2
	}
	switch c.arg(0) {
	case "names":
		names := make([]string, 0, len(stubs()))
		for name := range stubs() {
			names = append(names, name)
		}
		sort.Strings(names)
		c.print(strings.Join(names, "\n") + "\n")
		return 0
	case "vm":
		return c.status(c.update(func(st *stubstate.State) error { setStatus(st, c.arg(1), c.arg(2)); return nil }))
	case "size":
		return c.status(resize(c))
	case "keychain-set":
		v, err := io.ReadAll(c.stdin)
		if err != nil {
			return c.status(err)
		}
		return c.status(c.update(func(st *stubstate.State) error {
			st.Keychain[c.arg(1)] = strings.TrimRight(string(v), "\n")
			return nil
		}))
	case "agent-stop":
		return c.status(c.update(func(st *stubstate.State) error { st.Agent = nil; return nil }))
	}
	return show(c)
}

// show prints what vms, keychain and agent read.
func show(c *call) int {
	st, err := c.state()
	if err != nil {
		return c.status(err)
	}
	switch c.arg(0) {
	case "vms":
		names := make([]string, 0, len(st.Instances))
		for n, in := range st.Instances {
			if in.Status != "" {
				names = append(names, n)
			}
		}
		sort.Strings(names)
		for _, n := range names {
			c.print(n + " " + st.Instances[n].Status + "\n")
		}
	case "keychain":
		v, ok := st.Keychain[c.arg(1)]
		if !ok {
			return 1
		}
		c.print(v + "\n")
	case "agent":
		if st.Agent == nil {
			return 1
		}
		for _, k := range st.Agent.Keys {
			c.print(k + "\n")
		}
	}
	return 0
}

// resize sets a listed instance's size.
func resize(c *call) error {
	cpus, err := strconv.Atoi(c.arg(2))
	if err != nil {
		return err
	}
	mem, err := strconv.ParseInt(c.arg(3), 10, 64)
	if err != nil {
		return err
	}
	disk, err := strconv.ParseInt(c.arg(4), 10, 64)
	if err != nil {
		return err
	}
	return c.update(func(st *stubstate.State) error {
		in := st.Instances[c.arg(1)]
		if in == nil || in.Status == "" {
			c.say("stub: no instance " + c.arg(1))
			return errStatus(1)
		}
		in.CPUs, in.Memory, in.Disk = cpus, mem, disk
		return nil
	})
}

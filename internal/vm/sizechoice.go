package vm

import (
	"errors"
	"io/fs"
	"os"
	"strconv"
	"strings"

	"github.com/w3cj/devvm/internal/backend"
	"github.com/w3cj/devvm/internal/msg"
	"github.com/w3cj/devvm/internal/paths"
	"github.com/w3cj/devvm/internal/state"
)

// SizeChoice chooses the size new VMs get, the size file devvm size -g
// writes: from flags (Set), or asked for (Ask). Nothing rebuilds, and
// existing VMs keep their size.
type SizeChoice struct {
	Dirs   paths.Dirs
	Assets fs.FS // devvm's own files, whose base.yaml sets base's size
	Host   Host
	// Answer asks for a value and returns the line typed, "" for Enter;
	// Say prints the question's context, Warn a refused answer, Info
	// what's saved.
	Answer          func(msg.M) string
	Say, Warn, Info func(msg.M)
}

// Set saves the CPUs, memory and disk s sets as new VMs' size, after
// checking them against the host, and keeps what it leaves unset as
// saved.
func (c SizeChoice) Set(s Size) error {
	s, err := s.Parse()
	if err != nil {
		return err
	}
	if err := s.Check(c.Host, 0, ""); err != nil {
		return err
	}
	old, err := readDefaultSize(c.Dirs)
	if err != nil {
		return err
	}
	def := s.Or(old)
	if err := writeDefaultSize(c.Dirs, def); err != nil {
		return err
	}
	c.Info(msg.SizeDefaults(SizeText(def.Backend())))
	return nil
}

// Ask asks for new VMs' CPUs and memory, each starting from the saved
// size, else base's fitted to the host (Fitted). Enter keeps one; a value
// the host can't give is said, and asked for again. Only what's typed is
// saved, so keeping both writes nothing.
func (c SizeChoice) Ask() error {
	old, err := readDefaultSize(c.Dirs)
	if err != nil {
		return err
	}
	shown := old.Or(c.Fitted())
	c.Say(msg.SizeQuestion(strconv.Itoa(c.Host.CPUs), strconv.Itoa(c.Host.MemoryGiB)))
	var typed Size
	typed.CPUs = c.ask(msg.SizeCpusPrompt(shown.CPUs), func(v string) (Size, error) {
		v, err := ParseCPUs(v)
		return Size{CPUs: v}, err
	})
	typed.Memory = c.ask(msg.SizeMemoryPrompt(shown.Memory), func(v string) (Size, error) {
		v, err := ParseGiB("--memory", v)
		return Size{Memory: v}, err
	})
	if typed != (Size{}) {
		if err := writeDefaultSize(c.Dirs, typed.Or(old)); err != nil {
			return err
		}
	}
	c.Info(msg.SizeDefaults(SizeText(typed.Or(shown).Backend())))
	return nil
}

// ask asks with prompt until the answer is empty, or parse takes it and
// the host can give it; it returns the parsed value, "" for Enter.
func (c SizeChoice) ask(prompt msg.M, parse func(string) (Size, error)) string {
	for {
		v := strings.TrimSpace(c.Answer(prompt))
		if v == "" {
			return ""
		}
		s, err := parse(v)
		if err == nil {
			err = s.Check(c.Host, 0, "")
		}
		if err == nil {
			return s.CPUs + s.Memory
		}
		if m, ok := errors.AsType[msg.M](err); ok {
			c.Warn(m)
		}
	}
}

// Fitted is the CPUs and memory base.yaml gives base, fitted to the host
// (backend.FitCPUs, backend.FitMemory): base.yaml's own when the host
// has room for them.
func (c SizeChoice) Fitted() Size {
	cpus := backend.FitCPUs(int(baseSetting(c.Assets, KeyCPUs)), c.Host.CPUs)
	mem := backend.FitMemory(baseSetting(c.Assets, KeyMemory), c.Host.memoryBytes())
	return Size{CPUs: strconv.Itoa(cpus), Memory: strconv.FormatFloat(mem, 'g', -1, 64)}
}

// writeDefaultSize writes s to d's size file.
func writeDefaultSize(d paths.Dirs, s Size) error {
	return state.WriteSize(d, state.Size{CPUs: s.CPUs, Memory: s.Memory, Disk: s.Disk})
}

// firstRunSize asks for the size new VMs get on devvm's first run, before
// base's features are first chosen: in a terminal, when no size is saved
// and the user gave neither --cpus nor --memory. After it, base's
// features are chosen, so it doesn't ask again.
func (e creation) firstRunSize(given Size) error {
	if given.CPUs != "" || given.Memory != "" || !e.Interactive() {
		return nil
	}
	if _, err := os.Stat(e.Dirs.BaseFeatures()); !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if saved, err := state.ReadSize(e.Dirs); err != nil || saved != (state.Size{}) {
		return err
	}
	host, err := e.Host()
	if err != nil {
		return err
	}
	return SizeChoice{
		Dirs: e.Dirs, Assets: e.Assets, Host: host,
		Answer: e.Answer, Say: e.Say, Warn: e.Warn, Info: e.Info,
	}.Ask()
}

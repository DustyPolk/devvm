//go:build e2e

package e2e

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// gcSection: devvm gc deletes a stale image and an unused tool image,
// keeping base and a VM named like an image; devvm space lists them and
// changes nothing; gc --orphans forgets a VM limactl deleted.
func gcSection(s *S) {
	s.Section("gc")
	// A folder named img-*: its VM is named like an image (devvm-img-keep-<hash>), and isn't one.
	var vm, kg string
	g, err := s.NewProject("img-keep")
	if err == nil {
		vm, _, _ = s.Launch(g, "exit", "--network", "open")
	}
	if k, ok := strings.CutPrefix(s.Paths.State(vm)["chain"], "base="); ok && IsHex(k) {
		kg = k
	}
	gfix, _ := s.Layer("e2egc.sh", "#!/bin/bash\necho gc\n")
	s.Check("a fixture layer, edited after its build, is stale", func() error {
		if _, err := s.Devvm("", "build", "e2egc"); err != nil {
			return err
		}
		if err := AppendFile(gfix, "# edited\n"); err != nil {
			return err
		}
		r, err := s.Devvm("", "images")
		if err != nil {
			return err
		}
		if !Matches(r.Stdout, `^e2egc .* stale `) {
			return errors.New("devvm images doesn't show e2egc stale")
		}
		return nil
	})
	s.Own("e2egc")
	var ko string
	if keys := s.Paths.KeysOf("e2egc"); len(keys) > 0 {
		ko = keys[0]
	}
	// An old tool image no VM lists: unused only once there's a tools lock.
	old := filepath.Join(s.Paths.Share, "node-0.0.1-000000000000.erofs")
	lock := Exists(s.Paths.ToolLock)
	if lock {
		_ = os.WriteFile(old, nil, 0o600)
		_ = os.WriteFile(old+".json", []byte(`{"sha256": "x", "libc": "gnu", "backend": "lima"}`+"\n"), 0o600)
	}
	s.Check("gc deletes the stale image and its record; base and the img-* project VM stay", func() error {
		if vm == "" || kg == "" || ko == "" {
			return errNo
		}
		if _, err := s.Devvm("", "gc", "--force"); err != nil {
			return err
		}
		switch {
		case s.Status("devvm-img-"+ko) != "" || Exists(filepath.Join(s.Paths.Images, ko)):
			return errors.New("the stale image or its record is still there")
		case s.Status("devvm-img-"+kg) != "Stopped" || !Exists(filepath.Join(s.Paths.Images, kg, "record.json")):
			return errors.New("base or its record is gone")
		case s.Status(vm) != "Running":
			return errors.New(vm + " isn't running")
		}
		return nil
	})
	if lock {
		s.Check("gc deletes a tool image no VM uses, with its record", func() error {
			if Exists(old) || Exists(old+".json") {
				return errors.New("the old tool image or its record is still there")
			}
			return nil
		})
	} else {
		s.Skip("gc deletes a tool image no VM uses, with its record", "no tools lock yet")
	}
	s.Check("space lists the Lima VM and images, and changes nothing", func() error {
		r, err := s.Devvm("", "space")
		if err != nil {
			return err
		}
		if !Matches(r.Stdout, `^VMs +[0-9]+ `) || !Matches(r.Stdout, `^images +[0-9]+ `) {
			return errors.New("devvm space lists no VMs or images")
		}
		if s.Status(vm) != "Running" || !Exists(filepath.Join(s.Paths.VMDir(vm), "vm.json")) {
			return errors.New(vm + " changed")
		}
		return nil
	})
	// The img-* VM deleted outside devvm, its state an hour old: gc --orphans forgets what devvm kept.
	_, _ = s.Limactl("delete", "-f", vm)
	then := time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local)
	_ = os.Chtimes(filepath.Join(s.Paths.VMDir(vm), "vm.json"), then, then)
	s.Check("gc --orphans forgets a VM limactl deleted, and nothing of base's", func() error {
		if vm == "" || kg == "" {
			return errNo
		}
		r, err := s.Devvm("", "gc", "--orphans", "--force")
		if err != nil {
			return err
		}
		switch {
		case !strings.Contains(r.Out, s.Msg("forgot_orphan", "vm="+vm)):
			return errors.New("gc didn't say it forgot " + vm)
		case Exists(s.Paths.VMDir(vm)):
			return errors.New(vm + "'s folder is still there")
		case s.Status("devvm-img-"+kg) != "Stopped" || !Exists(filepath.Join(s.Paths.Images, kg, "record.json")):
			return errors.New("base or its record is gone")
		}
		return nil
	})
	_ = os.Remove(gfix)
	_ = os.Remove(old)
	_ = os.Remove(old + ".json")
	_, _ = s.Devvm(g, "rm", "--force", "--no-export")
}

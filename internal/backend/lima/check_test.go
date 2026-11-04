package lima

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// onlyOnPath puts a new folder alone on PATH, with programs that only
// exit.
func onlyOnPath(t *testing.T, progs ...string) {
	t.Helper()
	dir := t.TempDir()
	for _, p := range progs {
		if err := os.WriteFile(filepath.Join(dir, p), []byte("#!/bin/sh\n"), 0o700); err != nil { //nolint:gosec // a test's own programs
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
}

func TestProblems(t *testing.T) {
	const (
		v2      = "limactl version 2.2.1\n"
		apt     = "sudo apt-get install -y --no-install-recommends qemu-utils ipxe-qemu"
		aptArm  = apt + " qemu-system-arm qemu-efi-aarch64"
		aptX86  = apt + " qemu-system-x86 ovmf"
		usermod = "sudo usermod -aG kvm alice"
	)
	for _, tc := range []struct {
		name, os, arch string
		path           []string // the programs on PATH
		version        string   // limactl --version's output; fail for a failure
		macos          string
		kvm            string // the device: rw (the default), ro or missing
		globalConfig   bool
		want           []string // key|fix|message names
	}{
		{name: "macOS, all there", path: []string{"limactl"}, version: v2},
		{name: "Windows", os: "windows", want: []string{"os||lima_host_unsupported os=windows"}},
		{name: "riscv64", os: "linux", arch: "riscv64", want: []string{"arch||lima_arch_unsupported arch=riscv64"}},
		{name: "macOS 12", path: []string{"limactl"}, version: v2, macos: "12.7.6", want: []string{"macos||macos_old version=12.7.6"}},
		{name: "macOS unknown", path: []string{"limactl"}, version: v2, macos: "-"},
		{name: "no Lima", want: []string{"lima||lima_missing"}},
		{
			name: "no Lima, Homebrew", path: []string{"brew"},
			want: []string{"lima|brew install lima|lima_missing_fix fix=brew install lima"},
		},
		{
			name: "Lima 1", path: []string{"limactl"}, version: "limactl version 1.2.0\n",
			want: []string{"lima||lima_old version=1.2.0"},
		},
		{
			name: "Lima 1, Homebrew", path: []string{"limactl", "brew"}, version: "limactl version 1.2.0\n",
			want: []string{"lima|brew upgrade lima|lima_old_fix fix=brew upgrade lima version=1.2.0"},
		},
		{name: "limactl doesn't run", path: []string{"limactl"}, version: "fail", want: []string{"lima||lima_broken"}},
		{
			name: "limactl doesn't run, Homebrew", path: []string{"limactl", "brew"}, version: "fail",
			want: []string{"lima|brew reinstall lima|lima_broken_fix fix=brew reinstall lima"},
		},
		{name: "limactl prints nothing", path: []string{"limactl"}, want: []string{"lima||lima_broken"}},
		{
			name: "a Lima global config", path: []string{"limactl"}, version: v2, globalConfig: true,
			want: []string{"lima-config||lima_global_config files=<config>"},
		},
		{name: "Linux, all there", os: "linux", path: []string{"limactl", "qemu-system-aarch64", "qemu-img"}, version: v2},
		{
			name: "Linux arm64, apt", os: "linux", path: []string{"limactl", "apt-get"}, version: v2,
			want: []string{"qemu|" + aptArm + "|qemu_missing_fix fix=" + aptArm + " program=qemu-system-aarch64"},
		},
		{
			name: "Linux x86_64 with the arm64 emulator", os: "linux", arch: "amd64",
			path: []string{"limactl", "apt-get", "qemu-system-aarch64", "qemu-img"}, version: v2,
			want: []string{"qemu|" + aptX86 + "|qemu_missing_fix fix=" + aptX86 + " program=qemu-system-x86_64"},
		},
		{name: "Linux x86_64, all there", os: "linux", arch: "amd64", path: []string{"limactl", "qemu-system-x86_64", "qemu-img"}, version: v2},
		{
			name: "no qemu-img, dnf", os: "linux", path: []string{"limactl", "dnf", "qemu-system-aarch64"}, version: v2,
			want: []string{"qemu|sudo dnf install -y qemu-kvm qemu-img|qemu_missing_fix fix=sudo dnf install -y qemu-kvm qemu-img program=qemu-img"},
		},
		{
			name: "pacman", os: "linux", path: []string{"limactl", "pacman"}, version: v2,
			want: []string{"qemu|sudo pacman -S --noconfirm qemu-base|qemu_missing_fix fix=sudo pacman -S --noconfirm qemu-base program=qemu-system-aarch64"},
		},
		{
			name: "zypper", os: "linux", path: []string{"limactl", "zypper"}, version: v2,
			want: []string{"qemu|sudo zypper --non-interactive install qemu-kvm|qemu_missing_fix fix=sudo zypper --non-interactive install qemu-kvm program=qemu-system-aarch64"},
		},
		{
			name: "no /dev/kvm", os: "linux", path: []string{"limactl", "qemu-system-aarch64", "qemu-img"}, version: v2, kvm: "missing",
			want: []string{"kvm||no_kvm"},
		},
		{
			name: "no access to /dev/kvm", os: "linux", path: []string{"limactl", "qemu-system-aarch64", "qemu-img"}, version: v2, kvm: "ro",
			want: []string{"kvm-access|" + usermod + "|kvm_access fix=" + usermod},
		},
		{
			name: "Linux, nothing there", os: "linux", kvm: "missing", globalConfig: true,
			want: []string{
				"lima||lima_missing", "qemu||qemu_missing program=qemu-system-aarch64", "kvm||no_kvm",
				"lima-config||lima_global_config files=<config>",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.kvm == "ro" && os.Geteuid() == 0 {
				t.Skip("root opens any device")
			}
			l, f, _ := testLima(t, tc.os)
			if tc.os == "" {
				l.OS = "darwin"
			}
			if tc.arch != "" {
				l.Arch = tc.arch
			}
			if tc.macos != "" {
				v := tc.macos
				if v == "-" {
					v = ""
				}
				l.MacOS = func() string { return v }
			}
			l.Getenv = func(k string) string {
				if k == "USER" {
					return "alice"
				}
				return ""
			}
			switch tc.kvm {
			case "ro":
				if err := os.Chmod(l.KVM, 0o400); err != nil {
					t.Fatal(err)
				}
			case "missing":
				l.KVM = filepath.Join(t.TempDir(), "kvm")
			}
			config := filepath.Join(l.Dirs.Lima, "_config", "default.yaml")
			if tc.globalConfig {
				if err := os.MkdirAll(filepath.Dir(config), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(config, nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if tc.version == "fail" {
				f.fail = []string{"limactl --version"}
			} else {
				f.out["limactl --version"] = tc.version
			}
			onlyOnPath(t, tc.path...)

			var got []string
			for _, p := range l.Problems(t.Context()) {
				got = append(got, p.Key+"|"+p.Fix+"|"+p.Msg.Names())
			}
			want := slices.Clone(tc.want)
			for i, w := range want {
				if w == "lima-config||lima_global_config files=<config>" {
					want[i] = "lima-config||lima_global_config files=" + config
				}
			}
			if !slices.Equal(got, want) {
				t.Errorf("Problems:\n%q\nwant\n%q", got, want)
			}
		})
	}
}

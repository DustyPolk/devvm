package lima

import (
	"archive/tar"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/w3cj/devvm/internal/msg"
)

// writeTar writes a tarball of the files, name and contents, in order.
func writeTar(t *testing.T, path string, files ...[2]string) {
	t.Helper()
	fh, err := os.Create(path) //nolint:gosec // the test's own folder
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(fh)
	for _, f := range files {
		if err := tw.WriteHeader(&tar.Header{Name: f[0], Mode: 0o600, Size: int64(len(f[1]))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(f[1])); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := fh.Close(); err != nil {
		t.Fatal(err)
	}
}

// Docker's own image store names the config, containerd's the manifest.
func TestExportIs(t *testing.T) {
	dir := t.TempDir()
	h, m := strings.Repeat("a", 64), strings.Repeat("b", 64)
	both := filepath.Join(dir, "i.tar")
	writeTar(t, both, [2]string{"blobs/sha256/" + h, "{}"}, [2]string{"manifest.json", `[{"Config":"blobs/sha256/` + h + `"}]`},
		[2]string{"index.json", `{"manifests":[{"digest":"sha256:` + m + `"}]}`})
	old := filepath.Join(dir, "old.tar")
	writeTar(t, old, [2]string{"manifest.json", `[{"Config":"` + h + `.json"}]`})
	for _, c := range []struct {
		file, id string
		want     bool
	}{
		{both, "sha256:" + h, true},
		{both, "sha256:" + m, true},
		{both, "sha256:" + strings.Repeat("c", 64), false},
		{both, "sha256:x", false},
		{both, h, false},
		{old, "sha256:" + h, true},
		{old, "sha256:" + m, false},
		{filepath.Join(dir, "none.tar"), "sha256:" + h, false},
	} {
		if got := exportIs(c.file, c.id); got != c.want {
			t.Errorf("exportIs(%s, %s) = %v", filepath.Base(c.file), c.id, got)
		}
	}
}

// writeConverted writes what a good conversion writes into dir.
func writeConverted(t *testing.T, dir string) {
	t.Helper()
	disk := make([]byte, 2048)
	disk[1080], disk[1081] = 0x53, 0xef
	for name, data := range map[string]string{
		"disk.raw": string(disk), "kernel": "k", "initrd": "i",
		"cmdline": "root=/dev/vda rw console=hvc0\n", "libc": "musl\n",
	} {
		writeTestFile(t, filepath.Join(dir, name), data)
	}
}

func TestCheckConverted(t *testing.T) {
	t.Setenv("DEVVM_MESSAGES", "names")
	dir := t.TempDir()
	writeConverted(t, dir)
	if libc, cmdline, err := checkConverted(dir, "node:24"); err != nil || libc != "musl" || cmdline != "root=/dev/vda rw console=hvc0" {
		t.Fatalf("checkConverted = %q, %q, %v", libc, cmdline, err)
	}
	secret := filepath.Join(t.TempDir(), "secret")
	writeTestFile(t, secret, "hostile")
	for _, c := range []struct {
		name, file string
		bad        func(dir string)
	}{
		{"a symlink to a host file", "disk.raw", func(d string) {
			_ = os.Remove(filepath.Join(d, "disk.raw"))
			_ = os.Symlink(secret, filepath.Join(d, "disk.raw"))
		}},
		{"a folder", "kernel", func(d string) {
			_ = os.Remove(filepath.Join(d, "kernel"))
			_ = os.Mkdir(filepath.Join(d, "kernel"), 0o700)
		}},
		{"a missing file", "initrd", func(d string) { _ = os.Remove(filepath.Join(d, "initrd")) }},
		{"an unknown libc", "libc", func(d string) { writeTestFile(t, filepath.Join(d, "libc"), "gnu\nx\n") }},
		{"a command line that could change Lima's config", "cmdline", func(d string) {
			writeTestFile(t, filepath.Join(d, "cmdline"), "root=/dev/vda rw console=hvc0\"\ncopyToHost:\n- guest: /etc/hostname\n")
		}},
		{"an empty command line", "cmdline", func(d string) { writeTestFile(t, filepath.Join(d, "cmdline"), "") }},
		{"a qcow2 disk", "disk.raw", func(d string) { writeTestFile(t, filepath.Join(d, "disk.raw"), "QFI\xfb") }},
	} {
		d := t.TempDir()
		writeConverted(t, d)
		c.bad(d)
		_, _, err := checkConverted(d, "node:24")
		if want := "convert_bad_output file=" + c.file + " from=node:24"; err == nil || err.Error() != want {
			t.Errorf("%s: %v, want %s", c.name, err, want)
		}
	}
	if data, _ := os.ReadFile(secret); string(data) != "hostile" { //nolint:gosec // the test's own file
		t.Error("the host file changed")
	}
}

// Conversions take turns: one waits while another devvm holds the lock,
// saying so, and takes it once that one lets go; a lock file nothing
// holds, as a devvm that crashed leaves it, is taken at once.
func TestLockOCI(t *testing.T) {
	l, _, _ := testLima(t, "darwin")
	l.OCINote = 10 * time.Millisecond
	var infos msg.Recorder
	l.Info = infos.Add

	writeTestFile(t, l.Dirs.OCIBuildLock(), "")
	unlock, err := l.lockOCI(t.Context())
	if err != nil || len(infos.IDs()) != 0 {
		t.Fatalf("lockOCI on a file nothing holds: %v, infos %q", err, infos.IDs())
	}
	go func() {
		time.Sleep(200 * time.Millisecond)
		unlock()
	}()
	again, err := l.lockOCI(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(infos.IDs(), []string{"oci_waiting"}) {
		t.Errorf("infos: %q", infos.IDs())
	}
	again()
}

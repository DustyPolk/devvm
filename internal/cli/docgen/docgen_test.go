package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/spf13/cobra"

	"github.com/w3cj/devvm/internal/cli"
)

// checkedIn is the reference in the repo, from this package's folder.
const checkedIn = "../../../docs/content/docs/cli"

// differences lists the files that differ between dirs a and b, or are
// in only one of them.
func differences(t *testing.T, a, b string) []string {
	t.Helper()
	read := func(dir string) map[string][]byte {
		files := map[string][]byte{}
		names, err := filepath.Glob(filepath.Join(dir, "*.md"))
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range names {
			data, err := os.ReadFile(n) //nolint:gosec // the test's own folders
			if err != nil {
				t.Fatal(err)
			}
			files[filepath.Base(n)] = data
		}
		return files
	}
	fa, fb := read(a), read(b)
	var diff []string
	for n, data := range fa {
		if other, ok := fb[n]; !ok || !bytes.Equal(data, other) {
			diff = append(diff, n)
		}
	}
	for n := range fb {
		if _, ok := fa[n]; !ok {
			diff = append(diff, n)
		}
	}
	slices.Sort(diff)
	return diff
}

// generated writes root's reference into a temporary folder.
func generated(t *testing.T, root *cobra.Command) string {
	t.Helper()
	dir := t.TempDir()
	if err := write(root, dir); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestReferenceIsCurrent(t *testing.T) {
	if diff := differences(t, generated(t, cli.NewRoot()), checkedIn); diff != nil {
		t.Fatalf("docs/content/docs/cli is stale (%v): run mise run cli-docs", diff)
	}
}

func TestStaleReferenceIsCaught(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(root *cobra.Command)
		want []string
	}{
		{"a flag's description", func(root *cobra.Command) {
			build := find(t, root, "build")
			build.InitDefaultHelpFlag()
			build.Flags().Lookup("help").Usage = "edited"
		}, []string{"devvm_build.md"}},
		{"a command's summary", func(root *cobra.Command) {
			find(t, root, "rebuild").Short = "Edited"
		}, []string{"_index.md", "devvm.md", "devvm_rebuild.md"}},
		{"a new command", func(root *cobra.Command) {
			root.AddCommand(&cobra.Command{Use: "zap", Short: "Zap", Run: func(*cobra.Command, []string) {}})
		}, []string{"_index.md", "devvm.md", "devvm_zap.md"}},
		{"a command gone", func(root *cobra.Command) {
			root.RemoveCommand(find(t, root, "update")) // the last: no other page's place changes
		}, []string{"_index.md", "devvm.md", "devvm_update.md"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := cli.NewRoot()
			tc.edit(root)
			if got := differences(t, generated(t, root), checkedIn); !slices.Equal(got, tc.want) {
				t.Errorf("differences = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestHiddenCommandsAreLeftOut(t *testing.T) {
	files, err := generate(cli.NewRoot())
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for n := range files {
		names = append(names, n)
	}
	slices.Sort(names)
	want := []string{
		"_index.md", "devvm.md", "devvm_adopt.md", "devvm_build.md", "devvm_completion.md", "devvm_doctor.md", "devvm_log.md", "devvm_ls.md", "devvm_rebuild.md",
		"devvm_remove.md", "devvm_restart.md", "devvm_rm.md", "devvm_update.md",
	}
	if !slices.Equal(names, want) {
		t.Errorf("files = %v, want %v", names, want)
	}
	for n, data := range files {
		for _, s := range []string{"_msg", "_qemu", "devvm help", "devvm version", "__complete", "Auto generated"} {
			if bytes.Contains(data, []byte(s)) {
				t.Errorf("%s has %q", n, s)
			}
		}
	}
}

func TestWriteReplacesOldPages(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "devvm_gone.md")
	if err := os.WriteFile(old, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := write(cli.NewRoot(), dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("%s is still there (%v)", old, err)
	}
}

func TestTidy(t *testing.T) {
	in := "## devvm x\n\nShort\n\n### Synopsis\n\nLong\n\n```\ndevvm x [flags]\n```\n\n" +
		"### SEE ALSO\n\n* [devvm](devvm.md)\t - Root\n\n"
	want := "Short\n\n## Synopsis\n\nLong\n\n```text\ndevvm x [flags]\n```\n\n" +
		"## See also\n\n- [devvm](devvm.md): Root\n"
	if got := tidy(in); got != want {
		t.Errorf("tidy:\n%s\nwant:\n%s", got, want)
	}
}

// find returns root's subcommand name.
func find(t *testing.T, root *cobra.Command, name string) *cobra.Command {
	t.Helper()
	c, _, err := root.Find([]string{name})
	if err != nil || c.Name() != name {
		t.Fatalf("no %s command (%v)", name, err)
	}
	return c
}

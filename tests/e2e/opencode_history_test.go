//go:build e2e

package e2e

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// The fake API key planted in opencode's database in the VM, which no
// export may carry. It isn't an e2efake secret, which the section's
// later checks find in any file of the VM, as its database's free pages
// keep it once its row is deleted.
const opencodePlanted = "sk-ant-devvm-planted-opencode-key"

// netOpencodeHistory: devvm history export of a VM whose opencode
// database holds its login's placeholders and a key typed into opencode
// (planted with the VM's sqlite3, as opencode's credential row) gives
// opencode's database as its copy without either, never its WAL file,
// and leaves the VM's own database as it was.
func netOpencodeHistory(n *netRun) {
	s := n.s
	db := "~/.local/share/opencode/opencode.db"
	s.Check("devvm history export leaves opencode's logins out of its database, the key typed into opencode in the VM too", func() error {
		plant := "sqlite3 " + db + ` "INSERT OR REPLACE INTO credential (id, integration_id, label, value, active, time_created, time_updated) ` +
			`VALUES ('cred_e2e_planted', 'e2e-provider', 'API key', '{\"type\":\"key\",\"key\":\"` + opencodePlanted + `\"}', 1, 1, 1)"`
		if err := inVM(s, n.vm, plant); err != nil {
			return failed("planting a credential with the VM's sqlite3", err)
		}
		file := filepath.Join(s.sb.work, "opencode-export.tar.gz")
		_ = os.Remove(file)
		// The fake endpoints' company proxy, as the logins before it had:
		// the export's start writes the VM's proxy's upstream from it.
		if _, err := n.dv("", n.tokenEnv(), "history", "export", file); err != nil {
			return failed("devvm history export", err)
		}
		files, err := exportFiles(file)
		if err != nil {
			return err
		}
		got, ok := files[".local/share/opencode/opencode.db"]
		switch {
		case !ok:
			return errors.New("the export has no opencode.db")
		case bytes.Contains(got, []byte(opencodePlanted)), bytes.Contains(got, []byte("devvm-placeholder")):
			return errors.New("the export's opencode.db holds a login")
		}
		for name := range files {
			if strings.HasSuffix(name, ".db-wal") || strings.HasSuffix(name, ".db-shm") || strings.Contains(name, "devvm-clean") {
				return errors.New("the export has " + name)
			}
		}
		if has, err := vmHas(s, n.vm, true, "sqlite3 "+db+" 'SELECT value FROM credential'", opencodePlanted); err != nil || !has {
			return failed("the VM's own database keeps its credential", err)
		}
		return inVM(s, n.vm, "sqlite3 "+db+` "DELETE FROM credential WHERE id = 'cred_e2e_planted'"`)
	})
}

// exportFiles is each regular file of the gzipped tar at path, by name.
func exportFiles(path string) (map[string][]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{}
	tr := tar.NewReader(zr)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return files, nil
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, err
		}
		files[h.Name] = data
	}
}

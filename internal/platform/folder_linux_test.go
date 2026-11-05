package platform

import "testing"

// uniqueFS: the local file systems with stable inode numbers and birth
// times, and none of the rest.
func TestUniqueFS(t *testing.T) {
	for name, tc := range map[string]struct {
		fsType int64
		want   bool
	}{
		"ext4":     {0xef53, true},
		"xfs":      {0x58465342, true},
		"btrfs":    {0x9123683e, true},
		"tmpfs":    {0x01021994, true},
		"zfs":      {0x2fc12fc1, true},
		"f2fs":     {0xf2f52010, true},
		"bcachefs": {0xca451a4e, true},
		"fuse":     {0x65735546, false},
		"nfs":      {0x6969, false},
		"cifs":     {0xff534d42, false},
		"smb2":     {0xfe534d42, false},
		"9p":       {0x01021997, false},
		"overlay":  {0x794c7630, false},
		"vfat":     {0x4d44, false},
		"exfat":    {0x2011bab0, false},
		"virtiofs": {0x6a656a63, false},
	} {
		if got := uniqueFS(tc.fsType); got != tc.want {
			t.Errorf("uniqueFS(%s) = %v, want %v", name, got, tc.want)
		}
	}
}

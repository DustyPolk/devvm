package msg

import "testing"

// The project folder cache's messages put each value in its own field:
// most are said only on a Linux host, where QEMU's 9p mount has a cache
// mode, so this is where a macOS run checks their fields.
func TestMountCacheMessages(t *testing.T) {
	for _, tc := range []struct {
		m    M
		want string
	}{
		{MountCacheInvalid(`"mmap"`), `mount_cache_invalid value="mmap"`},
		{MountCacheUnchosen("mmap", "v"), "mount_cache_unchosen mode=mmap vm=v"},
		{MountCacheNoChoice("v"), "mount_cache_no_choice vm=v"},
		{MountCacheClone("none", "v"), "mount_cache_clone mode=none vm=v"},
		{MountCachePending("none", "v"), "mount_cache_pending mode=none vm=v"},
		{VMMountCache("none", "v"), "vm_mount_cache mode=none vm=v"},
		{MountCacheRemountFailed("mmap", "v", "none"), "mount_cache_remount_failed has=mmap vm=v want=none"},
		{MountCacheUnmounted("v"), "mount_cache_unmounted vm=v"},
		{MountCacheUnchecked("v"), "mount_cache_unchecked vm=v"},
		{ProjectUnmounted("v"), "project_unmounted vm=v"},
		{ProjectUnchecked("v"), "project_unchecked vm=v"},
		{Remounting("v"), "remounting vm=v"},
		{NextStartMountCache("v"), "next_start_mount_cache vm=v"},
		{NoVMMountCache("/p"), "no_vm_mount_cache dir=/p"},
		{MountCacheFailed("x"), "mount_cache_failed reason=x"},
	} {
		if got := tc.m.Names(); got != tc.want {
			t.Errorf("%s, want %s", got, tc.want)
		}
		if tc.m.Text() == "" {
			t.Errorf("%s has no text", tc.want)
		}
	}
}

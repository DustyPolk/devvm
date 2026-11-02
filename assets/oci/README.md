# oci

These scripts turn a container image into a disk that Lima boots directly, for `--from` ([Starting from a container image](../../docs/content/docs/using/from.md)). devvm runs them in a throwaway VM booted from Ubuntu's cloud image, so the conversion doesn't need Docker.

| File | What |
| --- | --- |
| `build.sh` | runs in the build VM as root: crane pulls the image and flattens its layers, `bootify.sh` runs in a chroot of it, and `mke2fs` writes the disk. Writes the kernel, initramfs and disk Lima boots, the kernel command line, and the image's libc (which builds of the shared tools its VMs get). When the build VM trusts your company's CAs, it's given `guest/ca.sh`, which it runs in the chroot so `bootify.sh`'s downloads trust them too, and runs again afterwards so the image keeps none of them |
| `bootify.sh` | in the chroot, for Debian, Ubuntu or Alpine: adds a kernel and initramfs, an init system, cloud-init, sshd and sudo, and pre-creates the Lima guest user, after removing an image's user that has its uid (Ubuntu's `ubuntu`, 1000) |
| `kernel.sh` | on arm64, turns a distro's `vmlinuz` into the uncompressed `Image` Lima's `vz` needs; elsewhere it copies it as is |
| `try.sh OUTDIR NAME` | boots a converted image as a throwaway Lima VM and runs the checks devvm relies on: init system, cloud-init, user, sudo, a writable host mount and port forwarding. macOS only (`vz`). The VM is left running: `limactl delete -f NAME` |

As in every image script, the downloads are pinned at the top of `build.sh` with their version and checksum. `build.sh` checks them with `sha256sum`, because the build VM has no `devvm-fetch`.

The container image's own code runs as root in the build VM, so devvm treats everything it writes as hostile. The build VM gets only a `vm/` subfolder of devvm's temp folder, and `oci/` read-only. devvm checks what the build VM wrote only once the build VM is deleted. It refuses outputs that aren't regular files, because a symlink could point Lima at a host file. It also refuses a libc other than `gnu` or `musl`, and a kernel command line with anything but letters, digits, spaces and `=,./:_-`.

## Gotchas

- Delete `/.dockerenv`, or cloud-init thinks it's in a container.
- `hosts`, `hostname` and `resolv.conf` are only placeholders in Docker images.
- Debian kernels need the virtio drivers listed for the initramfs.
- Container images have empty apt lists, and apt 3.2's first update on them ignores the snapshot (`apt_update` runs a second).
- `bootify.sh` runs in a chroot inside a running VM: apt there would start services, so `build.sh` adds a `policy-rc.d` while it runs.
- The build VM is a fresh cloud image: its first boot may run apt (unattended-upgrades), so `build.sh` uses no apt itself.

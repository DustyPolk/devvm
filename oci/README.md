# oci

Turns a Docker image into a disk Lima boots directly, for `--from` ([Starting from a container image](../docs/content/docs/from.md)). devvm runs it in a throwaway VM booted from Ubuntu's cloud image; no Docker needed.

| File | What |
|---|---|
| `build.sh` | runs in the build VM as root: crane pulls the image and flattens its layers, `bootify.sh` runs in a chroot of it, and `mke2fs` writes the disk. Writes the kernel, initramfs and disk Lima boots, the kernel command line, and the image's libc (which builds of the shared tools its VMs get) |
| `bootify.sh` | in the chroot, for Debian, Ubuntu or Alpine: adds a kernel and initramfs, an init system, cloud-init, sshd and sudo, and pre-creates the Lima guest user |
| `kernel.sh` | on arm64, turns a distro's `vmlinuz` into the uncompressed `Image` Lima's `vz` needs; elsewhere it copies it as is |
| `try.sh OUTDIR NAME` | boots a converted image as a throwaway Lima VM and runs the checks devvm relies on: init system, cloud-init, user, sudo, a writable host mount and port forwarding. macOS only (`vz`). The VM is left running: `limactl delete -f NAME` |

Like every image script, downloads are pinned at the top of `build.sh` (version and checksum), checked with `sha256sum` since the build VM has no `devvm-fetch`.

The image's own code runs as root in the build VM, so devvm treats everything it writes as hostile: the build VM gets only a `vm/` subfolder of devvm's temp folder, devvm writes `lima.yaml` outside it, and it refuses outputs that aren't regular files (a symlink could point Lima at a host file), a libc other than `gnu` or `musl`, or a kernel command line with anything but letters, digits, spaces and `=,./:_-`.

## Gotchas

- Delete `/.dockerenv`, or cloud-init thinks it's in a container.
- `hosts`, `hostname` and `resolv.conf` are only placeholders in Docker images.
- Debian kernels need the virtio drivers listed for the initramfs.
- Container images have empty apt lists, and apt 3.2's first update on them ignores the snapshot (`apt_update` runs a second).
- `bootify.sh` runs in a chroot inside a running VM: apt there would start services, so `build.sh` adds a `policy-rc.d` while it runs.
- The build VM is a fresh cloud image: its first boot may run apt (unattended-upgrades), so `build.sh` uses no apt itself.

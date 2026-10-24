#!/bin/sh
# Make a container image bootable as a Lima VM. Runs as root in a chroot of any Debian/Ubuntu or
# Alpine image (oci/build.sh). Adds what a container lacks: a kernel and initramfs, an init
# system, cloud-init (applies Lima's cidata: SSH key, guest agent, mounts, ports), sshd and sudo.
# Pre-creates the Lima guest user (USER_NAME/USER_UID/USER_HOME in the environment) so later layers
# can install into its home. Writes the kernel command line the image needs to /etc/devvm/cmdline.
set -eu
# shellcheck source=/dev/null
. /etc/os-release
case " $ID ${ID_LIKE:-} " in
  *" ubuntu "*|*" debian "*) family=debian ;;
  *" alpine "*)              family=alpine ;;
  *) echo "bootify: unsupported distro '$ID' (supported: Debian/Ubuntu and Alpine families)" >&2; exit 1 ;;
esac
mkdir -p /etc/devvm /etc/cloud/cloud.cfg.d

if [ "$family" = debian ]; then
  export DEBIAN_FRONTEND=noninteractive
  arch=$(dpkg --print-architecture)
  # On arm64, Virtualization.framework asks the guest to shut down with a GPIO power key, which
  # needs the gpio_keys driver: Ubuntu's virtual and Debian's generic kernels have it, Debian's
  # cloud kernel doesn't (Lima's stop then times out and hard-kills the VM, losing unsynced writes).
  if [ "$ID" = ubuntu ]; then extra="linux-image-virtual netplan.io"
  elif [ "$arch" = arm64 ]; then extra="linux-image-arm64"
  else extra="linux-image-cloud-$arch"; fi
  apt-get update
  # shellcheck disable=SC2086  # $extra is a package list
  apt-get install -y --no-install-recommends initramfs-tools $extra \
    systemd systemd-sysv systemd-timesyncd systemd-resolved libpam-systemd udev dbus kmod \
    cloud-init cloud-guest-utils e2fsprogs iproute2 openssh-server sudo bash \
    ca-certificates curl less locales tzdata lsb-release rsync
  rm -rf /var/lib/apt/lists/*
  # Debian kernels build the virtio drivers as modules, so the initramfs must load them to find
  # the root disk and console (Ubuntu's virtual kernel has them built in; listing them is harmless).
  printf '%s\n' virtio_pci virtio_mmio virtio_blk virtio_net virtio_console ext4 >> /etc/initramfs-tools/modules
  update-initramfs -u -k all
  # cloud-init sets LANG=en_US.UTF-8; Debian's locale-gen only builds locales listed in locale.gen.
  if [ -f /etc/locale.gen ]; then sed -i 's/^# *\(en_US.UTF-8 UTF-8\)/\1/' /etc/locale.gen && locale-gen; fi
  echo gpio_keys > /etc/modules-load.d/devvm.conf
  systemctl enable ssh systemd-networkd systemd-resolved
  # Without netplan (Debian), cloud-init writes Lima's eth0 config with RequiredForOnline=no, so
  # networkd-wait-online has no link to wait for: every boot waited 2 minutes and ended "degraded".
  # Wait for eth0 (Lima's network) itself instead.
  if [ "$ID" != ubuntu ]; then
    mkdir -p /etc/systemd/system/systemd-networkd-wait-online.service.d
    printf '[Service]\nExecStart=\nExecStart=/usr/lib/systemd/systemd-networkd-wait-online --interface=eth0 --timeout=30\n' \
      > /etc/systemd/system/systemd-networkd-wait-online.service.d/devvm.conf
  fi
  # No LLMNR/mDNS: their listeners on 5355/5353 would otherwise be forwarded to the host by Lima.
  mkdir -p /etc/systemd/resolved.conf.d
  printf '[Resolve]\nLLMNR=no\nMulticastDNS=no\n' > /etc/systemd/resolved.conf.d/devvm.conf
  # Keep the image's apt sources: cloud-init would otherwise rewrite them.
  printf 'datasource_list: [ NoCloud, None ]\napt:\n  preserve_sources_list: true\n' > /etc/cloud/cloud.cfg.d/90-devvm.cfg
  echo "root=/dev/vda rw console=hvc0" > /etc/devvm/cmdline
  useradd --create-home --home-dir "$USER_HOME" --uid "$USER_UID" --shell /bin/bash "$USER_NAME"
else
  apk add --no-cache linux-virt mkinitfs alpine-base openrc cloud-init e2fsprogs e2fsprogs-extra \
    openssh sudo bash shadow ca-certificates curl rsync tzdata blkid
  # cloud-init finds Lima's cidata disk by its ISO 9660 label: needs the full blkid (busybox's
  # can't read the label) and the isofs module loaded at boot.
  echo isofs >> /etc/modules
  # Services a booted Alpine needs (a container image has none enabled).
  for s in devfs dmesg mdev hwdrivers; do rc-update add "$s" sysinit; done
  for s in modules sysctl hostname bootmisc syslog networking; do rc-update add "$s" boot; done
  rc-update add sshd default
  setup-cloud-init
  echo 'datasource_list: [ NoCloud, None ]' > /etc/cloud/cloud.cfg.d/90-devvm.cfg
  echo "root=/dev/vda rootfstype=ext4 rw console=hvc0 modules=virtio_blk,ext4" > /etc/devvm/cmdline
  adduser -D -h "$USER_HOME" -u "$USER_UID" -s /bin/bash "$USER_NAME"
  usermod -p '*' "$USER_NAME"   # no password, but not "locked" (sshd refuses locked accounts)
fi

# The root filesystem is the whole disk (no partition table); Lima attaches it as /dev/vda.
echo '/dev/vda / ext4 defaults 0 1' > /etc/fstab
rm -f /etc/ssh/ssh_host_*   # generated per VM by cloud-init
echo "$USER_NAME ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/90-devvm
chmod 440 /etc/sudoers.d/90-devvm

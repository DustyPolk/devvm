#!/bin/sh
# Run as root in the tools build machine after assets/shared-tools/build.sh: smolvm's kernel
# mounts only uncompressed EROFS, so each image build.sh made is unpacked and made again without compression,
# its owners all root as build.sh has them. Unpacked on the machine's own disk: the host folder
# the images are in keeps no modes.
#   plain.sh DIR FILE...
set -eu
dir=$1; shift
tree=/var/tmp/devvm-plain
for file in "$@"; do
  img=$dir/$file
  rm -rf "$tree"
  fsck.erofs --extract="$tree" "$img"
  mkfs.erofs --all-root "$img.plain" "$tree" >/dev/null
  mv "$img.plain" "$img"
  rm -rf "$tree"
done

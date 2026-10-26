#!/bin/sh
# Run as root in a throwaway container of devvm's, with no network: the Docker backend's VMs can't
# mount the shared tools' images (no loop devices), so each is unpacked into Docker's tools volume, a
# folder named as the image is, which assets/guest/tools.sh links where it would mount the image.
# SRC (read-only) holds the images: the share, or a tools build's output folder. +FILE unpacks
# SRC/FILE into STORE/FILE (whole, or not at all); -FILE deletes STORE/FILE. Owners and modes are the
# image's (all root, as build.sh made them).
#   unpack.sh SRC STORE [+FILE | -FILE]...
set -eu
src=$1 store=$2
shift 2
for a in "$@"; do
  f=${a#?}
  case $f in
    ''|.*|*/*) echo "unpack.sh: not a tool image's name: $f" >&2; exit 1 ;;
  esac
  case $a in
    +*)
      rm -rf "${store:?}/$f.tmp"
      fsck.erofs --extract="$store/$f.tmp" "$src/$f" >/dev/null
      rm -rf "${store:?}/$f"
      mv "$store/$f.tmp" "$store/$f"
      ;;
    -*) rm -rf "${store:?}/$f" "${store:?}/$f.tmp" ;;
    *) echo "unpack.sh: $a: + or - first" >&2; exit 1 ;;
  esac
done

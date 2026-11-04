#!/bin/bash
# devvm's prelude, which comes first in each of devvm's own scripts that an image's build runs with
# bash: layers/base.sh and apt.sh, the built-in layers and features, shared-tools/build.sh and
# oci/build.sh. devvm puts it before the script on the script's standard input (oci/build.sh: in the
# conversion's copy), and the image's key hashes the two together. It sets no shell options and runs
# only uname and command -v. Each script that uses it checks that it came first:
#   : "${devvm_arch:?}" "${devvm_libc:?}"
# What it gives the script:
#   $devvm_arch      this machine's architecture as devvm's pins name it: arm64 or amd64
#   $devvm_libc      its C library: musl on Alpine (it has apk), gnu elsewhere
#   devvm_sha TOOL   TOOL's pinned SHA-256 for this machine, from the script's own pins:
#                    TOOL_sha256_<arch>_musl on musl where the script pins one, else TOOL_sha256_<arch>
case $(uname -m) in
  aarch64 | arm64) devvm_arch=arm64 ;;
  x86_64 | amd64) devvm_arch=amd64 ;;
  *) echo "devvm: devvm's downloads have no build for $(uname -m)" >&2; exit 1 ;;
esac
if command -v apk >/dev/null 2>&1; then devvm_libc=musl; else devvm_libc=gnu; fi
devvm_sha() {
  local v=${1}_sha256_$devvm_arch m
  m=${v}_musl
  if [[ $devvm_libc == musl && -n ${!m:-} ]]; then v=$m; fi
  [[ -n ${!v:-} ]] || { echo "devvm: this script pins no $v" >&2; return 1; }
  echo "${!v}"
}

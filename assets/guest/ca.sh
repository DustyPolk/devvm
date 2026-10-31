#!/bin/sh
# The CAs a VM or a build VM trusts besides its system's, run as root:
#   ca.sh DIR   trust DIR's devvm-*.crt files (one certificate each) and no other of devvm's
#   ca.sh       trust none of devvm's: what an image keeps
# guest/sync.sh runs it at each launch (devvm's proxy's CA, devvm-proxy.crt, with network policy,
# and the user's, devvm-extra-N.crt); a build VM before anything in it downloads, then without DIR
# once its scripts are done; oci/build.sh in the chroot of an image it converts, around bootify.sh.
# They go in /usr/local/share/ca-certificates, and with update-ca-certificates into the system's
# bundle and hashed links, which are made again from scratch. Without it (a container image before
# its ca-certificates package) they're added to the bundle file itself, and the bundle as it was
# is kept in $keep and put back once they go. /etc/ssl/devvm.pem holds them all, for tools that
# take one file of extra CAs (Node's NODE_EXTRA_CA_CERTS): empty with DIR and none in it, gone
# without DIR. DEVVM_CA_ROOT, for the unit tests alone, is a folder that stands for /.
set -eu
src=${1:-}
r=${DEVVM_CA_ROOT:-}
# update-ca-certificates is in sbin, which a machine's exec may leave out (smolvm's).
[ -n "$r" ] || PATH=$PATH:/usr/local/sbin:/usr/sbin:/sbin
dir=$r/usr/local/share/ca-certificates
bundle=$r/etc/ssl/certs/ca-certificates.crt
one=$r/etc/ssl/devvm.pem
keep=$r/var/lib/devvm-ca

hooks=$r/etc/ca-certificates/update.d

changed=false removed=""
for f in "$dir"/devvm-*.crt; do
  [ -e "$f" ] || continue
  if [ -z "$src" ] || [ ! -f "$src/${f##*/}" ]; then
    rm -f "$f"
    changed=true removed="$removed ${f##*/}"
  fi
done
if [ -n "$src" ]; then
  for f in "$src"/devvm-*.crt; do
    [ -e "$f" ] || continue
    if ! cmp -s "$f" "$dir/${f##*/}"; then
      mkdir -p "$dir"
      install -m 644 "$f" "$dir/${f##*/}"
      changed=true
    fi
  done
fi
any=false
for f in "$dir"/devvm-*.crt; do [ ! -e "$f" ] || any=true; done

if command -v update-ca-certificates >/dev/null 2>&1; then
  # Made again from scratch whenever devvm's changed, so none that went stays in the bundle.
  # Its warnings (openssl rehash skipping the bundle file) are noise; a failure still stops this.
  if $changed || [ -d "$keep" ]; then
    update-ca-certificates --fresh >/dev/null 2>&1 || update-ca-certificates >/dev/null 2>&1
  fi
  # update-ca-certificates tells its hooks (Java's cacerts, Mono's store) only of the CAs it links,
  # never of a file gone from $dir, so each hook gets the removed ones as Debian's would: a
  # "-/etc/ssl/certs/NAME.pem" line each on stdin. Hooks that rebuild from the bundle read no input.
  # run-parts' names only: no dots.
  if [ -n "$removed" ]; then
    for h in "$hooks"/*; do
      case ${h##*/} in *[!A-Za-z0-9_-]*) continue ;; esac
      [ -f "$h" ] && [ -x "$h" ] || continue
      for n in $removed; do echo "-/etc/ssl/certs/${n%.crt}.pem"; done | "$h" >/dev/null 2>&1 ||
        echo "devvm: $h failed removing devvm's CAs" >&2
    done
  fi
  rm -rf "$keep"   # a bundle kept from before the package came is the package's now
elif $any; then
  # The bundle as it was, kept once: a copy, or a mark that there was none.
  if [ ! -d "$keep" ]; then
    mkdir -p "$keep"
    if [ -f "$bundle" ]; then cp -p "$bundle" "$keep/bundle"; else : > "$keep/none"; fi
  fi
  mkdir -p "${bundle%/*}"
  if [ -f "$keep/bundle" ]; then cat "$keep/bundle" "$dir"/devvm-*.crt > "$bundle.devvm"; else cat "$dir"/devvm-*.crt > "$bundle.devvm"; fi
  chmod 644 "$bundle.devvm"
  mv -f "$bundle.devvm" "$bundle"
elif [ -d "$keep" ]; then
  if [ -f "$keep/bundle" ]; then cp -p "$keep/bundle" "$bundle"; else rm -f "$bundle"; fi
  rm -rf "$keep"
fi

if [ -z "$src" ]; then
  rm -f "$one"
else
  mkdir -p "${one%/*}"
  if $any; then cat "$dir"/devvm-*.crt > "$one.new"; else : > "$one.new"; fi
  chmod 644 "$one.new"
  if cmp -s "$one.new" "$one"; then rm -f "$one.new"; else mv -f "$one.new" "$one"; fi
fi

#!/bin/bash
# apt.sh begin|end, as root: base's packages, around base.sh and its features; apt.sh lists, before
# a layer's script, fetches package lists base keeps none of. What the build gives
# it in /var/cache/devvm/packages/in says how they come:
#   Packages and *.deb  replay: the packages this image's key recorded at its first build, from
#                       devvm's cache on the host, and nothing else (no network)
#   mirrors             record: from Ubuntu's archive, the mirrors listed tried in order, apt
#                       checking their signatures with Ubuntu's archive key; at the end, each
#                       package downloaded goes to out/ with what apt's lists say of it
#   neither             a --from image's own sources, as they are
# Both end with the image's own sources, no package lists and an empty apt cache, so a first build
# and a replay make the same image. A source that doesn't answer exits 75 at once: no retries.
set -euo pipefail
dir=/var/cache/devvm/packages archives=/var/cache/apt/archives
command -v apt-get >/dev/null || exit 0   # Alpine (--from alpine): apk, not pinned
export DEBIAN_FRONTEND=noninteractive
aside=/etc/apt/devvm-sources

begin() {
  printf 'Acquire::Retries "0";\nAcquire::http::Timeout "30";\nAcquire::https::Timeout "30";\n' \
    > /etc/apt/apt.conf.d/50devvm-build
  if [[ -f $dir/in/Packages ]]; then
    # The host checked each file against the SHA-256 recorded for it, and Packages lists those.
    sources_aside
    echo "deb [trusted=yes] file:$dir/in ./" > /etc/apt/sources.list.d/devvm-replay.list
    apt-get update
    return
  elif [[ -f $dir/in/mirrors ]]; then
    printf 'APT::Keep-Downloaded-Packages "true";\nBinary::apt::APT::Keep-Downloaded-Packages "true";\n' \
      > /etc/apt/apt.conf.d/50devvm-keep
    apt-get clean
    # Ubuntu's sources (deb822, as 26.04's are) with each archive's URI the mirrors listed, then
    # that archive. A one-line sources.list, which 26.04's holds only comments in, stays aside.
    sources_aside
    local f
    for f in "$aside"/sources.list.d/*.sources; do
      [[ -e $f ]] || continue
      mirrored "$f" "/etc/apt/sources.list.d/${f##*/}"
    done
  fi
  if ! apt-get update --error-on=any; then
    echo "devvm: no package source answered (above)" >&2
    exit 75
  fi
}

# mirrored FROM TO writes FROM, a deb822 sources file, to TO with each archive's URI the list
# $dir/mirrors-<n> of in/mirrors' mirror, then that archive.
mirrored() {
  local u n
  cp "$1" "$2"
  while read -r u; do
    n=$(( $(find "$dir" -maxdepth 1 -name 'mirrors-*' | wc -l) + 1 ))
    { cat "$dir/in/mirrors"; echo "$u"; } > "$dir/mirrors-$n"
    sed -i.bak "s|^URIs: *$u *\$|URIs: mirror+file:$dir/mirrors-$n|" "$2" && rm -f "$2.bak"
  done < <(sed -n 's/^URIs: *\([^ ]*\) *$/\1/p' "$1" | sort -u)
}

# sources_aside moves the image's sources out of apt's way, kept to put back.
sources_aside() {
  mkdir -p "$aside/sources.list.d"
  [[ ! -f /etc/apt/sources.list ]] || mv /etc/apt/sources.list "$aside/"
  find /etc/apt/sources.list.d -mindepth 1 -maxdepth 1 -exec mv {} "$aside/sources.list.d/" \;
}

# record moves each package the build downloaded to out/<sha256>.deb, and its entry in apt's lists,
# which apt checked against the archive's signature, to out/Packages. A package apt's lists don't
# have with that SHA-256 fails the build.
record() {
  local deb pkg ver arch sum entry
  mkdir -p "$dir/out"
  : > "$dir/out/Packages"
  for deb in "$archives"/*.deb; do
    [[ -e $deb ]] || continue
    pkg=$(dpkg-deb -f "$deb" Package) ver=$(dpkg-deb -f "$deb" Version) arch=$(dpkg-deb -f "$deb" Architecture)
    sum=$(sha256sum "$deb" | cut -d' ' -f1)
    # By name and version alone ("all" isn't an architecture apt names packages by): each
    # architecture's entry has its own SHA-256, which picks the one.
    entry=$(apt-cache show "$pkg=$ver" 2>/dev/null |
      awk -v s="$sum" 'BEGIN { RS = ""; ORS = "\n\n" } $0 ~ "(^|\n)SHA256: " s "(\n|$)" { print; exit }')
    if [[ -z $entry ]]; then
      echo "devvm: apt's lists have no $pkg $ver ($arch) with the SHA-256 of $deb" >&2
      exit 1
    fi
    printf '%s\n\n' "$entry" >> "$dir/out/Packages"
    mv "$deb" "$dir/out/$sum.deb"
  done
}

end() {
  [[ ! -f $dir/in/mirrors ]] || record
  if [[ -d $aside ]]; then
    rm -rf /etc/apt/sources.list.d
    mkdir -p /etc/apt/sources.list.d
    [[ ! -f $aside/sources.list ]] || mv "$aside/sources.list" /etc/apt/
    find "$aside/sources.list.d" -mindepth 1 -maxdepth 1 -exec mv {} /etc/apt/sources.list.d/ \;
    rm -rf "$aside"
  fi
  rm -f /etc/apt/apt.conf.d/50devvm-build /etc/apt/apt.conf.d/50devvm-keep
  apt-get clean
  rm -rf /var/lib/apt/lists/*
}

# lists fetches the package lists from the image's own sources when it has none, as base's image
# doesn't: a layer's script then installs as on any Ubuntu, from the archive as it is.
lists() {
  if ! compgen -G '/var/lib/apt/lists/*_Packages' >/dev/null && ! apt-get update --error-on=any; then
    echo "devvm: no package source answered (above)" >&2
    exit 75
  fi
}

case ${1:-} in
  begin) begin ;;
  end) end ;;
  lists) lists ;;
  *) echo "usage: apt.sh begin|end|lists" >&2; exit 2 ;;
esac

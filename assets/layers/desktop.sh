#!/bin/bash
# desktop: a built-in layer (devvm --layers desktop). An Xfce desktop with a VNC server, which devvm
# desktop opens (about 400 MB).
set -euxo pipefail
# No downloads of its own: its packages come from the distribution's archive as it is when the layer
# builds, which base's image has no package lists of (layers/apt.sh), so they aren't pinned. Alpine
# (--from alpine): Alpine's own packages, from the release's repositories.
if command -v apk >/dev/null; then
  sudo apk add --no-cache xfce4 xfce4-terminal tigervnc dbus-x11 xauth font-dejavu
else
  sudo apt-get update --error-on=any
  sudo DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    xfce4-session xfwm4 xfce4-panel xfdesktop4 xfce4-settings xfce4-terminal thunar \
    tigervnc-standalone-server tigervnc-tools dbus-x11 xauth fonts-dejavu-core
fi

# devvm-desktop start on|off, which devvm desktop runs, as the VM's user: the desktop, started
# unless it runs. Xvnc on display :1 listens on /run/devvm-desktop/vnc.sock alone, a socket in a
# folder of the user's own (0700), the socket 0600, and on no TCP port (-rfbport -1, -nolisten
# tcp). It asks for no password: devvm checks the viewer's on the host, before anything reaches
# the socket, and its relay is the VM's only way in. The clipboard is on or off as asked, changed
# in place (vncconfig) on a desktop that runs; devvm's relay keeps the viewer's clipboard from the
# VM when it's off whatever the server says. Then an Xfce session on it, in a D-Bus session of
# its own. A run that started nothing prints
# nothing; its own log is ~/.cache/devvm-desktop.log.
sudo install -m 755 /dev/stdin /usr/local/bin/devvm-desktop <<'S'
#!/bin/sh
set -eu
usage() { echo "usage: devvm-desktop start on|off" >&2; exit 2; }
[ "${1:-}" = start ] || usage
case ${2:-} in on) clip=1 ;; off) clip=0 ;; *) usage ;; esac
dir=/run/devvm-desktop
sock=$dir/vnc.sock pidf=$dir/xvnc.pid log=$HOME/.cache/devvm-desktop.log
export DISPLAY=:1 XAUTHORITY="$HOME/.Xauthority"
sudo install -d -m 700 -o "$(id -u)" -g "$(id -g)" "$dir"
pid=$(cat "$pidf" 2>/dev/null || true)
if [ -n "$pid" ] && [ "$(cat "/proc/$pid/comm" 2>/dev/null)" = Xvnc ] && [ -S "$sock" ]; then
  for p in AcceptCutText SendCutText SetPrimary SendPrimary; do vncconfig -set "$p=$clip"; done
  exit 0
fi
mkdir -p "$HOME/.cache"
rm -f "$sock"
umask 077
touch "$XAUTHORITY"
xauth -q remove "$DISPLAY" 2>/dev/null || true
xauth -q add "$DISPLAY" . "$(od -An -N16 -tx1 /dev/urandom | tr -d ' \n')"
setsid Xvnc "$DISPLAY" -rfbport -1 -rfbunixpath "$sock" -rfbunixmode 0600 -SecurityTypes None \
  -nolisten tcp -auth "$XAUTHORITY" -geometry 1600x1000 -depth 24 -AlwaysShared \
  -AcceptCutText="$clip" -SendCutText="$clip" -SetPrimary="$clip" -SendPrimary="$clip" \
  </dev/null >>"$log" 2>&1 &
echo "$!" > "$pidf"
i=0
until [ -S "$sock" ]; do
  i=$((i + 1))
  [ "$i" -le 150 ] || { echo "devvm-desktop: Xvnc didn't start: $log" >&2; exit 1; }
  sleep 0.1
done
setsid dbus-launch --exit-with-session startxfce4 </dev/null >>"$log" 2>&1 &
S

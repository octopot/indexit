#!/bin/sh
# Explicit administrator-only networking for the unprivileged Homebrew service.
# Does not touch Telegram credentials, start indexit, or change the main PF rules.
set -eu
PATH=/usr/bin:/bin:/usr/sbin:/sbin
export PATH
[ "$(uname -s)" = Darwin ] || { echo 'macOS only' >&2; exit 1; }
[ "$(id -u)" = 0 ] || { echo 'Run explicitly with sudo; indexit itself must run as your user.' >&2; exit 1; }
label=org.octolab.indexit.network
plist=/Library/LaunchDaemons/$label.plist
anchor=/etc/pf.anchors/$label
helper=/Library/PrivilegedHelperTools/$label
state=/var/run/$label.token
case "${1:-}" in
install)
  # Reuse Apple's existing wildcard anchor, without flushing unrelated rules.
  grep -q '^rdr-anchor "com.apple/\*"' /etc/pf.conf || { echo 'Custom PF configuration: configure the loopback redirect manually.' >&2; exit 1; }
  if grep -E '^[^#].*[[:space:]]indexit\.local([[:space:]]|$)' /etc/hosts | grep -v '# indexit-local-service$' >/dev/null; then
    echo 'indexit.local already has an unmanaged hosts entry; resolve it manually.' >&2; exit 1
  fi
  if lsof -nP -iTCP:80 -sTCP:LISTEN >/dev/null 2>&1; then
    echo 'Port 80 is already in use; resolve it before configuring indexit.' >&2; exit 1
  fi
  mkdir -p /Library/PrivilegedHelperTools
  cat > "$anchor" <<'RULE'
rdr pass on lo0 inet proto tcp from 127.0.0.0/8 to 127.0.0.1 port 80 -> 127.0.0.1 port 17840
RULE
  cat > "$helper" <<'HELPER'
#!/bin/sh
set -eu
PATH=/usr/bin:/bin:/usr/sbin:/sbin
export PATH
# PF enable references are per boot; /var/run is intentionally ephemeral.
state=/var/run/org.octolab.indexit.network.token
/sbin/pfctl -a com.apple/indexit -f /etc/pf.anchors/org.octolab.indexit.network
if [ ! -s "$state" ]; then
  output=$(/sbin/pfctl -E 2>&1)
  token=$(printf '%s\n' "$output" | awk '/Token/ { print $NF }')
  case "$token" in ''|*[!0-9]*) echo "$output" >&2; exit 1;; esac
  umask 077
  printf '%s\n' "$token" > "$state"
fi
HELPER
  chmod 755 "$helper"
  chown root:wheel "$helper" "$anchor"
  chmod 644 "$anchor"
  cat > "$plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>$label</string>
<key>ProgramArguments</key><array><string>$helper</string></array>
<key>RunAtLoad</key><true/>
<key>StandardOutPath</key><string>/var/log/org.octolab.indexit.network.log</string>
<key>StandardErrorPath</key><string>/var/log/org.octolab.indexit.network.log</string>
</dict></plist>
PLIST
  chown root:wheel "$plist"
  chmod 644 "$plist"
  if ! grep -q '# indexit-local-service$' /etc/hosts; then
    printf '\n127.0.0.1 indexit.local # indexit-local-service\n' >> /etc/hosts
  fi
  launchctl bootout "system/$label" 2>/dev/null || true
  # Configure synchronously so errors are visible before reporting success.
  # RunAtLoad then sees the existing token and does not acquire a second one.
  "$helper"
  pfctl -a com.apple/indexit -s nat | grep -q 17840 || { echo 'PF redirect verification failed' >&2; exit 1; }
  launchctl bootstrap system "$plist"
  dscacheutil -flushcache
  echo 'Networking configured. Start indexit with brew services as your normal user.'
  ;;
uninstall)
  launchctl bootout "system/$label" 2>/dev/null || true
  pfctl -a com.apple/indexit -F all
  if [ -s "$state" ]; then pfctl -X "$(cat "$state")"; fi
  sed -i '' '/# indexit-local-service$/d' /etc/hosts
  rm -f "$plist" "$helper" "$anchor" "$state"
  dscacheutil -flushcache
  echo 'indexit networking removed. Photographs and configuration preserved.'
  ;;
*) echo "Usage: sudo sh $0 install|uninstall" >&2; exit 2;;
esac

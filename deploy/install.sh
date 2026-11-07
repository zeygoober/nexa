#!/usr/bin/env bash
#
# Installs Evilion Chat as a macOS LaunchDaemon and wires it into the existing
# Cloudflare tunnel. Safe to run repeatedly; every step checks before changing.
#
#   sudo bash deploy/install.sh            install, configure, start
#   bash deploy/install.sh --check         print what it would do, change nothing
#   sudo bash deploy/install.sh --no-start set up but leave the daemon stopped
#
set -euo pipefail

CHECK=0; START=1
for a in "$@"; do case "$a" in --check) CHECK=1;; --no-start) START=0;; *) echo "unknown option $a" >&2; exit 2;; esac; done

ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
LABEL="xyz.evilion.chat"
TUNNEL_CFG="$ROOT/../server/tunnel/config.yml"

OS="$(uname -s)"
if [ "$OS" = Darwin ]; then
  UNIT="/Library/LaunchDaemons/$LABEL.plist"
  # the parent stack that runs the tunnel, restarted so cloudflared reloads
  # the ingress below
  SITE_LABEL="xyz.evilion.evilionxyz"
else
  UNIT="/etc/systemd/system/$LABEL.service"
  SITE_LABEL="evilionxyz"
fi

if [ -t 1 ]; then G=$'\033[32m'; Y=$'\033[33m'; R=$'\033[31m'; N=$'\033[0m'; else G=; Y=; R=; N=; fi
ok()   { echo "${G}ok${N}    $*"; }
todo() { echo "${Y}todo${N}  $*"; }
die()  { echo "${R}error${N} $*" >&2; exit 1; }
doit() { if [ "$CHECK" = 1 ]; then echo "      would: $*"; else "$@"; fi; }

# --- settings from chatd.env (defaults match config.go) ---------------------
env_get() { local k="$1" d="$2"; local v; v="$(grep -E "^${k}=" "$ROOT/chatd.env" 2>/dev/null | head -1 | cut -d= -f2- | sed -e 's/^"//' -e 's/"$//' -e "s/^'//" -e "s/'$//")"; echo "${v:-$d}"; }
LISTEN="$(env_get LISTEN 127.0.0.1:8090)"
PUBLIC_URL="$(env_get PUBLIC_URL https://nexa.pm)"
HOSTNAME_="${PUBLIC_URL#https://}"; HOSTNAME_="${HOSTNAME_#http://}"; HOSTNAME_="${HOSTNAME_%%/*}"

# --- who runs it ------------------------------------------------------------
# stat spells "owner" differently per OS, and GNU stat accepts -f with a
# completely different meaning (filesystem status) instead of failing, so
# this branches rather than trying one spelling and falling back.
if [ "$OS" = Darwin ]; then
  OWNER="$(stat -f %Su "$ROOT")"
else
  OWNER="$(stat -c %U "$ROOT")"
fi
GROUP="$(id -gn "$OWNER")"
if [ "$OS" = Darwin ]; then
  OWNER_HOME="$(dscl . -read "/Users/$OWNER" NFSHomeDirectory 2>/dev/null | awk '{print $2}')"
  [ -n "$OWNER_HOME" ] || OWNER_HOME="/Users/$OWNER"
else
  OWNER_HOME="$(getent passwd "$OWNER" 2>/dev/null | cut -d: -f6)"
  [ -n "$OWNER_HOME" ] || OWNER_HOME="/home/$OWNER"
fi
BASH_BIN="$(command -v bash)"
DAEMON_PATH="/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"

echo "Evilion Chat installer — root: $ROOT, user: $OWNER, listen: $LISTEN, host: $HOSTNAME_"

# 1. config file
if [ -f "$ROOT/chatd.env" ]; then ok "chatd.env present"; else
  todo "creating chatd.env from chatd.env.example (edit it if you want other settings)"
  doit cp "$ROOT/chatd.env.example" "$ROOT/chatd.env"; doit chmod 600 "$ROOT/chatd.env"; doit chown "$OWNER:$GROUP" "$ROOT/chatd.env"
fi

# 2. directories
for d in data logs; do
  if [ -d "$ROOT/$d" ]; then ok "$d/ present"; else todo "creating $d/"; doit mkdir -p "$ROOT/$d"; doit chown "$OWNER:$GROUP" "$ROOT/$d"; fi
done
doit chmod 700 "$ROOT/data" 2>/dev/null || true

# 3. binary
if [ -x "$ROOT/bin/chatd" ]; then ok "bin/chatd built"; else die "bin/chatd is missing — run 'make build' first"; fi
[ -f "$ROOT/web/dist/index.html" ] && ok "web/dist built" || todo "web/dist missing — run 'make web' (the server starts but serves a 503 page until then)"

# 4. service definition (needs root)
render_launchd() { sed -e "s|@ROOT@|$ROOT|g" -e "s|@USER@|$OWNER|g" -e "s|@GROUP@|$GROUP|g" -e "s|@HOME@|$OWNER_HOME|g" -e "s|@BASH@|$BASH_BIN|g" -e "s|@PATH@|$DAEMON_PATH|g" "$ROOT/deploy/$LABEL.plist.in"; }
render_systemd() { sed -e "s|@ROOT@|$ROOT|g" -e "s|@USER@|$OWNER|g" -e "s|@GROUP@|$GROUP|g" "$ROOT/deploy/$LABEL.service.in"; }
if [ "$CHECK" = 0 ] && [ "$(id -u)" != 0 ]; then
  if [ "$OS" = Darwin ]; then die "installing the LaunchDaemon needs root: sudo bash deploy/install.sh";
  else die "installing the systemd unit needs root: sudo bash deploy/install.sh"; fi
fi
if [ "$OS" = Darwin ]; then
  if [ -f "$UNIT" ] && diff -q <(render_launchd) "$UNIT" >/dev/null 2>&1; then ok "$UNIT up to date"; else
    todo "writing $UNIT"
    if [ "$CHECK" = 0 ]; then render_launchd > "$UNIT"; chown root:wheel "$UNIT"; chmod 644 "$UNIT"; fi
  fi
else
  if [ -f "$UNIT" ] && diff -q <(render_systemd) "$UNIT" >/dev/null 2>&1; then ok "$UNIT up to date"; else
    todo "writing $UNIT"
    if [ "$CHECK" = 0 ]; then render_systemd > "$UNIT"; chmod 644 "$UNIT"; fi
  fi
fi

# 4b. WARP as a local proxy for what people can make chatd fetch (link
# previews, the media proxy, the YouTube player); the rest of the machine's
# traffic goes out as usual. An older setup kept WARP up as a tunnel for the
# whole machine with a unit of its own; chatd keeps the proxy up now.
if [ "$OS" != Darwin ] && [ -f /etc/systemd/system/warp-autoconnect.service ]; then
  todo "retiring warp-autoconnect.service (WARP no longer tunnels the whole machine)"
  doit systemctl disable --now warp-autoconnect.service
  doit rm -f /etc/systemd/system/warp-autoconnect.service
  doit systemctl daemon-reload
fi
if [ "$CHECK" = 1 ]; then bash "$ROOT/deploy/warp-proxy.sh" --check; else bash "$ROOT/deploy/warp-proxy.sh"; fi

# 5. start / restart
wait_healthy() {
  for i in $(seq 1 30); do
    if curl -sf "http://$LISTEN/api/health" >/dev/null 2>&1; then ok "chatd answers on http://$LISTEN/api/health"; return 0; fi
    sleep 1
    [ "$i" = 30 ] && die "chatd did not come up — see $ROOT/logs/chatd.log"
  done
}
if [ "$START" = 1 ]; then
  if [ "$OS" = Darwin ]; then
    if launchctl print "system/$LABEL" >/dev/null 2>&1; then
      todo "restarting $LABEL"
      doit launchctl bootout "system/$LABEL" || true
      if [ "$CHECK" = 0 ]; then
        # bootout returns before the old copy is gone; bootstrapping into
        # that window fails with "service already loaded"
        for i in $(seq 1 60); do
          launchctl print "system/$LABEL" >/dev/null 2>&1 || break
          sleep 1
        done
      fi
    else
      todo "starting $LABEL"
    fi
    if [ "$CHECK" = 0 ]; then
      launchctl bootstrap system "$UNIT"
      wait_healthy
    fi
  else
    todo "restarting $LABEL"
    if [ "$CHECK" = 0 ]; then
      systemctl daemon-reload
      systemctl enable "$LABEL" >/dev/null 2>&1
      systemctl restart "$LABEL"
      wait_healthy
    fi
  fi
else
  ok "not starting (--no-start)"
fi

# 6. tunnel ingress
if [ -f "$TUNNEL_CFG" ]; then
  if grep -q "hostname: $HOSTNAME_" "$TUNNEL_CFG"; then ok "tunnel ingress for $HOSTNAME_ present"; else
    todo "adding ingress $HOSTNAME_ -> http://$LISTEN to $TUNNEL_CFG"
    if [ "$CHECK" = 0 ]; then
      python3 - "$TUNNEL_CFG" "$HOSTNAME_" "http://$LISTEN" <<'PY'
import sys, re
path, host, svc = sys.argv[1:4]
lines = open(path).read().split("\n")
out, done = [], False
for line in lines:
    if not done and re.match(r"\s*-\s*service:\s*http_status:404", line):
        indent = re.match(r"(\s*)", line).group(1)
        comments = []  # keep the catch-all's own comment block attached to it
        while out and out[-1].strip().startswith("#"):
            comments.insert(0, out.pop())
        out += [f"{indent}- hostname: {host}", f"{indent}  service: {svc}", ""] + comments
        done = True
    out.append(line)
if not done:
    sys.exit("catch-all rule not found in tunnel config")
open(path, "w").write("\n".join(out))
PY
      if [ "$OS" = Darwin ]; then
        if launchctl print "system/$SITE_LABEL" >/dev/null 2>&1; then
          todo "restarting $SITE_LABEL so cloudflared reloads its ingress"
          launchctl kickstart -k "system/$SITE_LABEL"
        else
          todo "$SITE_LABEL is not installed on this box — restart cloudflared yourself"
        fi
      else
        if systemctl cat "$SITE_LABEL.service" >/dev/null 2>&1; then
          todo "restarting $SITE_LABEL so cloudflared reloads its ingress"
          systemctl restart "$SITE_LABEL.service"
        else
          todo "$SITE_LABEL is not installed on this box — restart cloudflared yourself"
        fi
      fi
    fi
  fi
else
  todo "no tunnel config at $TUNNEL_CFG — add an ingress rule '$HOSTNAME_ -> http://$LISTEN' to your cloudflared config"
fi

# 7. DNS
TUNNEL_ID="$(grep -E '^tunnel:' "$TUNNEL_CFG" 2>/dev/null | awk '{print $2}')"
if dig +short "$HOSTNAME_" 2>/dev/null | grep -q .; then
  ok "DNS: $HOSTNAME_ resolves"
  if [ "$CHECK" = 0 ] && [ "$START" = 1 ]; then
    if curl -sf -m 15 "$PUBLIC_URL/api/health" >/dev/null 2>&1; then ok "$PUBLIC_URL/api/health answers through the tunnel"; else todo "$PUBLIC_URL does not answer yet (tunnel restarting or DNS propagating) — retry in a minute"; fi
  fi
else
  todo "DNS: no record for $HOSTNAME_. Either of these creates it:
        a) cloudflared tunnel login   (once, opens a browser)   then   bash deploy/route-dns.sh
        b) Cloudflare dashboard -> DNS -> add CNAME  $(echo "$HOSTNAME_" | cut -d. -f1)  ->  ${TUNNEL_ID:-<tunnel-uuid>}.cfargotunnel.com  (proxied / orange cloud)"
fi
echo "done."

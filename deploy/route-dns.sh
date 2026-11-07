#!/usr/bin/env bash
# Creates the CNAME for the chat hostname on the existing tunnel. Needs the
# account certificate from `cloudflared tunnel login` (~/.cloudflared/cert.pem).
set -euo pipefail
ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
CFG="$ROOT/../server/tunnel/config.yml"
HOST="${1:-$(grep -E '^PUBLIC_URL=' "$ROOT/chatd.env" 2>/dev/null | cut -d= -f2- | sed -E 's#^https?://##; s#/.*##')}"
HOST="${HOST:-nexa.pm}"
TUNNEL="$(grep -E '^tunnel:' "$CFG" | awk '{print $2}')"
[ -n "$TUNNEL" ] || { echo "no tunnel id in $CFG" >&2; exit 1; }
if [ ! -f "$HOME/.cloudflared/cert.pem" ]; then
  echo "No ~/.cloudflared/cert.pem. Run:  cloudflared tunnel login   (select your domain), then re-run this script." >&2
  exit 1
fi
echo "routing $HOST -> tunnel $TUNNEL"
cloudflared tunnel route dns "$TUNNEL" "$HOST"
echo "done — DNS usually resolves within a minute:  dig +short $HOST"

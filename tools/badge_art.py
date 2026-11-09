#!/usr/bin/env python3
"""Writes Nexa's profile badge art into web/src/assets/badges/.

Every badge is a Lucide glyph on nothing, taken from Android's lucide.json so
both apps draw the same lines, and coloured with a short gradient. The same
file shows on every theme, so each colour stays between relative luminance
0.20 and 0.27: at least 3:1 on Dark's badge pill and on Light's. The web imports the files; Android
copies them from the web (CopyWebAssets) and takes newer ones over the air
(data/Ota.kt). Run it again after changing anything here; the output is
committed.

    python3 tools/badge_art.py
"""

import json
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
OUT = ROOT / "web/src/assets/badges"
_LUCIDE = json.loads((ROOT / "android/app/src/main/assets/lucide.json").read_text())
LUCIDE = _LUCIDE["icons"]
BODIES = _LUCIDE["bodies"]

# a palette: the top of the icon, the bottom of it
PALETTES = {
    "staff": ("#a770ff", "#9654ff"),
    "supporter": ("#ff4387", "#f40059"),
    "pro": ("#a173ff", "#8f58ff"),
    "og": ("#10a084", "#0e8c73"),
    "years": ("#3592e7", "#1a7dd9"),
    "contributor": ("#a471ff", "#9356ff"),
    "dev": ("#4f8aff", "#2d73ff"),
    "bronze": ("#d87328", "#bc6422"),
    "silver": ("#7c90a8", "#667e9a"),
    "gold": ("#b88500", "#a07400"),
    "emerald": ("#0da267", "#0c8d5a"),
    "boost": ("#e24fd0", "#d43bbf"),
}

TIERS = ["bronze", "silver", "gold", "emerald", "prism"]

# a blacked out badge's line, heavier than the art's so it reads as one clean shape
LOCKED_STROKE = 2.8

# shapes left unfilled when blacked out, by glyph: filled, a face is a plain disc, a robot a pot and a mic a leaf
LOCKED_HOLLOW = {"face-slightly-smiling": {3}, "bot": {1}, "mic": {1}}

SPARKLE = "M0-2.6Q.35-.35 2.6 0Q.35.35 0 2.6Q-.35.35-2.6 0Q-.35-.35 0-2.6Z"


def badge(name, palette, glyph, *, sparkles=0, stroke=2.3, locked=False):
    gid = name.replace("-", "")
    if locked:
        ink = ""
    elif palette == "prism":
        ink = f"""<linearGradient id="{gid}p" x1="3" y1="2" x2="21" y2="22" gradientUnits="userSpaceOnUse">
      <stop offset="0" stop-color="#ff1585"/><stop offset=".5" stop-color="#9767ff"/><stop offset="1" stop-color="#0f8ec7"/>
    </linearGradient>"""
    else:
        top, bottom = PALETTES[palette]
        ink = f"""<linearGradient id="{gid}p" x1="12" y1="2" x2="12" y2="22" gradientUnits="userSpaceOnUse">
      <stop offset="0" stop-color="{top}"/><stop offset="1" stop-color="{bottom}"/>
    </linearGradient>"""
    # shapes get a tint inside, so a thin icon still has body at 20 px: closed outlines, and the open ones
    # android/tools/lucide_assets.py found to enclose something (any other open line would fill across its chord)
    shapes = BODIES.get(glyph, [])
    hollow = LOCKED_HOLLOW.get(glyph, set()) if locked else set()
    tints = "".join(f'<path d="{d}"/>' for i, d in enumerate(LUCIDE[glyph]) if (d.strip()[-1] in "zZ" or i in shapes) and i not in hollow)
    lines = "".join(f'<path d="{d}"/>' for d in LUCIDE[glyph])
    # a higher tier's sparkles sit in the corners, the icon drawn a little smaller to clear them
    scale = 0.86 if sparkles else 1
    off = 12 - 12 * scale
    place = f'transform="translate({off:.2f} {off + (0.8 if sparkles else 0):.2f}) scale({scale})"'
    twinkles = ""
    for x, y, s in [(20.9, 3.1, 1.0), (3.1, 3.6, 0.72)][:sparkles]:
        at = f'transform="translate({x} {y}) scale({s})" d="{SPARKLE}"'
        if locked:
            twinkles += f'<path {at} fill="#000"/>'
        else:
            twinkles += f'<path {at} fill="url(#{gid}p)"/>'
    if locked:
        # the Badge Index's blacked out badge: all black, inside too, in a heavier line so hairline gaps close up
        return f"""<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="24" height="24">
  <g {place} fill="#000">{tints}</g>
  <g {place} stroke="#000" stroke-width="{LOCKED_STROKE}" stroke-linecap="round" stroke-linejoin="round" fill="none">{lines}</g>
  {twinkles}
</svg>
"""
    return f"""<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="24" height="24">
  <defs>
    {ink}
  </defs>
  <g {place} fill="url(#{gid}p)" fill-opacity=".22">{tints}</g>
  <g {place} stroke="url(#{gid}p)" stroke-width="{stroke}" stroke-linecap="round" stroke-linejoin="round" fill="none">{lines}</g>
  {twinkles}
</svg>
"""


def all_badges():
    yield "staff-owner", "staff", "crown", 0
    yield "staff-developer", "staff", "code-xml", 0
    yield "staff-admin", "staff", "hammer", 0
    yield "staff-mod", "staff", "shield-check", 0
    yield "staff-helper", "staff", "life-buoy", 0
    yield "plan-supporter", "supporter", "heart", 0
    yield "plan-pro", "pro", "gem", 1
    yield "og", "og", "star", 0
    yield "years", "years", "cake", 0
    yield "contributor", "contributor", "wrench", 0
    yield "app-developer", "dev", "bot", 0  # Bot Maker's look before it had levels, for APKs from then
    for i, tier in enumerate(TIERS, 1):
        sparkles = 0 if i < 4 else i - 3
        yield f"bug-{i}", tier, "bug", sparkles
        yield f"ideas-{i}", tier, "lightbulb", sparkles
    for i, tier in enumerate(TIERS[:3], 1):
        yield f"gift-{i}", tier, "gift", 0
        yield f"bot-maker-{i}", tier, "bot", 0
        yield f"friendly-{i}", tier, "face-slightly-smiling", 0
        yield f"host-{i}", tier, "house", 0
        yield f"open-mic-{i}", tier, "mic", 0
    yield "booster", "boost", "rocket", 0
    for i, tier in enumerate(TIERS[:4], 1):
        yield f"regular-{i}", tier, "coffee", 1 if i == 4 else 0


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    names = set()
    for name, palette, glyph, sparkles in all_badges():
        (OUT / f"{name}.svg").write_text(badge(name, palette, glyph, sparkles=sparkles))
        names.add(f"{name}.svg")
        # staff ranks are not in the Badge Index, so they are never blacked out
        if not name.startswith("staff-"):
            (OUT / f"{name}-locked.svg").write_text(badge(name, palette, glyph, sparkles=sparkles, locked=True))
            names.add(f"{name}-locked.svg")
    for old in OUT.glob("*.svg"):
        if old.name not in names:
            old.unlink()
    print(f"wrote {len(names)} badges to {OUT.relative_to(ROOT)}")


if __name__ == "__main__":
    main()

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
LUCIDE = json.loads((ROOT / "android/app/src/main/assets/lucide.json").read_text())["icons"]

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
}

TIERS = ["bronze", "silver", "gold", "emerald", "prism"]

SPARKLE = "M0-2.6Q.35-.35 2.6 0Q.35.35 0 2.6Q-.35.35-2.6 0Q-.35-.35 0-2.6Z"


def badge(name, palette, glyph, *, sparkles=0, stroke=2.3):
    gid = name.replace("-", "")
    if palette == "prism":
        ink = f"""<linearGradient id="{gid}p" x1="3" y1="2" x2="21" y2="22" gradientUnits="userSpaceOnUse">
      <stop offset="0" stop-color="#ff1585"/><stop offset=".5" stop-color="#9767ff"/><stop offset="1" stop-color="#0f8ec7"/>
    </linearGradient>"""
    else:
        top, bottom = PALETTES[palette]
        ink = f"""<linearGradient id="{gid}p" x1="12" y1="2" x2="12" y2="22" gradientUnits="userSpaceOnUse">
      <stop offset="0" stop-color="{top}"/><stop offset="1" stop-color="{bottom}"/>
    </linearGradient>"""
    # closed outlines get a tint inside, so a thin icon still has body at 20 px;
    # an open line would fill across its chord
    tints = "".join(f'<path d="{d}"/>' for d in LUCIDE[glyph] if d.strip()[-1] in "zZ")
    lines = "".join(f'<path d="{d}"/>' for d in LUCIDE[glyph])
    # a higher tier's sparkles sit in the corners, the icon drawn a little smaller to clear them
    scale = 0.86 if sparkles else 1
    off = 12 - 12 * scale
    place = f'transform="translate({off:.2f} {off + (0.8 if sparkles else 0):.2f}) scale({scale})"'
    twinkles = ""
    for x, y, s in [(20.9, 3.1, 1.0), (3.1, 3.6, 0.72)][:sparkles]:
        twinkles += f'<path transform="translate({x} {y}) scale({s})" d="{SPARKLE}" fill="url(#{gid}p)"/>'
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
    yield "staff-owner", badge("staff-owner", "staff", "crown")
    yield "staff-developer", badge("staff-developer", "staff", "code-xml")
    yield "staff-admin", badge("staff-admin", "staff", "hammer")
    yield "staff-mod", badge("staff-mod", "staff", "shield-check")
    yield "staff-helper", badge("staff-helper", "staff", "life-buoy")
    yield "plan-supporter", badge("plan-supporter", "supporter", "heart")
    yield "plan-pro", badge("plan-pro", "pro", "gem", sparkles=1)
    yield "og", badge("og", "og", "star")
    yield "years", badge("years", "years", "cake")
    yield "contributor", badge("contributor", "contributor", "wrench")
    yield "app-developer", badge("app-developer", "dev", "bot")
    for i, tier in enumerate(TIERS, 1):
        sparkles = 0 if i < 4 else i - 3
        yield f"bug-{i}", badge(f"bug-{i}", tier, "bug", sparkles=sparkles)
        yield f"ideas-{i}", badge(f"ideas-{i}", tier, "lightbulb", sparkles=sparkles)
    for i, tier in enumerate(TIERS[:3], 1):
        yield f"gift-{i}", badge(f"gift-{i}", tier, "gift")


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    names = set()
    for name, svg in all_badges():
        (OUT / f"{name}.svg").write_text(svg)
        names.add(f"{name}.svg")
    for old in OUT.glob("*.svg"):
        if old.name not in names:
            old.unlink()
    print(f"wrote {len(names)} badges to {OUT.relative_to(ROOT)}")


if __name__ == "__main__":
    main()

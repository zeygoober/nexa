#!/usr/bin/env python3
"""Writes Nexa's profile badge art into web/src/assets/badges/.

Every badge is a shaped plate (a vertical gradient, a darker rim, a gloss
across the top) with a Lucide glyph on it, taken from Android's lucide.json so
both apps draw the same lines. The web imports the files; Android copies them
from the web (CopyWebAssets) and takes newer ones over the air (data/Ota.kt).
Run it again after changing anything here; the output is committed.

    python3 tools/badge_art.py
"""

import json
import math
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
OUT = ROOT / "web/src/assets/badges"
LUCIDE = json.loads((ROOT / "android/app/src/main/assets/lucide.json").read_text())["icons"]

# a palette: plate top, plate bottom, rim, glyph colour, glyph shadow
PALETTES = {
    "staff": ("#b56bff", "#6a00d6", "#4a0099", "#ffffff", "#2b005c"),
    "supporter": ("#ff8ab4", "#e23a74", "#a8164c", "#ffffff", "#7a0f37"),
    "pro": ("#c58bff", "#5b10e0", "#3d0a9e", "#ffffff", "#25065e"),
    "og": ("#7ae6d0", "#1f9e86", "#126b5a", "#ffffff", "#0b4a3e"),
    "years": ("#a9cdf2", "#4a7fb8", "#2f5a88", "#ffffff", "#1c3a5c"),
    "contributor": ("#d2b6ff", "#8a55e8", "#5f30b5", "#ffffff", "#3c1a7a"),
    "dev": ("#9cc2ff", "#3a74e0", "#2551a8", "#ffffff", "#163470"),
    "bronze": ("#f2b98a", "#b0632c", "#7a3f17", "#fff4ea", "#5a2c0e"),
    "silver": ("#f4f7fb", "#9aa6b4", "#6b7684", "#2f3a46", "#ffffff"),
    "gold": ("#ffe79a", "#e0a419", "#9c6c05", "#5a3b00", "#fff3c4"),
    "emerald": ("#8af5c4", "#13a86c", "#0a6e46", "#ffffff", "#064a2e"),
    "prism": (None, None, "#5a3fb0", "#ffffff", "#2a1a66"),
}

TIERS = ["bronze", "silver", "gold", "emerald", "prism"]


def plate(shape):
    """The plate's outline as a path in a 24 box."""
    if shape == "circle":
        return "M12 1.5a10.5 10.5 0 1 1 0 21a10.5 10.5 0 1 1 0-21z"
    if shape == "square":
        return "M7 1.5h10a5.5 5.5 0 0 1 5.5 5.5v10a5.5 5.5 0 0 1-5.5 5.5H7A5.5 5.5 0 0 1 1.5 17V7A5.5 5.5 0 0 1 7 1.5z"
    if shape in ("hexagon", "octagon"):
        n, r, turn = (6, 11.2, -90) if shape == "hexagon" else (8, 11.4, -90 + 22.5)
        pts = [(12 + r * math.cos(math.radians(turn + i * 360 / n)), 12 + r * math.sin(math.radians(turn + i * 360 / n))) for i in range(n)]
        return rounded(pts, 2.2)
    raise ValueError(shape)


def rounded(pts, rad):
    """A closed polygon with its corners rounded."""
    out = []
    n = len(pts)
    for i in range(n):
        p0, p1, p2 = pts[i - 1], pts[i], pts[(i + 1) % n]
        a = _toward(p1, p0, rad)
        b = _toward(p1, p2, rad)
        out.append(("M" if i == 0 else "L") + f"{a[0]:.2f} {a[1]:.2f}")
        out.append(f"Q{p1[0]:.2f} {p1[1]:.2f} {b[0]:.2f} {b[1]:.2f}")
    return "".join(out) + "Z"


def _toward(p, q, d):
    dx, dy = q[0] - p[0], q[1] - p[1]
    length = math.hypot(dx, dy)
    return (p[0] + dx / length * d, p[1] + dy / length * d)


SPARKLE = "M0-2.6Q.35-.35 2.6 0Q.35.35 0 2.6Q-.35.35-2.6 0Q-.35-.35 0-2.6Z"


def badge(name, shape, palette, glyph, *, scale=0.6, sparkles=0, stroke=2.5):
    top, bottom, rim, ink, shadow = PALETTES[palette]
    gid = name.replace("-", "")
    if palette == "prism":
        fill = f"""<linearGradient id="{gid}p" x1="3" y1="2" x2="21" y2="22" gradientUnits="userSpaceOnUse">
      <stop offset="0" stop-color="#ff8ac4"/><stop offset=".45" stop-color="#9b6bff"/><stop offset="1" stop-color="#3fc8ff"/>
    </linearGradient>"""
    else:
        fill = f"""<linearGradient id="{gid}p" x1="12" y1="1" x2="12" y2="23" gradientUnits="userSpaceOnUse">
      <stop offset="0" stop-color="{top}"/><stop offset="1" stop-color="{bottom}"/>
    </linearGradient>"""
    outline = plate(shape)
    off = 12 - 12 * scale
    paths = "".join(f'<path d="{d}"/>' for d in LUCIDE[glyph])
    lines = f'stroke-width="{stroke}" stroke-linecap="round" stroke-linejoin="round" fill="none"'
    twinkles = ""
    for x, y, s in [(19.2, 5.2, 0.75), (4.6, 18.6, 0.55)][:sparkles]:
        twinkles += f'<path transform="translate({x} {y}) scale({s})" d="{SPARKLE}" fill="#ffffff"/>'
    return f"""<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="24" height="24">
  <defs>
    {fill}
    <linearGradient id="{gid}g" x1="12" y1="1" x2="12" y2="12" gradientUnits="userSpaceOnUse">
      <stop offset="0" stop-color="#ffffff" stop-opacity=".55"/><stop offset="1" stop-color="#ffffff" stop-opacity="0"/>
    </linearGradient>
    <clipPath id="{gid}c"><path d="{outline}"/></clipPath>
  </defs>
  <path d="{outline}" fill="url(#{gid}p)"/>
  <g clip-path="url(#{gid}c)"><ellipse cx="12" cy="3.2" rx="13" ry="8.2" fill="url(#{gid}g)"/></g>
  <path d="{outline}" fill="none" stroke="{rim}" stroke-opacity=".7" stroke-width="1"/>
  <g transform="translate({off:.2f} {off + 0.55:.2f}) scale({scale})" stroke="{shadow}" stroke-opacity=".45" {lines}>{paths}</g>
  <g transform="translate({off:.2f} {off:.2f}) scale({scale})" stroke="{ink}" {lines}>{paths}</g>
  {twinkles}
</svg>
"""


def all_badges():
    yield "staff-owner", badge("staff-owner", "square", "staff", "crown")
    yield "staff-developer", badge("staff-developer", "square", "staff", "code-xml")
    yield "staff-admin", badge("staff-admin", "square", "staff", "hammer")
    yield "staff-mod", badge("staff-mod", "square", "staff", "shield-check")
    yield "staff-helper", badge("staff-helper", "square", "staff", "life-buoy")
    yield "plan-supporter", badge("plan-supporter", "circle", "supporter", "heart")
    yield "plan-pro", badge("plan-pro", "circle", "pro", "gem", sparkles=1)
    yield "og", badge("og", "circle", "og", "star")
    yield "years", badge("years", "circle", "years", "cake")
    yield "contributor", badge("contributor", "square", "contributor", "wrench")
    yield "app-developer", badge("app-developer", "square", "dev", "bot")
    for i, tier in enumerate(TIERS, 1):
        sparkles = 0 if i < 4 else i - 3
        yield f"bug-{i}", badge(f"bug-{i}", "hexagon", tier, "bug", scale=0.58, sparkles=sparkles)
        yield f"ideas-{i}", badge(f"ideas-{i}", "octagon", tier, "lightbulb", scale=0.6, sparkles=sparkles)
    for i, tier in enumerate(TIERS[:3], 1):
        yield f"gift-{i}", badge(f"gift-{i}", "circle", tier, "gift")


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

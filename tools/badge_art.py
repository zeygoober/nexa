#!/usr/bin/env python3
"""Writes Nexa's profile badge art into web/src/assets/badges/.

Every badge is a chunky, glossy icon on nothing, in a 64 unit box: the icon's
shape a little lower in a dark tone for depth, the icon in a gradient, one
curved gloss across its top, and its details in the deep tone. Only gradients,
masks and layered shapes are used, since Android's SVG renderer draws no
filters, and numerals are strokes, since it has no fonts. The same file shows
on every theme, so bodies stay mid-toned. The web imports the files; Android
copies them from the web (CopyWebAssets) and takes newer ones over the air
(data/Ota.kt). Run it again after changing anything here; the output is
committed.

    python3 tools/badge_art.py
"""

import math
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
OUT = ROOT / "web/src/assets/badges"

CX, CY = 32, 31


class M:
    """A material: the icon's body top to bottom, its depth, and its details."""

    def __init__(self, body, depth, cut=None):
        self.body, self.depth = body, depth
        self.cut = cut or depth


MATERIALS = {
    "bronze": M(("#f0a368", "#a9571f"), "#5f2c10"),
    "silver": M(("#d5dee9", "#76879f"), "#465064"),
    "gold": M(("#ffdc62", "#e5920a"), "#8a5200"),
    "emerald": M(("#58e6ab", "#0b9463"), "#06503a"),
    "prism": M(("#ff7cc4", "#ffcf66", "#6eeccb", "#74a8ff", "#b98aff"), "#2a1469"),
    "royal": M(("#c9a6ff", "#6a35d8"), "#2c1460"),
    "crown": M(("#ffdc62", "#e8950a"), "#7a4600", "#b3123f"),
    "steel": M(("#a8e2ee", "#2f8ba0"), "#1d4a58"),
    "boost": M(("#ff9be9", "#c92ebb"), "#6a1269"),
    "sky": M(("#8fd0ff", "#2277d6"), "#123f73"),
    "copper": M(("#ffb98c", "#b4552a"), "#54240f"),
    "jade": M(("#7ef0da", "#139484"), "#0b4f4a"),
    "violet": M(("#c8a8ff", "#6f3fdf"), "#35187a"),
    "rose": M(("#ffa3c9", "#d93a78"), "#7a1846"),
    "ruby": M(("#ff8a9c", "#c4183a"), "#5c0a1e"),
    "obsidian": M(("#a9adc2", "#444759"), "#14151f"),
    "clover": M(("#8ff07c", "#22963a"), "#0f4f1d"),
    "olive": M(("#d9d85c", "#7a8e14"), "#384306"),
    "canvas": M(("#ffd59a", "#cf7a26"), "#62300c"),
    "plum": M(("#e8a6e2", "#8c3088"), "#3f0f3d"),
}

YEAR_MATERIALS = ["copper", "silver", "gold", "jade", "sky", "violet", "rose", "ruby", "obsidian", "prism"]
TIERS = ["bronze", "silver", "gold", "emerald", "prism"]



def n(v):
    return f"{v:.2f}".rstrip("0").rstrip(".")


def _rounded(pts, r):
    """A polygon with its corners rounded by [r]."""
    out = []
    for i, p in enumerate(pts):
        a, b = pts[i - 1], pts[(i + 1) % len(pts)]
        def toward(q):
            d = math.dist(p, q)
            t = min(r / d, 0.5)
            return p[0] + (q[0] - p[0]) * t, p[1] + (q[1] - p[1]) * t
        s, e = toward(a), toward(b)
        out.append(f"{'M' if i == 0 else 'L'}{n(s[0])} {n(s[1])}Q{n(p[0])} {n(p[1])} {n(e[0])} {n(e[1])}")
    return "".join(out) + "Z"


# ---- glyphs, in the 64 box: (kind, data, ...)
#   F filled body with rounded corners; L a line of the body; C a cut detail
#   filled; CL a cut detail as a line; A a body part in its own colour


def dot(x, y, r):
    return f"M{x - r} {y}a{r} {r} 0 1 0 {2 * r} 0a{r} {r} 0 1 0 {-2 * r} 0Z"


def rrect(x0, y0, x1, y1, r):
    return f"M{x0 + r} {y0}H{x1 - r}Q{x1} {y0} {x1} {y0 + r}V{y1 - r}Q{x1} {y1} {x1 - r} {y1}H{x0 + r}Q{x0} {y1} {x0} {y1 - r}V{y0 + r}Q{x0} {y0} {x0 + r} {y0}Z"


def _leaf(angle):
    """One heart-shaped clover leaf, its tip at the clover's middle, turned [angle] degrees from pointing up."""
    pts = [(0, 0), (-6, -4), (-14, -9), (-14, -16), (-14, -21), (-10, -24), (-6.5, -24), (-3.5, -24), (-1, -22), (0, -19),
           (1, -22), (3.5, -24), (6.5, -24), (10, -24), (14, -21), (14, -16), (14, -9), (6, -4), (0, 0)]
    t = math.radians(angle)
    pts = [(x * 0.92, y * 0.92 - 3.2) for x, y in pts]
    q = [(32 + x * math.cos(t) - y * math.sin(t), 30 + x * math.sin(t) + y * math.cos(t)) for x, y in pts]
    out = f"M{n(q[0][0])} {n(q[0][1])}"
    for i in range(1, len(q), 3):
        out += "C" + " ".join(f"{n(x)} {n(y)}" for x, y in q[i:i + 3])
    return out + "Z"


def _laurel():
    """Two branches curving up from the bottom, a pair of leaves at each step."""
    parts = []
    r, cx, cy = 23, 32, 31
    def leaf(bx, by, dx, dy, length, width):
        d = math.hypot(dx, dy)
        dx, dy = dx / d, dy / d
        ex, ey = bx + dx * length, by + dy * length
        mx, my = (bx + ex) / 2, (by + ey) / 2
        px, py = -dy * width, dx * width
        return ("F", f"M{n(bx)} {n(by)}Q{n(mx + px)} {n(my + py)} {n(ex)} {n(ey)}Q{n(mx - px)} {n(my - py)} {n(bx)} {n(by)}Z")
    for side in (-1, 1):
        x = lambda v: cx + side * (v - cx)
        top = math.radians(238)
        parts.append(("L", f"M{n(x(cx - 4))} {n(cy + r)}A{r} {r} 0 0 {0 if side < 0 else 1} {n(x(cx + r * math.cos(top)))} {n(cy + r * math.sin(top))}", 3.4))
        for i, deg in enumerate((112, 140, 168, 196, 224)):
            t = math.radians(deg)
            bx, by = cx + r * math.cos(t), cy + r * math.sin(t)
            tx, ty = -math.sin(t), math.cos(t)  # along the branch, upwards
            ox, oy = math.cos(t), math.sin(t)  # away from the middle
            size = 1 - i * 0.06
            parts.append(leaf(x(bx), by, side * (tx * 0.75 + ox * 0.7), ty * 0.75 + oy * 0.7, 13 * size, 4.4 * size))
            parts.append(leaf(x(bx), by, side * (tx * 0.8 - ox * 0.6), ty * 0.8 - oy * 0.6, 10.5 * size, 3.6 * size))
        # the tip of the branch
        parts.append(leaf(x(cx + r * math.cos(top)), cy + r * math.sin(top), side * -math.sin(top), math.cos(top), 11, 4))
    return parts


# name: (rotation, parts)
GLYPHS = {
    "megaphone": (-10, [("L", "M22 40L26 55", 6.5), ("F", "M7 25H19L47 11V53L19 39H7Z"), ("L", "M54 22C58 27 58 37 54 42", 5), ("CL", "M19 28V36", 3.4)]),
    "palette": (0, [("F", "M32 7C50 7 60 19 58 32C57 40 50 41 45 39C40 37 36 40 38 46C40 52 36 57 30 57C15 57 5 46 5 32C5 18 16 7 32 7Z"),
                    ("A", dot(20, 22, 4.6), "#ff6f91"), ("A", dot(32, 16.5, 4.6), "#ffd166"), ("A", dot(44, 21, 4.6), "#5fd1ff"), ("A", dot(15.5, 35, 4.6), "#7ee08a"),
                    ("C", dot(27, 45, 4.4))]),
    "laurel": (0, _laurel()),
    "lantern": (0, [("L", "M26 11A6 6 0 0 1 38 11", 4.5), ("F", "M23 14H41L46 21H18Z"), ("F", rrect(17, 21, 47, 50, 6)), ("A", rrect(22.5, 26, 41.5, 45, 4), "#ffe7a3"),
                    ("A", "M32 29C36 34 37 37 35 40C34 42 30 42 29 40C27 37 28 34 32 29Z", "#ff9d2e"), ("F", rrect(19, 50, 45, 57, 3)),
                    ("CL", rrect(22.5, 26, 41.5, 45, 4), 2.6)]),
    "goblet": (0, [("F", "M14 7H50C50 25 44 35 32 35C20 35 14 25 14 7Z"), ("L", "M32 35V50", 6.5), ("F", "M17 57C17 51 23 49 32 49S47 51 47 57Z"), ("C", dot(32, 19, 4.8))]),
    "sunrise": (0, [("F", "M13 44A19 19 0 0 1 51 44Z"), ("L", "M5 52H59", 6), ("L", "M32 8V17", 6), ("L", "M10.5 19L17 25.5", 6), ("L", "M53.5 19L47 25.5", 6)]),
    "glass": (0, [("A", dot(26, 26, 12), "#d9fbf3"), ("L", dot(26, 26, 15), 8), ("L", "M38 38L55 55", 10)]),
    "eye": (0, [("F", "M3 32C13 12 51 12 61 32C51 52 13 52 3 32Z"), ("C", dot(32, 32, 11)), ("CL", "M26.5 27.5A7 7 0 0 1 32 24.5", 2.8)]),
    "clover": (0, [("L", "M32 34C34 43 39 51 47 58", 5.5), ("F", _leaf(45)), ("F", _leaf(135)), ("F", _leaf(225)), ("F", _leaf(315)), ("C", dot(32, 30, 3))]),
    "secret": (0, [("L", "M20 31V20A12 12 0 0 1 44 20V31", 8), ("F", rrect(11, 29, 53, 59, 7)), ("C", dot(32, 40, 5.5)), ("CL", "M32 42V50", 5.5)]),
    "crown": (0, [("F", "M15 43L12 21L23 30L32 14L41 30L52 21L49 43Z"), ("L", "M17 50H47", 5.5), ("C", dot(32, 33, 3.6))]),
    "code": (0, [("L", "M23 20L10 32L23 44", 7.5), ("L", "M41 20L54 32L41 44", 7.5), ("L", "M36 15L28 49", 6.5)]),
    "hammer": (-42, [("F", "M12 8H52V30H12Z"), ("L", "M32 32V57", 10), ("CL", "M22 8V30", 3.4), ("CL", "M42 8V30", 3.4)]),
    "check": (0, [("L", "M16 33L28 45L48 20", 10)]),
    "buoy": (0, [("L", dot(32, 32, 15), 12), ("CL", "M23.2 23.2L19 19", 5), ("CL", "M40.8 23.2L45 19", 5), ("CL", "M23.2 40.8L19 45", 5), ("CL", "M40.8 40.8L45 45", 5)]),
    "wrench": (42, [("F", "M25 5V16H39V5A15 15 0 1 1 25 5Z"), ("L", "M32 30V59", 10)]),
    "bot": (0, [("L", "M32 11V20", 4.5), ("F", dot(32, 9, 3.2)), ("L", "M9 30V41", 5.5), ("L", "M55 30V41", 5.5), ("F", rrect(15, 20, 49, 52, 8)), ("C", dot(24.5, 33, 5)), ("C", dot(39.5, 33, 5)), ("CL", "M25 44H39", 4)]),
    "bug": (0, [("L", "M17 29L7 23", 5), ("L", "M47 29L57 23", 5), ("L", "M15 39H5", 5), ("L", "M49 39H59", 5), ("L", "M18 49L9 56", 5), ("L", "M46 49L55 56", 5), ("L", "M26 11L21 4", 4.2), ("L", "M38 11L43 4", 4.2), ("F", dot(32, 16, 9)), ("F", "M32 21C43 21 49 30 49 39S42 58 32 58 15 48 15 39 21 21 32 21Z"), ("CL", "M32 26V55", 3.6), ("C", dot(24, 37, 3.2)), ("C", dot(40, 37, 3.2))]),
    "bulb": (0, [("F", "M32 6A19 19 0 0 1 43.5 40.1C41.4 41.8 41 43.5 41 46H23C23 43.5 22.6 41.8 20.5 40.1A19 19 0 0 1 32 6Z"), ("L", "M24 53H40", 6), ("L", "M28 61H36", 5.5), ("CL", "M24 23L32 31L40 23M32 31V45", 3.8)]),
    "gift": (0, [("L", "M32 18C27 4 12 6 17 15C19 18 26 18 32 18C38 18 45 18 47 15C52 6 37 4 32 18Z", 5), ("F", rrect(10, 20, 54, 32, 3)), ("F", rrect(14, 34, 50, 56, 3)), ("CL", "M32 22V53.5", 8)]),
    "face": (0, [("F", dot(32, 32, 20)), ("C", dot(24.5, 27, 4)), ("C", dot(39.5, 27, 4)), ("CL", "M21 38C25 47 39 47 43 38", 4.6)]),
    "house": (0, [("F", "M32 9L57 32H49V55H15V32H7Z"), ("C", rrect(26, 37, 38, 56, 2))]),
    "mic": (0, [("F", rrect(21, 4, 43, 40, 11)), ("L", "M12 30C12 44 21 50 32 50S52 44 52 30", 5.5), ("L", "M32 50V60", 5.5), ("L", "M22 61H42", 5.5), ("CL", "M27 16H37", 3.2), ("CL", "M27 24H37", 3.2)]),
    "rocket": (45, [("A", "M25 47H39L32 64Z", "#ffb23f"), ("F", "M18 36L6 50L19 48Z"), ("F", "M46 36L58 50L45 48Z"), ("F", "M32 2C46 13 48 32 44 47H20C16 32 18 13 32 2Z"), ("C", dot(32, 24, 7))]),
    "coffee": (0, [("L", "M46 28H50A7.5 7.5 0 0 1 50 43H45", 5.5), ("L", "M22 5C18 9 26 12 22 17", 4.6), ("L", "M35 5C31 9 39 12 35 17", 4.6), ("F", "M11 23H47V40C47 50 40 56 31 56H27C18 56 11 50 11 40Z"), ("CL", "M18 33V42", 3.4)]),
    "cake": (0, [("A", "M32 3C37 9 37 12 32 15C27 12 27 9 32 3Z", "#ffb23f"), ("L", "M32 18V27", 6), ("F", rrect(16, 28, 48, 40, 4)), ("F", rrect(9, 38, 55, 58, 5)), ("CL", "M10 46C15 52 19 52 24 46S32 40 37 46 49 52 54 46", 4)]),
}

DIGITS = {
    "0": "M10 0C3 0 0 7 0 16S3 32 10 32 20 25 20 16 17 0 10 0Z",
    "1": "M3 7L11 0V32",
    "2": "M1 8C1 3 5 0 10 0S19 3 19 8C19 13 15 16 11 21L1 32H20",
    "3": "M2 4C4 1 7 0 10 0C15 0 19 3 19 8S15 16 9 16C15 16 20 19 20 24S15 32 10 32C6 32 3 31 1 28",
    "4": "M15 32V0L0 22H20",
    "5": "M18 0H4L2 14C4 13 7 12 10 12C16 12 20 16 20 22S16 32 10 32C6 32 3 31 1 28",
    "6": "M16 1C8 3 1 10 0 21C0 28 4 32 10 32S20 28 20 22 16 12 10 12 0 16 0 22",
    "7": "M0 0H20L8 32",
    "8": "M10 15C5 15 2 12 2 7.5S5 0 10 0 18 3 18 7.5 15 15 10 15C16 15 20 18 20 23.5S16 32 10 32 0 29 0 23.5 4 15 10 15Z",
    "9": "M4 31C12 29 19 22 20 11C20 4 16 0 10 0S0 4 0 10 4 20 10 20 20 16 20 10",
}


# where each icon sits: its scale and offset, measured from its ink so it fills the box
FIT = {
    "megaphone": (1.062, -1.94, -5.44),
    "palette": (1.036, -0.78, -3.49),
    "laurel": (0.893, 3.43, 1.94),
    "lantern": (0.990, 0.33, -0.67),
    "goblet": (1.036, -1.14, -3.49),
    "secret": (1.023, -0.73, -2.09),
    "sunrise": (1.0, 0, 1),
    "glass": (1.094, -4.65, -5.65),
    "eye": (0.976, 0.82, -0.23),
    "clover": (1.025, -0.8, -2.31),
    "crown": (1.382, -12.24, -13.93),
    "code": (1.121, -3.86, -4.86),
    "hammer": (1.027, 4.75, 2.92),
    "check": (1.314, -10.06, -11.71),
    "buoy": (1.284, -9.10, -10.10),
    "wrench": (1.115, -5.25, -3.69),
    "bot": (1.167, -5.35, -2.74),
    "bug": (1.003, -0.11, 0.09),
    "bulb": (0.976, 0.75, -2.20),
    "gift": (1.109, -3.49, -3.99),
    "face": (1.243, -7.77, -8.77),
    "house": (1.124, -3.96, -4.96),
    "mic": (0.945, 1.77, -0.17),
    "rocket": (1.038, 3.83, -7.23),
    "coffee": (1.055, -4.65, -0.85),
    "cake": (0.993, 0.22, 0.71),
}
DEPTH = 2.6


def _paint(parts, paint):
    """The body's shapes, all in one [paint], or each in its own when [paint] is None."""
    out = ""
    for part in parts:
        kind, d = part[0], part[1]
        if kind == "F" or kind == "A":
            c = paint or (part[2] if kind == "A" else "url(#b)")
            out += f'<path d="{d}" fill="{c}" stroke="{c}" stroke-width="3.4" stroke-linejoin="round"/>'
        elif kind == "L":
            out += f'<path d="{d}" fill="none" stroke="{paint or "url(#b)"}" stroke-width="{part[2]}" stroke-linecap="round" stroke-linejoin="round"/>'
    return out


def _cuts(parts, colour):
    out = ""
    for part in parts:
        kind, d = part[0], part[1]
        if kind == "C":
            out += f'<path d="{d}" fill="{colour}"/>'
        elif kind == "CL":
            out += f'<path d="{d}" fill="none" stroke="{colour}" stroke-width="{part[2]}" stroke-linecap="round" stroke-linejoin="round"/>'
    return out


def placed(name):
    """The icon's transform, and the way back from the badge's coordinates into the icon's."""
    rotate, _ = GLYPHS[name]
    s, x, y = FIT[name]
    def local(px, py):
        qx, qy = (px - x) / s - 32, (py - y) / s - 32
        t = math.radians(-rotate)
        return 32 + qx * math.cos(t) - qy * math.sin(t), 32 + qx * math.sin(t) + qy * math.cos(t)
    return f"translate({n(x)} {n(y)}) scale({n(s)})" + (f" rotate({rotate} 32 32)" if rotate else ""), local


def stops(colours):
    if len(colours) == 2:
        return f'<stop stop-color="{colours[0]}"/><stop offset="1" stop-color="{colours[1]}"/>'
    last = len(colours) - 1
    return "".join(f'<stop offset="{n(i / last)}" stop-color="{c}"/>' for i, c in enumerate(colours))


def grad(gid, colours, a, b):
    return f'<linearGradient id="{gid}" gradientUnits="userSpaceOnUse" x1="{n(a[0])}" y1="{n(a[1])}" x2="{n(b[0])}" y2="{n(b[1])}">{stops(colours)}</linearGradient>'


def svg(defs, body):
    return f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64" width="64" height="64"><defs>{defs}</defs>{body}</svg>\n'


GLOSS = '<linearGradient id="s" gradientUnits="userSpaceOnUse" x1="0" y1="0" x2="0" y2="30"><stop stop-color="#fff" stop-opacity=".42"/><stop offset="1" stop-color="#fff" stop-opacity=".1"/></linearGradient>'


def glossy(mat, shapes, cuts, a, b):
    """[shapes] draws the icon in a paint, [a] and [b] are where its gradient runs."""
    defs = grad("b", mat.body, a, b) + GLOSS + f'<mask id="m" maskUnits="userSpaceOnUse" x="0" y="0" width="64" height="64">{shapes("#fff")}</mask>'
    return svg(defs, (
        f'<g transform="translate(0 {DEPTH})">{shapes(mat.depth)}</g>{shapes(None)}'
        f'<ellipse mask="url(#m)" cx="{CX}" cy="4" rx="46" ry="26" fill="url(#s)"/>{cuts}'
    ))


def glyph_badge(name, material):
    mat = MATERIALS[material]
    parts = GLYPHS[name][1]
    place, local = placed(name)
    prism = len(mat.body) > 2
    a, b = (local(6, 60), local(58, 4)) if prism else (local(CX, 2), local(CX, 60))
    return glossy(mat, lambda paint: f'<g transform="{place}">{_paint(parts, paint)}</g>', f'<g transform="{place}">{_cuts(parts, mat.cut)}</g>', a, b)


def locked_glyph(name):
    """The Badge Index's blacked out badge: the icon flat black, its details cut out, no edge."""
    parts = GLYPHS[name][1]
    place, _ = placed(name)
    defs = f'<mask id="m" maskUnits="userSpaceOnUse" x="0" y="0" width="64" height="64"><rect width="64" height="64" fill="#fff"/><g transform="{place}">{_cuts(parts, "#000")}</g></mask>'
    return svg(defs, f'<g mask="url(#m)"><g transform="{place}">{_paint(parts, "#000")}</g></g>')


def digits_badge(text, material):
    mat = MATERIALS[material]
    # a digit is 20 wide and 32 tall, a gap of 8 between two
    s = 1.5 if len(text) == 1 else 1.08
    width = 20 * len(text) + 8 * (len(text) - 1)
    x0, y = CX - width * s / 2, CY - 16 * s
    w = 7 if len(text) == 1 else 7.6
    def shapes(paint):
        return "".join(f'<path transform="translate({n(x0 + i * 28 * s)} {n(y)}) scale({n(s)})" d="{DIGITS[ch]}" fill="none" stroke="{paint or "url(#b)"}" stroke-width="{w}" stroke-linecap="round" stroke-linejoin="round"/>' for i, ch in enumerate(text))
    def local(py):
        return (10, (py - y) / s)
    if len(mat.body) > 2:
        a, b = (50, local(60)[1]), (-20, local(2)[1])
    else:
        a, b = local(2), local(60)
    return glossy(mat, shapes, "", a, b)


# ---- gems, which are the whole badge


def gem(outline, table, facets, colours, *, streak=""):
    """[colours]: depth, body top, body bottom, table top, table bottom, facet light, facet dark."""
    depth, top, bottom, ttop, tbottom, light, dark = colours
    defs = grad("b", (top, bottom), (0, 2), (0, 60)) + grad("t", (ttop, tbottom), (0, 2), (0, 60))
    shade = "".join(f'<path d="{d}" fill="{light if kind == "l" else dark}" opacity="{".55" if kind == "l" else ".5"}"/>' for kind, d in facets)
    body = (
        f'<path transform="translate(0 {DEPTH})" d="{outline}" fill="{depth}" stroke="{depth}" stroke-width="3" stroke-linejoin="round"/>'
        f'<path d="{outline}" fill="url(#b)" stroke="url(#b)" stroke-width="3" stroke-linejoin="round"/>{shade}'
        f'<path d="{table}" fill="url(#t)"/>{streak}'
    )
    return svg(defs, body)


HEART = "M32 56C14 44 5 34 5 22C5 12 12 6 20 6C25 6 29.5 8.5 32 13C34.5 8.5 39 6 44 6C52 6 59 12 59 22C59 34 50 44 32 56Z"
HEART_TABLE = "M32 41C22.5 34 16.5 28 16.5 22C16.5 18 19.5 16 22.5 16C26.5 16 30 19 32 23C34 19 37.5 16 41.5 16C44.5 16 47.5 18 47.5 22C47.5 28 41.5 34 32 41Z"
BRILLIANT = "M17 8H47L60 25L32 58L4 25Z"
BRILLIANT_TABLE = "M23.5 14H40.5L46 25H18Z"
STAR = _rounded([(CX + (29.5 if i % 2 == 0 else 14) * 1.08 * math.cos(math.radians(-90 + i * 36)), 33 + (29.5 if i % 2 == 0 else 14) * 1.08 * math.sin(math.radians(-90 + i * 36))) for i in range(10)], 3.4)
STAR_TABLE = _rounded([(CX + (17 if i % 2 == 0 else 8) * math.cos(math.radians(-90 + i * 36)), 33 + (17 if i % 2 == 0 else 8) * math.sin(math.radians(-90 + i * 36))) for i in range(10)], 1.6)


def heart_gem():
    streak = '<path d="M13 21C13 16 16 12.5 20.5 12.5" fill="none" stroke="#fff" stroke-opacity=".82" stroke-width="3.6" stroke-linecap="round"/>'
    facets = [("d", "M32 56L5 22C5 34 14 44 32 56Z"), ("d", "M32 56L59 22C59 34 50 44 32 56Z")]
    return gem(HEART, HEART_TABLE, facets, ("#7d0b3c", "#ff6aa6", "#e0145f", "#ffd0e3", "#ff7fb2", "#fff", "#8c0d44"), streak=streak)


def brilliant_gem():
    facets = [
        ("l", "M17 8H23.5L18 25H4Z"), ("d", "M47 8H40.5L46 25H60Z"), ("l", "M23.5 14L17 8H47L40.5 14Z"),
        ("l", "M18 25H32V58Z"), ("d", "M46 25H32V58Z"), ("d", "M4 25H18L32 58Z"),
    ]
    streak = '<path d="M26 17.5H33" stroke="#fff" stroke-opacity=".9" stroke-width="3" stroke-linecap="round"/>'
    return gem(BRILLIANT, BRILLIANT_TABLE, facets, ("#2a1469", "#a57bff", "#5a2fe0", "#f0e6ff", "#b491ff", "#efe4ff", "#2f1a9a"), streak=streak)


def star_gem():
    streak = '<path d="M26 22L30 15" stroke="#fff" stroke-opacity=".85" stroke-width="3" stroke-linecap="round"/>'
    return gem(STAR, STAR_TABLE, [], ("#8a4d00", "#ffe066", "#f29a0c", "#fff8cf", "#ffd23f", "#fff", "#b86b00"), streak=streak)


def locked_gem(outline):
    return svg("", f'<path d="{outline}" fill="#000" stroke="#000" stroke-width="3" stroke-linejoin="round"/>')


# ---- the set: name, how to draw it, how to black it out, whether the Badge Index has it


def icon(glyph, material):
    return (lambda: glyph_badge(glyph, material), lambda: locked_glyph(glyph))


def all_badges():
    yield "staff-owner", icon("crown", "crown"), False
    yield "staff-developer", icon("code", "royal"), False
    yield "staff-admin", icon("hammer", "royal"), False
    yield "staff-mod", icon("check", "royal"), False
    yield "staff-helper", icon("buoy", "royal"), False
    yield "plan-supporter", (heart_gem, lambda: locked_gem(HEART)), True
    yield "plan-pro", (brilliant_gem, lambda: locked_gem(BRILLIANT)), True
    yield "og", (star_gem, lambda: locked_gem(STAR)), True
    yield "years", icon("cake", "sky"), True
    for i, material in enumerate(YEAR_MATERIALS, 1):
        yield f"years-{i}", ((lambda m=material, t=str(i): digits_badge(t, m)), None), False
    yield "contributor", icon("wrench", "steel"), True
    # the Exclusive badges staff give by hand
    yield "legend", icon("laurel", "olive"), True
    yield "mentor", icon("lantern", "plum"), True
    yield "patron", icon("goblet", "ruby"), True
    yield "influencer", icon("megaphone", "rose"), True
    yield "artist", icon("palette", "canvas"), True
    yield "app-developer", icon("bot", "sky"), True  # Bot Maker's look before it had levels, for APKs from then
    for i, tier in enumerate(TIERS, 1):
        yield f"bug-{i}", icon("bug", tier), True
        yield f"ideas-{i}", icon("bulb", tier), True
    for i, tier in enumerate(TIERS[:3], 1):
        yield f"gift-{i}", icon("gift", tier), True
        yield f"bot-maker-{i}", icon("bot", tier), True
        yield f"friendly-{i}", icon("face", tier), True
        yield f"host-{i}", icon("house", tier), True
        yield f"open-mic-{i}", icon("mic", tier), True
    yield "booster", icon("rocket", "boost"), True
    for i, tier in enumerate(TIERS[:4], 1):
        yield f"regular-{i}", icon("coffee", tier), True
    # the Secret badges: locked, every one is the padlock, so none has a locked file of its own
    yield "hopeful", icon("sunrise", "copper"), False
    yield "curious", icon("glass", "jade"), False
    yield "nosy", icon("eye", "violet"), False
    yield "lucky", icon("clover", "clover"), False


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    names = set()
    for name, (draw, locked), indexed in all_badges():
        (OUT / f"{name}.svg").write_text(draw())
        names.add(f"{name}.svg")
        # staff ranks and the year numerals are not in the Badge Index, so they are never blacked out
        if indexed:
            (OUT / f"{name}-locked.svg").write_text(locked())
            names.add(f"{name}-locked.svg")
    # every locked Special badge in the Index, which says nothing of what it is; there is no unlocked one
    (OUT / "secret-locked.svg").write_text(locked_glyph("secret"))
    names.add("secret-locked.svg")
    for old in OUT.glob("*.svg"):
        if old.name not in names:
            old.unlink()
    print(f"wrote {len(names)} badges to {OUT.relative_to(ROOT)}")


if __name__ == "__main__":
    main()

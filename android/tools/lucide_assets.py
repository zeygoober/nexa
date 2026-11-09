#!/usr/bin/env python3
"""Writes app/src/main/assets/lucide.json from the web client's lucide-react.

Every Lucide icon, keyed by its file name ("triangle-alert"), as a list of SVG
path strings (circles, ellipses, rects, lines, polylines and polygons turned
into paths here), an alias table from every exported component name
("AlertTriangle", "Loader2") to its file, and which open paths enclose a shape. The app draws them at 24 x 24 with a
2 px round stroke, as lucide-react does, so a glyph looks the same in both.

  python3 android/tools/lucide_assets.py
"""
import json, math, re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
ESM = ROOT / "web/node_modules/lucide-react/dist/esm"
OUT = ROOT / "android/app/src/main/assets/lucide.json"
CSS = ROOT / "web/src/components/badges/iconBodies.css"

NODE = re.compile(r'\[\s*"(\w+)"\s*,\s*\{([^}]*)\}\s*\]')
ATTR = re.compile(r'(\w+):\s*"([^"]*)"')


def f(v):
    x = float(v)
    return ("%g" % x)


def to_path(tag, a):
    if tag == "path":
        return a["d"]
    if tag == "circle":
        cx, cy, r = float(a["cx"]), float(a["cy"]), float(a["r"])
        return f"M{f(cx - r)} {f(cy)}a{f(r)} {f(r)} 0 1 0 {f(2 * r)} 0a{f(r)} {f(r)} 0 1 0 {f(-2 * r)} 0"
    if tag == "ellipse":
        cx, cy, rx, ry = float(a["cx"]), float(a["cy"]), float(a["rx"]), float(a["ry"])
        return f"M{f(cx - rx)} {f(cy)}a{f(rx)} {f(ry)} 0 1 0 {f(2 * rx)} 0a{f(rx)} {f(ry)} 0 1 0 {f(-2 * rx)} 0"
    if tag == "rect":
        x, y, w, h = float(a.get("x", 0)), float(a.get("y", 0)), float(a["width"]), float(a["height"])
        rx = float(a.get("rx", a.get("ry", 0)))
        ry = float(a.get("ry", a.get("rx", 0)))
        rx, ry = min(rx, w / 2), min(ry, h / 2)
        if rx == 0 and ry == 0:
            return f"M{f(x)} {f(y)}h{f(w)}v{f(h)}h{f(-w)}Z"
        return (f"M{f(x + rx)} {f(y)}h{f(w - 2 * rx)}a{f(rx)} {f(ry)} 0 0 1 {f(rx)} {f(ry)}v{f(h - 2 * ry)}"
                f"a{f(rx)} {f(ry)} 0 0 1 {f(-rx)} {f(ry)}h{f(-(w - 2 * rx))}a{f(rx)} {f(ry)} 0 0 1 {f(-rx)} {f(-ry)}"
                f"v{f(-(h - 2 * ry))}a{f(rx)} {f(ry)} 0 0 1 {f(rx)} {f(-ry)}Z")
    if tag == "line":
        return f"M{f(a['x1'])} {f(a['y1'])}L{f(a['x2'])} {f(a['y2'])}"
    if tag in ("polyline", "polygon"):
        nums = [float(n) for n in re.split(r"[\s,]+", a["points"].strip()) if n]
        pts = list(zip(nums[0::2], nums[1::2]))
        d = "M" + " L".join(f"{f(x)} {f(y)}" for x, y in pts)
        return d + ("Z" if tag == "polygon" else "")
    raise ValueError(tag)


icons = {}
for p in sorted((ESM / "icons").glob("*.mjs")):
    text = p.read_text()
    m = re.search(r"node:\s*\[(.*)\]\s*\};", text, re.S)
    if not m:
        continue
    paths = []
    for tag, attrs in NODE.findall(m.group(1)):
        a = dict(ATTR.findall(attrs))
        paths.append(to_path(tag, a))
    icons[p.stem] = paths

aliases = {}
for line in (ESM / "lucide-react.mjs").read_text().splitlines():
    m = re.match(r"export \{ (.*) \} from '\./icons/([\w-]+)\.mjs';", line)
    if not m:
        continue
    for name in re.findall(r"default as (\w+)", m.group(1)):
        if name.startswith("Lucide") or name.endswith("Icon"):
            continue
        aliases[name] = m.group(2)



def points(d):
    """A path's subpaths as lists of points: curves through their control points, arcs sampled."""
    tokens = re.findall(r"[MmLlHhVvCcSsQqTtAaZz]|[-+]?(?:\d*\.\d+|\d+\.?)(?:[eE][-+]?\d+)?", d)
    i, cmd, x, y = 0, "", 0.0, 0.0
    subs, cur = [], []
    def num():
        nonlocal i
        i += 1
        return float(tokens[i - 1])
    # an arc's flags may run together with what follows, which the tokens above cannot split
    while i < len(tokens):
        if tokens[i].isalpha():
            cmd = tokens[i]
            i += 1
            if cmd in "Zz":
                if cur:
                    subs.append(cur)
                cur = []
                continue
        rel = cmd.islower()
        c = cmd.upper()
        ox, oy = x if rel else 0.0, y if rel else 0.0
        if c == "M":
            if cur:
                subs.append(cur)
            x, y = ox + num(), oy + num()
            cur = [(x, y)]
            cmd = "l" if rel else "L"
        elif c == "L":
            x, y = ox + num(), oy + num()
            cur.append((x, y))
        elif c == "H":
            x = ox + num()
            cur.append((x, y))
        elif c == "V":
            y = oy + num()
            cur.append((x, y))
        elif c in "CSQT":
            n = {"C": 3, "S": 2, "Q": 2, "T": 1}[c]
            for _ in range(n):
                px, py = ox + num(), oy + num()
                cur.append((px, py))
            x, y = px, py
        elif c == "A":
            rx, ry, rot = num(), num(), num()
            large = int(tokens[i][0])
            tokens[i] = tokens[i][1:]
            if not tokens[i]:
                i += 1
            sweep = int(tokens[i][0])
            tokens[i] = tokens[i][1:]
            if not tokens[i]:
                i += 1
            ex, ey = ox + num(), oy + num()
            cur.extend(arc(x, y, rx, ry, rot, large, sweep, ex, ey))
            x, y = ex, ey
    if cur:
        subs.append(cur)
    return subs


def arc(x1, y1, rx, ry, rot, large, sweep, x2, y2):
    """Points along an SVG arc (the specification's endpoint to centre conversion)."""
    if rx == 0 or ry == 0 or (x1 == x2 and y1 == y2):
        return [(x2, y2)]
    phi = math.radians(rot)
    cp, sp = math.cos(phi), math.sin(phi)
    dx, dy = (x1 - x2) / 2, (y1 - y2) / 2
    px, py = cp * dx + sp * dy, -sp * dx + cp * dy
    rx, ry = abs(rx), abs(ry)
    grow = px * px / (rx * rx) + py * py / (ry * ry)
    if grow > 1:
        rx, ry = rx * math.sqrt(grow), ry * math.sqrt(grow)
    top = rx * rx * ry * ry - rx * rx * py * py - ry * ry * px * px
    low = rx * rx * py * py + ry * ry * px * px
    k = math.sqrt(max(0, top / low)) * (-1 if large == sweep else 1)
    cx1, cy1 = k * rx * py / ry, -k * ry * px / rx
    cx, cy = cp * cx1 - sp * cy1 + (x1 + x2) / 2, sp * cx1 + cp * cy1 + (y1 + y2) / 2
    a1 = math.atan2((py - cy1) / ry, (px - cx1) / rx)
    a2 = math.atan2((-py - cy1) / ry, (-px - cx1) / rx)
    delta = a2 - a1
    if sweep and delta < 0:
        delta += 2 * math.pi
    if not sweep and delta > 0:
        delta -= 2 * math.pi
    out = []
    for n in range(1, 13):
        a = a1 + delta * n / 12
        ax, ay = rx * math.cos(a), ry * math.sin(a)
        out.append((cp * ax - sp * ay + cx, sp * ax + cp * ay + cy))
    return out


def encloses(pts):
    """Whether an open line draws a shape: it holds real area and its gap is small beside that area, which
    rules out strokes, ticks, chevrons and shallow arcs, whose fill would cut across their chord."""
    if len(pts) < 3:
        return False
    area = abs(sum(a[0] * b[1] - b[0] * a[1] for a, b in zip(pts, pts[1:] + pts[:1]))) / 2
    gap = math.dist(pts[0], pts[-1])
    return area >= 6 and area >= 0.3 * gap * gap


# which open paths of each icon are shapes all the same (a closed one ends in Z), for a badge's tint inside
bodies = {}
for name, paths in icons.items():
    found = [i for i, d in enumerate(paths) if d.strip()[-1] not in "zZ" and any(encloses(p) for p in points(d))]
    if found:
        bodies[name] = found

OUT.parent.mkdir(parents=True, exist_ok=True)
OUT.write_text(json.dumps({"icons": icons, "aliases": aliases, "bodies": bodies}, separators=(",", ":")))

# the same list for the web, whose icons load by name (components/badges/EventIcon.tsx)
CSS.write_text(
    "/* Written by android/tools/lucide_assets.py: the open paths that are shapes all the same. */\n"
    + ",".join(f'[data-badge-icon="{n}"] svg>:nth-child({i + 1})' for n, found in sorted(bodies.items()) for i in found)
    + "{fill:var(--badge-tint);fill-opacity:var(--badge-body,.22)}\n"
)
print(f"{len(icons)} icons, {len(aliases)} names, {OUT.stat().st_size // 1024} KiB -> {OUT.relative_to(ROOT)}")

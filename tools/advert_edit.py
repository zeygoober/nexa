"""The edit of tools/advert.py: the song measured, the cut laid on its beats,
drawn frame by frame, and the thumbnail.

The cut is a list of items (a take in a window, two windows side by side, a
phone beside a desk, a title, the wordmark), each with when it is on screen.
stage.html draws any moment of it; this file works out the moments from the
song and the takes, pulls the frames each take needs out of its film, and has
headless Chrome draw every frame of the advert, several pages at once.
"""
import json, os, re, shutil, subprocess, sys, time
from pathlib import Path

import numpy as np

ROOT = Path(__file__).resolve().parent.parent
WORK = Path(os.environ.get("ADVERT_WORK", "/tmp/nexa-advert"))
TAKES = WORK / "takes"
FRAMES = WORK / "frames"
URL_HOLD = 3.0  # seconds the ending holds on the address after the song
FPS = 60

# ---------------------------------------------------------------------------
# The song
# ---------------------------------------------------------------------------


def analyse(song, bpm=None):
    """The song's beat grid and its shape: when the drums first come in (the
    first drop), the gap where they stop (the breakdown) and come back (the
    second drop), and when they stop for good (the outro). Every one of those
    is put on a bar line counted from the first drop."""
    import librosa
    y, sr = librosa.load(song, sr=22050, mono=True)
    hop = 256
    env = librosa.onset.onset_strength(y=y, sr=sr, hop_length=hop)
    # librosa can take a song for two thirds or half its real tempo; bpm says
    # roughly where it is and the beats are still found in the audio
    tempo = bpm or float(np.atleast_1d(librosa.feature.tempo(onset_envelope=env, sr=sr, hop_length=hop))[0])
    _, beats = librosa.beat.beat_track(onset_envelope=env, sr=sr, hop_length=hop, start_bpm=tempo, units="time", tightness=800)
    beats = np.asarray(beats)
    # one steady period from the tracked beats: the cut needs a grid, not a wobble
    period = float(np.median(np.diff(beats)))
    k = np.round((beats - beats[0]) / period)
    period, phase = np.polyfit(k, beats, 1)
    # the drums: low, percussive energy, beat by beat
    _, perc = librosa.effects.hpss(y)
    spec = np.abs(librosa.stft(perc, hop_length=hop))
    low = spec[librosa.fft_frequencies(sr=sr) < 150].sum(0)
    t = librosa.times_like(low, sr=sr, hop_length=hop)
    n0 = int(np.ceil(-phase / period))
    grid = phase + period * np.arange(n0, int((len(y) / sr - phase) / period))
    per = np.array([low[(t >= b) & (t < b + period)].mean() for b in grid])
    drums = per > 0.25 * np.percentile(per, 90)
    # runs of beats with drums, ignoring single beats either way
    runs, i = [], 0
    while i < len(drums):
        if drums[i]:
            j = i
            while j < len(drums) and (drums[j] or (j + 1 < len(drums) and drums[j + 1])):
                j += 1
            if j - i >= 8:
                runs.append((i, j))
            i = j
        i += 1
    if len(runs) < 2:
        raise SystemExit(f"could not find the song's shape (drum runs: {runs})")
    gaps = [(runs[r + 1][0] - runs[r][1], r) for r in range(len(runs) - 1)]
    _, r = max(gaps)
    i_d1, i_b, i_d2, i_o = runs[0][0], runs[r][1], runs[r + 1][0], runs[-1][1]
    # a drop is the hardest hit of the song around where its drums start: the
    # run is only sure of itself a beat or two later, so look two bars back
    times = np.arange(len(env)) * hop / sr

    def hit(i):
        m = (times > grid[i] - 8 * period) & (times < grid[i] + period / 2)
        return float(times[m][np.argmax(env[m])])
    d1 = hit(i_d1)
    bar = 4 * period

    def on_bar(x):
        return d1 + round((x - d1) / bar) * bar
    # the piano's clearest notes before the first drop, for the intro to move on
    harm, _ = librosa.effects.hpss(y, margin=3.0)
    band = np.abs(librosa.stft(harm, hop_length=hop))
    f = librosa.fft_frequencies(sr=sr)
    penv = librosa.onset.onset_strength(S=librosa.amplitude_to_db(band[(f > 220) & (f < 2500)], ref=np.max), sr=sr, hop_length=hop)
    pon = librosa.onset.onset_detect(onset_envelope=penv, sr=sr, hop_length=hop, units="time")
    pst = penv[librosa.time_to_frames(pon, sr=sr, hop_length=hop)]
    # every clear piano note in the song, with how strong it is, for the
    # quiet parts (the breakdown, the outro) to move on as the intro does
    piano = [[round(float(t), 3), round(float(v) / float(pst.max()), 3)] for t, v in zip(pon, pst) if v > 0.25 * pst.max()]
    keep = (pon > 0.5) & (pon < d1 - 0.2)
    pon, pst = pon[keep], pst[keep]
    notes = sorted(float(t) for t in pon[np.argsort(-pst)[:14]])
    out = {
        "notes": notes, "piano": piano,
        "period": float(period), "bar": float(bar), "d1": d1,
        "breakdown": on_bar(grid[i_b]), "d2": on_bar(hit(i_d2)), "outro": on_bar(grid[i_o]),
        "end": len(y) / sr, "bpm": 60 / float(period),
    }
    print("song: %.1f bpm, first drop %.2f s, breakdown %.2f s, second drop %.2f s, outro %.2f s, ends %.2f s" % (
        out["bpm"], out["d1"], out["breakdown"], out["d2"], out["outro"], out["end"]))
    return out


# ---------------------------------------------------------------------------
# Takes
# ---------------------------------------------------------------------------


class Takes:
    def __init__(self):
        self.info = {}
        for f in TAKES.glob("*.json"):
            self.info[f.stem] = json.loads(f.read_text())
        self.ranges = {}  # (take, view) -> [lo, hi] of source time wanted

    def mark(self, take, label, view="a"):
        info = self.info[take]
        return info["marks"][label] - info["views"][view]["offset"]

    def at(self, take, label, default=(0.5, 0.5), dx=0.0, dy=0.0):
        """The middle of what a logged moment happened to, as a fraction of the
        picture (where the camera points), or `default` if none was logged."""
        f = self.info.get(take, {}).get("focus", {}).get(label)
        if not f:
            return default
        return (min(1.0, max(0.0, f[0] + f[2] / 2 + dx)), min(1.0, max(0.0, f[1] + f[3] / 2 + dy)))

    def has(self, take, label=None):
        return take in self.info and (label is None or label in self.info[take]["marks"])

    def file(self, take, view="a"):
        return TAKES / self.info[take]["views"][view]["file"]

    def want(self, take, view, lo, hi):
        key = (take, view)
        a, b = self.ranges.get(key, (lo, hi))
        self.ranges[key] = [min(a, lo), max(b, hi)]

    def extract(self, scale_w):
        """Pulls out the frames each take needs, as JPEGs at 60 a second."""
        FRAMES.mkdir(parents=True, exist_ok=True)
        out = {}
        for (take, view), (lo, hi) in sorted(self.ranges.items()):
            lo, hi = max(0.0, lo - 0.1), hi + 0.1
            src = self.file(take, view)
            phone = self.info[take]["views"][view]["phone"]
            d = FRAMES / f"{take}-{view}"
            stamp = d / "range.json"
            want = {"lo": round(lo, 3), "hi": round(hi, 3), "w": scale_w, "mtime": src.stat().st_mtime}
            if not (stamp.exists() and json.loads(stamp.read_text()) == want):
                shutil.rmtree(d, ignore_errors=True)
                d.mkdir(parents=True)
                size = "scale=-2:2340" if phone else f"scale={scale_w}:-2"
                subprocess.run(["ffmpeg", "-v", "error", "-ss", f"{lo:.3f}", "-i", str(src), "-t", f"{hi - lo:.3f}",
                                "-vf", f"fps={FPS},{size}:flags=lanczos", "-q:v", "2", str(d / "%05d.jpg")], check=True)
                stamp.write_text(json.dumps(want))
            n = len(list(d.glob("*.jpg")))
            out[f"{take}-{view}"] = {"dir": d.as_uri(), "lo": lo, "n": n}
            print(f"  frames {take}-{view}: {n}")
        return out


# ---------------------------------------------------------------------------
# The cut
# ---------------------------------------------------------------------------


class Cut:
    """The advert as the stage draws it. A rig is a window (or two, or a wall
    of them) placed in 3D for a stretch of the song, with how it comes in and
    goes out; a piece is what one window shows for a while; texts are words."""

    def __init__(self, song, takes):
        self.s, self.t = song, takes
        self.rigs, self.pieces, self.texts, self.bg, self.flashes = [], [], [], [], []

    def beat(self, n, base=None):
        return (self.s["d1"] if base is None else base) + n * self.s["period"]

    def bars(self, n, base=None):
        return self.beat(4 * n, base)

    # --- pictures --------------------------------------------------------------

    def source(self, take, view, t0, t1, anchors=(), src_in=None, speed=1.0, hold_until=None):
        """How timeline time maps to the take's own time between t0 and t1.
        anchors are (mark, timeline time): each logged moment lands on the
        moment of the song it is given, the film running a little faster or
        slower between them. Without anchors it runs from src_in."""
        pts = [(tl, self.t.mark(take, m, view)) for m, tl in anchors]
        if not pts:
            pts = [(t0, src_in)]
        pts.sort()
        for (a, sa), (b, sb) in zip(pts, pts[1:]):
            sp = (sb - sa) / (b - a)
            if not 0.6 <= sp <= 1.7:
                print(f"  note: {take} runs at {sp:.2f}x between {a:.2f} and {b:.2f}")
        first, last = pts[0], pts[-1]
        pts = [(t0, first[1] - (first[0] - t0) * speed)] + [q for q in pts if t0 < q[0] < t1] + [(t1, last[1] + (t1 - last[0]) * speed)]
        if hold_until:
            # still on one frame until then, as a picture waiting to play
            held = float(np.interp(hold_until, [a for a, _ in pts], [b for _, b in pts]))
            pts = [(t0, held), (hold_until, held)] + [q for q in pts if q[0] > hold_until]
        self.t.want(take, view, min(q[1] for q in pts), max(q[1] for q in pts))
        return {"key": f"{take}-{view}", "map": [[round(a, 4), round(b, 4)] for a, b in pts]}

    def rig(self, layout, s0, s1, enter="glide", exit="glide", cams=None, **opts):
        """A scene: windows placed in 3D from s0 to s1. cams holds each
        window's framing, (zoom, x, y, zoom at the end): one a scene, pushed
        in slowly across it, never moved at a cut."""
        self.rigs.append({"layout": layout, "s0": s0, "s1": s1, "enter": enter, "exit": exit, "cams": cams or [], **opts})
        return len(self.rigs) - 1

    def piece(self, rig, take, t0, t1, slot=0, view="a", cam=None, **src):
        """What one window shows from t0 to t1. cam gives this piece a framing
        of its own, for a different view inside the same scene (a cut)."""
        self.pieces.append({"rig": rig, "slot": slot, "t0": t0, "t1": t1, "cam": cam, "src": self.source(take, view, t0, t1, **src)})

    def shot(self, rig, take, base, at, n, mark, land=1, view="a", slot=0):
        """n beats of a take from beat `at` after `base`, its logged moment
        `mark` landing `land` beats after the cut."""
        if not self.t.has(take, mark):
            return
        p = self.s["period"]
        t0, t1 = base + at * p, base + (at + n) * p
        self.piece(rig, take, t0, t1, slot=slot, view=view, anchors=[(mark, t0 + land * p)])

    def montage(self, rig, take, labels, t0, n_beats, lead=0.06, every=1):
        """A logged moment every `every` beats, the cut and the change (a
        theme, a font) on the beat together."""
        p = self.s["period"] * every
        labels = [l for l in labels if self.t.has(take, l)][:int(n_beats / every)]
        for i, label in enumerate(labels):
            a = t0 + i * p
            b = t0 + (i + 1) * p if i < len(labels) - 1 else t0 + n_beats * self.s["period"]
            self.piece(rig, take, a, b, src_in=self.t.mark(take, label) - lead)

    # --- words -------------------------------------------------------------------

    def title(self, text, t0, t1, place="top", rig=None, **o):
        """A scene's title, a beat after the scene starts; with its rig it
        travels with the scene, and leaves with it."""
        p = self.s["period"]
        self.texts.append({"kind": "title", "t0": t0 + p, "t1": t1 - 0.12, "text": text, "place": place, "rig": rig, **o})

    def piano(self, a, b, n, gap=0.24):
        """n of the piano's clearest notes between a and b, at least gap apart,
        in order: what words land on in the quiet parts. Beats fill in when the
        piano is too sparse there."""
        chosen = []
        for t, v in sorted((x for x in self.s.get("piano", []) if a <= x[0] < b), key=lambda x: -x[1]):
            if len(chosen) < n and all(abs(t - c) >= gap for c in chosen):
                chosen.append(t)
        k = 0
        while len(chosen) < n:
            t = a + k * self.s["period"]
            if all(abs(t - c) >= gap for c in chosen):
                chosen.append(t)
            k += 1
        return sorted(chosen)

    def statement(self, text, t0, t1, **o):
        n = len(text.split(" "))
        self.texts.append({"kind": "statement", "t0": t0, "t1": t1, "text": text, "times": self.piano(t0 - 0.05, t1 - 0.5, n), **o})

    def colours(self, t, a, b, glow=1.0):
        self.bg.append([t, a, b, glow])

    # --- the advert --------------------------------------------------------------

    def _intro(self):
        """The piano intro: the headline, the wordmark, and when the app rises."""
        s, p = self.s, self.s["period"]
        d1 = s["d1"]
        # the intro, on the piano: the first line on its first notes, the rest
        # on the next phrase, the wordmark on the one after; then the app
        # rises as the headline drifts off, landing on the drop
        notes = s["notes"]

        def note_after(t, gap=0.2):
            return next((n for n in notes if n > t + gap), t + 1.0)
        n1 = notes[0]
        n2 = note_after(n1, 0.15)
        n3 = note_after(n2, 1.4)
        n4 = note_after(n3, 0.9)
        rise = next((n for n in notes if d1 - 2.7 < n < d1 - 1.6), d1 - 2.2)
        times = [n1, n2] + [n3 + k * 0.11 for k in range(6)]
        self.colours(0, "#0e0b16", "#0e0b16", 0.0)
        self.colours(n1, "#3a1a6e", "#14102a", 0.55)
        self.texts.append({"kind": "headline", "t0": 0.0, "t1": rise + 0.6, "y": 430,
                           "lines": ["Private Chat", "with All the Fun Left In"], "times": times})
        self.texts.append({"kind": "wordmark", "t0": n4, "t1": rise + 0.6, "size": 58, "y": 330})
        return rise

    def _breakdown(self):
        """The breakdown: privacy, said plainly, then the server's view from far off."""
        s, T, p = self.s, self.t, self.s["period"]
        brk, d2 = s["breakdown"], s["d2"]
        B = lambda n, base=brk: self.bars(n, base)  # noqa: E731
        have = T.has
        self.colours(brk, "#0b0b14", "#0b0b14", 0.15)
        self.statement("Every Message.", brk, B(1, brk) - 0.03)
        self.statement("End-to-End Encrypted.", B(1, brk), B(2, brk) - 0.03)
        if have("e2ee"):
            self.colours(B(2, brk), "#1d1342", "#0b0b14", 0.55)
            r = self.rig("single", B(2, brk), d2 - p, enter="far", enterDur=1.2, exit="recede", exitDur=0.5, cams=[[1.0, 0.5, 0.5, 1.05]])
            self.piece(r, "e2ee", B(2, brk), d2 - p, src_in=T.mark("e2ee", "cut") + 3.2)
            self.title("Not Even the Server Can Read It", B(2, brk), d2 - p)
        self.colours(d2 - p, "#000000", "#000000", 0.0)

    def _ending(self, tail=2.6):
        """The wordmark, the tagline and the address, then black."""
        s, p = self.s, self.s["period"]
        outro, end = s["outro"], s["end"]
        B = lambda n, base=outro: self.bars(n, base)  # noqa: E731
        self.colours(outro, "#3d1478", "#0c0a1a", 0.8)
        # the name, the promise and the address, each on a note of the piano
        mark, tag, url = self.piano(outro + 1.5 * p, B(2, outro), 3, gap=0.9)
        self.texts.append({"kind": "wordmark", "t0": mark, "t1": end, "size": 170, "y": 470})
        self.texts.append({"kind": "tagline", "t0": tag, "t1": end, "text": "Free. Private. Yours."})
        # the address stays on screen past the music, so nobody misses it
        hold = end + URL_HOLD
        self.texts.append({"kind": "url", "t0": url, "t1": hold, "text": "nexa.pm"})
        for t in self.texts:
            if t["t1"] == end:
                t["t1"] = hold
        return {"fps": FPS, "dur": hold, "rigs": self.rigs, "pieces": self.pieces, "texts": self.texts,
                "bg": self.bg, "fade": [hold - 1.3, hold - 0.2]}

    def build(self):
        s, T, p = self.s, self.t, self.s["period"]
        d1, d2, brk, outro, end = s["d1"], s["d2"], s["breakdown"], s["outro"], s["end"]
        B = lambda n, base=d1: self.bars(n, base)  # noqa: E731
        shot, have = self.shot, T.has

        rise = self._intro()

        # 1: a conversation as it happens, then someone's own typing indicator
        r = self.rig("single", rise, B(4), enter="rise", enterDur=d1 - rise, cams=[[1.22, 0.42, 0.64, 1.3]])
        self.piece(r, "hi", rise, B(3), src_in=0, anchors=[("hi", self.beat(1)), ("hey", self.beat(5)), ("how", self.beat(8)), ("good", self.beat(10))],
                   hold_until=d1)
        self.colours(d1, "#4c1d95", "#0f2a4a", 1.0)
        self.title("Chat in Real Time", d1, B(3))
        if have("typing"):
            self.piece(r, "typing", B(3), B(4), anchors=[("sent", B(4) - 0.05)], cam=[1.75, 0.36, 0.9, 1.82])
            self.texts.append({"kind": "title", "t0": B(3) + p * 0.5, "t1": B(4) - 0.12, "text": "Even Your Typing Indicator", "place": "top"})

        # 2: formatting, a kind every two beats, then the editor
        A2 = B(4)
        self.colours(A2, "#6d1d5b", "#1b1650", 1.0)
        r = self.rig("right", A2, B(8), cams=[[1.6, 0.52, 0.8, 1.68]])
        for k, label in enumerate(["bold", "colour", "spoiler-open", "code"]):
            shot(r, "formatting", A2, 2 * k, 2, label, land=1)
        if have("editor"):
            self.piece(r, "editor", A2 + 8 * p, B(8), anchors=[("callout", A2 + 10 * p), ("sent", A2 + 13.5 * p)],
                       cam=[1.15, 0.4, 0.5, 1.2])
        self.title("Say It\nYour Way", A2, B(8), place="left")

        # 3: calls
        A3 = B(8)
        self.colours(A3, "#0c4a4a", "#13204d", 1.0)
        r = self.rig("single", A3, B(12), cams=[[1.04, 0.5, 0.5, 1.1]])
        shot(r, "voice", A3, 0, 6, "joined", land=1)
        shot(r, "share", A3, 6, 10, "watch", land=1)
        self.title("Calls and 1080p60 Screen Share", A3, B(12))

        # 4: looks, a theme a beat, a season, then CSS; then back into the dark
        A4 = B(12)
        self.colours(A4, "#5a2a0c", "#3a0f3f", 1.0)
        r = self.rig("left", A4, brk, exit="recede", exitDur=0.6, cams=[[1.25, 0.62, 0.42, 1.3]])
        if have("themes"):
            self.montage(r, "themes", [f"theme{k}" for k in range(12)], A4, 8)
        if have("snow"):
            self.piece(r, "snow", A4 + 8 * p, A4 + 12 * p, anchors=[("sent", A4 + 10 * p)], cam=[1.1, 0.6, 0.5, 1.14])
        if have("css"):
            self.piece(r, "css", A4 + 12 * p, brk, anchors=[("closed", A4 + 14 * p)], cam=[1.0, 0.5, 0.5, 1.04])
        self.title("Make It\nYours", A4, brk, place="right")

        self._breakdown()

        # 5: names, a font or effect a beat, out of the dark on the second drop
        self.colours(d2, "#7a2e12", "#4a1060", 1.0)
        r = self.rig("right", d2, B(4, d2), enter="focus", enterDur=0.7, cams=[[1.72, 0.5, 0.38, 1.78]])
        if have("namestyle"):
            labels = [f"font{k}" for k in range(8)] + [f"effect{k}" for k in range(5)]
            self.montage(r, "namestyle", labels, d2, 16)
        self.title("Your Name,\nYour Style", d2, B(4, d2), place="left")

        # 6: notes, two people's screens at two depths
        A6 = B(4, d2)
        self.colours(A6, "#123e6b", "#2a1458", 1.0)
        r = self.rig("pair", A6, B(8, d2), cams=[[1.7, 0.62, 0.3, 1.78], [1.7, 0.62, 0.3, 1.78]], titles=["test1", "test2"])
        if have("notes"):
            for slot, view in ((0, "a"), (1, "b")):
                self.piece(r, "notes", A6, B(8, d2), slot=slot, view=view, anchors=[("typing", A6 + 3 * p)])
        self.title("Write Together, Live", A6, B(8, d2))

        # 7: a locked chat, then plugins
        A7 = B(8, d2)
        self.colours(A7, "#0f4a33", "#1a1c4d", 1.0)
        r = self.rig("single", A7, B(12, d2), cams=[[1.42, 0.42, 0.8, 1.48]])
        shot(r, "subchat", A7, 0, 4, "made", land=1)
        shot(r, "subchat", A7, 4, 4, "sent", land=2)
        self.texts.append({"kind": "title", "t0": A7 + p, "t1": A7 + 8 * p - 0.12, "text": "Lock Any Chat with a Password", "place": "top"})
        if have("plugins"):
            self.piece(r, "plugins", A7 + 8 * p, B(12, d2), anchors=[("waved", A7 + 14 * p)], cam=[1.25, 0.5, 0.68, 1.3])
            self.texts.append({"kind": "title", "t0": A7 + 8.5 * p, "t1": B(12, d2) - 0.12, "text": "Add Your Own Plugins", "place": "top"})

        # 8: everywhere, then the wall of everything, falling back into the dark
        A8 = B(12, d2)
        self.colours(A8, "#3b1a6e", "#0d2f4f", 1.0)
        r = self.rig("duo", A8, A8 + 8 * p, cams=[[1.12, 0.45, 0.6, 1.16]])
        if have("phone"):
            for slot, view in ((0, "a"), (1, "b")):
                self.piece(r, "phone", A8, A8 + 8 * p, slot=slot, view=view, anchors=[("arrived", A8 + 3 * p), ("back", A8 + 7 * p)])
        self.title("On Every Screen", A8, A8 + 8 * p)
        self._wall(A8 + 8 * p, outro + 3 * p)

        return self._ending()


    def scene(self, layout, s0, s1, title=None, place="top", cams=None, quick=False, **opts):
        """A rig with its title: the title on the top band, or beside a turned
        window. A quick scene comes and goes in about half the time."""
        if quick:
            opts.setdefault("enterDur", 0.45)
            opts.setdefault("exitDur", 0.34)
        r = self.rig(layout, s0, s1, cams=cams, **opts)
        if title:
            self.title(title, s0, s1, place=place, rig=r)
        return r

    def cam(self, *keys, drift=0.02):
        """A camera for a window: keys of [t, zoom, fx, fy, move time], each a
        framing it eases into and arrives at on t, then holds."""
        keys = [list(k) for k in keys]
        for k in keys[1:]:
            mv = k[4] if len(k) > 4 else 0.9
            k[4:] = [mv if mv < 0.05 else max(1.0, mv * 1.4)]
        return {"keys": keys, "drift": drift}

    def build_fast(self):
        """The trailer cut twice as often: a new scene every two bars, quicker
        glides, the same intro, breakdown and ending. Every scene opens on the
        whole app, then the camera eases in to what is happening, close enough
        to read, on the beat."""
        s, T, p = self.s, self.t, self.s["period"]
        d1, d2, outro = s["d1"], s["d2"], s["outro"]
        B = lambda n, base=d1: self.bars(n, base)  # noqa: E731
        shot, have, sc, cam, at = self.shot, T.has, self.scene, self.cam, T.at

        rise = self._intro()
        # the whole app as someone opens it: their messages, then a server with
        # something new in its channels, clicked on the drop
        r = self.rig("single", rise, B(2), enter="rise", enterDur=d1 - rise, exitDur=0.34,
                     cams=[cam([rise, 1.0, 0.5, 0.5], [d1 + 1.6 * p, 1.3, 0.28, 0.42, 0.9],
                               [B(1) + 1.6 * p, 1.55, *at("tour", "channel", (0.55, 0.85)), 0.9])])
        self.piece(r, "tour", rise, B(2), anchors=[("server", d1), ("channel", B(1) + 0.5 * p)])
        self.colours(d1, "#4c1d95", "#0f2a4a", 1.0)
        self.title("Everything You're Used To", d1 - p, B(2), rig=r)
        # then what everyone wants from it, fast; then what only Nexa has
        self.colours(B(2), "#3b1f8a", "#0f2a4a", 1.0)
        r = sc("single", B(2), B(4), "Chat in Real Time", quick=True,
               cams=[cam([B(2), 1.3, *at("hi", "hey", (0.45, 0.8))], [B(2) + 3.6 * p, 1.6, *at("hi", "how", (0.45, 0.85)), 0.8])])
        self.piece(r, "hi", B(2), B(4), anchors=[("hey", B(2) + p), ("how", B(2) + 4 * p), ("good", B(2) + 6.5 * p)])
        self.colours(B(4), "#6d1d5b", "#1b1650", 1.0)
        r = sc("right", B(4), B(6), "Say It\nYour Way", place="left", quick=True,
               cams=[cam([B(4), 1.5, *at("formatting", "bold", (0.55, 0.8))], [B(4) + 4 * p, 1.65, *at("formatting", "code", (0.55, 0.8)), 0.8])])
        for k, label in enumerate(["bold", "colour", "spoiler", "spoiler-open", "code", "quote"]):
            shot(r, "formatting", B(4), k, 1 if k < 5 else 3, label, land=0.5)
        self.colours(B(6), "#5a1a6e", "#1b1e50", 1.0)
        r = sc("left", B(6), B(8), "Or Build It\nVisually", place="right", quick=True,
               cams=[cam([B(6), 1.3, *at("editor", "open", (0.5, 0.6))], [B(6) + 2 * p, 1.55, *at("editor", "callout", (0.5, 0.6)), 0.8],
                         [B(6) + 6.6 * p, 1.5, *at("editor", "sent", (0.5, 0.8)), 0.6])])
        self.piece(r, "editor", B(6), B(8), anchors=[("callout", B(6) + 2 * p), ("sent", B(6) + 6.5 * p)])
        self.colours(B(8), "#0c4a4a", "#13204d", 1.0)
        r = sc("single", B(8), B(10), "Full-Quality Screen Share, Free", quick=True,
               cams=[cam([B(8), 1.0, 0.5, 0.5], [B(8) + 3 * p + 0.5, 1.12, *at("share", "watch", (0.55, 0.45)), 0.5])])
        shot(r, "voice", B(8), 0, 3, "joined", land=1)
        shot(r, "share", B(8), 3, 5, "watch", land=1)
        self.colours(B(10), "#5a2a0c", "#3a0f3f", 1.0)
        r = sc("right", B(10), B(12), "Make It\nYours", place="left", quick=True, cams=[cam([B(10), 1.04, 0.5, 0.5])])
        if have("themes"):
            self.montage(r, "themes", [f"theme{k}" for k in range(12)], B(10), 6)
        self.piece(r, "snow", B(10) + 6 * p, B(12), anchors=[("sent", B(10) + 6.5 * p)],
                   cam=cam([B(10) + 6 * p, 1.12, 0.55, 0.55], [B(12), 1.3, *at("snow", "sent", (0.55, 0.8)), 0.8]))
        self.colours(B(12), "#1f5a2a", "#13204d", 1.0)
        r = sc("left", B(12), B(14), "Your Own\nStickers", place="right", quick=True,
               cams=[cam([B(12), 1.45, *at("stickers", "installed", (0.5, 0.35))], [B(12) + 3 * p, 1.4, *at("stickers", "picker", (0.6, 0.6)), 0.01],
                         [B(12) + 5.6 * p, 1.55, *at("stickers", "sent", (0.5, 0.8)), 0.7])])
        if have("stickers"):
            shot(r, "stickers", B(12), 0, 3, "installed", land=1)
            shot(r, "stickers", B(12), 3, 5, "sent", land=2)
        self.colours(B(14), "#3b1f8a", "#122a52", 1.0)
        r = sc("single", B(14), s["breakdown"], "Your Own Typing Indicator", quick=True, exit="recede", exitDur=0.6,
               cams=[cam([B(14), 1.05, 0.5, 0.6], [B(14) + 2.2 * p, 1.9, *at("typing", "start", (0.4, 0.9), dy=-0.035), 1.0])])
        self.piece(r, "typing", B(14), s["breakdown"], anchors=[("sent", s["breakdown"] - 0.4)])

        self._breakdown()

        self.colours(d2, "#4a1d7a", "#10234a", 1.0)
        r = sc("single", d2, B(2, d2), "A Profile That's All You", enter="focus", enterDur=0.6, exitDur=0.34,
               cams=[cam([d2, 1.3, *at("showcase", "open")], [d2 + 3.2 * p, 1.5, *at("showcase", "play", (0.6, 0.45)), 0.9])])
        if have("showcase"):
            shot(r, "showcase", d2, 0, 3, "open", land=0.5)
            shot(r, "showcase", d2, 3, 5, "widgets", land=0.5)
        self.colours(B(2, d2), "#7a2e12", "#4a1060", 1.0)
        r = sc("right", B(2, d2), B(4, d2), "Your Name,\nYour Style", place="left", quick=True,
               cams=[cam([B(2, d2), 1.7, *at("namestyle", "font0", (0.5, 0.38), dy=0.06)])])
        if have("namestyle"):
            self.montage(r, "namestyle", [f"font{k}" for k in range(4)] + [f"effect{k}" for k in range(4)], B(2, d2), 8)
        self.colours(B(4, d2), "#123e6b", "#2a1458", 1.0)
        nk = cam([B(4, d2), 1.5, 0.62, 0.48], [B(4, d2) + 2 * p, 2.1, 0.6, 0.45, 0.8])
        r = sc("pair", B(4, d2), B(6, d2), "Write Together, Live", cams=[nk, nk], titles=["test1", "test2"], quick=True)
        for slot, view in ((0, "a"), (1, "b")):
            self.piece(r, "notes", B(4, d2), B(6, d2), slot=slot, view=view, anchors=[("typing", B(4, d2) + p)])
        self.colours(B(6, d2), "#0f4a33", "#1a1c4d", 1.0)
        r = sc("single", B(6, d2), B(8, d2), "Lock Any Chat with a Password", quick=True,
               cams=[cam([B(6, d2), 1.15, 0.5, 0.5], [B(6, d2) + 4 * p + 0.5, 1.5, *at("subchat", "sent", (0.5, 0.8)), 0.6])])
        shot(r, "subchat", B(6, d2), 0, 4, "made", land=3)
        shot(r, "subchat", B(6, d2), 4, 4, "sent", land=2)
        self.colours(B(8, d2), "#1c3a5e", "#1a1c4d", 1.0)
        r = sc("single", B(8, d2), B(10, d2), "Plugins and Custom CSS", quick=True,
               cams=[cam([B(8, d2), 1.45, *at("plugins", "waved", (0.5, 0.8))])])
        self.piece(r, "plugins", B(8, d2), B(9, d2), anchors=[("waved", B(8, d2) + 3 * p)])
        self.piece(r, "css", B(9, d2), B(10, d2), anchors=[("closed", B(9, d2) + 2 * p)], cam=cam([B(9, d2), 1.0, 0.5, 0.5]))
        self.colours(B(10, d2), "#3b1a6e", "#0d2f4f", 1.0)
        r = sc("duo", B(10, d2), B(12, d2), "On Every Screen", cams=[cam([B(10, d2), 1.12, 0.5, 0.6])], quick=True)
        for slot, view in ((0, "a"), (1, "b")):
            self.piece(r, "phone", B(10, d2), B(12, d2), slot=slot, view=view, anchors=[("arrived", B(10, d2) + 2 * p), ("back", B(10, d2) + 6 * p)])
        self._wall(B(12, d2), outro + 3 * p)
        return self._ending()

    def _wall(self, w0, w1):
        """Every screen at once, on a wall drifting back into the dark."""
        T, p = self.t, self.s["period"]
        tiles = [("hi", "good", 1.25, 0.42, 0.64), ("formatting", "code", 1.6, 0.52, 0.8), ("voice", "joined", 1.0, 0.5, 0.5),
                 ("themes", "theme2", 1.25, 0.62, 0.42), ("showcase", "widgets", 1.18, 0.56, 0.42), ("share", "watch", 1.0, 0.5, 0.5), ("notes", "typed", 1.7, 0.62, 0.3),
                 ("namestyle", "effect2", 1.72, 0.5, 0.38), ("snow", "sent", 1.1, 0.6, 0.5), ("editor", "sent", 1.25, 0.42, 0.75),
                 ("subchat", "inside", 1.0, 0.5, 0.5), ("plugins", "waved", 1.25, 0.5, 0.68), ("stickers", "sent", 1.3, 0.42, 0.45)]
        tiles = [x for x in tiles if T.has(x[0], x[1])][:12]
        r = self.rig("wall", w0, w1, enter="glide", exit="recede", exitDur=1.8, stagger=p / 4,
                     cams=[[z, fx, fy, z] for _, _, z, fx, fy in tiles])
        for k, (take, label, *_rest) in enumerate(tiles):
            self.piece(r, take, w0, w1, slot=k, src_in=T.mark(take, label) - 1.0)

    def build_short(self):
        """A 9:16 cut for phones, from the piano breakdown before the second
        drop to the end of the song: Nexa's own phone layout, filmed on a
        phone (the m- takes), each screen the frame's full width.

        The hook line lands word by word on the breakdown's piano, its last
        word on the drop; on the drop it rises into the title's place and the
        app comes up. Then a scene every two bars, each take playing at its
        own speed, the camera opening on the top of the screen (the header,
        the lists) and easing down and in to what happens, close enough to
        read. The ending's words move on the outro's piano."""
        s, T, p = self.s, self.t, self.s["period"]
        d2, outro = s["d2"], s["outro"]
        B = lambda n: self.bars(n, d2)  # noqa: E731
        have, cam = T.has, self.cam
        start, end = B(-2) - 0.05, outro + 10 * p

        def sc(k, n, title, *keys, **o):
            # every cut on a bar line, the push between scenes straddling it
            r = self.rig("phoneTall", B(k), B(k + n), cams=[{"keys": cam(*keys)["keys"], "drift": 0.0}], **o)
            if title:
                self.title(title, B(k), B(k + n), rig=r, y=196, size=70)
            return r

        # the hook, on the piano
        self.colours(start, "#3a1a6e", "#14102a", 0.7)
        words = "Private Chat with All the Fun Left In".split()
        times = self.piano(start + 0.02, d2 - 0.3, len(words) - 1, gap=0.28) + [d2 - 0.03]
        self.texts.append({"kind": "headline", "t0": start, "t1": B(2) - 0.05, "y": 700, "size": 108,
                           "lines": ["Private Chat with", "All the Fun Left In"], "times": times,
                           "move": [d2 + 0.02, d2 + 0.8, -510, 0.62]})
        # on the drop the app comes up: their messages, the server, a channel
        r = sc(0, 2, None, [d2, 1.0, 0.5, 0.0], [B(1) + 3.6 * p, 1.0, 0.5, 1.0, 0.9], enter="rise", enterDur=1.25)
        self.piece(r, "m-tour", d2, B(2), anchors=[("server", B(1))])
        self.colours(d2, "#4c1d95", "#0f2a4a", 1.0)

        # what only Nexa has, a feature every two bars
        self.colours(B(2), "#1f5a2a", "#13204d", 1.0)
        r = sc(2, 2, "Your Own Sticker Packs", [B(2), 1.0, 0.5, 0.0], [B(2) + 2 * p, 1.5, 0.0, T.at("m-stickers", "installed", (0.3, 0.3))[1], 0.9],
               [B(3), 1.0, 0.5, 1.0, 0.01], [B(3) + 2.4 * p, 1.32, 0.0, T.at("m-stickers", "sent", (0.3, 0.85))[1], 0.9])
        self.piece(r, "m-stickers", B(2), B(3), anchors=[("installed", B(2) + 2 * p)])
        self.piece(r, "m-stickers", B(3), B(4), anchors=[("sent", B(3) + 2 * p)])
        self.colours(B(4), "#3b1f8a", "#122a52", 1.0)
        r = sc(4, 2, "Your Own Typing Indicator", [B(4), 1.0, 0.5, 1.0], [B(4) + 3 * p, 1.45, 0.0, 1.0, 1.0])
        self.piece(r, "m-typing", B(4), B(6), anchors=[("sent", B(6) - 2 * p)])
        self.colours(B(6), "#5a2a0c", "#3a0f3f", 1.0)
        r = sc(6, 2, "Make It Yours", [B(6), 1.0, 0.5, 0.0])
        if have("m-themes"):
            self.montage(r, "m-themes", [f"theme{k}" for k in (0, 2, 5, 7)], B(6), 8, lead=0.25, every=2)
        self.colours(B(8), "#4a1d7a", "#10234a", 1.0)
        r = sc(8, 2, "A Profile That's All You", [B(8), 1.0, 0.5, 0.0], [B(9) + 1.4 * p, 1.0, 0.5, 1.0, 1.1])
        self.piece(r, "m-showcase", B(8), B(10), anchors=[("widgets", B(9))])
        self.colours(B(10), "#7a2e12", "#4a1060", 1.0)
        r = sc(10, 2, "Your Name, Your Style", [B(10), 1.0, 0.5, 0.0])
        if have("m-namestyle"):
            self.montage(r, "m-namestyle", [f"font{k}" for k in (0, 4)] + [f"effect{k}" for k in (1, 3)], B(10), 8, lead=0.25, every=2)
        self.colours(B(12), "#123e6b", "#2a1458", 1.0)
        r = sc(12, 2, "Write Together, Live", [B(12), 1.0, 0.5, 0.0], [B(12) + 2.4 * p, 1.85, 0.0, 0.0, 1.0])
        self.piece(r, "m-notes", B(12), B(14), anchors=[("typing", B(12) + 2 * p)])
        self.colours(B(14), "#0f4a33", "#1a1c4d", 1.0)
        # the password going in, then the locked chat it makes
        r = sc(14, 2, "Lock Chats with a Password", [B(14), 1.0, 0.5, 1.0], exit="recede", exitDur=0.7)
        self.piece(r, "m-subchat", B(14), B(16), anchors=[("made", B(14) + 4 * p)])

        # the end, on the outro's piano: the promise, then the name and the address
        self.colours(outro, "#3d1478", "#0c0a1a", 0.85)
        self.texts.append({"kind": "statement", "t0": outro - 0.05, "t1": outro + 4 * p - 0.05, "text": "End-to-End Encrypted.", "y": 860, "size": 92,
                           "times": self.piano(outro - 0.1, outro + 3 * p, 3, gap=0.35)})
        mark, tag, url = self.piano(outro + 4 * p, outro + 9 * p, 3, gap=0.7)
        self.texts.append({"kind": "wordmark", "t0": mark, "t1": end, "size": 150, "y": 860})
        self.texts.append({"kind": "tagline", "t0": tag, "t1": end, "text": "Free. Private. Yours.", "y": 1010})
        hold = end + URL_HOLD
        self.texts.append({"kind": "url", "t0": url, "t1": hold, "text": "nexa.pm", "y": 1110, "size": 96})
        for t in self.texts:
            if t["t1"] == end:
                t["t1"] = hold
        return {"fps": FPS, "dur": hold, "size": [1080, 1920], "span": [start, hold], "trans": 0.8, "rigs": self.rigs, "pieces": self.pieces,
                "texts": self.texts, "bg": self.bg, "fade": [hold - 1.4, hold - 0.1]}


# ---------------------------------------------------------------------------
# Drawing
# ---------------------------------------------------------------------------


def brand_paths():
    src = (ROOT / "web/src/components/common/Brand.tsx").read_text()
    grab = lambda name: re.search(name + r'\s*=\s*"([^"]+)"', src).group(1)  # noqa: E731
    return {"letters": grab("WORDMARK_LETTERS"), "spark": grab("WORDMARK_SPARK"), "mark": grab("const MARK")}


def font_path():
    return ROOT / "web/node_modules/@fontsource-variable/wix-madefor-display/files/wix-madefor-display-latin-wght-normal.woff2"


def font_url():
    return font_path().as_uri()


VIDEO = ROOT / "tools/advert-video"  # the Remotion project that draws the cut


class FileServer:
    """Hands the takes' frames and the song to Remotion's browser over
    localhost, so nothing gigabytes large is copied into its bundle."""

    def __init__(self, root):
        import functools, http.server, threading
        class Quiet(http.server.SimpleHTTPRequestHandler):
            def log_message(self, *a):
                pass
        handler = functools.partial(Quiet, directory=str(root))
        self.httpd = http.server.ThreadingHTTPServer(("127.0.0.1", 0), handler)
        self.base = f"http://127.0.0.1:{self.httpd.server_address[1]}/"
        threading.Thread(target=self.httpd.serve_forever, daemon=True).start()

    def close(self):
        self.httpd.shutdown()


def remotion_render(spec, dest, frames=None):
    """Draws the cut with Remotion: its motion blur, its renderer, the song
    laid in by it, one clock for picture and sound."""
    (VIDEO / "public/fonts").mkdir(parents=True, exist_ok=True)
    shutil.copy(font_path(), VIDEO / "public/fonts/madefor.woff2")
    if not (VIDEO / "node_modules").exists():
        subprocess.run(["npm", "ci"], cwd=VIDEO, check=True)
    srv = FileServer(WORK)
    try:
        spec = dict(spec, base=srv.base)
        props = WORK / "props.json"
        props.write_text(json.dumps({"spec": spec}))
        cmd = ["npx", "remotion", "render", "src/index.ts", "Advert", str(dest), f"--props={props}",
               f"--concurrency={os.environ.get('ADVERT_WORKERS', '6')}", "--crf=16", "--audio-bitrate=320k",
               "--jpeg-quality=95", "--timeout=120000"]
        if frames:
            cmd.append(f"--frames={frames[0]}-{frames[1] - 1}")
        # Remotion's own headless shell does not start on this Mac; Playwright's Chromium does
        found = sorted(Path.home().glob("Library/Caches/ms-playwright/chromium-*/chrome-mac*/*.app/Contents/MacOS/*"))
        browser = os.environ.get("ADVERT_BROWSER") or (str(found[-1]) if found else "")
        if browser:
            cmd.append(f"--browser-executable={browser}")
        subprocess.run(cmd, cwd=VIDEO, check=True)
    finally:
        srv.close()


def edit(args):
    if not args.song:
        print("--song is needed", file=sys.stderr)
        return 2
    song = analyse(args.song, args.bpm)
    takes = Takes()
    if not takes.info:
        print("no takes in", TAKES, "(film first)", file=sys.stderr)
        return 1
    print("takes:", ", ".join(sorted(takes.info)))
    cut = Cut(song, takes)
    spec = {"trailer": cut.build, "fast": cut.build_fast, "short": cut.build_short}[args.cut]()
    spec.setdefault("size", [1920, 1080])
    spec["frames"] = takes.extract(2560)
    (WORK / "cut.json").write_text(json.dumps(spec, indent=1))
    # a cut may use only part of the song (the short does)
    span = spec.setdefault("span", [0.0, spec["dur"]])
    shutil.copy(args.song, WORK / "song.mp3")
    spec["song"] = "song.mp3"
    # the song fades with the picture when the cut ends before the song does
    spec["songEnd"] = span[1] if span[1] < song["end"] - 0.5 else None
    spec["brand"] = brand_paths()
    dest = Path(args.out or (WORK / f"advert-{args.cut}.mp4"))
    frames = None
    if args.t_from is not None or args.t_to is not None:
        a = args.t_from if args.t_from is not None else span[0]
        b = min(args.t_to if args.t_to is not None else span[1], span[1])
        frames = (int(round((a - span[0]) * FPS)), int(round((b - span[0]) * FPS)))
    started = time.time()
    remotion_render(spec, dest, frames)
    print(f"wrote {dest} in {time.time() - started:.0f} s")
    return 0


def stage_page(spec):
    """The old stage page, which the thumbnail is still drawn on."""
    html = (ROOT / "tools/advert_stage.html").read_text()
    html = html.replace("%FONT%", font_url()).replace("/*SPEC*/null", json.dumps(spec)).replace("/*BRAND*/null", json.dumps(brand_paths()))
    page = WORK / "stage.html"
    page.write_text(html)
    return page


def thumb(args):
    """A still that makes people look: the app, a styled name and the line, big."""
    from playwright.sync_api import sync_playwright
    spec = json.loads((WORK / "cut.json").read_text())
    spec["thumb"] = True
    page = stage_page(spec)
    dest = Path(args.out or (WORK / "thumbnail.png"))
    with sync_playwright() as pw:
        br = pw.chromium.launch(channel="chromium", args=["--allow-file-access-from-files", "--disable-web-security"])
        pg = br.new_page(viewport={"width": 1280, "height": 720}, device_scale_factor=1.5)
        pg.goto(page.as_uri())
        pg.wait_for_function("() => window.ready === true", timeout=60000)
        phone = ["phone-b", spec["frames"]["phone-b"]["n"] - 30] if "phone-b" in spec["frames"] else None
        pg.evaluate("(a) => window.drawThumb(...a)", [args.frame_key, args.frame_index, args.line, args.pill, phone])
        pg.screenshot(path=str(dest))
        br.close()
    print("wrote", dest)
    return 0

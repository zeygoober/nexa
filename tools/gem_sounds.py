#!/usr/bin/env python3
"""Writes the Gems sounds into web/public/sounds (Android copies them and takes them over the air).

Pure Python, no numpy; ffmpeg encodes. Each sound is one piece the apps put
together: the fuse dial plays gem-tick once a peg as the marker passes it
(gem-edge at the arc's far edge on the last lap), then gem-win or gem-lose;
a capsule plays gem-rattle on the press, gem-build (gem-build-huge) through
the reveal, then gem-open-<rarity>. Seeds are fixed, so a run writes the same
sounds. Usage: python3 tools/gem_sounds.py [out dir]
"""
import math
import os
import random
import struct
import subprocess
import sys
import tempfile
import wave

SR = 44100



class Track:
    def __init__(self, seconds):
        n = int(seconds * SR)
        self.l = [0.0] * n
        self.r = [0.0] * n

    def add(self, at, mono, gain=1.0, pan=0.0):
        """Mixes a mono sound in at `at` seconds; pan -1 left to 1 right."""
        start = int(at * SR)
        gl = gain * math.cos((pan + 1) * math.pi / 4) * math.sqrt(2)
        gr = gain * math.sin((pan + 1) * math.pi / 4) * math.sqrt(2)
        for i, v in enumerate(mono):
            j = start + i
            if 0 <= j < len(self.l):
                self.l[j] += v * gl
                self.r[j] += v * gr


def env_exp(n, tau):
    return [math.exp(-i / (tau * SR)) for i in range(n)]


def noise(n, seed=None):
    rnd = random.Random(seed)
    return [rnd.uniform(-1, 1) for _ in range(n)]


def one_pole_hp(x, fc):
    a = math.exp(-2 * math.pi * fc / SR)
    y, prev_x, prev_y = [], 0.0, 0.0
    for v in x:
        o = a * (prev_y + v - prev_x)
        y.append(o)
        prev_x, prev_y = v, o
    return y


def one_pole_lp(x, fc):
    a = math.exp(-2 * math.pi * fc / SR)
    y, s = [], 0.0
    for v in x:
        s = (1 - a) * v + a * s
        y.append(s)
    return y


def click(pitch=2100.0, bright=1.0, seed=None):
    """One peg of a prize wheel: a hard transient and a short woody ring."""
    rnd = random.Random(seed)
    f = pitch * rnd.uniform(0.95, 1.05)
    n = int(0.045 * SR)
    trans = one_pole_hp(noise(int(0.0025 * SR), seed), 1500)
    out = [0.0] * n
    for i, v in enumerate(trans):
        out[i] += v * 0.9 * (1 - i / len(trans)) * bright
    for i in range(n):
        t = i / SR
        out[i] += (0.55 * math.sin(2 * math.pi * f * t) * math.exp(-t / 0.006)
                   + 0.25 * math.sin(2 * math.pi * f * 2.63 * t) * math.exp(-t / 0.003)
                   + 0.35 * math.sin(2 * math.pi * f * 0.31 * t) * math.exp(-t / 0.012))
    return out


def bell(f, dur=1.6, gain=1.0, detune=1.0):
    partials = [(1.0, 1.0, 1.1), (2.0, 0.42, 0.7), (2.76, 0.3, 0.45), (5.4, 0.12, 0.22), (8.93, 0.05, 0.12)]
    n = int(dur * SR)
    out = [0.0] * n
    attack = int(0.004 * SR)
    for i in range(n):
        t = i / SR
        a = min(1.0, i / attack)
        v = 0.0
        for ratio, amp, tau in partials:
            v += amp * math.sin(2 * math.pi * f * ratio * detune * t) * math.exp(-t / tau)
        out[i] = v * a * gain
    return out


def thud(f0=150, f1=80, dur=0.45):
    n = int(dur * SR)
    out, ph = [], 0.0
    for i in range(n):
        t = i / SR
        f = f1 + (f0 - f1) * math.exp(-t / 0.05)
        ph += 2 * math.pi * f / SR
        out.append(math.sin(ph) * math.exp(-t / 0.13) * min(1, i / 60))
    return out


def soft_note(f, dur=0.6, tau=0.22):
    """A muted mallet note: a few odd harmonics, low-passed, quick to fade."""
    n = int(dur * SR)
    raw = []
    for i in range(n):
        t = i / SR
        v = math.sin(2 * math.pi * f * t) + 0.25 * math.sin(2 * math.pi * 3 * f * t) * math.exp(-t / 0.05)
        raw.append(v * math.exp(-t / tau) * min(1, i / 120))
    return one_pole_lp(raw, 1800)


def snap():
    n = int(0.09 * SR)
    nz = one_pole_hp(noise(n, 7), 900)
    out, ph = [], 0.0
    for i in range(n):
        t = i / SR
        f = 300 + 1300 * math.exp(-t / 0.012)
        ph += 2 * math.pi * f / SR
        out.append(0.8 * nz[i] * math.exp(-t / 0.008) + 0.7 * math.sin(ph) * math.exp(-t / 0.03))
    return out


def riser(dur, seed=3):
    """Air through the capsule's seam as it builds: band-limited noise that opens up and swells."""
    n = int(dur * SR)
    nz = noise(n, seed)
    out, lp, a_lp = [], 0.0, 0.0
    for i in range(n):
        x = i / n
        fc = 300 + 5000 * x ** 2
        a = math.exp(-2 * math.pi * fc / SR)
        lp = (1 - a) * nz[i] + a * lp
        a_lp = (1 - 0.02) * a_lp + 0.02 * lp
        out.append((lp - a_lp) * (x ** 1.6) * 0.9)
    return out


def rattle(seed):
    """The gem rolling inside the shell: two or three small knocks close together."""
    rnd = random.Random(seed)
    out = [0.0] * int(0.06 * SR)
    for k in range(rnd.randint(2, 3)):
        c = click(pitch=rnd.uniform(1100, 1500), bright=0.5, seed=seed * 10 + k)
        off = int(rnd.uniform(0.0, 0.03) * SR)
        for i, v in enumerate(c):
            if off + i < len(out):
                out[off + i] += v * rnd.uniform(0.4, 0.8)
    return out


def reverb(tr, wet=0.16):
    def comb(x, d, g):
        buf, y, i = [0.0] * d, [], 0
        lp = 0.0
        for v in x:
            o = buf[i]
            lp = 0.6 * o + 0.4 * lp
            buf[i] = v + g * lp
            i = (i + 1) % d
            y.append(o)
        return y

    def allpass(x, d, g):
        buf, y, i = [0.0] * d, [], 0
        for v in x:
            b = buf[i]
            o = -g * v + b
            buf[i] = v + g * o
            i = (i + 1) % d
            y.append(o)
        return y

    def room(x, delays):
        acc = [0.0] * len(x)
        for d in delays:
            for i, v in enumerate(comb(x, int(d * SR / 1000), 0.8)):
                acc[i] += v / len(delays)
        return allpass(allpass(acc, int(5.0 * SR / 1000), 0.7), int(1.7 * SR / 1000), 0.7)

    wl = room(tr.l, [29.7, 37.1, 41.1, 43.7])
    wr = room(tr.r, [31.3, 36.7, 40.1, 45.3])
    tr.l = [d + wet * w for d, w in zip(tr.l, wl)]
    tr.r = [d + wet * w for d, w in zip(tr.r, wr)]


def write(tr, path, peak_db=-4.0):
    peak = max(max(abs(v) for v in tr.l), max(abs(v) for v in tr.r)) or 1
    k = (10 ** (peak_db / 20)) / peak
    with wave.open(path, "wb") as w:
        w.setnchannels(2)
        w.setsampwidth(2)
        w.setframerate(SR)
        frames = bytearray()
        for a, b in zip(tr.l, tr.r):
            frames += struct.pack("<hh", int(max(-1, min(1, a * k)) * 32767), int(max(-1, min(1, b * k)) * 32767))
        w.writeframes(bytes(frames))




C6, E6, G6, C7, E7 = 1046.5, 1318.5, 1568.0, 2093.0, 2637.0
CHIMES = {"common": [G6], "rare": [E6, G6], "epic": [C6, E6, G6], "huge": [C6, E6, G6, C7, E7]}


def tick(tr):
    tr.add(0.0, click(pitch=2100, seed=11))


def edge(tr):
    tr.add(0.0, click(pitch=900, bright=0.7, seed=999))


def win(tr):
    tr.add(0.0, thud(120, 70, 0.5), gain=0.5)
    for i, f in enumerate([C6, E6, G6, C7]):
        tr.add(i * 0.07, bell(f, 1.8), gain=0.32, pan=-0.3 + i * 0.2)
        tr.add(i * 0.07 + 0.003, bell(f, 1.8, detune=1.003), gain=0.12, pan=0.3 - i * 0.2)


def lose(tr):
    tr.add(0.0, thud(150, 85, 0.45), gain=0.7)
    tr.add(0.02, soft_note(392.0), gain=0.38)
    tr.add(0.2, soft_note(311.1, 0.9, 0.3), gain=0.32)


def build(length):
    """The reveal: knocks that quicken as the capsule rocks harder, over air through its seam; ends where it splits."""
    def make(tr):
        tr.add(0.0, riser(length), gain=0.35)
        t, k = 0.0, 0
        while t < length - 0.05:
            x = t / length
            tr.add(t, rattle(300 + k), gain=0.45 + 0.4 * x, pan=(-0.4 if k % 2 else 0.4) * (0.3 + x))
            t += 0.30 - 0.23 * x
            k += 1
    return make


def opened(rarity):
    def make(tr):
        tr.add(0.0, snap(), gain=0.9)
        notes = CHIMES[rarity]
        for i, f in enumerate(notes):
            tr.add(0.03 + i * 0.075, bell(f, 2.2 if rarity == "huge" else 1.6), gain=0.3, pan=-0.35 + 0.7 * i / max(1, len(notes) - 1))
        if rarity == "huge":
            tr.add(0.0, thud(90, 45, 1.2), gain=0.9)
            tr.add(0.45, riser(1.2, seed=9)[::-1], gain=0.2)
    return make


def shake(tr):
    tr.add(0.0, rattle(100), gain=0.5)


# name: (how it is made, seconds, peak in dB, reverb); the peaks set how loud each is beside the others
SOUNDS = {
    "gem-tick": (tick, 0.2, -7.0, 0.08),
    "gem-edge": (edge, 0.3, -5.0, 0.12),
    "gem-win": (win, 2.4, -1.5, 0.16),
    "gem-lose": (lose, 1.4, -2.5, 0.16),
    "gem-rattle": (shake, 0.3, -6.0, 0.1),
    "gem-build": (build(1.5), 1.5, -4.0, 0.12),
    "gem-build-huge": (build(2.3), 2.3, -4.0, 0.12),
    "gem-open-common": (opened("common"), 2.0, -1.5, 0.16),
    "gem-open-rare": (opened("rare"), 2.0, -1.5, 0.16),
    "gem-open-epic": (opened("epic"), 2.2, -1.5, 0.16),
    "gem-open-huge": (opened("huge"), 2.8, -1.0, 0.18),
}


def main():
    out = sys.argv[1] if len(sys.argv) > 1 else os.path.join(os.path.dirname(__file__), "..", "web", "public", "sounds")
    with tempfile.TemporaryDirectory() as tmp:
        for name, (make, seconds, peak, wet) in SOUNDS.items():
            tr = Track(seconds)
            make(tr)
            reverb(tr, wet)
            wav = os.path.join(tmp, name + ".wav")
            write(tr, wav, peak)
            subprocess.run(["ffmpeg", "-y", "-loglevel", "error", "-i", wav, "-ar", "48000", "-c:a", "aac", "-b:a", "96k",
                            "-map_metadata", "-1", "-fflags", "+bitexact", os.path.join(out, name + ".m4a")], check=True)
    print(f"{len(SOUNDS)} sounds written")


if __name__ == "__main__":
    main()

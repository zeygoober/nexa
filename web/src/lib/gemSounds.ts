import type { GemRarity } from "@/api/gems";
import { useSettings } from "@/stores/settings";
import { effects, primeEffects, type Effects } from "./sounds";

/**
 * The Gems sounds (made by tools/gem_sounds.py, the same files on Android):
 * the fuse dial clicks once a peg as the marker passes it, knocks as it
 * crosses the line it ends beside and answers with a win or a loss; a capsule
 * rattles while the server draws, builds while it rocks and opens with
 * chimes by rarity. Off with the synced `gemSounds`.
 */
const FUSE = ["gem-tick", "gem-edge", "gem-win", "gem-lose"] as const;
const CAPSULE = ["gem-rattle", "gem-build", "gem-build-huge", "gem-open-common", "gem-open-rare", "gem-open-epic", "gem-open-huge"] as const;
/** Degrees between the dial's pegs (FuseMachine.kt has the same). */
export const PEG = 15;

export function gemSoundsOn(): boolean {
  return useSettings.getState().settings.gemSounds !== false;
}

/** Something playing that a skip, a close or the next phase stops. */
export interface Playing {
  stop(): void;
}

const nothing: Playing = { stop() {} };

/**
 * Starts a set once audio is ready and calls `go` with how late that was in
 * seconds, so what is scheduled keeps to the screen's clock.
 */
function start(names: readonly string[], go: (fx: Effects, late: number) => void | (() => void), always = false): Playing {
  if (!always && !gemSoundsOn()) return nothing;
  const t0 = performance.now();
  let stopped = false;
  let fx: Effects | null = null;
  let more: (() => void) | void;
  void effects(names).then((e) => {
    if (!e || stopped) return;
    fx = e;
    more = go(e, (performance.now() - t0) / 1000);
  });
  return {
    stop() {
      stopped = true;
      more?.();
      fx?.stop();
    },
  };
}

/** Wakes audio from the press that starts a fuse or a capsule (iOS plays nothing otherwise). */
export function primeGemSounds(which: "fuse" | "capsule"): void {
  if (gemSoundsOn()) primeEffects(which === "fuse" ? FUSE : CAPSULE);
}

/**
 * The roll's clicks: one each time `angle` (degrees, by ms into the roll)
 * passes a peg in `total` ms, louder the faster it goes and lower while it
 * winds back; a knock each time it crosses `line`, the line it ends beside,
 * either way, once wound up and on the last lap (null when not close).
 */
export function fuseRoll(angle: (ms: number) => number, total: number, windMs: number, line: number | null): Playing {
  return start(FUSE, (fx, late) => {
    let last = Math.floor(angle(0) / PEG);
    let side = line === null ? 0 : Math.sign(angle(0) - line);
    for (let ms = 0; ms <= total; ms++) {
      const a = angle(ms);
      const at = ms / 1000 - late;
      const peg = Math.floor(a / PEG);
      if (peg !== last) {
        last = peg;
        if (at >= 0) {
          const speed = Math.abs(angle(ms + 1) - a) * 1000;
          const g = 0.35 + 0.4 * Math.min(1, speed / 600);
          const winding = ms < windMs;
          fx.play("gem-tick", at, { gain: winding ? g * 0.6 : g, rate: (winding ? 0.81 : 1) * (0.96 + Math.random() * 0.08), pan: -0.15 });
        }
      }
      if (line !== null && ms >= windMs) {
        const now = Math.sign(a - line);
        if (now !== 0 && now !== side) {
          if (side !== 0 && a > line - 180 && at >= 0) fx.play("gem-edge", at, { gain: 0.8, pan: 0.1 });
          side = now;
        }
      }
    }
  });
}

/** The answer of a fuse. */
export function fuseResult(won: boolean): Playing {
  return start(FUSE, (fx) => fx.play(won ? "gem-win" : "gem-lose"));
}

/** The gem rolling about inside the capsule while the server draws. */
export function capsuleWait(): Playing {
  return start(CAPSULE, (fx) => {
    let k = 0;
    const knock = () => fx.play("gem-rattle", 0, { gain: 0.9, pan: ((k++ * 37) % 7) / 10 - 0.3, rate: 0.95 + Math.random() * 0.1 });
    knock();
    const timer = window.setInterval(knock, 350);
    return () => window.clearInterval(timer);
  });
}

/** The capsule rocking harder as it builds, for the 1.5 s reveal (2.3 s for a Huge). */
export function capsuleBuild(rarity: GemRarity): Playing {
  return start(CAPSULE, (fx, late) => fx.play(rarity === "huge" ? "gem-build-huge" : "gem-build", -late));
}

/** The capsule splitting, with chimes by rarity. */
export function capsuleOpen(rarity: GemRarity): Playing {
  return start(CAPSULE, (fx) => fx.play(`gem-open-${rarity}`));
}

/** What Settings, Sounds plays to hear them, on or off: a dial slowing to a stop, then a win. */
export function previewGemSounds(): Playing {
  primeEffects(FUSE);
  return start(
    FUSE,
    (fx) => {
      let at = 0;
      for (let i = 0; i < 9; i++) {
        fx.play("gem-tick", at, { gain: 0.75 - i * 0.04, rate: 0.96 + Math.random() * 0.08, pan: -0.15 });
        at += 0.06 + i * i * 0.006;
      }
      fx.play("gem-win", at + 0.35);
    },
    true,
  );
}

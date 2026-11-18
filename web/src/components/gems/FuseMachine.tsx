import { useEffect, useRef, useState } from "react";
import { gems as gemsApi, type GemsView } from "@/api/gems";
import type { TradeLine } from "@/api/types";
import { BadgePill } from "@/components/badges/Badges";
import { Button } from "@/components/common/Button";
import { Modal } from "@/components/common/Modal";
import { t } from "@/i18n";
import { describeError } from "@/lib/errors";
import { fuseResult, fuseRoll, primeGemSounds } from "@/lib/gemSounds";
import { motionOff } from "@/lib/motion";
import { useAuth } from "@/stores/auth";
import { useUI } from "@/stores/ui";
import s from "./FuseMachine.module.css";

const MATERIALS = 5;
/** The dial's ring: its radius and length in the 200 box. */
const R = 84;
const RING = 2 * Math.PI * R;
/**
 * The roll, the same on Android (FuseMachine.kt). The marker is pulled back
 * WIND_DEG over WIND_MS, let go, and slows on a cubic over a spin of 3 or 4
 * laps and 4.4 to 5.6 s. Where it stops varies: clear of both lines, close to
 * one (the arc's far edge or the top where it starts), or a teeter, where it
 * stops on the wrong side of a line, hangs there leaning, then creeps over or
 * falls back. It sits still for HOLD_MS before the answer shows.
 */
export const WIND_MS = 450;
export const HOLD_MS = 450;
const WIND_DEG = 12;
/** How long a teeter takes to move from where it hung to where it rests. */
const SETTLE_MS = 800;
/** The least distance between where the marker rests and a line, so its disc never sits on one. */
const CLEAR = 7;
/** The arc's round caps reach this far past its ends (half the 12 wide stroke on the ring of R). */
const CAP_DEG = (6 / R) * (180 / Math.PI);

/** A roll worked out before it starts: every angle is degrees clockwise from the top, counting laps. */
export interface RollPlan {
  /** Where the spin slows to a stop. */
  stop: number;
  /** Where the marker rests: `stop` unless it teeters. */
  end: number;
  spin: number;
  /** How long it hangs at `stop` before moving to `end`; 0 without a teeter. */
  hang: number;
  /** The line it ends up beside, where it is decided, or null when it is not close (or the fuse is sure). */
  line: number | null;
  total: number;
}

const between = (a: number, b: number) => a + Math.random() * (b - a);

/** Plans where a roll goes for an answer the server has already given. */
export function rollPlan(chance: number, won: boolean): RollPlan {
  const laps = (Math.random() < 0.5 ? 3 : 4) * 360;
  const spin = between(4400, 5600);
  const plain = (rest: number, line: number | null): RollPlan => ({ stop: rest, end: rest, spin, hang: 0, line, total: WIND_MS + spin });
  if (chance >= 1) return plain(laps + between(0, 360), null);
  const arc = chance * 360;
  // the side's two lines, in order round the dial: a win lies between the top and the far edge, a loss after it
  const [from, to] = won ? [0, arc] : [arc, 360];
  const kind = Math.random();
  if (kind < 0.35) return plain(laps + between(from + 2 * CLEAR, to - 2 * CLEAR), null);
  // close to one line or the other: `after` when the right side lies past it, clockwise
  const after = Math.random() < 0.5;
  const line = laps + (after ? from : to);
  const side = after ? 1 : -1;
  const rest = line + side * between(CLEAR, CLEAR + 6);
  if (kind < 0.75) return plain(rest, line);
  const stop = line - side * between(6, 10);
  const hang = between(700, 1300);
  return { stop, end: rest, spin, hang, line, total: WIND_MS + spin + hang + SETTLE_MS };
}

/** The marker's angle `ms` into a roll. */
export function angleAt(ms: number, p: RollPlan): number {
  if (ms < WIND_MS) {
    const x = ms / WIND_MS;
    return -WIND_DEG * (x < 0.5 ? 2 * x * x : 1 - (-2 * x + 2) ** 2 / 2);
  }
  let t = ms - WIND_MS;
  if (t < p.spin) return -WIND_DEG + (p.stop + WIND_DEG) * (1 - (1 - t / p.spin) ** 3);
  t -= p.spin;
  if (p.hang <= 0) return p.end;
  const move = p.end - p.stop;
  // it leans toward where it will go and back while it hangs
  if (t < p.hang) return p.stop + move * 0.18 * Math.sin((Math.PI * t) / p.hang);
  t -= p.hang;
  if (t < SETTLE_MS) {
    const x = t / SETTLE_MS;
    return p.stop + move * (x < 0.5 ? 2 * x * x : 1 - (-2 * x + 2) ** 2 / 2);
  }
  return p.end;
}

/** A collectible's key says its material last: `event:<id>-<m>`. */
export function materialOf(key: string): number {
  const m = Number(key.slice(key.lastIndexOf("-") + 1));
  return Number.isFinite(m) ? m : 1;
}

type Phase = "idle" | "waiting" | "rolling" | "won" | "lost";

/**
 * The fuse machine: put 1 to `fuse_cost` copies in, each an equal share of the
 * chance at the next material. The server rolls first; the dial then sweeps the
 * marker round to where that roll lands, so what it shows is what happened.
 */
export function FuseMachine({ line, view, name, onView, onClose }: { line: TradeLine; view: GemsView; name: string; onView: (v: GemsView) => void; onClose: () => void }) {
  const me = useAuth((a) => a.me);
  const toast = useUI((u) => u.toast);
  const cost = view.fuse_cost;
  const held = view.collection.find((l) => l.key === line.key)?.qty ?? 0;
  const [copies, setCopies] = useState(() => Math.max(1, Math.min(cost, held)));
  const [phase, setPhase] = useState<Phase>("idle");
  const [plan, setPlan] = useState<RollPlan | null>(null);
  const [next, setNext] = useState<GemsView | null>(null);
  // the roll's answer, kept for the sweep's end or a press that skips to it
  const pending = useRef<{ won: boolean; v: GemsView } | null>(null);
  const m = materialOf(line.key);
  const chance = copies / cost;
  const pct = Math.round(chance * 100);
  const badge = line.badge ? { ...line.badge, level: m } : undefined;
  const nextBadge = line.badge ? { ...line.badge, level: m + 1 } : undefined;
  const busy = phase === "waiting" || phase === "rolling";
  const marker = useRef<SVGGElement>(null);

  // the roll itself, sampled from angleAt so both apps move alike
  useEffect(() => {
    const el = marker.current;
    if (phase !== "rolling" || !el?.animate) return;
    if (!plan) return;
    const total = plan.total;
    // a frame every 16 ms, so the hang's lean moves smoothly
    const steps = Math.ceil(total / 16);
    const frames = Array.from({ length: steps + 1 }, (_, i) => ({ transform: `rotate(${angleAt((total * i) / steps, plan)}deg)` }));
    const run = el.animate(frames, { duration: total, easing: "linear", fill: "forwards" });
    const sound = fuseRoll((ms) => angleAt(ms, plan), total, WIND_MS, plan.line);
    let hold: number | undefined;
    run.onfinish = () => {
      hold = window.setTimeout(finish, HOLD_MS);
    };
    return () => {
      window.clearTimeout(hold);
      run.cancel();
      sound.stop();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [phase, plan]);

  const settle = (won: boolean, v: GemsView) => {
    setPhase(won ? "won" : "lost");
    fuseResult(won);
    onView(v);
  };

  const fuse = async () => {
    primeGemSounds("fuse");
    setPhase("waiting");
    try {
      const res = await gemsApi.fuse(line.key, copies);
      setNext(res.gems);
      if (motionOff()) {
        settle(res.upgraded, res.gems);
        return;
      }
      pending.current = { won: res.upgraded, v: res.gems };
      setPlan(rollPlan(chance, res.upgraded));
      setPhase("rolling");
    } catch (err) {
      setPhase("idle");
      toast(describeError(err, t("gems.fuseFailed")), "error");
    }
  };
  const finish = () => {
    const p = pending.current;
    if (!p) return;
    pending.current = null;
    settle(p.won, p.v);
  };

  const again = () => {
    const left = (next ?? view).collection.find((l) => l.key === line.key)?.qty ?? 0;
    setCopies(Math.max(1, Math.min(cost, left)));
    setNext(null);
    setPhase("idle");
  };
  const left = phase === "won" || phase === "lost" ? ((next ?? view).collection.find((l) => l.key === line.key)?.qty ?? 0) : held;

  const shown = phase === "won" ? nextBadge : badge;
  const material = (n: number) => t(`gems.material.${n}`);
  const words =
    phase === "won" ? t("gems.fusing.won", { material: material(m + 1), name }) :
    phase === "lost" ? t("gems.fusing.lost", { count: copies }) :
    copies >= cost ? t("gems.fusing.sure", { count: cost }) : t("gems.fusing.warn");

  return (
    <Modal
      open
      onClose={busy ? () => {} : onClose}
      closeButton={!busy}
      title={t("gems.fusing.title", { name })}
      width={420}
      footer={
        phase === "won" || phase === "lost" ? (
          <>
            <Button variant="secondary" onClick={onClose} data-testid="fuse-done">{t("gems.fusing.done")}</Button>
            {left > 0 ? <Button onClick={again} data-testid="fuse-again">{t("gems.fusing.again")}</Button> : null}
          </>
        ) : (
          <>
            <Button variant="secondary" disabled={busy} onClick={onClose}>{t("chat.editMessage.cancel")}</Button>
            <Button loading={busy} disabled={held < 1} onClick={() => void fuse()} data-testid="fuse-go">{t("gems.fusing.go", { percent: pct })}</Button>
          </>
        )
      }
    >
      <div className={s.machine} data-phase={phase}>
        <button
          type="button"
          className={s.dial}
          disabled={phase !== "rolling"}
          onClick={finish}
          aria-label={phase === "rolling" ? t("gems.skip") : undefined}
          tabIndex={phase === "rolling" ? 0 : -1}
          data-testid="fuse-dial"
        >
          <svg viewBox="0 0 200 200" aria-hidden="true" className={s.ring}>
            <circle cx="100" cy="100" r={R} className={s.track} />
            {/* drawn inside its round caps, so the arc seen is exactly the chance */}
            <circle cx="100" cy="100" r={R} className={s.arc} strokeDasharray={chance >= 1 ? `${RING} 0` : `${Math.max(0.01, RING * chance - 12)} ${RING}`} transform={`rotate(${chance >= 1 ? -90 : -90 + CAP_DEG} 100 100)`} />
            <g ref={marker} className={s.marker} style={{ ["--end" as string]: `${plan?.end ?? 0}deg` }} data-testid="fuse-marker">
              <circle cx="100" cy={100 - R} r="10" className={s.markerBack} />
              <circle cx="100" cy={100 - R} r="6" className={s.markerDot} />
            </g>
          </svg>
          <span className={s.centre} key={phase === "won" ? "won" : "was"} data-lost={phase === "lost" || undefined}>
            {shown && me ? <BadgePill badge={shown} user={me} size={76} /> : null}
          </span>
        </button>
        <p className={s.chance} aria-live="polite">
          {phase === "won" || phase === "lost" ? (
            <strong data-testid="fuse-outcome" data-lost={phase === "lost" || undefined}>{phase === "won" ? material(m + 1) : `${pct}%`}</strong>
          ) : (
            <>
              <strong>{pct}%</strong>
              <span>{material(m)} → {material(Math.min(MATERIALS, m + 1))}</span>
            </>
          )}
        </p>
        {phase === "won" || phase === "lost" ? null : (
          <div className={s.copies} role="radiogroup" aria-label={t("gems.fusing.copies")}>
            {Array.from({ length: cost }, (_, i) => {
              const n = i + 1;
              const on = n <= copies;
              return (
                <button
                  key={n}
                  type="button"
                  role="radio"
                  aria-checked={n === copies}
                  aria-label={t("gems.fusing.copyCount", { count: n, percent: Math.round((n / cost) * 100) })}
                  className={s.copy}
                  data-on={on || undefined}
                  disabled={n > held || busy}
                  onClick={() => setCopies(n)}
                  data-testid={`fuse-copies-${n}`}
                >
                  {on && badge && me ? <BadgePill badge={badge} user={me} still size={26} /> : null}
                </button>
              );
            })}
          </div>
        )}
        <p className={s.words} data-tone={phase === "lost" ? "lost" : phase === "won" ? "won" : copies < cost ? "risk" : undefined}>{words}</p>
        {phase === "won" || phase === "lost" ? null : <p className={s.held}>{t("gems.fusing.held", { count: held })}</p>}
      </div>
    </Modal>
  );
}

import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { Check, RefreshCw, Wrench } from "lucide-react";
import { QUEST_KINDS, quests as questsApi, type Quest, type QuestBoard, type QuestDebug, type QuestKind } from "@/api/quests";
import { Button } from "@/components/common/Button";
import { Select } from "@/components/common/Select";
import { TextInput } from "@/components/common/TextInput";
import { GemIcon } from "@/components/gems/GemIcon";
import { Spinner } from "@/components/common/Spinner";
import chat from "@/components/chat/Chat.module.css";
import { t } from "@/i18n";
import { describeError } from "@/lib/errors";
import { motionOff } from "@/lib/motion";
import { serverNow } from "@/lib/serverClock";
import { can, useMyRank } from "@/lib/staffRank";
import { useQuests } from "@/stores/quests";
import { useUI } from "@/stores/ui";
import st from "./Settings.module.css";
import s from "./QuestsPage.module.css";

const fmt = (n: number) => new Intl.NumberFormat(document.documentElement.lang || undefined).format(n);

/** "5 h 12 min" until the daily quests change. */
function untilReset(at: number, now: number): string {
  const mins = Math.max(1, Math.ceil((at - now) / 60000));
  const h = Math.floor(mins / 60);
  const m = mins % 60;
  return h ? t("quests.time.hm", { h: fmt(h), m: fmt(m) }) : t("quests.time.m", { m: fmt(m) });
}

/** What a quest asks, in words. */
export function questText(q: Quest): string {
  return t(`quests.kind.${q.kind}`, { count: q.target });
}

function Reward({ q, fresh }: { q: Quest; fresh: boolean }) {
  const done = q.done_at > 0;
  return (
    <span className={s.reward} data-done={done || undefined} data-fresh={fresh || undefined} data-testid="quest-reward">
      {done ? <Check size={14} strokeWidth={3} aria-hidden="true" /> : <GemIcon size={16} />}
      <span aria-hidden="true">{fmt(q.reward)}</span>
      <span className="visually-hidden">{done ? t("quests.done") : ""} {t("gems.count", { count: q.reward })}</span>
    </span>
  );
}

function Progress({ q }: { q: Quest }) {
  const done = q.done_at > 0;
  const shown = done ? q.target : Math.min(q.progress, q.target);
  return (
    <div className={s.progress}>
      <div
        className={s.track}
        role="progressbar"
        aria-valuemin={0}
        aria-valuemax={q.target}
        aria-valuenow={shown}
        aria-label={questText(q)}
        data-done={done || undefined}
      >
        <div className={s.fill} style={{ width: `${q.target ? (shown / q.target) * 100 : 0}%` }} />
      </div>
      <span className={s.amount}>{done ? t("quests.done") : t("quests.progress", { progress: fmt(shown), target: fmt(q.target) })}</span>
    </div>
  );
}

/** The climb's beats: how long a finished tower step shows full, then the line drawing up to the next rung, then the
 * view climbing to the next step's frame (Android's are the same). */
const CLIMB_HOLD_MS = 900;
const CLIMB_DRAW_MS = 240;
const CLIMB_PAN_MS = 560;
const CLIMB_EASE = "cubic-bezier(0.5, 0, 0.2, 1)";

/** Runs one request that answers the board, and says whether it worked. */
type Act = (key: string, run: () => Promise<QuestBoard>) => Promise<boolean>;

/** The owner's tools under one open quest: add to it, finish it, or make it another quest. */
function DebugStrip({ q, busy, act }: { q: Quest; busy: string | null; act: Act }) {
  const [changing, setChanging] = useState(false);
  const [kind, setKind] = useState<QuestKind>(q.kind);
  const [target, setTarget] = useState(String(q.target));
  const debug = (key: string, body: QuestDebug) => act(`debug:${q.id}:${key}`, () => questsApi.debug(body));
  const off = busy !== null;
  const n = Math.max(1, Math.floor(Number(target) || 0));
  if (changing) {
    return (
      <form
        className={s.debug}
        onSubmit={(e) => {
          e.preventDefault();
          void debug("set", { action: "set", quest_id: q.id, kind, target: n }).then((ok) => ok && setChanging(false));
        }}
        data-testid="quest-debug-change"
      >
        <Select className={s.debugKind} value={kind} onChange={(e) => setKind(e.target.value as QuestKind)} aria-label={t("quests.debug.kind")}>
          {QUEST_KINDS.map((k) => <option key={k} value={k}>{t(`quests.kind.${k}`, { count: n })}</option>)}
        </Select>
        <TextInput
          className={s.debugNumber}
          type="number"
          inputMode="numeric"
          min={1}
          value={target}
          onChange={(e) => setTarget(e.target.value)}
          aria-label={t("quests.debug.target")}
        />
        <span className={s.debugEnd}>
          <Button type="button" variant="ghost" size="sm" onClick={() => setChanging(false)}>{t("quests.debug.cancel")}</Button>
          <Button type="submit" size="sm" loading={busy === `debug:${q.id}:set`} disabled={off}>{t("quests.debug.save")}</Button>
        </span>
      </form>
    );
  }
  return (
    <div className={s.debug} data-testid="quest-debug">
      <Wrench className={s.debugMark} size={14} aria-hidden="true" />
      {[1, 10].map((a) => (
        <Button
          key={a}
          variant="secondary"
          size="sm"
          className={s.debugAdd}
          aria-label={t("quests.debug.add", { n: fmt(a) })}
          loading={busy === `debug:${q.id}:${a}`}
          disabled={off}
          onClick={() => void debug(String(a), { action: "progress", quest_id: q.id, amount: a })}
        >
          +{fmt(a)}
        </Button>
      ))}
      <Button
        variant="secondary"
        size="sm"
        loading={busy === `debug:${q.id}:finish`}
        disabled={off}
        onClick={() => void debug("finish", { action: "progress", quest_id: q.id, amount: Math.max(1, q.target - q.progress) })}
        data-testid="quest-debug-finish"
      >
        {t("quests.debug.finish")}
      </Button>
      <Button
        variant="ghost"
        size="sm"
        disabled={off}
        onClick={() => {
          setKind(q.kind);
          setTarget(String(q.target));
          setChanging(true);
        }}
      >
        {t("quests.debug.change")}
      </Button>
    </div>
  );
}

/** Settings, Quests: today's three quests and their bonus, and the Quest Tower. */
export default function QuestsPage() {
  const { board, error, load, set } = useQuests();
  const toast = useUI((u) => u.toast);
  const [now, setNow] = useState(() => serverNow());
  const [busy, setBusy] = useState<string | null>(null);
  const owner = can(useMyRank(), "ownerTools");
  const [debugging, setDebugging] = useState(false);
  const [step, setStep] = useState("");
  // quests already finished when the page opened draw still; one that finishes while it is open pops its reward once
  const [doneAtOpen] = useState(() => new Set<string>());
  const [seeded, setSeeded] = useState(false);
  if (board && !seeded) {
    for (const q of [...board.daily, board.bonus, board.trial]) if (q && q.done_at) doneAtOpen.add(q.id);
    setSeeded(true);
  }
  const fresh = (q: Quest) => q.done_at > 0 && !doneAtOpen.has(q.id);

  // the climb: a tower step finished while the page is open fills and pops where it stands, the line draws up to the
  // ghost rung above, and the view climbs the tower to the next step's own frame, the finished one going down out of
  // sight. Decided while rendering, so the finished step never shows its old state for a frame; a swap or a jump
  // is instant.
  const [shown, setShown] = useState<Quest | null>(null);
  const [climb, setClimb] = useState<{ from: Quest; phase: "hold" | "draw" | "pan" } | null>(null);
  const trialNow = board?.trial ?? null;
  if (trialNow !== shown) {
    if (shown && trialNow && trialNow.id !== shown.id) {
      setClimb(trialNow.level === shown.level + 1 && !motionOff()
        ? { from: { ...shown, done_at: shown.done_at || 1 }, phase: "hold" }
        : null);
    }
    setShown(trialNow);
  }
  const phase = climb?.phase;
  useEffect(() => {
    if (phase !== "hold" && phase !== "draw") return;
    const id = window.setTimeout(
      () => setClimb((c) => (c ? { ...c, phase: c.phase === "hold" ? "draw" : "pan" } : c)),
      phase === "hold" ? CLIMB_HOLD_MS : CLIMB_DRAW_MS,
    );
    return () => window.clearTimeout(id);
  }, [phase]);
  const ladderRef = useRef<HTMLDivElement>(null);
  const trackRef = useRef<HTMLDivElement>(null);
  const ghostRef = useRef<HTMLDivElement>(null);
  const nextRef = useRef<HTMLDivElement>(null);
  const linkRef = useRef<HTMLDivElement>(null);
  const foldRef = useRef<HTMLDivElement>(null);
  const clearedRef = useRef<HTMLDivElement>(null);
  // the pan, started before its first frame is painted: the ladder is a window onto the tower, which slides down by
  // the next frame and its rung so the view climbs from the finished frame to the next, while the window's height
  // eases from the one to the other. The rung between them goes from "Step n" to "n - 1 steps cleared" on the way.
  useLayoutEffect(() => {
    const ladder = ladderRef.current, track = trackRef.current, ghost = ghostRef.current, next = nextRef.current;
    const link = linkRef.current, fold = foldRef.current, done = clearedRef.current;
    if (phase !== "pan" || !ladder || !track || !ghost || !next || !link || !fold || !done) return;
    // each holds its last frame until the climb ends and they are cancelled, or the tower would jump back for the
    // frame between its animation ending and the state that settles it
    const timing: KeyframeAnimationOptions = { duration: CLIMB_PAN_MS, easing: CLIMB_EASE, fill: "forwards" };
    const shift = ghost.offsetHeight + next.offsetHeight;
    const before = link.offsetHeight + fold.offsetHeight + done.offsetHeight;
    const after = ghost.offsetHeight + next.offsetHeight + link.offsetHeight;
    const [was, now] = link.querySelectorAll("[data-label]");
    const runs = [
      track.animate([{ transform: `translateY(${-shift}px)` }, { transform: "translateY(0)" }], timing),
      ladder.animate([{ height: `${before}px` }, { height: `${after}px` }], timing),
      ...(was && now
        ? [
            was.animate([{ opacity: 1 }, { opacity: 0, offset: 0.4 }, { opacity: 0 }], timing),
            now.animate([{ opacity: 0 }, { opacity: 0, offset: 0.6 }, { opacity: 1 }], timing),
          ]
        : []),
    ];
    let live = true;
    void runs[0]!.finished.then(() => live && setClimb(null), () => undefined);
    return () => {
      live = false;
      for (const r of runs) r.cancel();
    };
  }, [phase]);

  useEffect(() => {
    void load();
  }, [load]);
  // the countdown, and a new board once the day turns
  useEffect(() => {
    const id = window.setInterval(() => setNow(serverNow()), 30_000);
    return () => window.clearInterval(id);
  }, []);
  const resetsAt = board?.resets_at ?? 0;
  useEffect(() => {
    if (resetsAt && now >= resetsAt) void load();
  }, [now, resetsAt, load]);

  const act: Act = async (key: string, run: () => Promise<QuestBoard>) => {
    setBusy(key);
    try {
      set(await run());
      return true;
    } catch (err) {
      toast(describeError(err, t("quests.actionFailed")), "error");
      return false;
    } finally {
      setBusy(null);
    }
  };

  if (error && !board) {
    return (
      <>
        <h2 className={st.title}>{t("quests.title")}</h2>
        <p className={s.note}>
          {t("quests.failed")}{" "}
          <button type="button" className={chat.loadRetry} onClick={() => void load()}>{t("chat.messageList.tryAgain")}</button>
        </p>
      </>
    );
  }
  if (!board) {
    return (
      <>
        <h2 className={st.title}>{t("quests.title")}</h2>
        <div className={s.loading}><Spinner size={24} label={t("chat.pins.loading")} /></div>
      </>
    );
  }

  const { daily, bonus, trial } = board;
  const doneCount = daily.filter((q) => q.done_at > 0).length;
  const debug = owner && debugging;
  // one tower step's contents, drawn in the window's lane; a finished step keeps no controls
  const controls = (q: Quest, held: boolean) => (
    <>
          {board.skip_left && (held || !q.done_at) ? (
            <Button
              variant="secondary"
              size="sm"
              className={s.skip}
              loading={busy === "trial"}
              disabled={busy !== null && busy !== "trial"}
              onClick={() => void act("trial", () => questsApi.skipTrial())}
              data-testid="quest-skip"
            >
              {t("quests.trial.skip")}
            </Button>
          ) : null}
          {debug && (held || !q.done_at) ? <DebugStrip key={`${q.id}:${q.kind}:${q.target}`} q={q} busy={busy} act={act} /> : null}
    </>
  );
  // one tower step's contents in its frame. A step finished during a climb keeps the controls it had, out of sight and
  // out of reach, so its frame holds its height as the view climbs away from it.
  const stepLayer = (q: Quest, held = false) => (
    <>
      <strong className={s.step} aria-label={t("quests.trial.step", { step: fmt(q.level) })}>{fmt(q.level)}</strong>
      <div className={s.body}>
        <div className={s.ask}>
          <p className={s.text}>{questText(q)}</p>
          <Reward q={q} fresh={fresh(q)} />
        </div>
        <Progress q={q} />
        {held ? (
          <div className={s.held} aria-hidden="true">
            {controls(q, true)}
          </div>
        ) : controls(q, false)}
      </div>
    </>
  );
  const from = climb && trial && climb.from.id !== trial.id ? climb.from : null;
  const panning = !!from && phase === "pan";
  // the ghost rung names the step above the finished one until the view climbs; the rung under the finished frame
  // keeps its count while it goes down out of sight
  const at = from && !panning ? from : trial;
  const cleared = from ? Math.max(0, from.level - 1) : board.trial_cleared;
  const clearedText = (n: number) => (n ? t("quests.trial.cleared", { count: n }) : t("quests.trial.first"));
  const goTo = Math.floor(Number(step) || 0);

  return (
    <>
      <div className={s.titleRow}>
        <h2 className={st.title}>{t("quests.title")}</h2>
        {owner ? (
          <Button
            variant={debugging ? "primary" : "secondary"}
            size="sm"
            aria-pressed={debugging}
            onClick={() => setDebugging((d) => !d)}
            data-testid="quests-debug-toggle"
          >
            <Wrench size={14} aria-hidden="true" /> {t("quests.debug.toggle")}
          </Button>
        ) : null}
      </div>
      <p className={s.note}>{t("quests.hint")}</p>
      {debug ? <p className={`${s.note} ${s.debugHint}`}>{t("quests.debug.hint")}</p> : null}

      <section className={s.section} data-testid="quests-daily">
        <div className={s.head}>
          <h3 className={`${st.h5} ${s.sectionTitle}`}>{t("quests.daily.title")}</h3>
          <span className={s.reset} data-testid="quests-reset">{t("quests.resetsIn", { time: untilReset(board.resets_at, now) })}</span>
        </div>
        <ol className={s.list}>
          {daily.map((q) => {
            const done = q.done_at > 0;
            return (
              <li key={q.id} className={s.row} data-done={done || undefined} data-testid="quest-daily">
                <div className={s.top}>
                  <span className={s.level}>{t(`quests.level.${q.level}`)}</span>
                  <span className={s.ends}>
                    {!done && board.reroll_left ? (
                      <button
                        type="button"
                        className={s.swap}
                        title={t("quests.swap")}
                        aria-label={`${t("quests.swap")}: ${questText(q)}`}
                        disabled={busy !== null}
                        aria-busy={busy === q.id || undefined}
                        onClick={() => void act(q.id, () => questsApi.reroll(q.id))}
                        data-testid="quest-swap"
                      >
                        {busy === q.id ? <Spinner size={14} /> : <RefreshCw size={16} aria-hidden="true" />}
                      </button>
                    ) : null}
                    <Reward q={q} fresh={fresh(q)} />
                  </span>
                </div>
                <p className={s.text}>{questText(q)}</p>
                <Progress q={q} />
                {debug && !done ? <DebugStrip key={`${q.kind}:${q.target}`} q={q} busy={busy} act={act} /> : null}
              </li>
            );
          })}
          {bonus ? (
            <li className={`${s.row} ${s.bonus}`} data-done={bonus.done_at > 0 || undefined} data-testid="quest-bonus">
              <div className={s.bonusLine}>
                <span className={s.pips} role="img" aria-label={t("quests.progress", { progress: fmt(doneCount), target: fmt(daily.length) })}>
                  {daily.map((q) => <i key={q.id} data-on={q.done_at > 0 || undefined} />)}
                </span>
                <p className={s.text}>{t("quests.bonus", { count: bonus.reward })}</p>
                <Reward q={bonus} fresh={fresh(bonus)} />
              </div>
            </li>
          ) : null}
        </ol>
        {board.reroll_left && daily.some((q) => !q.done_at) ? <p className={s.note}>{t("quests.swapHint")}</p> : null}
        {debug ? (
          <div className={s.debugFoot} data-testid="quests-debug-day">
            <Button variant="secondary" size="sm" loading={busy === "debug:day"} disabled={busy !== null}
              onClick={() => void act("debug:day", () => questsApi.debug({ action: "new_day" }))}>
              {t("quests.debug.newDay")}
            </Button>
            <Button variant="secondary" size="sm" loading={busy === "debug:uses"} disabled={busy !== null}
              onClick={() => void act("debug:uses", () => questsApi.debug({ action: "uses" }))}>
              {t("quests.debug.resetUses")}
            </Button>
          </div>
        ) : null}
      </section>

      {trial ? (
        <section className={s.section} data-testid="quests-trial">
          <h3 className={`${st.h5} ${s.sectionTitle}`}>{t("quests.trial.title")}</h3>
          <p className={`${s.note} ${s.trialHint}`}>{t("quests.trial.hint")}</p>
          <div ref={ladderRef} className={s.ladder} data-panning={panning ? "" : undefined}>
            <div ref={trackRef} className={s.tower}>
              <div
                key="next"
                ref={ghostRef}
                className={s.rung}
                data-next=""
                data-drawing={(from && phase === "draw") || undefined}
                aria-hidden="true"
              >
                <span className={s.rail} />
                <span>{t("quests.trial.step", { step: fmt((at?.level ?? 0) + 1) })}</span>
              </div>
              {from && !panning ? null : (
                <div key={trial.id} ref={nextRef} className={s.window} data-testid="quest-trial">
                  {stepLayer(trial)}
                </div>
              )}
              {from && panning ? (
                <div key="link" ref={linkRef} className={s.rung} aria-hidden="true">
                  <span className={s.rail} />
                  <span className={s.labels}>
                    <span data-label="">{t("quests.trial.step", { step: fmt(from.level + 1) })}</span>
                    <span data-label="">{clearedText(board.trial_cleared)}</span>
                  </span>
                </div>
              ) : null}
              {/* keyed by step, so the finished frame keeps its filled bar and popped reward as the view climbs away */}
              {from ? (
                <div key={from.id} ref={foldRef} className={s.window} data-left={panning ? "" : undefined} aria-hidden={panning || undefined}
                  data-testid={panning ? undefined : "quest-trial"}>
                  {stepLayer(from, true)}
                </div>
              ) : null}
              <div key="cleared" ref={clearedRef} className={s.rung} data-done="">
                <span className={s.rail} />
                <span data-testid={panning ? undefined : "quests-cleared"}>{clearedText(cleared)}</span>
              </div>
            </div>
          </div>
          {board.skip_left && at && !at.done_at ? <p className={s.note}>{t("quests.trial.skipHint")}</p> : null}
          {debug ? (
            <form
              className={s.debugFoot}
              onSubmit={(e) => {
                e.preventDefault();
                if (goTo >= 1) void act("debug:step", () => questsApi.debug({ action: "step", step: goTo })).then((ok) => ok && setStep(""));
              }}
              data-testid="quests-debug-step"
            >
              <TextInput
                className={s.debugNumber}
                type="number"
                inputMode="numeric"
                min={1}
                max={100000}
                placeholder={fmt(trial.level)}
                value={step}
                onChange={(e) => setStep(e.target.value)}
                aria-label={t("quests.debug.step")}
              />
              <Button type="submit" variant="secondary" size="sm" loading={busy === "debug:step"} disabled={busy !== null || goTo < 1}>
                {t("quests.debug.goToStep")}
              </Button>
            </form>
          ) : null}
        </section>
      ) : null}
    </>
  );
}

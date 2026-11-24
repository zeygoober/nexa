import { useEffect, useState } from "react";
import { Check, RefreshCw } from "lucide-react";
import { quests as questsApi, type Quest, type QuestBoard } from "@/api/quests";
import { Button } from "@/components/common/Button";
import { GemIcon } from "@/components/gems/GemIcon";
import { Spinner } from "@/components/common/Spinner";
import chat from "@/components/chat/Chat.module.css";
import { t } from "@/i18n";
import { describeError } from "@/lib/errors";
import { serverNow } from "@/lib/serverClock";
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

/** Settings, Quests: today's three quests and their bonus, and the Quest Tower. */
export default function QuestsPage() {
  const { board, error, load, set } = useQuests();
  const toast = useUI((u) => u.toast);
  const [now, setNow] = useState(() => serverNow());
  const [busy, setBusy] = useState<string | null>(null);
  // quests already finished when the page opened draw still; one that finishes while it is open pops its reward once
  const [doneAtOpen] = useState(() => new Set<string>());
  const [seeded, setSeeded] = useState(false);
  if (board && !seeded) {
    for (const q of [...board.daily, board.bonus, board.trial]) if (q && q.done_at) doneAtOpen.add(q.id);
    setSeeded(true);
  }
  const fresh = (q: Quest) => q.done_at > 0 && !doneAtOpen.has(q.id);

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

  const act = async (key: string, run: () => Promise<QuestBoard>) => {
    setBusy(key);
    try {
      set(await run());
    } catch (err) {
      toast(describeError(err, t("quests.actionFailed")), "error");
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

  return (
    <>
      <h2 className={st.title}>{t("quests.title")}</h2>
      <p className={s.note}>{t("quests.hint")}</p>

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
      </section>

      {trial ? (
        <section className={s.section} data-testid="quests-trial">
          <h3 className={`${st.h5} ${s.sectionTitle}`}>{t("quests.trial.title")}</h3>
          <p className={`${s.note} ${s.trialHint}`}>{t("quests.trial.hint")}</p>
          <div className={s.ladder}>
            <div className={s.rung} data-next="" aria-hidden="true">
              <span className={s.rail} />
              <span>{t("quests.trial.step", { step: fmt(trial.level + 1) })}</span>
            </div>
            <div className={s.window} data-testid="quest-trial">
              <strong className={s.step} aria-label={t("quests.trial.step", { step: fmt(trial.level) })}>{fmt(trial.level)}</strong>
              <div className={s.body}>
                <div className={s.ask}>
                  <p className={s.text}>{questText(trial)}</p>
                  <Reward q={trial} fresh={fresh(trial)} />
                </div>
                <Progress q={trial} />
                {board.skip_left && !trial.done_at ? (
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
              </div>
            </div>
            <div className={s.rung} data-done="">
              <span className={s.rail} />
              <span data-testid="quests-cleared">
                {board.trial_cleared ? t("quests.trial.cleared", { count: board.trial_cleared }) : t("quests.trial.first")}
              </span>
            </div>
          </div>
          {board.skip_left && !trial.done_at ? <p className={s.note}>{t("quests.trial.skipHint")}</p> : null}
        </section>
      ) : null}
    </>
  );
}

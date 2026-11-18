import { useState } from "react";
import { reports, type ReportDetail } from "@/api/reports";
import { Button } from "@/components/common/Button";
import { TextArea } from "@/components/common/TextInput";
import { t } from "@/i18n";
import { describeError } from "@/lib/errors";
import { formatShortDate } from "@/lib/time";
import { useAuth } from "@/stores/auth";
import { useUI } from "@/stores/ui";
import s from "./Reports.module.css";
import g from "./GemFarming.module.css";

/** One UTC day of the timing a farming check keeps (store.GemWatchDay): counts only, never what was said. */
interface WatchDay {
  day: number;
  earns: number;
  quick: number;
  alone: number;
  hours: number;
  sizes: number;
  score: number;
}
interface Farming {
  days?: WatchDay[];
  held?: number;
  earned?: number;
  alts?: number;
}

export const isGemFarming = (d: ReportDetail) => d.report.reason === "gem_farming";
const farming = (d: ReportDetail) => (d.report.snapshot["gem_farming"] ?? {}) as Farming;
const DAY = 86_400_000;

/** What a Gem Farming report shows staff: the held Gems, linked accounts and each watched day's timing. */
export function GemFarmingSummary({ detail }: { detail: ReportDetail }) {
  const f = farming(detail);
  const days = [...(f.days ?? [])].sort((a, b) => b.day - a.day);
  return (
    <div className={g.summary} data-testid="gem-farming-summary">
      <p className={s.muted}>{t("gems.report.about", { held: f.held ?? 0, earned: f.earned ?? 0, alts: f.alts ?? 0 })}</p>
      {days.length ? (
        <div className={g.scroll}>
          <table className={g.table}>
            <thead>
              <tr>
                <th scope="col">{t("gems.report.day")}</th>
                <th scope="col">{t("gems.report.earns")}</th>
                <th scope="col">{t("gems.report.quick")}</th>
                <th scope="col">{t("gems.report.alone")}</th>
                <th scope="col">{t("gems.report.hours")}</th>
                <th scope="col">{t("gems.report.sizes")}</th>
                <th scope="col">{t("gems.report.score")}</th>
              </tr>
            </thead>
            <tbody>
              {days.map((d) => (
                <tr key={d.day}>
                  <th scope="row">{formatShortDate(d.day * DAY)}</th>
                  <td>{d.earns}</td>
                  <td>{d.quick}</td>
                  <td>{d.alone}</td>
                  <td>{d.hours}</td>
                  <td>{d.sizes}</td>
                  <td>{d.score}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
      <p className={s.muted}>{t("gems.report.source")}</p>
    </div>
  );
}

/** Take Action on a Gem Farming report: the held Gems go, earning stops for 30 days, and the member gets a case to appeal. */
export function GemFarmingPanel({ detail, onCancel, onDone }: { detail: ReportDetail; onCancel: () => void; onDone: () => void }) {
  const v = detail.report;
  const me = useAuth((a) => a.me?.id ?? "0");
  const subject = detail.subject;
  const name = subject ? subject.display_name || subject.username : "";
  const [reason, setReason] = useState(() => t("reports.reason.gem_farming"));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const ok = reason.trim().length >= 3;

  const submit = async () => {
    if (busy || !ok) return;
    setBusy(true);
    setError("");
    try {
      await reports.review(v.id, { version: v.version, status: "actioned", note: reason.trim(), assigned_to: v.assigned_to === "0" ? me : v.assigned_to, reporter_message: "", resolve_related: false });
      useUI.getState().toast(t("gems.report.done"), "success");
      onDone();
    } catch (err) {
      setError(describeError(err, t("reports.failed")));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className={s.decide} data-kind="act" data-testid="gem-farming-panel">
      <h3 className={s.decideTitle}>{t("reports.act.takeAction")}</h3>
      <label className={s.decideField}>
        <span className={s.decideLabel}>{t("gems.report.reason")}</span>
        <TextArea value={reason} onChange={(e) => setReason(e.target.value)} rows={2} maxLength={1000} disabled={busy} data-testid="gem-farming-reason" />
      </label>
      <ul className={s.willList} aria-label={t("reports.act.whatHappens")}>
        <li>{t("gems.report.willTake", { count: farming(detail).held ?? 0 })}</li>
        <li>{t("gems.report.willStop", { name })}</li>
        <li>{t("gems.report.willCase", { name })}</li>
      </ul>
      {error ? <p role="alert" className={s.error}>{error}</p> : null}
      <div className={s.decideFoot}>
        <Button variant="ghost" onClick={onCancel} disabled={busy}>{t("reports.decision.cancel")}</Button>
        <Button variant="danger" loading={busy} disabled={!ok} onClick={() => void submit()} data-testid="gem-farming-confirm">{t("gems.report.confirm")}</Button>
      </div>
    </div>
  );
}

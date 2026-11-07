import { ChevronDown, Play } from "lucide-react";
import { useId, useState } from "react";
import type { BugContext, BugFile, BugKind, BugStatus } from "@/api/bugs";
import { useBugFileURL } from "@/lib/bugMedia";
import { bugStatusLabel, bugStatusTone, contextRows, isBugVideo } from "@/lib/bugs";
import { t } from "@/i18n";
import s from "./Bugs.module.css";

/** A report's status, as a small coloured label. */
export function BugStatusChip({ status, kind, className }: { status: BugStatus; kind?: BugKind | undefined; className?: string | undefined }) {
  return (
    <span className={[s.status, className ?? ""].join(" ")} data-tone={bugStatusTone(status) || undefined} data-status={status}>
      {bugStatusLabel(status, kind)}
    </span>
  );
}

/** The facts a report came with, folded away until asked for. */
export function BugFacts({ context }: { context: BugContext }) {
  const [open, setOpen] = useState(false);
  const id = useId();
  const rows = contextRows(context);
  if (!rows.length) return <p className={s.hint}>{t("bugs.card.noDetails")}</p>;
  return (
    <div className={s.details}>
      <button type="button" className={s.disclose} aria-expanded={open} aria-controls={id} onClick={() => setOpen((v) => !v)} data-testid="bug-card-details">
        {open ? t("bugs.card.hideDetails") : t("bugs.card.showDetails")}
        <ChevronDown size={15} aria-hidden="true" />
      </button>
      {open ? (
        <dl className={s.facts} id={id}>
          {rows.map((r) => (
            <FactRow key={r.key} label={r.label} value={r.value} />
          ))}
        </dl>
      ) : null}
    </div>
  );
}

function FactRow({ label, value }: { label: string; value: string }) {
  return (
    <>
      <dt>{label}</dt>
      <dd dir="auto">{value}</dd>
    </>
  );
}

/** A small strip of a report's files, for the rows of a list. */
export function BugThumbStrip({ files, max = 3 }: { files: BugFile[]; max?: number }) {
  const shown = files.slice(0, max);
  const more = files.length - shown.length;
  return (
    <span className={s.strip} aria-label={t("bugs.card.files", { count: files.length })}>
      {shown.map((f) => (isBugVideo(f.mime) ? <span key={f.id} className={[s.stripItem, s.stripVideo].join(" ")} aria-hidden="true"><Play size={12} /></span> : <StripThumb key={f.id} file={f} />))}
      {more > 0 ? <span className={[s.stripItem, s.stripMore].join(" ")} aria-hidden="true">+{more}</span> : null}
    </span>
  );
}

function StripThumb({ file }: { file: BugFile }) {
  const { url } = useBugFileURL(file.id, 64);
  return <span className={s.stripItem} aria-hidden="true">{url ? <img src={url} alt="" draggable={false} /> : null}</span>;
}

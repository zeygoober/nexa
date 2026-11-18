import { Clock, MessagesSquare, Server, Sticker } from "lucide-react";
import { useLayoutEffect, useRef, useState } from "react";
import type { CaseDetails, MemberCase, PackField, People, ResetField, ServerField, StaffCase } from "@/api/moderation";
import { Button } from "@/components/common/Button";
import { useRenderCtx } from "@/lib/markdownCtx";
import { actionIcon, caseStanding, fieldLabel, formatSpan, fullDate, noticeExplainer, noticeTitle, packFieldLabel, relativeTo, severity } from "@/lib/moderation";
import { formatBytes, formatTimestamp } from "@/lib/time";
import { Markdown } from "@/markdown";
import { t } from "@/i18n";
import { PersonChip } from "./NexaBits";
import s from "./Nexa.module.css";

interface Props {
  c: MemberCase | StaffCase;
  people: People;
  /** Staff see the values a reset took away and whether the notice was read. */
  perspective: "member" | "staff";
  /** Shows the button that opens the case's thread. */
  onOpenThread?: () => void;
  /** Leaves out the headline, for where the page around it already says it. */
  bare?: boolean;
}

function isStaffCase(c: MemberCase | StaffCase): c is StaffCase {
  return "user_id" in c;
}

type AnyField = ResetField | ServerField | PackField;

/** Fields whose words are formatted where people read them, so they are shown formatted here too. */
const RICH = new Set<AnyField>(["about_me", "description"]);

/**
 * What a field said before a reset took it, for the staff: in full, as
 * people saw it, folding after a few lines until asked for the rest.
 */
function WasValue({ field, value }: { field: AnyField; value: string }) {
  const ctx = useRenderCtx();
  const ref = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  const [folds, setFolds] = useState(false);
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el || open) return;
    setFolds(el.scrollHeight > el.clientHeight + 1);
  }, [value, open]);
  return (
    <div className={s.was}>
      <span className={s.wasLabel}>{t("nexa.notice.was")}</span>
      <div ref={ref} className={s.wasValue} data-open={open} data-rich={RICH.has(field) || undefined}>
        {RICH.has(field) ? <Markdown source={value} ctx={ctx} /> : field === "username" ? `@${value}` : value}
      </div>
      {folds || open ? (
        <button type="button" className={s.wasMore} aria-expanded={open} onClick={() => setOpen((v) => !v)}>
          {open ? t("chat.embeds.showLess") : t("nexa.notice.showAll")}
        </button>
      ) : null}
    </div>
  );
}

/** What a field reads now: the stand-in a reset wrote, or that it was removed or cleared. */
function nowValue(f: AnyField, d: CaseDetails): string {
  switch (f) {
    case "public":
      return t("nexa.notice.madePrivate");
    case "status":
    case "nicknames":
    case "server_profiles":
    case "description":
      return t("nexa.notice.cleared");
    case "icon":
    case "banner":
      return t("nexa.notice.removedShort");
    case "discovery":
      return t("nexa.notice.hiddenShort");
    case "username":
      return `@${d.new?.username ?? ""}`;
    default:
      return d.new?.[f] ?? "";
  }
}

/** The fields a reset took, each with what it says now, and, for the staff, what it said before. */
function ChangedFields({ d, staff, label, labelOf = (f) => fieldLabel(f as ResetField | ServerField) }: { d: CaseDetails; staff: boolean; label: string; labelOf?: (f: AnyField) => string }) {
  if (!d.fields?.length) return null;
  return (
    <div className={s.fields} role="table" aria-label={label}>
      {d.fields.map((f) => (
        <div className={s.fieldRow} role="row" key={f}>
          <span className={s.fieldName} role="cell">{labelOf(f)}</span>
          <span className={s.fieldValue} role="cell">
            <span className={s.newValue} data-word={f === "status" || f === "nicknames" || f === "server_profiles" || f === "description" || f === "icon" || f === "banner" || f === "public" || f === "discovery" || undefined}>{nowValue(f, d)}</span>
            {staff && d.old?.[f] ? <WasValue field={f} value={d.old[f]!} /> : null}
          </span>
        </div>
      ))}
    </div>
  );
}

/**
 * A case, as its notice: what was done, why, by whom, and where it stands.
 * The same card is the Nexa message, the pop-up, the ban screen and the top of
 * the case's thread, so the member reads the same thing wherever they meet it.
 */
export function NoticeCard({ c, people, perspective, onOpenThread, bare = false }: Props) {
  const Icon = actionIcon(c.action);
  const tone = severity(c.action);
  const standing = caseStanding(c);
  const d = c.details;
  const staff = perspective === "staff";
  return (
    <article className={s.notice} data-tone={tone} aria-label={noticeTitle(c.action, d)}>
      {!bare ? (
        <header className={s.noticeHead}>
          <span className={s.noticeIcon} data-tone={tone} aria-hidden="true">
            <Icon size={18} />
          </span>
          <div className={s.noticeTitles}>
            <h3 className={s.noticeTitle}>{noticeTitle(c.action, d)}</h3>
            <p className={s.noticeExplainer}>{noticeExplainer(c)}</p>
          </div>
          {standing ? (
            <span className={s.chip} data-tone={standing.tone}>
              {standing.label}
            </span>
          ) : null}
        </header>
      ) : null}

      <section className={s.reason}>
        <span className={s.label}>{t("nexa.notice.reason")}</span>
        <p className={s.reasonText} dir="auto">{c.reason}</p>
      </section>

      {c.action === "avatar_remove" ? (
        <p className={s.detailLine}>
          {t("nexa.notice.removed", { what: [d.avatar ? t("moderation.field.avatar") : "", d.banner ? t("moderation.field.banner") : ""].filter(Boolean).join(", ") })}
        </p>
      ) : null}

      {c.action === "field_reset" ? <ChangedFields d={d} staff={staff} label={t("nexa.notice.resetFields")} /> : null}

      {c.action === "server_reset" ? (
        <>
          {d.server_name ? (
            <p className={s.detailLine}>
              <Server size={14} aria-hidden="true" />
              <span>{t("nexa.notice.serverNamed", { name: d.server_name })}</span>
            </p>
          ) : null}
          <ChangedFields d={d} staff={staff} label={t("nexa.notice.serverFields")} />
        </>
      ) : null}

      {c.action === "expression_takedown" ? <TakenDownParts d={d} staff={staff} /> : null}

      {c.action === "field_reset" && d.fields?.includes("username") && d.new?.username ? (
        <p className={s.callout}>
          {staff
            ? t("nexa.notice.signInNoteStaff", { old: `@${d.old?.username ?? ""}` })
            : t("nexa.notice.signInNote", { next: d.new.username, old: d.old?.username ?? "" })}
        </p>
      ) : null}

      {c.action === "mute" || c.action === "ban" || c.action === "gem_farming" ? (
        <p className={s.detailLine}>
          <Clock size={14} aria-hidden="true" />
          {c.expires_at === null
            ? t("nexa.notice.permanent")
            : t("nexa.notice.duration", { span: d.duration_ms ? formatSpan(d.duration_ms) : relativeTo(c.expires_at), until: fullDate(c.expires_at) })}
        </p>
      ) : null}

      {c.action === "delete" && d.counts && staff ? (
        <p className={s.detailLine}>
          {t("moderation.delete.removedSummary", { messages: d.counts.messages, files: d.counts.attachments, size: formatBytes(d.counts.attachment_bytes), servers: d.counts.owned_servers, groups: d.counts.owned_groups })}
          {d.complete === false ? ` ${t("moderation.delete.incomplete")}` : ""}
        </p>
      ) : null}

      {c.lifted_at ? (
        <div className={s.lifted}>
          <p className={s.liftedLine}>
            {t("nexa.notice.liftedBy")} {c.lifted_by ? <PersonChip id={c.lifted_by} person={people[c.lifted_by]} size={16} /> : null}
            <time className={s.meta} dateTime={new Date(c.lifted_at).toISOString()} title={fullDate(c.lifted_at)}>
              {formatTimestamp(c.lifted_at)}
            </time>
          </p>
          {c.lift_reason ? <p className={s.liftReason}>“{c.lift_reason}”</p> : null}
        </div>
      ) : null}

      <footer className={s.noticeFoot}>
        <span className={s.credit}>
          {t("nexa.notice.actionedBy")} <PersonChip id={c.moderator_id} person={people[c.moderator_id]} size={18} withUsername />
        </span>
        <span className={s.meta}>
          {t("nexa.notice.caseNumber", { number: c.number })} · <time dateTime={new Date(c.created_at).toISOString()} title={fullDate(c.created_at)}>{formatTimestamp(c.created_at)}</time>
        </span>
        {staff && isStaffCase(c) ? (
          <span className={s.meta}>{c.notice_ack_at ? t("moderation.case.seenAt", { when: formatTimestamp(c.notice_ack_at) }) : t("moderation.case.notSeen")}</span>
        ) : null}
      </footer>

      {onOpenThread ? (
        <div className={s.noticeActions}>
          <Button size="sm" variant="secondary" onClick={onOpenThread} data-testid="open-case-thread">
            <MessagesSquare size={16} aria-hidden="true" /> {c.action === "delete" ? t("nexa.notice.viewCase") : t("nexa.notice.openThread")}
            {c.unread > 0 ? <span className={s.pill}>{c.unread}</span> : null}
          </Button>
        </div>
      ) : null}
    </article>
  );
}

/**
 * What an expression_takedown took: which pack or server, the stickers or
 * emoji by name (their pictures may be blocked, so names are what is left),
 * the pack's parts it reset, and whether the pictures can come back.
 */
function TakenDownParts({ d, staff }: { d: CaseDetails; staff: boolean }) {
  const items = d.emojis?.length ? d.emojis.map((e) => `:${e.name}:`) : (d.stickers ?? []).map((x) => x.name);
  const emoji = !!d.emojis?.length || d.pack_kind === "emoji";
  return (
    <>
      {d.pack_name || d.server_name ? (
        <p className={s.detailLine}>
          {d.pack_name ? <Sticker size={14} aria-hidden="true" /> : <Server size={14} aria-hidden="true" />}
          <span>{d.pack_name ? t("nexa.notice.packNamed", { name: d.pack_name }) : t("nexa.notice.serverNamed", { name: d.server_name ?? "" })}</span>
        </p>
      ) : null}
      {items.length ? (
        <div className={s.fields} role="table" aria-label={t("nexa.notice.takenDownItems")}>
          <div className={s.fieldRow} role="row">
            <span className={s.fieldName} role="cell">{emoji ? t("nexa.notice.emojiCount", { count: items.length }) : t("nexa.notice.stickerCount", { count: items.length })}</span>
            <span className={s.fieldValue} role="cell">
              {/* names people gave them, read as words rather than as a value a reset wrote */}
              <span className={s.newValue} data-word="true" dir="auto">{items.join(", ")}</span>
            </span>
          </div>
        </div>
      ) : null}
      {!d.deleted ? <ChangedFields d={d} staff={staff} label={t("nexa.notice.packFields")} labelOf={(f) => packFieldLabel(f as PackField)} /> : null}
      {d.blocked ? <p className={s.callout}>{t("nexa.notice.picturesBlocked")}</p> : null}
    </>
  );
}

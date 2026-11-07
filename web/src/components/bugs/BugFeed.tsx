import { CircleDot, MessagesSquare, Paperclip } from "lucide-react";
import type { FeedItem, People } from "@/api/moderation";
import { Button } from "@/components/common/Button";
import { PersonChip } from "@/components/nexa/NexaBits";
import { bugKind, bugNumber, bugStatusLabel, bugStatusTone } from "@/lib/bugs";
import { parse, plainText } from "@/markdown";
import { t } from "@/i18n";
import { BugStatusChip } from "./BugReportCard";
import s from "./Bugs.module.css";
import n from "@/components/nexa/Nexa.module.css";

export type BugItem = Extract<FeedItem, { kind: "bug" | "bug_reply" | "bug_status" }>;

/** Whether an item of the Nexa conversation is about a bug report. */
export function isBugItem(it: FeedItem): it is BugItem {
  return it.kind === "bug" || it.kind === "bug_reply" || it.kind === "bug_status";
}

/** What kind of item it is, over it in the conversation. */
export function bugFeedKind(it: BugItem): string {
  if (it.kind === "bug" && bugKind(it.bug) === "suggestion") return t("suggestions.feed.kind");
  return it.kind === "bug" ? t("bugs.feed.kindReport") : it.kind === "bug_reply" ? t("bugs.feed.kindReply") : t("bugs.feed.kindUpdate");
}

/**
 * A bug report in the member's Nexa conversation: the report they sent, a
 * reply from the team, or a change of its status. Each opens the report's
 * thread, where the conversation about it happens. A suggestion comes the
 * same way, in its own words.
 */
export function BugFeedEntry({ item, people, onOpen }: { item: BugItem; people: People; onOpen: (reportId: string) => void }) {
  const b = item.bug;
  const kind = bugKind(b);
  const f = kind === "suggestion" ? "suggestions.feed" : "bugs.feed";
  const open = (
    <Button size="sm" variant="secondary" onClick={() => onOpen(b.id)} data-testid="open-bug-thread">
      <MessagesSquare size={16} aria-hidden="true" /> {t("bugs.feed.openThread")}
      {b.unread > 0 ? <span className={n.pill}>{b.unread}</span> : null}
    </Button>
  );
  if (item.kind === "bug_status") {
    const status = item.message.body as typeof b.status;
    return (
      <div className={s.statusLine} data-tone={bugStatusTone(status) || undefined}>
        <CircleDot size={18} aria-hidden="true" />
        <span>{t(`${f}.statusChanged`, { number: bugNumber(b.number), status: bugStatusLabel(status, kind) })}</span>
        <PersonChip id={item.message.author_id} person={people[item.message.author_id]} size={16} />
        <button type="button" className={s.linkButton} onClick={() => onOpen(b.id)}>
          {t(`${f}.view`)}
        </button>
      </div>
    );
  }
  if (item.kind === "bug_reply") {
    return (
      <div className={s.feedCard}>
        <div className={s.feedTop}>
          <p className={s.feedTitle}>{t(`${f}.replied`, { number: bugNumber(b.number) })}</p>
        </div>
        <p className={s.feedSubject} dir="auto">
          {b.subject}
        </p>
        <p className={s.quote} dir="auto">
          {plainText(parse(item.message.body))}
        </p>
        <div className={s.feedMeta}>
          <span className={n.credit}>
            {t("nexa.thread.sentBy")} <PersonChip id={item.message.author_id} person={people[item.message.author_id]} size={16} />
          </span>
        </div>
        <div className={n.noticeActions}>{open}</div>
      </div>
    );
  }
  return (
    <div className={s.feedCard}>
      <div className={s.feedTop}>
        <p className={s.feedTitle} dir="auto">
          {b.subject}
        </p>
        <BugStatusChip status={b.status} kind={kind} />
      </div>
      <p className={s.feedExcerpt} dir="auto">
        {plainText(parse(b.description))}
      </p>
      <div className={s.feedMeta}>
        <span>{t(`${f}.youSent`, { number: bugNumber(b.number) })}</span>
        {b.attachments.length ? (
          <span>
            <Paperclip size={13} aria-hidden="true" /> {t("bugs.card.files", { count: b.attachments.length })}
          </span>
        ) : null}
      </div>
      <div className={n.noticeActions}>{open}</div>
    </div>
  );
}

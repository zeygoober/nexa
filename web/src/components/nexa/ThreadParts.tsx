import type { ReactNode } from "react";
import type { MemberBug, StaffBug } from "@/api/bugs";
import { BugFacts } from "@/components/bugs/BugReportCard";
import { bugKind } from "@/lib/bugs";
import { useRenderCtx } from "@/lib/markdownCtx";
import { Markdown } from "@/markdown";
import { ThreadFiles } from "./ThreadFiles";
import chat from "@/components/chat/Chat.module.css";
import s from "./Nexa.module.css";

/**
 * The start of a case's or a report's conversation, as any conversation
 * starts: who it is with, big, what it is about, and who can read it.
 */
export function ThreadIntro({ avatar, title, sub, children }: { avatar: ReactNode; title: string; sub?: ReactNode; children: ReactNode }) {
  return (
    <div className={chat.empty} data-testid="thread-intro">
      <div className={chat.emptyAvatar}>{avatar}</div>
      <h2 className={chat.emptyName} dir="auto">
        {title}
      </h2>
      {sub ? (
        <div className={chat.emptyUser} dir="auto">
          {sub}
        </div>
      ) : null}
      <p className={chat.emptyText}>{children}</p>
    </div>
  );
}

/**
 * A bug report as the first message of its conversation, from whoever sent
 * it: its subject, what they wrote, their screenshots and recording as a
 * message carries them, and the facts about their device folded away. A
 * suggestion never carries device facts, so it says nothing about them.
 */
export function ReportOpener({ report }: { report: MemberBug | StaffBug }) {
  const ctx = useRenderCtx();
  return (
    <>
      <p className={s.reportSubject} dir="auto">
        {report.subject}
      </p>
      <div dir="auto">
        <Markdown source={report.description} ctx={ctx} />
      </div>
      {report.attachments.length ? <ThreadFiles files={report.attachments} kind="bugs" from={"reporter_display_name" in report ? report.reporter_display_name || report.reporter_username : undefined} /> : null}
      {bugKind(report) === "suggestion" ? null : <BugFacts context={report.context} />}
    </>
  );
}

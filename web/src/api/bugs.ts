/**
 * Bug reports, both sides of them.
 *
 * A member sends one from the Report a Bug dialog: its files go up first, one
 * request each, and the report names them. The team answers in a thread the
 * member reads in their Nexa conversation; staff work the reports from the
 * Bug Reports tab of the moderation panel. None of it is end-to-end encrypted,
 * since the team has to read it, and the dialog says so.
 */
import { api, type UploadOptions } from "./client";
import type { People } from "./moderation";
import type { Snowflake } from "./types";

export type BugStatus = "open" | "in_progress" | "resolved" | "wont_fix" | "closed";
/** What a report is: something broken, or an idea. Both travel the same way; the words and the staff tab differ. */
export type BugKind = "bug" | "suggestion";
export const BUG_STATUSES: BugStatus[] = ["open", "in_progress", "resolved", "wont_fix", "closed"];

/** A screenshot or recording sent with a report. */
export interface BugFile {
  /** The file's blob id: its address is /media/bugs/{id}. */
  id: Snowflake;
  name: string;
  mime: string;
  size: number;
  width: number | null;
  height: number | null;
}

/** The facts about the reporter's app and device a report can carry, all shown to them first. */
export interface BugContext {
  app?: string;
  app_version?: string;
  build?: string;
  /** Added by the server: its own version when the report arrived. */
  server?: string;
  browser?: string;
  os?: string;
  device?: string;
  screen?: string;
  window?: string;
  language?: string;
  theme?: string;
  custom_css?: string;
  plugins?: string;
  opened_from?: string;
}

/** A report as its reporter sees it. */
export interface MemberBug {
  id: Snowflake;
  /** Older servers leave it out: those reports are all bugs. */
  kind?: BugKind;
  number: number;
  subject: string;
  description: string;
  context: BugContext;
  status: BugStatus;
  status_at: number | null;
  status_by: Snowflake | null;
  awaiting: "staff" | "member" | "none";
  attachments: BugFile[];
  /** Staff replies and status changes the reporter has not read. */
  unread: number;
  last_activity_at: number;
  created_at: number;
}

/** One line of a report's thread; the same shape as a case's, so the thread view is shared. */
export interface BugMessage {
  id: Snowflake;
  report_id: Snowflake;
  author_id: Snowflake;
  kind: "member" | "staff" | "system";
  /** For system lines: "status", with the new status as the body. */
  event: "" | "status";
  body: string;
  created_at: number;
  /** The pictures and recordings sent with it; absent when it has none. */
  files?: BugFile[];
}

/** A report as the staff see it. */
export interface StaffBug extends MemberBug {
  user_id: Snowflake;
  reporter_username: string;
  reporter_display_name: string;
  last_message: BugMessage | null;
}

export interface BugCounts {
  waiting: number;
  open: number;
  in_progress: number;
  resolved: number;
  wont_fix: number;
  closed: number;
  all: number;
}

/** The views of the staff list: one status, the active ones, those waiting on staff, or all. */
export type BugView = "waiting" | "active" | BugStatus | "all";
export type BugSort = "activity" | "newest" | "oldest";

export interface NewBug {
  kind?: BugKind;
  subject: string;
  description: string;
  context?: BugContext;
  attachments: { id: Snowflake; name: string }[];
}

interface BugThread<B> {
  report: B;
  messages: BugMessage[];
  people: People;
}

export const bugs = {
  /** Uploads one file for a report that is still being written. */
  upload(file: Blob, name: string, opts?: Pick<UploadOptions, "onProgress" | "signal">): Promise<BugFile> {
    const q = name ? `?name=${encodeURIComponent(name.slice(0, 120))}` : "";
    return api.upload<BugFile>(`/bugs/attachments${q}`, file, { ...opts, headers: { "Content-Type": file.type || "application/octet-stream" } });
  },
  create(body: NewBug): Promise<BugThread<MemberBug>> {
    return api.request("POST", "/bugs", body);
  },
  mine(): Promise<{ reports: MemberBug[] }> {
    return api.request("GET", "/bugs");
  },
  thread(id: Snowflake): Promise<BugThread<MemberBug>> {
    return api.request("GET", `/bugs/${id}`);
  },
  reply(id: Snowflake, body: string, files: BugFile[] = []): Promise<{ message: BugMessage; messages: BugMessage[]; report: MemberBug }> {
    return api.request("POST", `/bugs/${id}/messages`, { body, files: files.map((f) => ({ id: f.id, name: f.name })) });
  },
  read(id: Snowflake, messageId: Snowflake): Promise<void> {
    return api.request("POST", `/bugs/${id}/read`, { message_id: messageId });
  },
  /** Where a file is fetched from, with the bearer token; `size` asks for a picture no larger than that. */
  fileUrl(id: Snowflake, size?: number): string {
    return api.url(`/media/bugs/${id}`, size ? { size } : undefined);
  },
};

export const staffBugs = {
  list(p: { kind?: BugKind; view: BugView; files?: boolean; q?: string; sort?: BugSort; before?: StaffBug; limit?: number }): Promise<{ reports: StaffBug[]; counts: BugCounts; people: People; has_more: boolean }> {
    const status = p.view === "waiting" || p.view === "all" ? undefined : p.view;
    const sort = p.sort ?? "activity";
    return api.request("GET", "/moderation/bugs", undefined, {
      query: {
        kind: p.kind ?? "bug",
        status,
        waiting: p.view === "waiting" ? "1" : undefined,
        files: p.files ? "1" : undefined,
        q: p.q?.trim() || undefined,
        sort,
        before_at: p.before && sort === "activity" ? p.before.last_activity_at : undefined,
        before_id: p.before?.id,
        limit: p.limit,
      },
    });
  },
  thread(id: Snowflake): Promise<BugThread<StaffBug>> {
    return api.request("GET", `/moderation/bugs/${id}`);
  },
  reply(id: Snowflake, body: string, files: BugFile[] = []): Promise<{ message: BugMessage; messages: BugMessage[]; report: StaffBug }> {
    return api.request("POST", `/moderation/bugs/${id}/messages`, { body, files: files.map((f) => ({ id: f.id, name: f.name })) });
  },
  /** Puts one file up for a staff line not sent yet. */
  upload(file: Blob, name: string, opts?: Pick<UploadOptions, "onProgress" | "signal">): Promise<BugFile> {
    const q = name ? `?name=${encodeURIComponent(name.slice(0, 120))}` : "";
    return api.upload<BugFile>(`/moderation/bugs/attachments${q}`, file, { ...opts, headers: { "Content-Type": file.type || "application/octet-stream" } });
  },
  /** Reads one report up to a line, or all of it. */
  read(id: Snowflake, messageId?: Snowflake): Promise<import("./moderation").ModUnread> {
    return api.request("POST", `/moderation/bugs/${id}/read`, messageId ? { message_id: messageId } : {});
  },
  setStatus(id: Snowflake, status: BugStatus): Promise<{ report: StaffBug; messages: BugMessage[]; people: People }> {
    return api.request("POST", `/moderation/bugs/${id}/status`, { status });
  },
};

import { PersonName } from "@/components/common/PersonName";
import { Bug, ChevronDown, ChevronLeft, CircleCheck, Inbox, Lightbulb, Paperclip, RotateCcw, UserRound } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { BUG_STATUSES, staffBugs, type BugCounts, type BugKind, type BugMessage, type BugSort, type BugStatus, type BugView, type StaffBug } from "@/api/bugs";
import type { People } from "@/api/moderation";
import { Avatar } from "@/components/common/Avatar";
import { Button } from "@/components/common/Button";
import { menuAnchor } from "@/components/common/ContextMenu";
import { LoadError } from "@/components/common/LoadError";
import { SearchBox } from "@/components/common/SearchBox";
import { Select } from "@/components/common/Select";
import { Spinner } from "@/components/common/Spinner";
import { BugStatusChip, BugThumbStrip } from "@/components/bugs/BugReportCard";
import { LockNote, ThreadChat } from "@/components/nexa/ThreadChat";
import { ReportOpener, ThreadIntro } from "@/components/nexa/ThreadParts";
import { bugFinished, bugKind, bugNumber, bugStatusHint, bugStatusLabel } from "@/lib/bugs";
import { describeError } from "@/lib/errors";
import { personName, systemLineText } from "@/lib/moderation";
import { formatRelative } from "@/lib/time";
import { parse, plainText } from "@/markdown";
import { useModEvents, useModeration } from "@/stores/moderation";
import { useUI } from "@/stores/ui";
import { t, Tr } from "@/i18n";
import { button, defineMenu } from "@/lib/contextMenus/registry";

/** A bug report's status, or a suggestion's, set by the staff. */
const bugStatusMenu = defineMenu<{ status: BugStatus; kind: BugKind; setStatus: (st: BugStatus) => void }>("bugStatus", [
  [button("setBugStatus", ({ status, kind, setStatus }) => BUG_STATUSES.map((st) => ({ id: st, label: bugStatusLabel(st, kind), checked: st === status, onSelect: () => setStatus(st) })))],
]);
import m from "./Moderation.module.css";
import b from "./BugsTab.module.css";

const PAGE = 40;

/** Where each kind's tab is in the panel. */
function baseFor(kind: BugKind): string {
  return kind === "suggestion" ? "/channels/@admin/suggestions" : "/channels/@admin/bugs";
}

/** The staff words that differ between the two tabs: a key under bugs.staff or suggestions.staff. */
function staffText(kind: BugKind, key: string, values?: Record<string, string | number>): string {
  return t(`${kind === "suggestion" ? "suggestions" : "bugs"}.staff.${key}`, values);
}

/** Whether a report belongs in a view of the list. */
function inView(r: StaffBug, view: BugView, files: boolean): boolean {
  if (files && r.attachments.length === 0) return false;
  switch (view) {
    case "waiting":
      return r.awaiting === "staff";
    case "active":
      return r.status === "open" || r.status === "in_progress";
    case "all":
      return true;
    default:
      return r.status === view;
  }
}

function order(sort: BugSort): (a: StaffBug, z: StaffBug) => number {
  const byId = (a: StaffBug, z: StaffBug) => (BigInt(z.id) > BigInt(a.id) ? 1 : BigInt(z.id) < BigInt(a.id) ? -1 : 0);
  if (sort === "newest") return byId;
  if (sort === "oldest") return (a, z) => byId(z, a);
  return (a, z) => z.last_activity_at - a.last_activity_at || byId(a, z);
}

/** The newest line of a report's thread, as the list previews it. */
function preview(r: StaffBug, people: People): React.ReactNode {
  const last = r.last_message;
  if (!last) return plainText(parse(r.description));
  if (last.kind === "system") return systemLineText(last, people, undefined, "staff");
  const who = personName(people[last.author_id]) || r.reporter_display_name;
  return <><PersonName userId={last.author_id} user={people[last.author_id]}>{who}</PersonName>: {plainText(parse(last.body))}</>;
}

/**
 * The Bug Reports tab: every report members sent, in the view picked, with
 * the one chosen open beside the list (on a phone, instead of it): the report
 * in full, its files, the device it came from, and the thread with its
 * reporter, where staff answer as Nexa and move it along. The Suggestions
 * tab is the same, for the ideas members sent (`kind`).
 */
export function BugsTab({ kind = "bug" }: { kind?: BugKind }) {
  const { reportId } = useParams();
  const nav = useNavigate();
  const BASE = baseFor(kind);
  const mergePeople = useModeration((s) => s.mergePeople);
  const counts$ = useModeration((s) => (kind === "suggestion" ? s.suggestionCounts : s.bugCounts));
  const [view, setView] = useState<BugView>("waiting");
  const [files, setFiles] = useState(false);
  const [query, setQuery] = useState("");
  const [q, setQ] = useState("");
  const [sort, setSort] = useState<BugSort>("activity");
  const [reports, setReports] = useState<StaffBug[] | null>(null);
  const [counts, setCounts] = useState<BugCounts | null>(counts$);
  const [people, setPeople] = useState<People>({});
  const [more, setMore] = useState(false);
  const [loading, setLoading] = useState(false);
  const [failed, setFailed] = useState(false);

  // the search waits for a pause in typing
  useEffect(() => {
    const h = setTimeout(() => setQ(query.trim()), 250);
    return () => clearTimeout(h);
  }, [query]);

  // only the newest request may fill the list: a slower answer for the filters picked before would land in this one
  const request = useRef(0);

  const load = useCallback(
    async (after?: StaffBug) => {
      const id = ++request.current;
      setLoading(true);
      setFailed(false);
      try {
        const r = await staffBugs.list({ kind, view, files, q, sort, before: after, limit: PAGE });
        if (id !== request.current) return;
        setReports((cur) => (after ? [...(cur ?? []), ...r.reports.filter((x) => !cur?.some((y) => y.id === x.id))] : r.reports));
        setCounts(r.counts);
        useModeration.getState().setBugCounts(r.counts, kind);
        setPeople((p) => ({ ...p, ...r.people }));
        mergePeople(r.people);
        setMore(r.has_more);
      } catch {
        // said as a failure, never as an empty list: "nothing waiting" during
        // an outage tells staff nobody is waiting when somebody is
        if (id === request.current) setFailed(true);
      } finally {
        if (id === request.current) setLoading(false);
      }
    },
    [kind, view, files, q, sort, mergePeople],
  );

  useEffect(() => {
    setReports(null);
    void load();
  }, [load]);

  useEffect(() => {
    if (counts$) setCounts(counts$);
  }, [counts$]);

  const upsert = (r: StaffBug, p?: People) => {
    if (p) setPeople((cur) => ({ ...cur, ...p }));
    setReports((cur) => {
      if (!cur) return cur;
      const had = cur.some((x) => x.id === r.id);
      const rest = cur.filter((x) => x.id !== r.id);
      // a search is the server's to answer; a report that moves only updates where it is already listed
      const keep = q ? had : inView(r, view, files) || r.id === reportId;
      return keep ? [r, ...rest].sort(order(sort)) : rest;
    });
  };
  useModEvents((e) => {
    if (e.type === "bug" && bugKind(e.report) === kind) upsert(e.report, e.people);
  });

  const views: { id: BugView; label: string; n?: number }[] = [
    { id: "waiting", label: t("bugs.staff.viewWaiting"), n: counts?.waiting },
    { id: "open", label: bugStatusLabel("open", kind), n: counts?.open },
    { id: "in_progress", label: bugStatusLabel("in_progress", kind), n: counts?.in_progress },
    { id: "resolved", label: bugStatusLabel("resolved", kind), n: counts?.resolved },
    { id: "wont_fix", label: bugStatusLabel("wont_fix", kind), n: counts?.wont_fix },
    { id: "closed", label: bugStatusLabel("closed", kind), n: counts?.closed },
    { id: "all", label: t("bugs.staff.viewAll"), n: counts?.all },
  ];
  const filtered = !!q || files;

  return (
    <div className={[m.workspace, b.workspace].join(" ")} data-open={!!reportId} data-testid="bugs-tab">
      <div className={m.listCol}>
        <div className={b.tools}>
          <div className={b.toolRow}>
            <SearchBox className={b.search} value={query} onChange={setQuery} placeholder={staffText(kind, "searchShort")} aria-label={staffText(kind, "search")} title={staffText(kind, "search")} data-testid="bugs-search" />
            <Select className={b.sort} value={sort} onChange={(e) => setSort(e.target.value as BugSort)} aria-label={staffText(kind, "sort")} data-testid="bugs-sort">
              <option value="activity">{t("bugs.staff.sortActivity")}</option>
              <option value="newest">{t("bugs.staff.sortNewest")}</option>
              <option value="oldest">{t("bugs.staff.sortOldest")}</option>
            </Select>
          </div>
          <div className={b.chips} role="radiogroup" aria-label={staffText(kind, "views")}>
            {views.map((v) => (
              <button key={v.id} type="button" role="radio" aria-checked={view === v.id} className={m.chip} data-selected={view === v.id} onClick={() => setView(v.id)} data-testid={`bugs-view-${v.id}`}>
                {v.label}
                {v.n !== undefined ? <span className={m.chipCount}>{v.n}</span> : null}
              </button>
            ))}
            <button type="button" className={[m.chip, b.filesChip].join(" ")} aria-pressed={files} data-selected={files} onClick={() => setFiles((f) => !f)} data-testid="bugs-files">
              <Paperclip size={14} aria-hidden="true" />
              {t("bugs.staff.withFiles")}
            </button>
          </div>
        </div>
        {failed && !reports?.length ? (
          <LoadError message={t("moderation.loadFailed")} onRetry={() => void load()} busy={loading} center />
        ) : reports === null ? (
          <div className={m.loadingRow}>
            <Spinner size={24} label={t("chat.pins.loading")} />
          </div>
        ) : reports.length === 0 ? (
          <div className={m.empty}>
            <Inbox size={36} aria-hidden="true" />
            <span className={m.emptyTitle}>{filtered ? staffText(kind, "noMatches") : view === "waiting" ? t("bugs.staff.caughtUp") : staffText(kind, "none")}</span>
            {filtered ? (
              <Button size="sm" variant="ghost" onClick={() => { setQuery(""); setFiles(false); }}>
                {t("moderation.clearFilters")}
              </Button>
            ) : null}
          </div>
        ) : (
          <ul className={b.list} data-testid="bugs-list">
            {reports.map((r) => (
              <BugRow key={r.id} r={r} kind={kind} people={people} selected={reportId === r.id} onPick={() => nav(`${BASE}/${r.id}`)} />
            ))}
            {more ? (
              <li className={m.loadingRow}>
                <Button size="sm" variant="ghost" loading={loading} onClick={() => void load(reports[reports.length - 1])}>
                  {t("moderation.loadMore")}
                </Button>
              </li>
            ) : null}
          </ul>
        )}
      </div>
      <div className={m.pane}>
        {reportId ? (
          <StaffBugPane key={reportId} reportId={reportId} kind={kind} onBack={() => nav(BASE)} onChanged={(r) => upsert(r)} />
        ) : (
          <div className={m.paneEmpty}>
            {kind === "suggestion" ? <Lightbulb size={40} aria-hidden="true" /> : <Bug size={40} aria-hidden="true" />}
            <span className={m.emptyTitle}>{staffText(kind, "pick")}</span>
            <span>{staffText(kind, "pickHint")}</span>
          </div>
        )}
      </div>
    </div>
  );
}

/**
 * One report in the list: its number and subject, who sent it (a way to
 * their profile), whether it waits on staff and where it stands, the start of
 * what it says, and its files.
 */
function BugRow({ r, kind, people, selected, onPick }: { r: StaffBug; kind: BugKind; people: People; selected: boolean; onPick: () => void }) {
  const openModal = useUI((u) => u.openModal);
  const fresh = useModeration((st) => (kind === "suggestion" ? st.unread.suggestions : st.unread.bugs).includes(r.id));
  const who = people[r.user_id];
  const name = who ? personName(who) : r.reporter_display_name || r.reporter_username;
  const gone = !who || who.deleted;
  const openProfile = () => {
    if (!gone) openModal("profile", { userId: r.user_id });
  };
  return (
    <li className={b.row} data-selected={selected || undefined} data-waiting={r.awaiting === "staff" || undefined} data-unread={fresh || undefined} data-report={r.id}>
      <span className={b.rowAvatar} onClick={openProfile} aria-hidden="true">
        <Avatar id={r.user_id} avatarId={who?.avatar_id ?? null} name={name} size={36} />
      </span>
      <button type="button" className={b.rowMain} onClick={onPick} aria-current={selected || undefined} data-testid="bug-row">
        <span className={b.rowNumber}>{bugNumber(r.number)}</span>
        <span className={b.rowSubject} dir="auto">
          {r.subject}
        </span>
        {fresh ? (
          <span className={m.unreadDot}>
            <span className="visually-hidden">{t("sidebar.serverSidebar.unread")}</span>
          </span>
        ) : null}
        <time className={b.rowTime} dateTime={new Date(r.last_activity_at).toISOString()}>
          {formatRelative(r.last_activity_at)}
        </time>
      </button>
      <span className={b.rowWho}>
        <button type="button" className={b.reporter} onClick={openProfile} disabled={gone} dir="auto" title={t("nexa.viewProfile")}>
          <PersonName userId={gone ? null : r.user_id} user={gone ? null : who}>{name}</PersonName>
        </button>
        {r.awaiting === "staff" ? (
          <span className={m.tag} data-tone="danger">
            {t("bugs.staff.waiting")}
          </span>
        ) : null}
        <BugStatusChip status={r.status} kind={kind} className={b.rowStatus} />
      </span>
      <span className={b.rowPreview} dir="auto">
        {preview(r, people)}
      </span>
      {r.attachments.length ? (
        <span className={b.rowFiles}>
          <BugThumbStrip files={r.attachments} />
        </span>
      ) : null}
    </li>
  );
}

/**
 * A report for the staff: the report in full, its thread, and what can be
 * done about it: answer as Nexa, move it to another status, or mark it
 * resolved (a suggestion, done) in one step.
 */
function StaffBugPane({ reportId, kind, onBack, onChanged }: { reportId: string; kind: BugKind; onBack: () => void; onChanged: (r: StaffBug) => void }) {
  const nav = useNavigate();
  const toast = useUI((u) => u.toast);
  const mergePeople = useModeration((s) => s.mergePeople);
  const [data, setData] = useState<{ report: StaffBug; messages: BugMessage[]; people: People } | null>(null);
  const [failed, setFailed] = useState(false);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let alive = true;
    setData(null);
    setFailed(false);
    staffBugs
      .thread(reportId)
      .then((r) => {
        if (!alive) return;
        setData(r);
        mergePeople(r.people);
      })
      .catch(() => alive && setFailed(true));
    return () => {
      alive = false;
    };
  }, [reportId, mergePeople]);

  useModEvents((e) => {
    if (e.type !== "bug" || e.report.id !== reportId) return;
    setData((cur) => {
      if (!cur) return cur;
      const seen = new Set(cur.messages.map((x) => x.id));
      return { report: e.report, messages: [...cur.messages, ...e.messages.filter((x) => !seen.has(x.id))], people: { ...cur.people, ...e.people } };
    });
  });

  const apply = (r: StaffBug, lines: BugMessage[] = [], people?: People) => {
    setData((cur) => (cur ? { report: r, messages: [...cur.messages, ...lines.filter((x) => !cur.messages.some((y) => y.id === x.id))], people: { ...cur.people, ...people } } : cur));
    onChanged(r);
  };

  const setStatus = async (status: BugStatus) => {
    if (!data || status === data.report.status) return;
    setBusy(true);
    try {
      const r = await staffBugs.setStatus(reportId, status);
      apply(r.report, r.messages, r.people);
      toast(t("bugs.staff.statusSet", { status: bugStatusLabel(status, kind) }), "success");
    } catch (err) {
      toast(describeError(err, t("moderation.failed")), "error");
    } finally {
      setBusy(false);
    }
  };

  const statusMenu = (el: HTMLElement) => {
    if (!data) return;
    const anchor = menuAnchor(el);
    bugStatusMenu.open({ status: data.report.status, kind, setStatus: (st) => void setStatus(st) }, { x: anchor.left, y: anchor.bottom + 4, anchor, label: t("bugs.staff.setStatus") });
  };

  if (failed) {
    return (
      <div className={m.paneEmpty}>
        <p>{t("bugs.staff.missing")}</p>
        <Button size="sm" variant="secondary" onClick={onBack}>
          {t("moderation.back")}
        </Button>
      </div>
    );
  }
  if (!data) {
    return (
      <div className={m.paneEmpty}>
        <Spinner label={t("chat.pins.loading")} />
      </div>
    );
  }
  const r = data.report;
  const reporter = data.people[r.user_id];
  const gone = !reporter || reporter.deleted;
  const finished = bugFinished(r.status);
  const title = t(kind === "suggestion" ? "suggestions.page.title" : "bugs.page.title", { number: bugNumber(r.number) });
  return (
    <>
      <div className={[m.paneHeader, b.paneHead].join(" ")}>
        <button type="button" className={[m.iconBtn, m.mobileBack].join(" ")} onClick={onBack} aria-label={t("moderation.back")}>
          <ChevronLeft size={18} className={m.flip} />
        </button>
        <span className={[m.paneTitle, b.paneTitle].join(" ")}>
          <span className={b.paneIcon} aria-hidden="true">
            {kind === "suggestion" ? <Lightbulb size={16} /> : <Bug size={16} />}
          </span>
          <span dir="auto">{`${bugNumber(r.number)} ${r.subject}`}</span>
        </span>
        <span className={[m.paneActions, b.paneActs].join(" ")}>
          <Button size="sm" variant="secondary" disabled={busy} onClick={(e) => statusMenu(e.currentTarget)} aria-haspopup="menu" title={bugStatusHint(r.status, kind)} data-testid="bug-status-menu">
            {t("bugs.staff.statusIs", { status: bugStatusLabel(r.status, kind) })}
            <ChevronDown size={14} aria-hidden="true" />
          </Button>
          {finished ? (
            <Button size="sm" variant="secondary" disabled={busy} onClick={() => void setStatus("open")} data-testid="bug-reopen">
              <RotateCcw size={15} aria-hidden="true" /> {t("bugs.staff.reopen")}
            </Button>
          ) : (
            <Button size="sm" variant="positive" disabled={busy} onClick={() => void setStatus("resolved")} data-testid="bug-resolve">
              <CircleCheck size={15} aria-hidden="true" /> {staffText(kind, "markResolved")}
            </Button>
          )}
          {!gone ? (
            <Button size="sm" variant="ghost" onClick={() => nav(`/channels/@admin/users/${r.user_id}`)} title={t("bugs.staff.openMember")} aria-label={t("moderation.case.openPerson", { name: personName(reporter) })}>
              <UserRound size={15} aria-hidden="true" /> <PersonName userId={r.user_id} user={reporter} className={m.btnLabel}>{personName(reporter)}</PersonName>
            </Button>
          ) : null}
        </span>
      </div>
      <ThreadChat
        lines={data.messages}
        people={data.people}
        perspective="staff"
        fileKind="bugs"
        reportKind={kind}
        intro={
          <ThreadIntro avatar={<Avatar id={r.user_id} avatarId={reporter?.avatar_id ?? null} name={personName(reporter) || r.reporter_display_name} size={80} />} title={title} sub={r.subject}>
            {<Tr id={kind === "suggestion" ? "suggestions.chat.staffStart" : "moderation.chat.reportStart"} values={{ name: <PersonName userId={r.user_id} user={reporter}>{personName(reporter) || r.reporter_display_name || r.reporter_username}</PersonName> }} />}
          </ThreadIntro>
        }
        opener={{ from: r.user_id, at: r.created_at, body: <ReportOpener report={r} /> }}
        onSend={
          gone || finished
            ? undefined
            : async (body, files) => {
                const res = await staffBugs.reply(reportId, body, files);
                apply(res.report, res.messages);
              }
        }
        upload={staffBugs.upload}
        placeholder={t("bugs.thread.staffPlaceholder")}
        note={gone || finished ? undefined : <Tr id="moderation.case.replyNote" values={{ name: <PersonName userId={r.user_id} user={reporter}>{personName(reporter)}</PersonName> }} />}
        dropName={title}
        onSeen={(lastId) => useModeration.getState().readThread("bug", reportId, lastId)}
        locked={
          gone ? (
            <LockNote text={t("moderation.case.goneNote")} />
          ) : (
            <LockNote
              text={staffText(kind, "finishedLocked", { status: bugStatusLabel(r.status, kind) })}
              action={
                <Button size="sm" variant="secondary" disabled={busy} onClick={() => void setStatus("open")} data-testid="bug-reopen-locked">
                  <RotateCcw size={15} aria-hidden="true" /> {t("bugs.staff.reopen")}
                </Button>
              }
            />
          )
        }
      />
    </>
  );
}

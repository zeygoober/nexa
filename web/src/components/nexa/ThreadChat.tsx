import { PersonName } from "@/components/common/PersonName";
import type { BugKind } from "@/api/bugs";
import { ArrowDown, ChevronUp, CircleCheck, CircleDot, CirclePlus, Clock, FileIcon, History, Lock, LockOpen, RotateCcw, SendHorizontal, ShieldAlert, Smile, Trash2, Type, type LucideIcon } from "lucide-react";
import { Fragment, Suspense, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type ClipboardEvent, type KeyboardEvent, type MouseEvent, type ReactNode } from "react";
import type { UploadOptions } from "@/api/client";
import type { CaseFile, ModAction, People } from "@/api/moderation";
import { Avatar } from "@/components/common/Avatar";
import { menuAnchor } from "@/components/common/ContextMenu";
import { DropOverlay } from "@/components/composer/DropOverlay";
import { ExpressionPicker } from "@/components/pickers/ExpressionPicker";
import { copyText } from "@/lib/clipboard";
import { describeError } from "@/lib/errors";
import { lazyPage } from "@/lib/lazyChunk";
import { useRenderCtx } from "@/lib/markdownCtx";
import { personName, systemLineText } from "@/lib/moderation";
import type { PrivateFileKind } from "@/lib/bugMedia";
import { formatBytes, formatDateDivider, formatFull, formatTime, formatTimestamp, sameDay } from "@/lib/time";
import { Markdown } from "@/markdown";
import { useNexa } from "@/stores/nexa";
import { openProfilePopout } from "@/stores/profilePopout";
import { useUI } from "@/stores/ui";
import { t } from "@/i18n";
import { button, defineMenu } from "@/lib/contextMenus/registry";

/** A line of a case thread or bug report: copying what it says. */
const threadLineMenu = defineMenu<string>("threadLine", [[button("copyThreadLine", (body) => ({ id: "copy", label: t("chat.messageMenu.copyText"), onSelect: () => void copyText(body) }))]]);

/** The phone's one button in a case thread's or bug report's box, for formatting and emoji. */
const threadToolsMenu = defineMenu<{ openFormatting: () => void; openEmoji: () => void }>("threadTools", [
  [
    button("threadFormattingEditor", ({ openFormatting }) => ({ id: "formatting", label: t("composer.more.formatting"), onSelect: openFormatting })),
    button("threadEmojiPicker", ({ openEmoji }) => ({ id: "emoji", label: t("composer.more.emoji"), onSelect: openEmoji })),
  ],
]);
import { NexaAvatar, OfficialBadge } from "./NexaBits";
import { ThreadFiles } from "./ThreadFiles";
import chat from "@/components/chat/Chat.module.css";
import box from "@/components/composer/Composer.module.css";
import s from "./Nexa.module.css";

/** The most a line can hold; the server refuses anything longer. */
export const MAX_THREAD_MESSAGE = 2000;
/** How many files one line carries; the server holds a line to the same. */
export const MAX_THREAD_FILES = 4;
/** Lines from the same writer this close together share one header, as in any conversation. */
const GROUP_GAP = 7 * 60_000;
/** What a thread takes: pictures and short recordings. */
const FILE_TYPES = ["image/png", "image/jpeg", "image/gif", "image/webp", "video/mp4", "video/webm", "video/quicktime"];
/** The size limits the server keeps to, so a file too big is turned away before it goes up. */
const IMAGE_MAX = 10 << 20;
const VIDEO_MAX = 50 << 20;

const loadFormatEditor = () => import("@/components/composer/format/FormatEditor").then((m) => ({ default: m.FormatEditor }));
const FormatEditor = lazyPage(loadFormatEditor);

/** A line of a case's thread or a bug report's: a person's, the team's, or one the server writes about what happened. */
export interface ThreadLine {
  id: string;
  author_id: string;
  kind: "member" | "staff" | "system";
  event: string;
  body: string;
  created_at: number;
  files?: CaseFile[] | undefined;
}

/** Puts one file up for a line not sent yet. */
export type ThreadUpload = (file: Blob, name: string, opts?: Pick<UploadOptions, "onProgress" | "signal">) => Promise<CaseFile>;

/** The first message of the conversation: the notice Nexa sent, or the report its sender wrote. */
export interface ThreadOpener {
  /** "nexa" for a notice; a person's id for a report. */
  from: string;
  at: number;
  body: ReactNode;
}

interface Props {
  lines: ThreadLine[];
  people: People;
  perspective: "member" | "staff";
  /** Where the thread's files are fetched from. */
  fileKind: PrivateFileKind;
  /** The case's action, which some of its system lines name; a bug report has none. */
  action?: ModAction | undefined;
  /** A report's kind, whose statuses its system lines name in its own words. */
  reportKind?: BugKind | undefined;
  /** The notice or the report, as the first message; left out where the page shows it already. */
  opener?: ThreadOpener | undefined;
  /** The start of the conversation, above everything: what it is and who reads it. */
  intro?: ReactNode;
  /** Sends a line and its files; without it the lock bar stands where the box would be. */
  onSend?: ((body: string, files: CaseFile[]) => Promise<void>) | undefined;
  upload?: ThreadUpload | undefined;
  /** In place of the box once nobody can write here: why, and maybe a way to change it. */
  locked?: ReactNode;
  /** The quiet line over the box, where a conversation says who is typing. */
  note?: ReactNode;
  placeholder: string;
  /** What a file dropped on the window goes to, for the drop overlay. */
  dropName: string;
  /** Called with the newest line when it is on screen in front of the person, so it can be marked read. */
  onSeen?: ((lastId: string | undefined) => void) | undefined;
}

/**
 * A case's thread or a bug report's, as a conversation: the notice or the
 * report first, then everyone's messages with their names, avatars and
 * times, grouped as a conversation groups them, the days apart, and a line
 * for everything that happened to it. The box at the foot is the
 * conversation's own, with files, emoji and formatting; once staff close the
 * thread, a lock bar takes its place.
 */
export function ThreadChat({ lines, people, perspective, fileKind, action, reportKind, opener, intro, onSend, upload, locked, note, placeholder, dropName, onSeen }: Props) {
  const scroller = useRef<HTMLDivElement>(null);
  const atBottom = useRef(true);
  const [showJump, setShowJump] = useState(false);
  const focused = useUI((u) => u.focused);
  const seenRef = useRef(onSeen);
  seenRef.current = onSeen;
  const last = lines[lines.length - 1]?.id;

  // Follow new lines while the reader is at the bottom; leave them alone when
  // they have scrolled up to read something earlier.
  useLayoutEffect(() => {
    const el = scroller.current;
    if (el && atBottom.current) el.scrollTop = el.scrollHeight;
  }, [last, opener?.at]);
  // Seen means seen: at the bottom, with the app in front of the person.
  useEffect(() => {
    if (atBottom.current && focused) seenRef.current?.(last);
  }, [last, focused]);
  // pictures arriving after the first paint must not leave the newest line below the fold
  useEffect(() => {
    const el = scroller.current;
    if (!el || typeof ResizeObserver === "undefined") return;
    const inner = el.firstElementChild;
    if (!inner) return;
    const ro = new ResizeObserver(() => {
      if (atBottom.current) el.scrollTop = el.scrollHeight;
    });
    ro.observe(inner);
    return () => ro.disconnect();
  }, []);

  const onScroll = () => {
    const el = scroller.current;
    if (!el) return;
    const bottom = el.scrollHeight - el.scrollTop - el.clientHeight < 48;
    if (bottom && !atBottom.current && focused) seenRef.current?.(lines[lines.length - 1]?.id);
    atBottom.current = bottom;
    setShowJump(el.scrollHeight - el.scrollTop - el.clientHeight > 600);
  };
  const jump = () => {
    const el = scroller.current;
    if (!el) return;
    el.scrollTo({ top: el.scrollHeight, behavior: "smooth" });
  };

  return (
    <div className={s.thread}>
      <div className={chat.listWrap}>
        <div className={s.threadScroll} ref={scroller} onScroll={onScroll} data-scroller="" role="region" tabIndex={0} aria-label={t("modals.quickSwitcher.messages")} data-testid="case-thread">
          <div className={s.threadInner}>
            {intro}
            {opener ? <OpenerRow opener={opener} people={people} /> : null}
            <Lines lines={lines} people={people} perspective={perspective} fileKind={fileKind} action={action} reportKind={reportKind} openerAt={opener?.at} />
          </div>
        </div>
        {showJump ? (
          <button className={chat.jumpBtn} aria-label={t("chat.messageList.jumpToPresent")} onClick={jump}>
            <ArrowDown size={18} />
          </button>
        ) : null}
      </div>
      {note ? <div className={s.threadNote}>{note}</div> : null}
      {onSend ? (
        <ThreadComposer
          onSend={async (body, files) => {
            await onSend(body, files);
            // what you just sent is what you want to see, wherever you had scrolled to
            atBottom.current = true;
            requestAnimationFrame(() => {
              const el = scroller.current;
              if (el) el.scrollTop = el.scrollHeight;
            });
          }}
          upload={upload}
          placeholder={placeholder}
          dropName={dropName}
        />
      ) : locked ? (
        <div className={box.wrap} data-thread-box="">
          <div className={[box.box, box.locked, s.lockBar].join(" ")} data-testid="case-locked">
            {locked}
          </div>
        </div>
      ) : null}
    </div>
  );
}

/** What stands where the box was, once nobody can write here: a lock, why, and maybe a way to open it again. */
export function LockNote({ text, action }: { text: string; action?: ReactNode }) {
  return (
    <>
      <Lock size={18} aria-hidden="true" className={s.lockIcon} />
      <span className={s.lockText}>{text}</span>
      {action ? <span className={s.lockAction}>{action}</span> : null}
    </>
  );
}

/** The person a line is from, as the reader sees them: the team writes as Nexa to the member, as themselves to each other. */
function useAuthor(id: string, kind: ThreadLine["kind"], people: People, perspective: "member" | "staff") {
  const nexaId = useNexa((n) => n.state?.account_id ?? "");
  const person = people[id];
  const asNexa = kind === "staff" && perspective === "member";
  return { person, asNexa, nexaId, name: asNexa ? "Nexa" : personName(person) };
}

function OpenerRow({ opener, people }: { opener: ThreadOpener; people: People }) {
  const nexaId = useNexa((n) => n.state?.account_id ?? "");
  const fromNexa = opener.from === "nexa";
  const person = fromNexa ? undefined : people[opener.from];
  const name = fromNexa ? "Nexa" : personName(person);
  const profile = (e: MouseEvent<HTMLElement>) => {
    const id = fromNexa ? nexaId : opener.from;
    if (id && !person?.deleted) openProfilePopout(id, e.currentTarget);
  };
  return (
    <div role="article" className={chat.msg} data-group-start="true" data-row="" data-testid="thread-opener">
      <button type="button" className={chat.avatar} onClick={profile} tabIndex={-1} aria-hidden="true">
        {fromNexa ? <NexaAvatar size={40} /> : <Avatar id={opener.from} avatarId={person?.avatar_id ?? null} name={name} size={40} />}
      </button>
      <div className={chat.headerRow}>
        <button type="button" className={chat.author} onClick={profile} dir="auto">
          {name}
        </button>
        {fromNexa ? <OfficialBadge compact /> : null}
        <Time at={opener.at} />
      </div>
      <div className={[chat.content, s.openerBody].join(" ")}>{opener.body}</div>
    </div>
  );
}

function Lines({ lines, people, perspective, fileKind, action, reportKind, openerAt }: { lines: ThreadLine[]; people: People; perspective: "member" | "staff"; fileKind: PrivateFileKind; action: ModAction | undefined; reportKind: BugKind | undefined; openerAt: number | undefined }) {
  return (
    <>
      {lines.map((m, i) => {
        const prev = lines[i - 1];
        const before = prev?.created_at ?? openerAt;
        const newDay = before === undefined || !sameDay(before, m.created_at);
        const grouped = !!prev && !newDay && m.kind !== "system" && prev.kind === m.kind && prev.author_id === m.author_id && m.created_at - prev.created_at < GROUP_GAP;
        return (
          <Fragment key={m.id}>
            {newDay ? (
              <div className={chat.dateDivider} role="separator">
                <span>{formatDateDivider(m.created_at)}</span>
              </div>
            ) : null}
            {m.kind === "system" ? <SystemRow m={m} people={people} action={action} perspective={perspective} reportKind={reportKind} /> : <Row m={m} people={people} perspective={perspective} grouped={grouped} fileKind={fileKind} />}
          </Fragment>
        );
      })}
    </>
  );
}

function Row({ m, people, perspective, grouped, fileKind }: { m: ThreadLine; people: People; perspective: "member" | "staff"; grouped: boolean; fileKind: PrivateFileKind }) {
  const ctx = useRenderCtx();
  const { person, asNexa, nexaId, name } = useAuthor(m.author_id, m.kind, people, perspective);
  const staffLine = m.kind === "staff";
  const profile = (e: MouseEvent<HTMLElement>, who = asNexa ? nexaId : m.author_id) => {
    if (who && !(who === m.author_id && person?.deleted)) openProfilePopout(who, e.currentTarget);
  };
  const menu = (e: MouseEvent<HTMLElement>) => {
    if (!m.body) return;
    if (e.target instanceof Element && e.target.closest("img, video, a, button")) return;
    e.preventDefault();
    const x = e.clientX || e.currentTarget.getBoundingClientRect().left + 72;
    const y = e.clientY || e.currentTarget.getBoundingClientRect().top + 8;
    threadLineMenu.open(m.body, { x, y });
  };
  return (
    <div role="article" className={chat.msg} data-group-start={!grouped} data-row="" data-kind={m.kind} onContextMenu={menu} data-testid={`case-line-${m.kind}`}>
      {grouped ? (
        <span className={chat.gutterTime} title={formatFull(m.created_at)} aria-hidden="true">
          {formatTime(m.created_at)}
        </span>
      ) : (
        <>
          <button type="button" className={chat.avatar} onClick={(e) => profile(e)} tabIndex={-1} aria-hidden="true">
            {asNexa ? <NexaAvatar size={40} /> : <Avatar id={m.author_id} avatarId={person?.avatar_id ?? null} name={name} size={40} />}
          </button>
          <div className={chat.headerRow}>
            <button type="button" className={chat.author} onClick={(e) => profile(e)} dir="auto">
              {asNexa ? name : <PersonName userId={person?.deleted ? null : m.author_id} user={person?.deleted ? null : person}>{name}</PersonName>}
            </button>
            {asNexa ? (
              <>
                <OfficialBadge compact />
                <span className={s.staffCredit}>
                  {t("nexa.thread.sentBy")}{" "}
                  <button type="button" className={s.inlineLink} onClick={(e) => profile(e, m.author_id)}>
                    <PersonName userId={person?.deleted ? null : m.author_id} user={person?.deleted ? null : person}>{personName(person)}</PersonName>
                  </button>
                </span>
              </>
            ) : staffLine ? (
              <span className={s.staffTag}>{t("nexa.thread.asNexa")}</span>
            ) : null}
            <Time at={m.created_at} />
          </div>
        </>
      )}
      {m.body ? (
        <div className={chat.content} dir="auto">
          <Markdown source={m.body} ctx={ctx} />
        </div>
      ) : null}
      {m.files?.length ? <ThreadFiles files={m.files} kind={fileKind} from={name} /> : null}
    </div>
  );
}

function Time({ at }: { at: number }) {
  return (
    <time className={chat.time} title={formatFull(at)} dateTime={new Date(at).toISOString()}>
      {formatTimestamp(at)}
    </time>
  );
}

function SystemRow({ m, people, action, perspective, reportKind }: { m: ThreadLine; people: People; action: ModAction | undefined; perspective: "member" | "staff"; reportKind: BugKind | undefined }) {
  return (
    <div className={chat.system} data-event={m.event} data-status={m.event === "status" ? m.body : undefined} data-testid="case-line-system">
      <span className={chat.sysIcon} aria-hidden="true">
        <SystemIcon event={m.event} />
      </span>
      <span>
        {systemLineText(m, people, action, perspective, reportKind)}
        <span className={chat.time}>{formatTimestamp(m.created_at)}</span>
      </span>
    </div>
  );
}

const SYSTEM_ICONS: Record<string, LucideIcon> = { lifted: LockOpen, closed: CircleCheck, reopened: RotateCcw, expired: Clock, replaced: ShieldAlert, status: CircleDot };

function SystemIcon({ event }: { event: string }) {
  const Icon = SYSTEM_ICONS[event] ?? History;
  return <Icon size={18} />;
}

/** A file picked for the next line, on its way up or there. */
interface Staged {
  key: string;
  file: File;
  preview: string | null;
  progress: number;
  done?: CaseFile;
  failed?: string;
  abort: AbortController;
}

/** A phone-sized touch screen, where Return starts a new line, as it does in a conversation's box. */
function phoneKeyboard(): boolean {
  return typeof window !== "undefined" && !!window.matchMedia?.("(pointer: coarse) and (max-width: 767px)").matches;
}

function narrowScreen(): boolean {
  return typeof window !== "undefined" && !!window.matchMedia?.("(max-width: 767px)").matches;
}

/**
 * The conversation's own message box: the plus for files, the words, emoji
 * and formatting beside them, and Send. A file can be pasted or dropped on
 * the window too. What is picked shows in the tray over the box while it
 * goes up.
 */
function ThreadComposer({ onSend, upload, placeholder, dropName }: { onSend: (body: string, files: CaseFile[]) => Promise<void>; upload: ThreadUpload | undefined; placeholder: string; dropName: string }) {
  const toast = useUI((u) => u.toast);
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const [staged, setStaged] = useState<Staged[]>([]);
  const [picker, setPicker] = useState(false);
  const [formatting, setFormatting] = useState(false);
  const area = useRef<HTMLTextAreaElement>(null);
  const picked = useRef<HTMLInputElement>(null);
  const length = useMemo(() => [...text].length, [text]);
  const over = length > MAX_THREAD_MESSAGE;
  const uploading = staged.some((f) => !f.done && !f.failed);
  const ready = staged.filter((f) => f.done);
  const mobile = narrowScreen();

  useLayoutEffect(() => {
    const el = area.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${Math.min(el.scrollHeight, Math.round(window.innerHeight * 0.4))}px`;
  }, [text]);

  // what is still staged when the thread closes is let go: its uploads stop, its pictures are freed
  const stagedRef = useRef(staged);
  stagedRef.current = staged;
  useEffect(
    () => () => {
      for (const f of stagedRef.current) {
        f.abort.abort();
        if (f.preview) URL.revokeObjectURL(f.preview);
      }
    },
    [],
  );

  const add = useCallback(
    (list: File[] | FileList) => {
      const files = [...list];
      if (!upload || !files.length) return;
      const room = MAX_THREAD_FILES - stagedRef.current.length;
      if (room <= 0) {
        toast(t("nexa.thread.tooManyFiles", { count: MAX_THREAD_FILES }), "error");
        return;
      }
      const accepted = files.filter((f) => FILE_TYPES.includes(f.type));
      if (accepted.length < files.length) toast(t("nexa.thread.fileType"), "error");
      const fits = accepted.filter((f) => (f.type.startsWith("video/") ? f.size <= VIDEO_MAX : f.size <= IMAGE_MAX));
      if (fits.length < accepted.length) toast(t("nexa.chat.fileTooBig"), "error");
      if (fits.length > room) toast(t("nexa.thread.tooManyFiles", { count: MAX_THREAD_FILES }), "error");
      for (const file of fits.slice(0, room)) {
        const key = `${file.name}:${file.size}:${Math.random().toString(36).slice(2)}`;
        const item: Staged = { key, file, preview: URL.createObjectURL(file), progress: 0, abort: new AbortController() };
        setStaged((cur) => [...cur, item]);
        upload(file, file.name, { signal: item.abort.signal, onProgress: (loaded, total) => setStaged((cur) => cur.map((x) => (x.key === key ? { ...x, progress: total ? loaded / total : 0 } : x))) })
          .then((done) => setStaged((cur) => cur.map((x) => (x.key === key ? { ...x, done, progress: 1 } : x))))
          .catch((err: unknown) => {
            if (item.abort.signal.aborted) return;
            setStaged((cur) => cur.map((x) => (x.key === key ? { ...x, failed: describeError(err, t("nexa.thread.uploadFailed")) } : x)));
          });
      }
    },
    [upload, toast],
  );

  const drop = (key: string) => {
    setStaged((cur) => {
      const gone = cur.find((x) => x.key === key);
      if (gone) {
        gone.abort.abort();
        if (gone.preview) URL.revokeObjectURL(gone.preview);
      }
      return cur.filter((x) => x.key !== key);
    });
  };

  const send = async (words = text) => {
    const body = words.trim();
    if ((!body && !ready.length) || [...body].length > MAX_THREAD_MESSAGE || busy || uploading) return false;
    setBusy(true);
    try {
      await onSend(body, ready.map((f) => f.done!));
      setText("");
      for (const f of staged) if (f.preview) URL.revokeObjectURL(f.preview);
      setStaged([]);
      return true;
    } catch (err) {
      toast(describeError(err, t("nexa.thread.couldNotSend")), "error");
      return false;
    } finally {
      setBusy(false);
      area.current?.focus({ preventScroll: true });
    }
  };

  const onKey = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey && !phoneKeyboard() && !e.nativeEvent.isComposing) {
      e.preventDefault();
      void send();
    }
  };

  // a screenshot pasted goes with the next line
  const onPaste = (e: ClipboardEvent<HTMLTextAreaElement>) => {
    if (!upload) return;
    const files = [...e.clipboardData.files];
    if (!files.length) return;
    e.preventDefault();
    add(files);
  };

  const insert = (what: string) => {
    const el = area.current;
    const start = el?.selectionStart ?? text.length;
    const end = el?.selectionEnd ?? start;
    const next = text.slice(0, start) + what + text.slice(end);
    setText(next);
    requestAnimationFrame(() => {
      el?.focus({ preventScroll: true });
      el?.setSelectionRange(start + what.length, start + what.length);
    });
  };

  const tools = (el: HTMLElement) => {
    const anchor = menuAnchor(el, "end");
    threadToolsMenu.open({ openFormatting: () => setFormatting(true), openEmoji: () => setPicker(true) }, { x: anchor.right, y: anchor.top - 4, anchor, above: true, label: t("composer.more.label") });
  };

  const canSend = (!!text.trim() || ready.length > 0) && !over && !busy && !uploading;
  return (
    <div className={box.wrap} data-thread-box="">
      <DropOverlay name={dropName} hash={false} onFiles={(list) => add(list)} disabled={!upload || busy} />
      {formatting ? (
        <Suspense fallback={null}>
          <FormatEditor
            initial={text}
            placeholder={placeholder}
            max={MAX_THREAD_MESSAGE}
            onClose={() => {
              setFormatting(false);
              requestAnimationFrame(() => area.current?.focus({ preventScroll: true }));
            }}
            onInsert={(md) => {
              setFormatting(false);
              setText(md);
            }}
            onSend={(md) => {
              if ([...md.trim()].length > MAX_THREAD_MESSAGE) return false;
              setFormatting(false);
              void send(md);
              return true;
            }}
          />
        </Suspense>
      ) : null}
      {picker ? <ExpressionPicker onClose={() => setPicker(false)} onEmoji={(em) => insert(`${em.unicode} `)} /> : null}
      {staged.length ? (
        <div className={box.trayWrap}>
          <div className={box.trayHead}>
            <span>{t("composer.composer.trayCount", { count: staged.length, max: MAX_THREAD_FILES })}</span>
            <span className={box.traySize}>{formatBytes(staged.reduce((n, f) => n + f.file.size, 0))}</span>
          </div>
          <div className={box.tray} data-testid="case-staged">
            {staged.map((f) => (
              <div key={f.key} className={box.card} data-failed={!!f.failed || undefined} title={f.failed ?? f.file.name}>
                <div className={box.thumb}>
                  {f.preview && f.file.type.startsWith("image/") ? <img src={f.preview} alt="" /> : f.preview ? <video src={f.preview} muted /> : <FileIcon size={26} aria-hidden="true" />}
                </div>
                <div className={box.fname}>{f.file.name}</div>
                <div className={box.fsize}>{f.failed ? <span className={s.stagedFailed}>{f.failed}</span> : f.done ? formatBytes(f.file.size) : t("nexa.chat.uploading", { percent: Math.round(f.progress * 100) })}</div>
                {!f.done && !f.failed ? <span className={s.stagedBar} style={{ inlineSize: `${Math.round(f.progress * 100)}%` }} aria-hidden="true" /> : null}
                <div className={box.cardBtns}>
                  <button className={[box.cardBtn, box.del].join(" ")} aria-label={t("nexa.thread.removeFile", { name: f.file.name })} title={t("composer.composer.remove")} onClick={() => drop(f.key)}>
                    <Trash2 size={16} />
                  </button>
                </div>
              </div>
            ))}
          </div>
        </div>
      ) : null}
      <div className={box.box}>
        <input
          ref={picked}
          type="file"
          accept={FILE_TYPES.join(",")}
          multiple
          hidden
          onChange={(e) => {
            if (e.target.files) add(e.target.files);
            e.target.value = "";
          }}
          data-testid="case-file-input"
        />
        {upload ? (
          <button type="button" className={box.attach} aria-label={t("composer.composer.uploadAFile")} title={t("composer.composer.uploadAFile")} onClick={() => picked.current?.click()} disabled={busy || staged.length >= MAX_THREAD_FILES} data-testid="case-attach">
            <CirclePlus size={24} />
          </button>
        ) : (
          <span className={box.attachSpacer} />
        )}
        <textarea
          ref={area}
          className={box.ta}
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={onKey}
          onPaste={onPaste}
          rows={1}
          placeholder={placeholder}
          aria-label={placeholder}
          data-markdown=""
          data-testid="case-composer"
        />
        <div className={box.btns}>
          {mobile ? (
            <button type="button" className={box.iconBtn} data-on={picker || formatting} data-picker-toggle={picker || undefined} aria-label={t("composer.more.label")} aria-haspopup="menu" onClick={(e) => (picker ? setPicker(false) : tools(e.currentTarget))} data-testid="case-tools">
              <ChevronUp size={24} />
            </button>
          ) : (
            <>
              <button type="button" className={box.iconBtn} data-on={formatting} aria-label={t("composer.format.open")} title={t("composer.format.open")} onClick={() => setFormatting(true)} onPointerEnter={() => void loadFormatEditor().catch(() => undefined)}>
                <Type size={22} />
              </button>
              <button type="button" className={box.iconBtn} data-on={picker} data-picker-toggle={picker || undefined} aria-label={t("composer.composer.selectEmoji")} onClick={() => setPicker((v) => !v)}>
                <Smile size={24} />
              </button>
            </>
          )}
          <button
            type="button"
            className={[box.iconBtn, box.send, box.sendMobile, box.sendShown].join(" ")}
            aria-label={t("composer.composer.sendMessage")}
            title={t("composer.composer.sendMessage")}
            onMouseDown={(e) => {
              if (document.activeElement === area.current) e.preventDefault();
            }}
            onClick={() => void send()}
            disabled={!canSend}
            data-testid="case-send"
          >
            <SendHorizontal size={22} />
          </button>
        </div>
      </div>
      {length > MAX_THREAD_MESSAGE - 200 ? (
        <span className={box.counter} data-over={over || undefined}>
          {MAX_THREAD_MESSAGE - length}
        </span>
      ) : null}
    </div>
  );
}

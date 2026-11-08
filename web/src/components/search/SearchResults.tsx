import { PersonName } from "@/components/common/PersonName";
/**
 * Search results, one look for both search boxes: tabs for messages, media,
 * links and files, each with how many matched; a sort; and the results
 * themselves, read the way the chat shows them.
 *
 * Messages sorted by time sit under their day, as in the chat. Media is a
 * gallery under its month, and the picture viewer steps through all of it.
 * Links show their site as a letter, not its icon: fetching a site's icon
 * would tell that site what someone searched.
 */
import { ArrowDownUp, ExternalLink, Film, Image as ImageIcon, Link as LinkIcon, Download, Paperclip, Play, SearchX, Sticker as StickerIcon, Users } from "lucide-react";
import { memo, useEffect, useId, useMemo, useRef, useState, type KeyboardEvent, type ReactNode } from "react";
import type { Channel } from "@/api/types";
import { Avatar } from "@/components/common/Avatar";
import { Button } from "@/components/common/Button";
import { isolate } from "@/components/server/bidi";
import { menuAnchor } from "@/components/common/ContextMenu";
import { Lightbox, type LightboxItem } from "@/components/chat/Lightbox";
import type { AttachmentMeta } from "@/crypto/message";
import { t, translator } from "@/i18n";
import { avatarColor } from "@/lib/avatarColor";
import { saveAttachment } from "@/lib/download";
import { moveFocusInGrid } from "@/lib/gridKeys";
import { useRenderCtx } from "@/lib/markdownCtx";
import { mediaSrc, useMediaSrc } from "@/lib/mediaLinks";
import { highlight } from "@/lib/search";
import { formatBytes, formatDateDivider, formatFull, formatShortDate, formatTime, formatTimestamp, sameDay } from "@/lib/time";
import { parse, plainText } from "@/markdown";
import { filesOf, isVideo, linksOf, mediaOf, type LinkItem, type MediaItem, type SearchDoc } from "@/search/doc";
import { TABS, type Sort, type Tab } from "@/search/engine";
import { button, defineMenu } from "@/lib/contextMenus/registry";
import { excerptAround } from "@/search/text";
import { useChannels } from "@/stores/channels";
import { useSettings } from "@/stores/settings";
import { useUI } from "@/stores/ui";
import { fileGlyph } from "./SearchSuggestions";
import type { Names } from "./scope";
import type { SearchRun } from "./useSearch";
import { useThumb } from "./useThumb";
import s from "./Search.module.css";

const TAB_LABEL: Record<Tab, string> = { messages: "search.tab.messages", media: "search.tab.media", links: "search.tab.links", files: "search.tab.files" };
const SORT_LABEL: Record<Sort, string> = { newest: "search.sort.newest", oldest: "search.sort.oldest", relevance: "search.sort.relevance" };

/** How search results are sorted. */
const searchSortMenu = defineMenu<{ sort: Sort; onSort: (s: Sort) => void }>("searchSort", [
  [button("searchSortOrder", ({ sort, onSort }) => (["newest", "oldest", "relevance"] as const).map((o) => ({ id: `sort-${o}`, label: t(SORT_LABEL[o]), checked: sort === o, onSelect: () => onSort(o) })))],
]);

function count(n: number): string {
  try {
    return new Intl.NumberFormat(translator().locale).format(n);
  } catch {
    return String(n);
  }
}

/** "Today", "Yesterday", or the date: the heading of a day's results. */
function dayLabel(ms: number, now = Date.now()): string {
  if (sameDay(ms, now)) return t("search.date.today");
  const y = new Date(now);
  y.setDate(y.getDate() - 1);
  if (sameDay(ms, y.getTime())) return t("search.date.yesterday");
  return formatDateDivider(ms);
}

function monthLabel(ms: number): string {
  try {
    return new Intl.DateTimeFormat(translator().locale, { month: "long", year: "numeric" }).format(new Date(ms));
  } catch {
    return formatShortDate(ms);
  }
}

function hostOf(url: string): string {
  try {
    return new URL(url).hostname.replace(/^www\./, "");
  } catch {
    return url;
  }
}

/**
 * Where a result is, as its row says it: "#general" and its server, "@Bob",
 * a group by its name, a subchat by its name and then its conversation's.
 */
export function useWhere(names: Names): (channelId: string) => { label: string; server: string | null; group: boolean } {
  const byId = useChannels((c) => c.byId);
  return useMemo(
    () => (channelId: string) => {
      const ch: Channel | undefined = byId[channelId];
      if (!ch) return { label: "", server: null, group: false };
      if (ch.type === "dm") return { label: `@${names.channel(ch)}`, server: null, group: false };
      if (ch.type === "subchat") {
        const parent = ch.parent_id ? byId[ch.parent_id] : undefined;
        return { label: names.channel(ch), server: parent ? (parent.type === "dm" ? `@${names.channel(parent)}` : names.channel(parent)) : null, group: true };
      }
      return { label: names.channel(ch), server: names.serverName(ch.server_id), group: ch.type === "group_dm" };
    },
    [byId, names],
  );
}

/** A picture in a result: decrypted near the screen, blurred when it is a spoiler. */
function Thumb({ att, className }: { att: AttachmentMeta; className?: string | undefined }) {
  const ref = useRef<HTMLSpanElement>(null);
  const video = isVideo(att.type);
  const { url, failed } = useThumb(video ? null : att, ref);
  return (
    <span ref={ref} className={className} data-spoiler={att.spoiler || undefined}>
      {url ? <img src={url} alt="" draggable={false} /> : <span className={s.thumbEmpty} data-failed={failed || undefined}>{video ? <Film size={18} /> : <ImageIcon size={18} />}</span>}
      {video ? <span className={s.thumbPlay} aria-hidden="true"><Play size={14} /></span> : null}
    </span>
  );
}

/** A picture a message links to (a GIF, say), through this server's proxy like the chat's. */
function LinkThumb({ item, className }: { item: Extract<MediaItem, { kind: "link" }>; className?: string | undefined }) {
  const src = useMediaSrc(item.embed.type === "video" ? (item.embed.image ?? null) : (item.embed.image ?? item.embed.url));
  return (
    <span className={className}>
      {src ? <img src={src} alt="" loading="lazy" referrerPolicy="no-referrer" draggable={false} /> : <span className={s.thumbEmpty}>{item.embed.type === "video" ? <Film size={18} /> : <ImageIcon size={18} />}</span>}
      {item.embed.type === "video" ? <span className={s.thumbPlay} aria-hidden="true"><Play size={14} /></span> : null}
    </span>
  );
}

function MediaThumb({ item, className }: { item: MediaItem; className?: string | undefined }) {
  return item.kind === "file" ? <Thumb att={item.att} className={className} /> : <LinkThumb item={item} className={className} />;
}

/** One message found: a button of its own, or the inside of an option in the global search's list. */
export const MessageHit = memo(function MessageHit({ doc, needles, names, showWhere, dated, onJump, asOption = false }: { doc: SearchDoc; needles: string[]; names: Names; showWhere: boolean; dated: boolean; onJump: (d: SearchDoc) => void; asOption?: boolean }) {
  const serverId = useChannels((c) => c.byId[doc.c]?.server_id);
  const renderCtx = useRenderCtx(serverId);
  const where = useWhere(names)(doc.c);
  const imagePreviews = useSettings((st) => st.settings.imagePreviews);
  // a webhook's message may say everything in its embeds
  const readable = useMemo(() => (doc.t ? plainText(parse(doc.t), renderCtx) : doc.hx ? plainText(parse(doc.hx), renderCtx) : ""), [doc.t, doc.hx, renderCtx]);
  const runs = useMemo(() => highlight(excerptAround(readable, needles), needles), [readable, needles]);
  const media = mediaOf(doc).filter((m) => m.kind === "file" || imagePreviews);
  const files = filesOf(doc);
  const name = doc.hn ?? names.person(doc.a);
  const author = names.users[doc.a];
  const inside = (
    <>
      <Avatar id={doc.a} avatarId={author?.avatar_id ?? null} name={name} size={32} />
      <span className={s.hitMain}>
        <span className={s.hitHead}>
          <span className={s.hitName} dir="auto">{doc.hn ? name : <PersonName userId={doc.a} serverId={serverId}>{name}</PersonName>}</span>
          {showWhere && where.label ? (
            <span className={s.hitWhere} dir="auto">
              {/* a group has no # or @ to say what it is */}
              {where.group ? <Users size={12} aria-hidden="true" className={s.hitWhereIcon} /> : null}
              {where.label}
              {where.server ? <span className={s.hitServer}>{where.server}</span> : null}
            </span>
          ) : null}
          <time className={s.hitTime} dateTime={new Date(doc.ts).toISOString()} title={formatFull(doc.ts)}>{dated ? formatTime(doc.ts) : formatTimestamp(doc.ts)}</time>
        </span>
        {runs.length ? (
          <span className={s.hitText} dir="auto">
            {runs.map((r, i) => (r.hit ? <mark key={i}>{r.text}</mark> : <span key={i}>{r.text}</span>))}
          </span>
        ) : null}
        {media.length ? (
          <span className={s.thumbs}>
            {media.slice(0, 4).map((m, i) => (
              <MediaThumb key={i} item={m} className={s.thumb} />
            ))}
            {media.length > 4 ? <span className={s.thumbMore}>+{count(media.length - 4)}</span> : null}
          </span>
        ) : null}
        {files.length ? (
          <span className={s.fileChips}>
            {files.slice(0, 3).map((f) => {
              const Icon = fileGlyph(f.type, f.name);
              return (
                <span key={f.blob_id} className={s.fileChip}>
                  <Icon size={14} aria-hidden="true" />
                  <span dir="auto">{f.name}</span>
                </span>
              );
            })}
          </span>
        ) : null}
        {doc.st ? (
          <span className={s.stickerNote}>
            <StickerIcon size={14} aria-hidden="true" />
            <span>{doc.st.name ? t("chat.messageItem.stickerNamed", { name: doc.st.name }) : t("attachmentViews.sticker")}</span>
          </span>
        ) : null}
      </span>
    </>
  );
  if (asOption) return <div className={s.hit} data-compact="">{inside}</div>;
  return (
    <button type="button" className={s.hit} onClick={() => onJump(doc)} data-result={doc.id} data-testid="search-result">
      {inside}
    </button>
  );
});

function Skeleton() {
  return (
    <div className={s.skeleton} aria-hidden="true">
      {[0, 1, 2].map((i) => (
        <div key={i} className={s.skelRow}>
          <span className={s.skelAvatar} />
          <span className={s.skelLines}>
            <span style={{ width: "34%" }} />
            <span style={{ width: `${88 - i * 14}%` }} />
          </span>
        </div>
      ))}
    </div>
  );
}

/** Moves focus up and down a list of results with the arrow keys. */
function listKeys(e: KeyboardEvent<HTMLElement>): void {
  if (e.key !== "ArrowDown" && e.key !== "ArrowUp") return;
  const rows = [...e.currentTarget.querySelectorAll<HTMLElement>("[data-result]")];
  const at = rows.indexOf(document.activeElement as HTMLElement);
  if (at < 0) return;
  const next = rows[at + (e.key === "ArrowDown" ? 1 : -1)];
  if (!next) return;
  e.preventDefault();
  next.focus();
}

function MessageList({ run, names, sort, hereChannelId, onJump }: { run: SearchRun; names: Names; sort: Sort; hereChannelId?: string | undefined; onJump: (d: SearchDoc) => void }) {
  const dated = sort !== "relevance" || run.needles.length === 0;
  const out: ReactNode[] = [];
  let lastDay = "";
  for (const doc of run.result.docs) {
    if (dated) {
      const day = new Date(doc.ts).toDateString();
      if (day !== lastDay) {
        lastDay = day;
        out.push(<div key={`day-${doc.id}`} className={s.day} role="presentation">{dayLabel(doc.ts)}</div>);
      }
    }
    out.push(<MessageHit key={doc.id} doc={doc} needles={run.needles} names={names} showWhere={doc.c !== hereChannelId} dated={dated} onJump={onJump} />);
  }
  return <div className={s.hits} onKeyDown={listKeys}>{out}</div>;
}

interface Tile {
  key: string;
  doc: SearchDoc;
  item: MediaItem;
}

function tileName(item: MediaItem): string {
  if (item.kind === "file") return item.att.name;
  return item.embed.type === "gif" ? t("chat.embeds.gif") : hostOf(item.embed.url);
}

function MediaGrid({ run, names, sort, onJump }: { run: SearchRun; names: Names; sort: Sort; onJump: (d: SearchDoc) => void }) {
  const imagePreviews = useSettings((st) => st.settings.imagePreviews);
  const tiles = useMemo<Tile[]>(() => run.result.docs.flatMap((doc) => mediaOf(doc).filter((m) => m.kind === "file" || imagePreviews).map((item, i) => ({ key: `${doc.id}-${i}`, doc, item }))), [run.result.docs, imagePreviews]);
  const [open, setOpen] = useState<number | null>(null);
  const [focus, setFocus] = useState(0);
  const grid = useRef<HTMLDivElement>(null);
  const dated = sort !== "relevance" || run.needles.length === 0;
  // the viewer steps through every picture and clip found, not only one message's
  const items: LightboxItem[] = useMemo(
    () =>
      tiles.map(({ doc, item }) => {
        const from = t("search.media.from", { name: isolate((doc.hn ?? names.person(doc.a))), date: formatShortDate(doc.ts) });
        const jump = () => {
          setOpen(null);
          onJump(doc);
        };
        if (item.kind === "file") return { name: item.att.name, type: item.att.type, att: item.att, from, jump };
        const url = item.embed.type === "video" ? item.embed.url : (item.embed.image ?? item.embed.url);
        return { name: tileName(item), type: item.embed.type === "video" ? (item.embed.mime ?? "video/mp4") : "image/*", src: mediaSrc(url) ?? undefined, from, jump };
      }),
    [tiles, names, onJump],
  );
  const out: ReactNode[] = [];
  let lastMonth = "";
  tiles.forEach((tile, i) => {
    if (dated) {
      const d = new Date(tile.doc.ts);
      const month = `${d.getFullYear()}-${d.getMonth()}`;
      if (month !== lastMonth) {
        lastMonth = month;
        out.push(<div key={`m-${tile.key}`} className={s.month} role="presentation">{monthLabel(tile.doc.ts)}</div>);
      }
    }
    const video = tile.item.kind === "file" ? isVideo(tile.item.att.type) : tile.item.embed.type === "video";
    const label = t(video ? "search.media.videoFrom" : "search.media.imageFrom", { name: isolate((tile.doc.hn ?? names.person(tile.doc.a))), date: formatShortDate(tile.doc.ts) });
    out.push(
      <button key={tile.key} type="button" className={s.tile} data-tile="" data-testid="search-media-tile" tabIndex={i === focus ? 0 : -1} aria-label={label} title={tileName(tile.item)} onFocus={() => setFocus(i)} onClick={() => setOpen(i)}>
        <MediaThumb item={tile.item} className={s.tileImg} />
      </button>,
    );
  });
  return (
    <>
      <div
        ref={grid}
        className={s.grid}
        onKeyDown={(e) => {
          // up from the first picture goes back to the search box (see SearchResults)
          if (e.key === "ArrowUp" && document.activeElement === e.currentTarget.querySelector("[data-tile]")) return;
          if (moveFocusInGrid(e.currentTarget, "[data-tile]", e.key)) e.preventDefault();
        }}
      >
        {out}
      </div>
      {open !== null && items[open] ? <Lightbox items={items} start={open} onClose={() => setOpen(null)} /> : null}
    </>
  );
}

function LinkRows({ run, names, onJump }: { run: SearchRun; names: Names; onJump: (d: SearchDoc) => void }) {
  const where = useWhere(names);
  const rows = run.result.docs.flatMap((doc) => linksOf(doc).map((link: LinkItem, i) => ({ key: `${doc.id}-${i}`, doc, link })));
  return (
    <div className={s.hits} onKeyDown={listKeys}>
      {rows.map(({ key, doc, link }) => {
        const host = hostOf(link.url);
        const title = link.preview?.title;
        const w = where(doc.c);
        return (
          <div key={key} className={s.rowWrap} data-testid="search-link">
            <button type="button" className={s.linkRow} data-result={doc.id} onClick={() => onJump(doc)}>
              <span className={s.mono} style={{ background: avatarColor(host) }} aria-hidden="true">{(host[0] ?? "?").toUpperCase()}</span>
              <span className={s.rowMain}>
                <span className={s.rowHead}>
                  <span className={s.rowTitle} dir="ltr">{host}</span>
                  <time className={s.hitTime} dateTime={new Date(doc.ts).toISOString()} title={formatFull(doc.ts)}>{formatShortDate(doc.ts)}</time>
                </span>
                <span className={s.linkTitle} dir="auto">{title ?? link.url.replace(/^https?:\/\//, "")}</span>
                <span className={s.rowMeta} dir="auto">{t("search.byIn", { name: isolate((doc.hn ?? names.person(doc.a))), where: isolate(w.label) })}</span>
              </span>
            </button>
            <a className={s.rowAction} href={link.url} target="_blank" rel="noopener noreferrer" aria-label={t("search.links.open", { host: isolate(host) })} title={t("search.links.open", { host: isolate(host) })}>
              <ExternalLink size={16} />
            </a>
          </div>
        );
      })}
    </div>
  );
}

function FileRows({ run, names, onJump }: { run: SearchRun; names: Names; onJump: (d: SearchDoc) => void }) {
  const where = useWhere(names);
  const toast = useUI((u) => u.toast);
  const rows = run.result.docs.flatMap((doc) => filesOf(doc).map((att) => ({ key: `${doc.id}-${att.blob_id}`, doc, att })));
  return (
    <div className={s.hits} onKeyDown={listKeys}>
      {rows.map(({ key, doc, att }) => {
        const Icon = fileGlyph(att.type, att.name);
        const w = where(doc.c);
        return (
          <div key={key} className={s.rowWrap} data-testid="search-file">
            <button type="button" className={s.linkRow} data-result={doc.id} onClick={() => onJump(doc)}>
              <span className={s.fileTile} aria-hidden="true"><Icon size={20} /></span>
              <span className={s.rowMain}>
                <span className={s.rowHead}>
                  <span className={s.rowTitle} dir="auto">{att.name}</span>
                  <span className={s.hitTime}>{formatBytes(att.size)}</span>
                </span>
                <span className={s.rowMeta} dir="auto">{t("search.byIn", { name: isolate((doc.hn ?? names.person(doc.a))), where: isolate(w.label) })}</span>
                <span className={s.rowMeta}>{formatShortDate(doc.ts)}</span>
              </span>
            </button>
            <button
              type="button"
              className={s.rowAction}
              aria-label={t("search.files.download", { name: isolate(att.name) })}
              title={t("search.files.download", { name: isolate(att.name) })}
              onClick={() => void saveAttachment(att).catch(() => toast(t("search.files.downloadFailed"), "error"))}
            >
              <Download size={16} />
            </button>
          </div>
        );
      })}
    </div>
  );
}

/** When nothing matched: what was searched, and what to try. */
function Nothing({ tab, scoped, onWiden, filters, walking, otherTab }: { tab: Tab; scoped: boolean; onWiden?: (() => void) | undefined; filters: boolean; walking: boolean; otherTab: boolean }) {
  const title = tab === "messages" ? t("search.none.messages") : tab === "media" ? t("search.none.media") : tab === "links" ? t("search.none.links") : t("search.none.files");
  return (
    <div className={s.nothing} data-testid="search-nothing">
      <SearchX size={28} aria-hidden="true" className={s.nothingIcon} />
      <p className={s.nothingTitle}>{title}</p>
      <ul className={s.tips}>
        {otherTab ? <li>{t("search.none.tipOtherTab")}</li> : <li>{t("search.none.tipSpelling")}</li>}
        {filters ? <li>{t("search.none.tipFilters")}</li> : null}
        {walking ? <li>{t("search.none.tipIndexing")}</li> : null}
      </ul>
      {scoped && onWiden ? (
        <Button size="sm" variant="secondary" onClick={onWiden} data-testid="search-widen">
          {t("search.everywhere")}
        </Button>
      ) : null}
    </div>
  );
}

export function SearchResults({
  run,
  tab,
  onTab,
  sort,
  onSort,
  names,
  onJump,
  hereChannelId,
  scoped,
  onWiden,
  walking,
  onTop,
}: {
  run: SearchRun;
  tab: Tab;
  onTab: (t: Tab) => void;
  sort: Sort;
  onSort: (s: Sort) => void;
  names: Names;
  onJump: (d: SearchDoc) => void;
  /** The conversation on screen, whose results need not say where they are. */
  hereChannelId?: string | undefined;
  /** The search is narrowed to one conversation or server, and can be widened. */
  scoped: boolean;
  onWiden?: (() => void) | undefined;
  /** The index is still walking what this search covers. */
  walking: boolean;
  /** Up from the first result: back to the search box. */
  onTop?: (() => void) | undefined;
}) {
  const id = useId();
  const scroller = useRef<HTMLDivElement>(null);
  const sentinel = useRef<HTMLDivElement>(null);
  const { counts } = run.result;
  // A new search, a new tab or a new order starts at the top. By what is
  // asked, not the object: names change with every member and profile update,
  // which remade it and threw a reader back to the start mid-scroll.
  const asked = JSON.stringify(run.resolved);
  useEffect(() => {
    scroller.current?.scrollTo?.({ top: 0 });
  }, [asked, tab, sort]);
  // more come in as the end of the list comes near
  useEffect(() => {
    const el = sentinel.current;
    if (!el || !run.hasMore || typeof IntersectionObserver === "undefined") return;
    const io = new IntersectionObserver((entries) => entries.some((e) => e.isIntersecting) && run.more(), { root: scroller.current, rootMargin: "600px 0px" });
    io.observe(el);
    return () => io.disconnect();
  }, [run.hasMore, run.more, run.result.docs.length]);

  const tabKeys = (e: KeyboardEvent<HTMLButtonElement>) => {
    const at = TABS.indexOf(tab);
    const rtl = document.documentElement.dir === "rtl";
    let next = -1;
    if (e.key === "ArrowRight") next = (at + (rtl ? -1 : 1) + TABS.length) % TABS.length;
    else if (e.key === "ArrowLeft") next = (at + (rtl ? 1 : -1) + TABS.length) % TABS.length;
    else if (e.key === "Home") next = 0;
    else if (e.key === "End") next = TABS.length - 1;
    if (next < 0) return;
    e.preventDefault();
    onTab(TABS[next]!);
    const list = e.currentTarget.parentElement;
    requestAnimationFrame(() => list?.querySelector<HTMLElement>(`[data-tab="${TABS[next]}"]`)?.focus());
  };

  const filters = run.parsed.tokens.some((tk) => tk.kind === "filter" && tk.value);
  const shownEmpty = !run.pending && run.result.docs.length === 0;
  let body: ReactNode;
  if (run.pending && run.result.docs.length === 0) body = <Skeleton />;
  else if (shownEmpty) body = <Nothing tab={tab} scoped={scoped} onWiden={onWiden} filters={filters} walking={walking} otherTab={tab !== "messages" && counts.messages > 0} />;
  else if (tab === "media") body = <MediaGrid run={run} names={names} sort={sort} onJump={onJump} />;
  else if (tab === "links") body = <LinkRows run={run} names={names} onJump={onJump} />;
  else if (tab === "files") body = <FileRows run={run} names={names} onJump={onJump} />;
  else body = <MessageList run={run} names={names} sort={sort} hereChannelId={hereChannelId} onJump={onJump} />;

  return (
    <div className={s.results}>
      <div className={s.tabsRow}>
        <div className={s.tabs} role="tablist" aria-label={t("search.tab.label")}>
          {TABS.map((tb) => (
            <button
              key={tb}
              type="button"
              role="tab"
              id={`${id}-${tb}`}
              data-tab={tb}
              aria-selected={tab === tb}
              aria-controls={`${id}-panel`}
              tabIndex={tab === tb ? 0 : -1}
              className={s.tab}
              data-empty={counts[tb] === 0 || undefined}
              onClick={() => onTab(tb)}
              onKeyDown={tabKeys}
              data-testid={`search-tab-${tb}`}
            >
              {tb === "messages" ? null : tb === "media" ? <ImageIcon size={14} aria-hidden="true" /> : tb === "links" ? <LinkIcon size={14} aria-hidden="true" /> : <Paperclip size={14} aria-hidden="true" />}
              <span>{t(TAB_LABEL[tb])}</span>
              <span className={s.tabCount}>{count(counts[tb])}</span>
            </button>
          ))}
        </div>
        <button
          type="button"
          className={s.sortBtn}
          aria-label={t("search.sort.label", { sort: t(SORT_LABEL[sort]) })}
          title={t("search.sort.label", { sort: t(SORT_LABEL[sort]) })}
          data-testid="search-sort"
          onClick={(e) => {
            const anchor = menuAnchor(e.currentTarget);
            searchSortMenu.open({ sort, onSort }, { x: anchor.left, y: anchor.bottom + 4, anchor, label: t("search.sort.menu") });
          }}
        >
          <ArrowDownUp size={16} aria-hidden="true" />
          <span className={s.sortLabel}>{t(SORT_LABEL[sort])}</span>
        </button>
      </div>
      <div
        ref={scroller}
        className={s.scroller}
        data-search-scroller=""
        role="tabpanel"
        id={`${id}-panel`}
        aria-labelledby={`${id}-${tab}`}
        aria-busy={run.pending || undefined}
        tabIndex={-1}
        onKeyDown={(e) => {
          if (e.key !== "ArrowUp" || e.defaultPrevented || !onTop) return;
          const first = scroller.current?.querySelector("[data-result], [data-tile]");
          if (first && document.activeElement === first) {
            e.preventDefault();
            onTop();
          }
        }}
      >
        {/* the count is said as it changes, politely */}
        <span className="visually-hidden" role="status">{run.pending ? "" : t("search.announce", { count: counts[tab] })}</span>
        {body}
        {run.hasMore ? (
          <div ref={sentinel} className={s.moreRow}>
            <Button size="sm" variant="ghost" onClick={run.more} data-testid="search-more">{t("search.more")}</Button>
          </div>
        ) : null}
      </div>
    </div>
  );
}

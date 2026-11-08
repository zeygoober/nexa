/**
 * Search, from the box above a conversation.
 *
 * It looks in that conversation (a DM or a group, with its subchats) or in
 * that server, and says so with a chip that widens it to everything when
 * taken away. Beside the chat on a wide screen; on a phone or a tablet it is
 * a sheet over the whole screen with its own box, closed by the back arrow,
 * the back gesture or Escape.
 *
 * Everything below the box is the same as in the global search
 * (QuickSwitcher): the chips, the suggestions, the tabs and the results.
 */
import { ArrowLeft, X } from "lucide-react";
import { useCallback, useEffect, useId, useMemo, useRef, useState, type KeyboardEvent, type MutableRefObject } from "react";
import { createPortal } from "react-dom";
import { useNavigate } from "react-router-dom";
import type { Channel } from "@/api/types";
import { isTopLayer } from "@/components/common/Modal";
import { SearchBox } from "@/components/common/SearchBox";
import { isolate } from "@/components/server/bidi";
import { useFocusTrap } from "@/components/common/useFocusTrap";
import { t } from "@/i18n";
import type { SearchDoc } from "@/search/doc";
import type { Sort, Tab } from "@/search/engine";
import { searchableChannels } from "@/search/indexer";
import { clearRecentSearches, rememberSearch } from "@/search/recent";
import { compareIds } from "@/lib/ids";
import { useChannels } from "@/stores/channels";
import { useSettings } from "@/stores/settings";
import { useUI } from "@/stores/ui";
import { IndexStatus, useScopeProgress } from "./IndexStatus";
import { QueryChips } from "./QueryChips";
import { EVERYWHERE, scopeName, scopeOf, useNames, type SearchScope } from "./scope";
import { SearchResults } from "./SearchResults";
import { SearchSuggestions, useSuggestionKeys } from "./SearchSuggestions";
import { replacePiece, partialAt, suggest, type Suggestion } from "./suggest";
import { useSearch } from "./useSearch";
import s from "./Search.module.css";

/** The words in the box of a conversation's search: "Search @Bob", "Search Nexa Design Lab". */
export function useSearchPlaceholder(channel: Channel): string {
  const byId = useChannels((c) => c.byId);
  const names = useNames();
  const scope = scopeOf(channel, byId);
  return t("search.box.placeholder", { name: isolate(scopeName(scope, byId, names)) });
}

/** The conversations offered for in:, the ones in scope first, then by how recently they were used. */
export function suggestionChannels(byId: Record<string, Channel>, first: string[] | null): Channel[] {
  const all = searchableChannels(byId).sort((a, b) => compareIds(b.last_message_id ?? b.id, a.last_message_id ?? a.id));
  if (!first) return all;
  const inside = new Set(first);
  return [...all.filter((c) => inside.has(c.id)), ...all.filter((c) => !inside.has(c.id))];
}

export function SearchPanel({
  channel,
  query,
  onQuery,
  caret,
  inline = false,
  onClose,
  onJump,
  keysRef,
  onSuggest,
}: {
  channel: Channel;
  query: string;
  onQuery: (v: string) => void;
  /** Where the caret is in the header's box, for the suggestions. */
  caret?: number;
  /** On a phone or tablet: a sheet over the screen with its own box. */
  inline?: boolean;
  onClose: () => void;
  onJump?: (() => void) | undefined;
  /** Filled with the key handler, for the box in the chat's header. */
  keysRef?: MutableRefObject<((e: KeyboardEvent<HTMLInputElement>) => boolean) | null>;
  /** The chosen suggestion's id and the list's, for the header box's aria attributes. */
  onSuggest?: (active: string | undefined, listId: string | undefined) => void;
}) {
  const nav = useNavigate();
  const byId = useChannels((c) => c.byId);
  const names = useNames();
  const home = useMemo(() => scopeOf(channel, byId), [channel, byId]);
  const homeKey = home.kind === "all" ? "all" : `${home.kind}:${home.id}`;
  const [scope, setScope] = useState<SearchScope>(home);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => setScope(home), [homeKey]);
  const [tab, setTab] = useState<Tab>("messages");
  const [sort, setSort] = useState<Sort>("newest");
  const run = useSearch({ raw: query, scope, tab, sort, names });
  const { st, p } = useScopeProgress(run.channels);
  const recent = useSettings((x) => x.settings.searchRecent);
  const homeLabel = scopeName(home, byId, names);
  const listId = useId();
  const input = useRef<HTMLInputElement | null>(null);
  const [ownCaret, setOwnCaret] = useState(query.length);
  const at = inline ? ownCaret : (caret ?? query.length);

  // Suggestions, put away with Escape until the query changes again.
  // A choice that finishes a filter puts them away too, or the next ones came
  // up over the results it had just asked for.
  const [dismissed, setDismissed] = useState(false);
  const finished = useRef<string | null>(null);
  useEffect(() => setDismissed(finished.current === query), [query]);
  const nearby = useMemo(() => (home.kind === "conversation" ? (byId[home.id]?.recipient_ids ?? []) : []), [home, byId]);
  const candidates = useMemo(() => suggestionChannels(byId, run.channels), [byId, run.channels]);
  const groups = useMemo(
    () => (dismissed ? [] : suggest({ raw: query, caret: Math.min(at, query.length), names, channels: candidates, nearby, recent, now: Date.now() })),
    [dismissed, query, at, names, candidates, nearby, recent],
  );
  const pick = useCallback(
    (sg: Suggestion) => {
      if (sg.kind === "recent") rememberSearch(sg.text);
      finished.current = sg.kind === "key" ? null : sg.text;
      onQuery(sg.text);
      setOwnCaret(sg.caret);
      // the caret goes where the suggestion left it: after a finished filter, or inside one still to fill
      requestAnimationFrame(() => {
        const el = input.current ?? document.querySelector<HTMLInputElement>("input[data-header-search]");
        el?.focus({ preventScroll: true });
        el?.setSelectionRange(sg.caret, sg.caret);
      });
    },
    [onQuery],
  );
  const onDate = useCallback(
    (key: string, value: string) => {
      const piece = partialAt(query, Math.min(at, query.length));
      if (!piece) return;
      const next = replacePiece(query, piece, `${key}:${value}`, true);
      pick({ id: "date", kind: "date", text: next.text, caret: next.caret });
    },
    [query, at, pick],
  );
  const dismiss = useCallback(() => setDismissed(true), []);
  const sk = useSuggestionKeys(query.trim() ? groups : [], pick, dismiss);
  const floating = query.trim() ? groups : [];
  const activeId = sk.activeId ? `${listId}-${sk.activeId}` : undefined;
  useEffect(() => onSuggest?.(activeId, floating.length ? listId : undefined), [activeId, floating.length, listId, onSuggest]);

  const panel = useRef<HTMLDivElement>(null);
  const keys = (e: KeyboardEvent<HTMLInputElement>): boolean => {
    if (sk.key(e)) return true;
    if (e.key === "Enter" && !e.nativeEvent.isComposing) {
      e.preventDefault();
      rememberSearch(query);
      return true;
    }
    // down from the box goes into the results
    if (e.key === "ArrowDown") {
      const first = panel.current?.querySelector<HTMLElement>("[data-result], [data-tile]");
      if (first) {
        e.preventDefault();
        first.focus();
        return true;
      }
    }
    return false;
  };
  if (keysRef) keysRef.current = keys;

  const jump = (d: SearchDoc) => {
    const ch = byId[d.c];
    if (!ch) return;
    rememberSearch(query);
    nav(ch.server_id ? `/channels/${ch.server_id}/${ch.id}/${d.id}` : `/channels/@me/${ch.id}/${d.id}`);
    useUI.getState().setJumpTo({ channelId: ch.id, messageId: d.id });
    onJump?.();
  };

  // Covering the screen, the sheet is a dialog: focus stays in it, Escape and
  // the back gesture close it, and focus goes back to what opened it.
  useFocusTrap(panel, inline, () => input.current);
  useEffect(() => {
    if (!inline) return;
    const opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const onKey = (e: globalThis.KeyboardEvent) => {
      if (e.key !== "Escape" || e.defaultPrevented || !isTopLayer(panel.current)) return;
      e.preventDefault();
      e.stopPropagation();
      onClose();
    };
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("keydown", onKey);
      if (opener?.isConnected) opener.focus({ preventScroll: true });
    };
  }, [inline, onClose]);

  const walking = p.done < p.total && st.mode !== "closed";
  const chips = (
    <QueryChips
      raw={query}
      parsed={run.parsed}
      resolved={run.resolved}
      names={names}
      onChange={onQuery}
      scope={scope}
      scopeLabel={scopeName(scope, byId, names)}
      onWiden={() => setScope(EVERYWHERE)}
      narrowTo={homeLabel}
      onNarrow={() => setScope(home)}
    />
  );
  const body = (
    <div
      className={s.panelBody}
      // pressing or scrolling what is under the suggestions is done with them
      onPointerDownCapture={(e) => {
        if (floating.length && !(e.target as Element).closest?.("[data-testid=search-suggest]")) dismiss();
      }}
    >
      {floating.length ? <SearchSuggestions groups={floating} active={sk.active} listId={listId} names={names} onPick={pick} onHover={sk.setActive} onDate={onDate} /> : null}
      {run.empty ? (
        <div className={s.intro}>
          <p className={s.introTitle}>{scope.kind === "all" ? t("search.intro.everywhere") : t("search.intro.scoped", { name: isolate(scopeName(scope, byId, names)) })}</p>
          <p className={s.introText}>{t("search.intro.private")}</p>
          {/* with a filter half typed, its suggestions are the ones floating above */}
          {query.trim() ? null : <SearchSuggestions groups={groups} active={-1} listId={`${listId}-intro`} names={names} onPick={pick} onHover={() => undefined} onDate={onDate} onClearRecent={clearRecentSearches} floating={false} />}
        </div>
      ) : (
        <SearchResults run={run} tab={tab} onTab={setTab} sort={sort} onSort={setSort} names={names} onJump={jump} hereChannelId={channel.id} scoped={scope.kind !== "all"} onWiden={() => setScope(EVERYWHERE)} walking={walking} onTop={() => (input.current ?? document.querySelector<HTMLInputElement>("input[data-header-search]"))?.focus()} />
      )}
    </div>
  );

  if (!inline) {
    return (
      <aside ref={panel} className={s.panel} data-testid="search-panel" aria-label={t("search.panel.label")}>
        <div className={s.panelTop}>
          {chips ?? <span className={s.panelTitle}>{t("search.panel.label")}</span>}
          <button type="button" className={s.close} aria-label={t("search.panel.close")} title={t("search.panel.close")} onClick={onClose}>
            <X size={18} />
          </button>
        </div>
        {body}
        <IndexStatus channels={run.channels} scope={scope} />
      </aside>
    );
  }
  return createPortal(
    <div ref={panel} className={s.sheet} role="dialog" aria-modal="true" aria-label={t("search.panel.label")} data-modal-layer="" data-testid="search-panel">
      <div className={s.sheetTop}>
        <button type="button" className={s.back} aria-label={t("search.panel.close")} onClick={onClose} data-testid="search-back">
          <ArrowLeft size={22} />
        </button>
        <SearchBox
          ref={input}
          className={s.sheetBox}
          value={query}
          onChange={(v) => {
            onQuery(v);
            setOwnCaret(v.length);
          }}
          placeholder={scope.kind === "all" ? t("search.box.placeholderEverywhere") : t("search.box.placeholder", { name: isolate(scopeName(scope, byId, names)) })}
          shorterPlaceholders={[t("chat.chat.search")]}
          aria-label={t("search.box.label")}
          autoFocus
          data-testid="search-input-mobile"
          dir="auto"
          role="combobox"
          aria-expanded={floating.length > 0}
          aria-controls={floating.length ? listId : undefined}
          aria-activedescendant={activeId}
          aria-autocomplete="list"
          onSelect={(e) => setOwnCaret(e.currentTarget.selectionStart ?? query.length)}
          onKeyDown={(e) => void keys(e)}
        />
      </div>
      {chips}
      {body}
      <IndexStatus channels={run.channels} scope={scope} />
    </div>,
    document.body,
  );
}

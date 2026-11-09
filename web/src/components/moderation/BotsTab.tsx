import { PersonName } from "@/components/common/PersonName";
import { Bot, CalendarDays, Eye, Gavel, Search, Server, UserRound } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { ApiError } from "@/api/client";
import { moderation as modApi, type AppSort, type ModApp, type People } from "@/api/moderation";
import { AppIcon } from "@/components/apps/AppIcon";
import { Avatar } from "@/components/common/Avatar";
import { Button } from "@/components/common/Button";
import { Select } from "@/components/common/Select";
import { Spinner } from "@/components/common/Spinner";
import w from "@/components/chat/webhook/Webhook.module.css";
import { profileTheme } from "@/lib/avatarColor";
import { personName, previewText } from "@/lib/moderation";
import { formatShortDate } from "@/lib/time";
import { useModeration } from "@/stores/moderation";
import { useUI } from "@/stores/ui";
import { t, Tr } from "@/i18n";
import m from "./Moderation.module.css";
import p from "./Sheet.module.css";

const PAGE = 48;

/**
 * Every app on Nexa: its picture, name and description, who owns it, where
 * it is added and whether it reads every message it can see. Its bot is an
 * account, moderated from its own page like anyone's; a banned or turned off
 * bot's app does nothing anywhere.
 */
export function BotsTab() {
  const nav = useNavigate();
  const mergePeople = useModeration((s) => s.mergePeople);
  const [q, setQ] = useState("");
  const [sort, setSort] = useState<AppSort>("newest");
  const [apps, setApps] = useState<ModApp[] | null>(null);
  const [people, setPeople] = useState<People>({});
  const [total, setTotal] = useState<number | null>(null);
  const [more, setMore] = useState(false);
  const [loading, setLoading] = useState(false);
  const [failed, setFailed] = useState(false);
  const request = useRef(0);
  const sentinel = useRef<HTMLDivElement>(null);
  const scroller = useRef<HTMLDivElement>(null);

  const load = useCallback(
    async (offset: number) => {
      const id = ++request.current;
      setLoading(true);
      setFailed(false);
      try {
        const r = await modApi.apps({ q, sort, offset, limit: PAGE });
        if (id !== request.current) return;
        setApps((cur) => (offset ? [...(cur ?? []), ...r.apps.filter((x) => !cur?.some((y) => y.id === x.id))] : r.apps));
        setPeople((cur) => ({ ...cur, ...r.people }));
        mergePeople(r.people);
        setTotal(r.total);
        setMore(r.has_more);
        if (!offset) scroller.current?.scrollTo({ top: 0 });
      } catch {
        if (id === request.current) setFailed(true);
      } finally {
        if (id === request.current) setLoading(false);
      }
    },
    [q, sort, mergePeople],
  );

  // a new search or order starts the list again (typing waits a moment)
  useEffect(() => {
    request.current++;
    setMore(false);
    const h = setTimeout(() => {
      setApps(null);
      void load(0);
    }, q ? 250 : 0);
    return () => clearTimeout(h);
  }, [q, sort, load]);

  // the next page loads as the end of the list comes into view
  useEffect(() => {
    const el = sentinel.current;
    if (!el || !more || failed) return;
    const io = new IntersectionObserver((entries) => {
      if (entries.some((e) => e.isIntersecting) && !loading) void load(apps?.length ?? 0);
    }, { rootMargin: "400px" });
    io.observe(el);
    return () => io.disconnect();
  }, [more, loading, load, failed, apps?.length]);

  return (
    <div className={m.scroll} ref={scroller} data-testid="bots-tab">
      <div className={m.toolbar}>
        <label className={m.search}>
          <Search size={17} aria-hidden="true" />
          <input value={q} onChange={(e) => setQ(e.target.value)} placeholder={t("moderation.bots.search")} aria-label={t("moderation.bots.search")} data-testid="bots-search" />
        </label>
        {total !== null ? <span className={p.total}>{t("moderation.bots.total", { count: total })}</span> : null}
        <Select className={m.sort} value={sort} onChange={(e) => setSort(e.target.value as AppSort)} aria-label={t("moderation.sort.label")}>
          <option value="newest">{t("moderation.sort.newest")}</option>
          <option value="oldest">{t("moderation.sort.oldest")}</option>
          <option value="servers">{t("moderation.bots.sortServers")}</option>
          <option value="name">{t("moderation.sort.name")}</option>
        </Select>
      </div>

      {apps === null && !failed ? (
        <div className={m.grid} aria-busy="true">
          {Array.from({ length: 6 }, (_, i) => (
            <div key={i} className={m.skeleton} />
          ))}
        </div>
      ) : null}
      {failed && !apps?.length ? (
        <div className={m.empty}>
          <p>{t("moderation.loadFailed")}</p>
          <Button size="sm" variant="secondary" onClick={() => void load(0)}>
            {t("moderation.retry")}
          </Button>
        </div>
      ) : null}
      {apps && apps.length === 0 ? (
        <div className={m.empty}>
          <Bot size={40} aria-hidden="true" />
          <span className={m.emptyTitle}>{q ? t("moderation.noMatches") : t("moderation.bots.none")}</span>
          {q ? (
            <Button size="sm" variant="secondary" onClick={() => setQ("")}>
              {t("moderation.clearFilters")}
            </Button>
          ) : null}
        </div>
      ) : null}
      {apps && apps.length > 0 ? (
        <div className={m.grid}>
          {apps.map((a) => (
            <AppCard key={a.id} a={a} people={people} onModerate={() => nav(`/channels/@admin/users/${a.id}`)} onOwner={() => nav(`/channels/@admin/users/${a.owner_id}`)} onChange={(next) => setApps((cur) => cur?.map((x) => (x.id === next.id ? next : x)) ?? null)} />
          ))}
        </div>
      ) : null}
      <div ref={sentinel} className={m.sentinel} />
      {loading && apps ? (
        <div className={m.loadingRow}>
          <Spinner size={24} label={t("chat.pins.loading")} />
        </div>
      ) : null}
    </div>
  );
}

function AppCard({ a, people, onModerate, onOwner, onChange }: { a: ModApp; people: People; onModerate: () => void; onOwner: () => void; onChange: (a: ModApp) => void }) {
  const owner = people[a.owner_id];
  const bot = people[a.id];
  const toast = useUI((u) => u.toast);
  const [busy, setBusy] = useState(false);
  const setHidden = async (hidden: boolean) => {
    setBusy(true);
    try {
      await modApi.setAppDiscovery(a.id, hidden);
      onChange({ ...a, discovery_hidden: hidden });
    } catch (err) {
      toast(err instanceof ApiError ? err.message : t("moderation.loadFailed"), "error");
    } finally {
      setBusy(false);
    }
  };
  return (
    <article className={m.card} data-testid="bot-card" data-app={a.id} data-state={a.disabled ? "banned" : undefined}>
      <div className={m.cardBanner} style={{ background: profileTheme(null, null, a.id) }} />
      <div className={m.cardAvatarWrap}>
        <span className={p.cardIcon}>
          <AppIcon app={{ id: a.id, name: a.name, icon_id: a.icon_id, bot: bot ?? null }} size={64} square />
        </span>
      </div>
      <div className={m.cardBody}>
        <div className={m.cardName}>
          <span className={m.cardNameText} dir="auto">{a.name}</span>
          <span className={w.appTag}>{t("webhook.appTag")}</span>
        </div>
        {/* the one thing about an app the staff most need to see at a glance */}
        {a.reads_all ? (
          <span className={m.readsAll} data-testid="bot-card-reads-all">
            <Eye size={13} aria-hidden="true" /> {t("moderation.bots.readsAll")}
          </span>
        ) : null}
        <button type="button" className={p.ownerChip} onClick={onOwner} data-testid="bot-card-owner">
          <Avatar id={a.owner_id} avatarId={owner?.avatar_id ?? null} name={personName(owner)} size={18} />
          <span>{<Tr id="moderation.servers.ownedBy" values={{ name: <PersonName user={owner}>{personName(owner)}</PersonName> }} />}</span>
        </button>
        <p className={m.cardAbout} data-empty={!a.description || undefined} dir="auto">
          {a.description ? previewText(a.description) : t("moderation.servers.noDescription")}
        </p>
        <div className={m.cardMeta}>
          <span>
            <Server size={13} aria-hidden="true" />
            {t("apps.portal.inServers", { count: a.servers })}
          </span>
          <span>
            <UserRound size={13} aria-hidden="true" />
            {t("apps.portal.onAccounts", { count: a.accounts })}
          </span>
          <span>
            <CalendarDays size={13} aria-hidden="true" />
            {formatShortDate(a.created_at)}
          </span>
        </div>
        <div className={m.cardActions}>
          <Button size="sm" variant="primary" onClick={onModerate} data-testid="bot-card-moderate">
            <Gavel size={15} aria-hidden="true" /> {t("moderation.moderate")}
          </Button>
          {a.listed || a.discovery_hidden ? (
            <Button size="sm" variant="secondary" loading={busy} onClick={() => void setHidden(!a.discovery_hidden)} data-testid="bot-card-discovery">
              {t(a.discovery_hidden ? "moderation.bots.allow" : "moderation.bots.hide")}
            </Button>
          ) : null}
        </div>
        {a.disabled ? <p className={m.blockedNote}>{t("moderation.bots.disabled")}</p> : null}
      </div>
    </article>
  );
}

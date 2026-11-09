import { Bot, Eye, Plus } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { discovery as discoveryApi, type ListedApp } from "@/api/discovery";
import { AppIcon } from "@/components/apps/AppIcon";
import { Button } from "@/components/common/Button";
import { SearchBox } from "@/components/common/SearchBox";
import { t } from "@/i18n";
import chat from "@/components/chat/Chat.module.css";
import s from "./Discovery.module.css";

type Sort = "popular" | "newest";
const SORTS: Sort[] = ["popular", "newest"];

/** A bot on Discover: what it is, whether it reads everything, and the way to add it (the consent page says the rest). */
function BotCard({ app }: { app: ListedApp }) {
  const nav = useNavigate();
  const location = useLocation();
  return (
    <article className={s.botCard} data-testid="bot-discovery-card" data-app={app.id}>
      <div className={s.botTop}>
        <AppIcon app={{ id: app.id, name: app.name, icon_id: app.icon_id, bot: { avatar_id: app.avatar_id } }} size={56} square />
        <div className={s.botNames}>
          <h3 className={s.name} dir="auto"><span>{app.name}</span></h3>
          <span className={s.botServers}>{t("apps.portal.inServers", { count: app.servers })}</span>
        </div>
      </div>
      {app.description ? <p className={s.desc} dir="auto">{app.description}</p> : null}
      {app.tags.length ? <div className={s.cardTags}>{app.tags.map((tag) => <span key={tag} className={s.cardTag} dir="auto">{tag}</span>)}</div> : null}
      {app.reads_all ? <span className={s.botReads} data-testid="bot-discovery-reads-all"><Eye size={13} aria-hidden="true" /> {t("moderation.bots.readsAll")}</span> : null}
      <div className={s.botAdd}>
        <Button size="sm" block onClick={() => nav(`/oauth2/authorize?client_id=${app.id}`, { state: { from: location.pathname + location.search } })} data-testid="bot-discovery-add">
          <Plus size={14} aria-hidden="true" /> {t("apps.profile.addApp")}
        </Button>
      </div>
    </article>
  );
}

/** Discover's bots: the public apps whose developers listed them. */
export function BotDiscovery() {
  const [q, setQ] = useState("");
  const [sort, setSort] = useState<Sort>("popular");
  const [bots, setBots] = useState<ListedApp[] | null>(null);
  const [next, setNext] = useState<number | null>(null);
  const [failed, setFailed] = useState(false);
  const gen = useRef(0);
  const load = useCallback(async (offset: number) => {
    const mine = ++gen.current;
    setFailed(false);
    try {
      const res = await discoveryApi.bots({ q: q.trim(), sort, limit: 24, offset });
      if (mine !== gen.current) return;
      setBots((cur) => (offset && cur ? [...cur, ...res.bots] : res.bots));
      setNext(res.has_more ? (res.next_offset ?? null) : null);
    } catch {
      if (mine === gen.current) setFailed(true);
    }
  }, [sort, q]);
  useEffect(() => {
    ++gen.current;
    const timer = setTimeout(() => void load(0), q ? 300 : 0);
    return () => clearTimeout(timer);
  }, [load, q]);
  const searched = !!q.trim();
  return (
    <>
      <div className={s.lead}>
        <h1>{t("discovery.bots.heading")}</h1>
        <p>{t("discovery.bots.lead")}</p>
      </div>
      <SearchBox className={s.searchBig} value={q} onChange={setQ} placeholder={t("discovery.bots.search")} aria-label={t("discovery.bots.search")} data-testid="bot-discovery-search" />
      <div className={s.controls}>
        <div className={s.sorts} role="radiogroup" data-glide="" aria-label={t("discovery.bots.sortLabel")}>
          {SORTS.map((x) => (
            <button key={x} type="button" role="radio" aria-checked={sort === x} onClick={() => setSort(x)}>{t(`discovery.packs.sort.${x}`)}</button>
          ))}
        </div>
      </div>
      {failed ? (
        <div className={s.note}>
          {t("discovery.bots.failed")}{" "}
          <button type="button" className={chat.loadRetry} onClick={() => void load(0)}>{t("chat.messageList.tryAgain")}</button>
        </div>
      ) : null}
      {bots === null && !failed ? <div className={s.note}>{t("discovery.packs.loading")}</div> : null}
      {bots && bots.length === 0 ? (
        <div className={s.empty}>
          <Bot size={36} aria-hidden="true" />
          <h2>{searched ? t("discovery.bots.noMatches") : t("discovery.bots.none")}</h2>
          {searched ? null : <p>{t("discovery.bots.noneHint")}</p>}
        </div>
      ) : null}
      {bots?.length ? <div className={s.grid}>{bots.map((b) => <BotCard key={b.id} app={b} />)}</div> : null}
      {next !== null ? <button type="button" className={s.more} onClick={() => void load(next)}>{t("discovery.packs.more")}</button> : null}
    </>
  );
}

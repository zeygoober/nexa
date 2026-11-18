import { ArrowLeftRight, Ban, Check, Clock, Search, X } from "lucide-react";
import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { moderation as modApi } from "@/api/moderation";
import type { PublicUser, Snowflake, Trade, TradeStatus } from "@/api/types";
import { Avatar } from "@/components/common/Avatar";
import { Button } from "@/components/common/Button";
import { LoadError } from "@/components/common/LoadError";
import { PersonName } from "@/components/common/PersonName";
import { Spinner } from "@/components/common/Spinner";
import { LineList } from "@/components/trades/TradeLines";
import { byId } from "./BadgeOwnersModal";
import { fullDate } from "@/lib/moderation";
import { formatTimestamp } from "@/lib/time";
import { t } from "@/i18n";
import m from "./Moderation.module.css";
import s from "./TradesTab.module.css";

const FILTERS: { id: TradeStatus | ""; label: () => string }[] = [
  { id: "", label: () => t("moderation.trades.all") },
  { id: "accepted", label: () => t("moderation.trades.completed") },
  { id: "pending", label: () => t("moderation.trades.open") },
  { id: "declined", label: () => t("moderation.trades.declined") },
  { id: "cancelled", label: () => t("moderation.trades.cancelled") },
];

const STATUS_ICON = { accepted: Check, pending: Clock, declined: X, cancelled: Ban } as const;

/**
 * The owner's Trade Log: every trade since trading began, newest first, each
 * drawn as the two people see it in their messages, one side each.
 */
export function TradesTab() {
  const nav = useNavigate();
  const [status, setStatus] = useState<TradeStatus | "">("");
  const [q, setQ] = useState("");
  const [trades, setTrades] = useState<Trade[] | null>(null);
  const [users, setUsers] = useState<Record<Snowflake, PublicUser>>({});
  const [more, setMore] = useState(false);
  const [loading, setLoading] = useState(false);
  const [failed, setFailed] = useState(false);
  const asked = useRef(0);

  const load = useCallback(
    async (before?: Snowflake) => {
      const n = ++asked.current;
      setLoading(true);
      setFailed(false);
      try {
        const r = await modApi.tradeLog({ status, q: q.trim(), before });
        if (n !== asked.current) return;
        setTrades((cur) => (before && cur ? [...cur, ...r.trades] : r.trades));
        setUsers((cur) => ({ ...cur, ...byId(r.users) }));
        setMore(r.more);
      } catch {
        if (n === asked.current) setFailed(true);
      } finally {
        if (n === asked.current) setLoading(false);
      }
    },
    [status, q],
  );

  // a name is looked for once typing stops
  useEffect(() => {
    const timer = setTimeout(() => void load(), q ? 250 : 0);
    return () => clearTimeout(timer);
  }, [load, q]);

  const person = (id: Snowflake) => {
    const u = users[id];
    const name = u?.display_name || u?.username || t("moderation.trades.gone");
    return (
      <button type="button" className={s.person} onClick={() => nav(`/channels/@admin/users/${id}`)} disabled={!u}>
        <Avatar id={id} avatarId={u?.avatar_id ?? null} name={name} size={28} />
        <PersonName className={s.name} userId={id} user={u}>{name}</PersonName>
      </button>
    );
  };
  const named = (id: Snowflake) => users[id]?.display_name || users[id]?.username || t("moderation.trades.gone");

  return (
    <div className={m.scroll} data-testid="trades-tab">
      <div className={s.page}>
        <p className={s.hint}>{t("moderation.trades.hint")}</p>
        <div className={m.toolbar}>
          <label className={m.search}>
            <Search size={17} aria-hidden="true" />
            <input value={q} onChange={(e) => setQ(e.target.value)} placeholder={t("moderation.trades.search")} aria-label={t("moderation.trades.search")} data-testid="trades-search" />
          </label>
          <div className={m.chips} role="radiogroup" aria-label={t("moderation.trades.filter")}>
            {FILTERS.map((f) => (
              <button key={f.id || "all"} type="button" role="radio" aria-checked={status === f.id} className={m.chip} data-selected={status === f.id} onClick={() => setStatus(f.id)} data-testid={`trades-filter-${f.id || "all"}`}>
                {f.label()}
              </button>
            ))}
          </div>
        </div>

        {failed && !trades?.length ? (
          <LoadError message={t("moderation.loadFailed")} onRetry={() => void load()} busy={loading} center />
        ) : trades === null ? (
          <div className={m.loadingRow}>
            <Spinner label={t("chat.pins.loading")} />
          </div>
        ) : trades.length === 0 ? (
          <div className={m.empty}>
            <ArrowLeftRight size={40} aria-hidden="true" />
            <span className={m.emptyTitle}>{q || status ? t("moderation.noMatches") : t("moderation.trades.none")}</span>
          </div>
        ) : (
          <>
            <ol className={s.list}>
              {trades.map((tr) => {
                const Icon = STATUS_ICON[tr.status];
                const when = tr.resolved_at ?? tr.created_at;
                // only a completed trade moved anything; the rest were offers
                const side = (id: Snowflake) => t(tr.status === "accepted" ? "moderation.trades.gave" : "moderation.trades.offered", { name: named(id) });
                return (
                  <li key={tr.id} className={s.trade} data-testid="trade-log-row" data-status={tr.status}>
                    <div className={s.top}>
                      <span className={s.people}>
                        {person(tr.from_id)}
                        <ArrowLeftRight size={16} className={s.swap} aria-label={t("moderation.trades.with")} />
                        {person(tr.to_id)}
                      </span>
                      <span className={s.state}>
                        <span className={s.status} data-status={tr.status}>
                          <Icon size={15} aria-hidden="true" />
                          {FILTERS.find((f) => f.id === tr.status)!.label()}
                        </span>
                        <time className={s.when} dateTime={new Date(when).toISOString()} title={fullDate(when)}>
                          {formatTimestamp(when)}
                        </time>
                      </span>
                    </div>
                    <div className={s.sides}>
                      <section className={s.side} aria-label={side(tr.from_id)}>
                        <h3 className={s.sideTitle}>{side(tr.from_id)}</h3>
                        <LineList lines={tr.from_offer} />
                      </section>
                      <section className={s.side} aria-label={side(tr.to_id)}>
                        <h3 className={s.sideTitle}>{side(tr.to_id)}</h3>
                        <LineList lines={tr.to_offer} />
                      </section>
                    </div>
                  </li>
                );
              })}
            </ol>
            {more ? (
              <div className={m.loadingRow}>
                <Button size="sm" variant="ghost" loading={loading} onClick={() => void load(trades[trades.length - 1]!.id)}>
                  {t("moderation.trades.loadMore")}
                </Button>
              </div>
            ) : null}
          </>
        )}
      </div>
    </div>
  );
}

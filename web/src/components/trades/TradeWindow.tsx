import { ArrowLeft, ArrowRight, Check, Minus, Search } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { trades } from "@/api/trades";
import type { Trade, TradeLine } from "@/api/types";
import { Avatar } from "@/components/common/Avatar";
import { BadgePill } from "@/components/badges/Badges";
import { Button } from "@/components/common/Button";
import { LoadError } from "@/components/common/LoadError";
import { Modal } from "@/components/common/Modal";
import { PersonName } from "@/components/common/PersonName";
import { Spinner } from "@/components/common/Spinner";
import { t, Tr } from "@/i18n";
import { describeError } from "@/lib/errors";
import { useAuth } from "@/stores/auth";
import { useTrades } from "@/stores/trades";
import { useUI } from "@/stores/ui";
import { displayName, useUsers } from "@/stores/users";
import { lineName, NOBODY } from "./TradeLines";
import s from "./Trades.module.css";

/** How many slots the other person's side always shows, empty or not. */
const MIN_SLOTS = 6;
/** How long a change waits for the next one before it is sent. */
const SEND_AFTER_MS = 250;

const asLines = (sel: Record<string, number>) => Object.entries(sel).filter(([, n]) => n > 0).map(([key, qty]) => ({ key, qty }));
const asSel = (lines: TradeLine[]) => Object.fromEntries(lines.map((l) => [l.key, l.qty]));

/**
 * Trade Badges: a window two people share. Yours is the grid of badges you
 * hold, the other person's the slots their offer fills as they pick. Each
 * change goes to the server at once and to them live, and clears both Ready
 * marks; when both of you are ready, it happens. Opened with a person it starts
 * the trade (or finds the one already open), with a trade it joins it, which
 * works just as well when the other person comes by later.
 */
export function TradeWindow({ userId, tradeId, onClose }: { userId?: string | undefined; tradeId?: string | undefined; onClose: () => void }) {
  const toast = useUI((u) => u.toast);
  const me = useAuth((a) => a.me?.id);
  const [id, setId] = useState<string | null>(tradeId ?? null);
  const trade = useTrades((x) => (id ? x.byId[id] : undefined));
  const [mine, setMine] = useState<TradeLine[] | null>(null);
  const [failed, setFailed] = useState(false);
  const [sel, setSel] = useState<Record<string, number>>({});
  const [q, setQ] = useState("");
  const [busy, setBusy] = useState(false);
  const selRef = useRef(sel);
  selRef.current = sel;
  const dirty = useRef(false);
  const timer = useRef<number | undefined>(undefined);
  const endedByMe = useRef(false);
  const fromMe = !!trade && trade.from_id === me;
  const otherId = trade ? (fromMe ? trade.to_id : trade.from_id) : (userId ?? "");
  const other = useUsers((u) => u.byId[otherId]);
  const self = useUsers((u) => (me ? u.byId[me] : undefined));
  const otherName = displayName(other);

  const load = () => {
    setFailed(false);
    (async () => {
      const opened = tradeId ? await trades.get(tradeId) : await trades.open(userId ?? "");
      useTrades.getState().set(opened);
      setId(opened.id);
      setMine(await trades.mine());
    })().catch((err) => {
      setFailed(true);
      toast(describeError(err, t("trades.couldNotOpen")), "error");
    });
  };
  useEffect(load, [tradeId, userId]); // eslint-disable-line react-hooks/exhaustive-deps

  const myOffer = trade ? (fromMe ? trade.from_offer : trade.to_offer) : [];
  const theirOffer = trade ? (fromMe ? trade.to_offer : trade.from_offer) : [];
  const myReady = !!trade && (fromMe ? trade.from_ready : trade.to_ready);
  const theirReady = !!trade && (fromMe ? trade.to_ready : trade.from_ready);
  // what the server holds of mine is what shows, unless a change of mine is on its way
  useEffect(() => {
    if (!dirty.current && trade) setSel(asSel(myOffer));
  }, [trade?.id, trade?.version]); // eslint-disable-line react-hooks/exhaustive-deps

  // it ended: by the other person, or by the trade happening
  useEffect(() => {
    if (!trade || trade.status === "pending" || endedByMe.current) return;
    toast(trade.status === "accepted" ? t("trades.done") : t("trades.endedByThem", { name: otherName }));
    onClose();
  }, [trade?.status]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => () => window.clearTimeout(timer.current), []);

  /** Sends a change that is waiting; the trade as it stands after. */
  const flush = async (): Promise<Trade | undefined> => {
    window.clearTimeout(timer.current);
    if (!dirty.current || !id) return trade;
    dirty.current = false;
    try {
      const next = await trades.offer(id, asLines(selRef.current));
      useTrades.getState().set(next);
      return next;
    } catch (err) {
      toast(describeError(err, t("trades.couldNotUpdate")), "error");
      const fresh = await trades.get(id).catch(() => undefined);
      if (fresh) {
        useTrades.getState().set(fresh);
        setSel(asSel(fresh.from_id === me ? fresh.from_offer : fresh.to_offer));
      }
      return fresh;
    }
  };
  const change = (key: string, n: number) => {
    const next = { ...selRef.current, [key]: n };
    if (n <= 0) delete next[key];
    selRef.current = next;
    setSel(next);
    dirty.current = true;
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => void flush(), SEND_AFTER_MS);
  };

  const press = async () => {
    if (!id || !trade || busy) return;
    setBusy(true);
    try {
      const waiting = dirty.current;
      const now = (await flush()) ?? trade;
      // a change just sent cleared the marks, so what is wanted then is Ready
      const done = await trades.ready(id, waiting ? true : !myReady, now.version);
      useTrades.getState().set(done);
    } catch (err) {
      toast(describeError(err, t("trades.couldNotUpdate")), "error");
      const fresh = await trades.get(id).catch(() => undefined);
      if (fresh) useTrades.getState().set(fresh);
    } finally {
      setBusy(false);
    }
  };
  const cancel = async () => {
    if (id && trade?.status === "pending") {
      endedByMe.current = true;
      try {
        useTrades.getState().set(await trades.end(id));
      } catch (err) {
        endedByMe.current = false;
        toast(describeError(err, t("trades.couldNotUpdate")), "error");
        return;
      }
    }
    onClose();
  };
  const close = () => {
    void flush();
    onClose();
  };

  const shown = useMemo(() => {
    const needle = q.trim().toLowerCase();
    return (mine ?? []).filter((l) => !needle || lineName(l).toLowerCase().includes(needle));
  }, [mine, q]);
  const loading = !trade || !mine;
  const slots = Math.max(MIN_SLOTS, Math.ceil(theirOffer.length / 3) * 3);

  return (
    <Modal
      open
      onClose={close}
      title={t("trades.title")}
      width={760}
      footer={
        <>
          <Button variant="danger" onClick={() => void cancel()} data-testid="trade-cancel">{t("chat.editMessage.cancel")}</Button>
          <Button variant={myReady ? "secondary" : "positive"} loading={busy} disabled={loading || trade?.status !== "pending"} onClick={() => void press()} data-testid="trade-ready">
            {myReady ? t("trades.notReady") : t("trades.ready")}
          </Button>
        </>
      }
    >
      {failed ? (
        <LoadError message={t("trades.couldNotLoad")} onRetry={load} />
      ) : loading ? (
        <div className={s.loading}><Spinner size={24} /></div>
      ) : (
        <>
          <div className={s.heads}>
            <div className={s.who}>
              <Avatar id={me ?? ""} avatarId={self?.avatar_id ?? null} name={displayName(self)} size={36} />
              <span className={s.whoName}>{t("trades.yourOffer")}</span>
              {myReady ? <span className={s.readyTag} data-testid="trade-my-ready"><Check size={14} aria-hidden="true" />{t("trades.ready")}</span> : null}
            </div>
            <div className={`${s.who} ${s.whoEnd}`}>
              {theirReady ? <span className={s.readyTag} data-testid="trade-their-ready"><Check size={14} aria-hidden="true" />{t("trades.ready")}</span> : null}
              <span className={s.whoName}><Tr id="trades.theirOffer" values={{ name: <PersonName userId={otherId}>{otherName}</PersonName> }} /></span>
              <Avatar id={otherId} avatarId={other?.avatar_id ?? null} name={otherName} size={36} />
            </div>
          </div>
          <div className={s.window}>
            <section className={s.pane} data-ready={myReady || undefined} aria-label={t("trades.yourOffer")} data-testid="trade-mine">
              <div className={`${s.who} ${s.paneHead}`}>
                <Avatar id={me ?? ""} avatarId={self?.avatar_id ?? null} name={displayName(self)} size={28} />
                <span className={s.whoName}>{t("trades.yourOffer")}</span>
                {myReady ? <span className={s.readyTag}><Check size={14} aria-hidden="true" />{t("trades.ready")}</span> : null}
              </div>
              {mine.length ? (
                <>
                  <ul className={s.inv}>
                    {shown.map((l) => {
                      const n = sel[l.key] ?? 0;
                      const name = lineName(l) || l.key;
                      return (
                        <li key={l.key} className={s.cell}>
                          <button
                            type="button"
                            className={s.tile}
                            data-on={n > 0 || undefined}
                            aria-pressed={n > 0}
                            aria-label={`${name}, ${t("trades.have", { count: l.qty })}`}
                            title={name}
                            onClick={() => change(l.key, Math.min(l.qty, n + 1))}
                            onDoubleClick={() => change(l.key, l.qty)}
                            data-testid="trade-tile"
                          >
                            {l.badge ? <BadgePill badge={l.badge} user={NOBODY} still size={36} /> : null}
                            {n > 0 ? <span className={s.chipOn}>{n}/{l.qty}</span> : l.qty > 1 ? <span className={s.chip}>×{l.qty}</span> : null}
                          </button>
                          {n > 0 ? <button type="button" className={s.minus} aria-label={t("trades.removeOne")} onClick={() => change(l.key, n - 1)}><Minus size={12} aria-hidden="true" /></button> : null}
                        </li>
                      );
                    })}
                  </ul>
                  <label className={s.search}>
                    <Search size={15} aria-hidden="true" />
                    <input type="search" value={q} placeholder={t("trades.search")} aria-label={t("trades.search")} onChange={(e) => setQ(e.target.value)} />
                  </label>
                </>
              ) : <p className={s.empty}>{t("trades.noneToGive")}</p>}
            </section>
            <div className={s.arrows} aria-hidden="true">
              <ArrowRight size={22} className={s.arrowOut} />
              <ArrowLeft size={22} className={s.arrowIn} />
            </div>
            <section className={s.pane} data-ready={theirReady || undefined} aria-label={t("trades.theirOffer", { name: otherName })} data-testid="trade-theirs">
              <div className={`${s.who} ${s.paneHead}`}>
                <Avatar id={otherId} avatarId={other?.avatar_id ?? null} name={otherName} size={28} />
                <span className={s.whoName}><Tr id="trades.theirOffer" values={{ name: <PersonName userId={otherId}>{otherName}</PersonName> }} /></span>
                {theirReady ? <span className={s.readyTag}><Check size={14} aria-hidden="true" />{t("trades.ready")}</span> : null}
              </div>
              <ul className={s.slots}>
                {Array.from({ length: slots }, (_, i) => {
                  const l = theirOffer[i];
                  return (
                    <li key={l?.key ?? `e${i}`} className={s.slot} data-empty={!l || undefined} data-testid={l ? "trade-their-slot" : undefined}>
                      {l?.badge ? <BadgePill badge={l.badge} user={NOBODY} size={36} /> : null}
                      {l && l.qty > 1 ? <span className={s.chip}>×{l.qty}</span> : null}
                    </li>
                  );
                })}
              </ul>
              <p className={s.theirState}>{theirReady ? t("trades.theirReady", { name: otherName }) : t("trades.theirNotReady", { name: otherName })}</p>
            </section>
          </div>
          <p className={s.hint}>{t("trades.hint")}</p>
        </>
      )}
    </Modal>
  );
}

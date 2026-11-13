import { ArrowLeftRight, Check } from "lucide-react";
import { trades } from "@/api/trades";
import { Button } from "@/components/common/Button";
import { PersonName } from "@/components/common/PersonName";
import { t, Tr } from "@/i18n";
import { describeError } from "@/lib/errors";
import { formatTimestamp } from "@/lib/time";
import { useAuth } from "@/stores/auth";
import { useMessages, type DecryptedMessage } from "@/stores/messages";
import { openProfilePopout } from "@/stores/profilePopout";
import { useUI } from "@/stores/ui";
import { displayName, useUsers } from "@/stores/users";
import { LineList } from "./TradeLines";
import chat from "@/components/chat/Chat.module.css";
import s from "./Trades.module.css";

/**
 * A trade in the messages of the two people, as each sees it: what they give
 * on the left and what they get on the right. While it is open either can open
 * the window to make their side and press Ready (now, or later when the other
 * person is not here), or end it; afterwards it says how it ended. It is the
 * server's own line, not end-to-end encrypted, since the server is what moves
 * the badges.
 */
export function TradeCard({ msg }: { msg: DecryptedMessage }) {
  const me = useAuth((a) => a.me?.id);
  const toast = useUI((u) => u.toast);
  const openModal = useUI((u) => u.openModal);
  const trade = msg.trade;
  const mine = trade?.from_id === me;
  const otherId = trade ? (mine ? trade.to_id : trade.from_id) : msg.author_id;
  const other = useUsers((u) => u.byId[otherId]);
  const name = displayName(other);

  const end = async () => {
    if (!trade) return;
    try {
      const done = await trades.end(trade.id);
      useMessages.getState().setTrade(done.channel_id, done.message_id, done);
    } catch (err) {
      toast(describeError(err, t("trades.couldNotUpdate")), "error");
    }
  };

  const who = <button type="button" className={chat.sysName} dir="auto" onClick={(e) => openProfilePopout(otherId, e.currentTarget)}><PersonName userId={otherId}>{name}</PersonName></button>;
  const gives = trade ? (mine ? trade.from_offer : trade.to_offer) : [];
  const gets = trade ? (mine ? trade.to_offer : trade.from_offer) : [];
  const theirReady = !!trade && (mine ? trade.to_ready : trade.from_ready);
  const myReady = !!trade && (mine ? trade.from_ready : trade.to_ready);
  return (
    <div className={chat.system} data-id={msg.id} data-trade="" data-testid="trade-card" data-status={trade?.status ?? "gone"}>
      <span className={chat.sysIcon}><ArrowLeftRight size={18} /></span>
      <span className={s.col}>
        <span className={s.sentence}>
          {mine ? <Tr id="trades.card.sentByYou" values={{ name: who }} /> : <Tr id="trades.card.sentToYou" values={{ name: who }} />}
          <span className={chat.time}>{formatTimestamp(msg.created_at)}</span>
        </span>
        <div className={s.card}>
          {trade ? (
            <>
              <div className={s.cardSides}>
                <div className={s.cardSide}>
                  <h4 className={s.cardTitle}>{t("trades.youGive")}</h4>
                  <LineList lines={gives} />
                </div>
                <div className={s.cardSide}>
                  <h4 className={s.cardTitle}>{t("trades.youGet")}</h4>
                  <LineList lines={gets} />
                </div>
              </div>
              {trade.status === "pending" ? (
                <div className={s.cardActions}>
                  <span className={s.cardNote}>
                    {myReady ? <><Check size={14} aria-hidden="true" /> {t("trades.ready")}</> : theirReady ? t("trades.theirReady", { name }) : null}
                  </span>
                  <Button size="sm" variant="danger" onClick={() => void end()} data-testid="trade-end">{mine ? t("trades.card.cancelTrade") : t("trades.card.decline")}</Button>
                  <Button size="sm" variant="positive" onClick={() => openModal("trade", { tradeId: trade.id })} data-testid="trade-open">{mine ? t("trades.card.open") : t("trades.card.join")}</Button>
                </div>
              ) : (
                <p className={s.cardStatus} data-status={trade.status} data-testid="trade-status">
                  {trade.status === "accepted" ? <Check size={16} aria-hidden="true" /> : null}
                  {t(`trades.status.${trade.status}`)}
                </p>
              )}
            </>
          ) : (
            <p className={s.cardStatus}>{t("trades.status.gone")}</p>
          )}
        </div>
      </span>
    </div>
  );
}

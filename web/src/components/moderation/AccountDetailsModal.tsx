import { Copy, X } from "lucide-react";
import { useEffect, useState } from "react";
import { moderation as modApi, type ModAccount, type ModUser } from "@/api/moderation";
import { Avatar } from "@/components/common/Avatar";
import { Button } from "@/components/common/Button";
import { Modal } from "@/components/common/Modal";
import { Spinner } from "@/components/common/Spinner";
import { copyAndSay } from "@/lib/copyText";
import { describeDevice } from "@/lib/deviceLabel";
import { describeError } from "@/lib/errors";
import { formatRelative, formatShortDate } from "@/lib/time";
import { t } from "@/i18n";
import m from "./Moderation.module.css";
import b from "./BadgesTab.module.css";

/** A plan's name as the panel says it, "" being no plan. */
function planLabel(plan: string): string {
  if (plan === "pro") return t("support.plan.pro");
  if (plan === "supporter") return t("support.plan.supporter");
  return t("moderation.account.planFree");
}

/**
 * What only the owner can read about an account: where it has been, what is
 * signed in to it, and what it pays for. Asking for it writes a line in the
 * log, so it is asked for only when this dialog opens, never with the page.
 */
export function AccountDetailsModal({ user, onClose }: { user: ModUser; onClose: () => void }) {
  const [data, setData] = useState<ModAccount | null>(null);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    let alive = true;
    modApi
      .account(user.id)
      .then((r) => alive && setData(r))
      .catch((err) => alive && setError(describeError(err, t("moderation.loadFailed"))));
    return () => {
      alive = false;
    };
  }, [user.id]);
  const sub = data?.subscription;
  const ip = (addr: string) => (
    <button type="button" className={m.iconBtn} aria-label={t("moderation.account.copyIp")} title={t("moderation.account.copyIp")} onClick={() => copyAndSay(addr, t("moderation.copied"))}>
      <Copy size={15} />
    </button>
  );
  return (
    <Modal open onClose={onClose} bare width={560} ariaLabel={t("moderation.account.title", { name: user.display_name })}>
      <div className={m.modal} data-testid="account-modal">
        <div className={m.modalTop}>
          <Avatar id={user.id} avatarId={user.avatar_id} name={user.display_name} size={40} />
          <div style={{ minWidth: 0 }}>
            <h2 className={m.modalTitle}>{t("moderation.account.title", { name: user.display_name })}</h2>
            <div className={m.modalTarget}>
              <span>@{user.username}</span>
            </div>
          </div>
          <button type="button" className={m.modalClose} onClick={onClose} aria-label={t("chat.attachments.close")}>
            <X size={20} />
          </button>
        </div>
        <div className={m.modalScroll}>
          <p className={m.note}>{t("moderation.account.note")}</p>
          {error ? <p className={m.danger} role="alert">{error}</p> : null}
          {!data && !error ? (
            <div style={{ padding: 16, display: "grid", placeItems: "center" }}>
              <Spinner size={24} label={t("chat.pins.loading")} />
            </div>
          ) : null}
          {data ? (
            <>
              <dl className={b.facts}>
                <div>
                  <dt>{t("moderation.account.joined")}</dt>
                  <dd>{formatShortDate(data.created_at)}</dd>
                </div>
                <div>
                  <dt>{t("moderation.account.plan")}</dt>
                  <dd>{planLabel(data.plan)}</dd>
                </div>
                <div>
                  <dt>{t("moderation.account.subscription")}</dt>
                  <dd>
                    {sub ? `${planLabel(sub.plan)} (${sub.status.replace(/_/g, " ")})` : t("moderation.account.noSubscription")}
                    {sub?.period_end ? <small>{t(sub.cancel_at_end ? "moderation.account.ends" : "moderation.account.renews", { when: formatShortDate(sub.period_end) })}</small> : null}
                  </dd>
                </div>
              </dl>
              <p className={b.listHint}>{t("moderation.account.noEmail")}</p>
              <h3 className={b.listTitle}>{t("moderation.account.addresses")}</h3>
              {data.addresses.length ? (
                <ul className={b.records} data-testid="account-addresses">
                  {data.addresses.map((a) => (
                    <li key={a.ip}>
                      <span className={b.recordText}>
                        <code>{a.ip}</code>
                        <span>{t("moderation.account.seen", { first: formatShortDate(a.first_seen_at), last: formatRelative(a.last_seen_at) })}</span>
                      </span>
                      {ip(a.ip)}
                    </li>
                  ))}
                </ul>
              ) : (
                <p className={m.note}>{t("moderation.account.noAddresses")}</p>
              )}
              <h3 className={`${b.listTitle} ${b.ladderTitle}`}>{t("moderation.account.sessions")}</h3>
              {data.sessions.length ? (
                <ul className={b.records} data-testid="account-sessions">
                  {data.sessions.map((x) => {
                    const d = describeDevice(x.user_agent);
                    return (
                      <li key={`${x.created_at}-${x.ip}`}>
                        <span className={b.recordText}>
                          <strong>{d.label}</strong>
                          <code>{x.ip}</code>
                          <span>{t("moderation.account.session", { first: formatShortDate(x.created_at), last: formatRelative(x.last_seen_at) })}</span>
                        </span>
                        {ip(x.ip)}
                      </li>
                    );
                  })}
                </ul>
              ) : (
                <p className={m.note}>{t("moderation.account.noSessions")}</p>
              )}
            </>
          ) : null}
        </div>
        <div className={m.modalFoot}>
          <span className={m.grow} />
          <Button variant="ghost" onClick={onClose}>{t("moderation.alts.close")}</Button>
        </div>
      </div>
    </Modal>
  );
}

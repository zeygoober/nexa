import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { moderation as modApi, type BadgeOwner } from "@/api/moderation";
import type { PublicUser, Snowflake } from "@/api/types";
import { Avatar } from "@/components/common/Avatar";
import { Button } from "@/components/common/Button";
import { LoadError } from "@/components/common/LoadError";
import { Modal } from "@/components/common/Modal";
import { PersonName } from "@/components/common/PersonName";
import { Spinner } from "@/components/common/Spinner";
import { BadgePill } from "@/components/badges/Badges";
import { lineName, NOBODY } from "@/components/trades/TradeLines";
import { describeError } from "@/lib/errors";
import { useUI } from "@/stores/ui";
import { t } from "@/i18n";
import type { HoldersOf } from "./BadgesTab";
import h from "./BadgeHoldersModal.module.css";
import s from "./BadgeOwnersModal.module.css";

const PAGE = 50;

/** The people a page names, by id. */
export const byId = (list: PublicUser[]) => Object.fromEntries(list.map((u) => [u.id, u])) as Record<Snowflake, PublicUser>;

/**
 * The owner's view of a badge: who holds copies of it now, most first. Unlike
 * the holders list it counts what trades moved, and each level or material
 * someone holds is drawn with how many. A name opens that person in the panel.
 */
export function BadgeOwnersModal({ of, onClose }: { of: HoldersOf | null; onClose: () => void }) {
  const toast = useUI((u) => u.toast);
  const nav = useNavigate();
  const [owners, setOwners] = useState<BadgeOwner[] | null>(null);
  const [users, setUsers] = useState<Record<Snowflake, PublicUser>>({});
  const [total, setTotal] = useState(0);
  const [copies, setCopies] = useState(0);
  const [failed, setFailed] = useState(false);
  const [more, setMore] = useState(false);
  const badge = of?.badge.id ?? "";

  const load = () => {
    if (!badge) return;
    setFailed(false);
    modApi
      .badgeOwners(badge, 0, PAGE)
      .then((r) => {
        setOwners(r.owners);
        setUsers(byId(r.users));
        setTotal(r.total);
        setCopies(r.copies);
      })
      .catch(() => setFailed(true));
  };
  useEffect(() => {
    setOwners(null);
    load();
  }, [badge]); // eslint-disable-line react-hooks/exhaustive-deps

  const loadMore = async () => {
    if (!owners) return;
    setMore(true);
    try {
      const r = await modApi.badgeOwners(badge, owners.length, PAGE);
      setOwners([...owners, ...r.owners]);
      setUsers((cur) => ({ ...cur, ...byId(r.users) }));
      setTotal(r.total);
    } catch (err) {
      toast(describeError(err, t("moderation.loadFailed")), "error");
    } finally {
      setMore(false);
    }
  };

  const open = (id: Snowflake) => {
    onClose();
    nav(`/channels/@admin/users/${id}`);
  };

  return (
    <Modal open={of !== null} onClose={onClose} title={of ? t("badges.owners.title", { name: of.label }) : ""} width={540}>
      {of ? (
        <div className={h.body} data-testid="badge-owners">
          <div className={h.head}>
            <BadgePill badge={of.badge} user={NOBODY} still named />
            {owners ? (
              <span className={`${h.count} ${s.totals}`}>
                <span>{t("badges.owners.people", { count: total })}</span>
                <span className={s.copiesTotal}>{t("badges.index.copies", { count: copies })}</span>
              </span>
            ) : null}
          </div>
          <p className={h.note}>{t("badges.owners.hint")}</p>
          {failed ? (
            <LoadError message={t("moderation.loadFailed")} onRetry={load} />
          ) : owners === null ? (
            <Spinner label={t("chat.pins.loading")} />
          ) : owners.length === 0 ? (
            <p className={h.none}>{t("badges.owners.nobody")}</p>
          ) : (
            <>
              <ul className={h.list}>
                {owners.map((o) => {
                  const u = users[o.user_id];
                  const name = u?.display_name || u?.username || o.user_id;
                  return (
                    <li key={o.user_id}>
                      <button type="button" className={`${h.row} ${s.row}`} onClick={() => open(o.user_id)} data-testid={`owner-${o.user_id}`}>
                        <Avatar id={o.user_id} avatarId={u?.avatar_id ?? null} name={name} size={32} />
                        <span className={h.who}>
                          <PersonName className={h.name} userId={o.user_id} user={u}>{name}</PersonName>
                          <span className={h.meta}>
                            {u?.username ? <span className={h.handle}>@{u.username}</span> : null}
                            {o.traded ? <span className={h.chip} data-source="given">{t("badges.owners.traded")}</span> : null}
                          </span>
                        </span>
                        <ul className={s.copies} aria-label={t("badges.owners.copiesOf", { count: o.total })}>
                          {o.lines.map((l) => (
                            <li key={l.key} className={s.copy} title={lineName(l) || l.key}>
                              {l.badge ? <BadgePill badge={l.badge} user={NOBODY} still size={22} /> : <span className={s.blank} aria-hidden="true" />}
                              <span className={s.qty} aria-label={`${lineName(l) || l.key}, ${t("badges.index.copies", { count: l.qty })}`}>×{l.qty}</span>
                            </li>
                          ))}
                        </ul>
                      </button>
                    </li>
                  );
                })}
              </ul>
              {owners.length < total ? (
                <Button className={h.more} variant="secondary" size="sm" loading={more} onClick={() => void loadMore()}>
                  {t("badges.holders.more")}
                </Button>
              ) : null}
            </>
          )}
        </div>
      ) : null}
    </Modal>
  );
}

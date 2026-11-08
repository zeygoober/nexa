import { useEffect, useRef, useState } from "react";
import { moderation as modApi, type ModUser } from "@/api/moderation";
import type { BadgeHolder } from "@/api/types";
import { Avatar } from "@/components/common/Avatar";
import { Button } from "@/components/common/Button";
import { LoadError } from "@/components/common/LoadError";
import { Modal } from "@/components/common/Modal";
import { PersonName } from "@/components/common/PersonName";
import { SearchBox } from "@/components/common/SearchBox";
import { Select } from "@/components/common/Select";
import { Spinner } from "@/components/common/Spinner";
import { BadgePill } from "@/components/badges/Badges";
import { describeError } from "@/lib/errors";
import { useUI } from "@/stores/ui";
import { t } from "@/i18n";
import type { HoldersOf } from "./BadgesTab";
import s from "./BadgeHoldersModal.module.css";

const PAGE = 50;

/** The level choices for a tiered badge: 1 to top, years counted as years. */
function levelLabel(badge: string, level: number): string {
  if (badge === "years") return t("badges.years", { count: level });
  return t("badges.holders.level", { level });
}

function Levels({ badge, top, value, onChange, label }: { badge: string; top: number; value: number; onChange: (v: number) => void; label: string }) {
  return (
    <Select className={s.level} value={value} onChange={(e) => onChange(Number(e.target.value))} aria-label={label}>
      {Array.from({ length: top }, (_, i) => i + 1).map((n) => (
        <option key={n} value={n}>{levelLabel(badge, n)}</option>
      ))}
    </Select>
  );
}

/**
 * Who holds a badge, earned or given, and who staff took an earned one from,
 * with Give To at the top. Staff ranks and plans are only listed: they follow
 * the account and change elsewhere.
 */
export function BadgeHoldersModal({ of, onClose, onChanged }: { of: HoldersOf | null; onClose: () => void; onChanged: () => void }) {
  const toast = useUI((u) => u.toast);
  const [holders, setHolders] = useState<BadgeHolder[] | null>(null);
  const [taken, setTaken] = useState<BadgeHolder[]>([]);
  const [total, setTotal] = useState(0);
  const [failed, setFailed] = useState(false);
  const [more, setMore] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const [query, setQuery] = useState("");
  const [found, setFound] = useState<ModUser[] | null>(null);
  const [level, setLevel] = useState(1);
  const asked = useRef(0);
  const badge = of?.badge.id ?? "";
  const top = of?.top ?? 0;

  const load = () => {
    if (!of) return;
    setFailed(false);
    modApi
      .badgeHolders(badge, 0, PAGE)
      .then((r) => {
        setHolders(r.holders);
        setTaken(r.taken);
        setTotal(r.total);
      })
      .catch(() => setFailed(true));
  };
  useEffect(() => {
    setHolders(null);
    setTaken([]);
    setQuery("");
    setFound(null);
    setLevel(1);
    load();
  }, [badge]); // eslint-disable-line react-hooks/exhaustive-deps

  // people to give it to, asked for as the name is typed
  useEffect(() => {
    const q = query.trim();
    if (!q || top === 0) {
      setFound(null);
      return;
    }
    const n = ++asked.current;
    const timer = setTimeout(() => {
      modApi
        .users({ q, limit: 6 })
        .then((r) => {
          if (n === asked.current) setFound(r.users);
        })
        .catch(() => {
          if (n === asked.current) setFound([]);
        });
    }, 200);
    return () => clearTimeout(timer);
  }, [query, top]);

  const loadMore = async () => {
    if (!holders) return;
    setMore(true);
    try {
      const r = await modApi.badgeHolders(badge, holders.length, PAGE);
      setHolders([...holders, ...r.holders]);
      setTotal(r.total);
    } catch (err) {
      toast(describeError(err, t("moderation.loadFailed")), "error");
    } finally {
      setMore(false);
    }
  };

  const act = async (userId: string, state: "give" | "take" | "auto", lv = 1) => {
    setBusy(userId + state);
    try {
      await modApi.setBadgeHolder(badge, userId, state, lv);
      if (state === "give") {
        setQuery("");
        setFound(null);
      }
      load();
      onChanged();
    } catch (err) {
      toast(describeError(err, t("moderation.failed")), "error");
    } finally {
      setBusy(null);
    }
  };

  const tiered = top > 1;
  const held = new Set((holders ?? []).map((h) => h.user_id));
  const sourceText = (h: BadgeHolder) =>
    h.source === "account" ? t("badges.holders.account") : h.source === "given" ? (h.overridden ? t("badges.holders.givenOver") : t("badges.holders.given")) : t("badges.holders.earned");

  const row = (h: BadgeHolder, gone: boolean) => {
    const u = h.user;
    const name = u?.display_name || u?.username || h.user_id;
    return (
      <li key={h.user_id} className={s.row} data-testid={`holder-${h.user_id}`}>
        <Avatar id={h.user_id} avatarId={u?.avatar_id ?? null} name={name} size={32} />
        <span className={s.who}>
          <PersonName className={s.name} userId={h.user_id} user={u}>{name}</PersonName>
          <span className={s.meta}>
            <span className={s.chip} data-source={gone ? "taken" : h.source}>{gone ? t("badges.holders.takenChip") : sourceText(h)}</span>
            {tiered && !gone ? <span>{levelLabel(badge, h.level)}</span> : null}
            {u?.username ? <span className={s.handle}>@{u.username}</span> : null}
          </span>
        </span>
        {top > 0 ? (
          <span className={s.actions}>
            {gone ? (
              <Button size="sm" variant="secondary" loading={busy === h.user_id + "auto"} onClick={() => void act(h.user_id, "auto")} data-testid="holder-restore">
                {t("badges.holders.restore")}
              </Button>
            ) : (
              <>
                {h.overridden ? (
                  <Button size="sm" variant="ghost" loading={busy === h.user_id + "auto"} onClick={() => void act(h.user_id, "auto")} title={t("badges.holders.automaticHint")}>
                    {t("badges.holders.automatic")}
                  </Button>
                ) : null}
                <Button size="sm" variant="danger" loading={busy === h.user_id + "take"} onClick={() => void act(h.user_id, "take")} data-testid="holder-take">
                  {t("badges.holders.take")}
                </Button>
              </>
            )}
          </span>
        ) : null}
      </li>
    );
  };

  return (
    <Modal open={of !== null} onClose={onClose} title={of ? t("badges.holders.title", { name: of.label }) : ""} width={540}>
      {of ? (
        <div className={s.body}>
          <div className={s.head}>
            <BadgePill badge={of.badge} user={{ id: "0", flags: 1, plan: "pro" }} still named />
            <span className={s.count}>{holders ? t("badges.staff.holders", { count: total }) : null}</span>
          </div>
          {top === 0 ? <p className={s.note}>{t("badges.staff.followsAccount")}</p> : null}

          {top > 0 ? (
            <section className={s.give} aria-label={t("badges.holders.giveTo")}>
              <h3 className={s.title}>{t("badges.holders.giveTo")}</h3>
              <div className={s.giveRow}>
                <SearchBox className={s.search} value={query} onChange={setQuery} placeholder={t("badges.holders.searchPlaceholder")} aria-label={t("badges.holders.giveTo")} data-testid="holders-search" />
                {tiered ? <Levels badge={badge} top={top} value={level} onChange={setLevel} label={t("badges.holders.levelLabel")} /> : null}
              </div>
              {found !== null ? (
                <ul className={s.found}>
                  {found.length === 0 ? <li className={s.none}>{t("badges.holders.noMatch")}</li> : null}
                  {found.map((u) => {
                    const name = u.display_name || u.username;
                    const has = held.has(u.id);
                    return (
                      <li key={u.id} className={s.foundRow}>
                        <Avatar id={u.id} avatarId={u.avatar_id} name={name} size={24} />
                        <span className={s.who}>
                          <PersonName className={s.name} userId={u.id} user={u}>{name}</PersonName>
                        </span>
                        <span className={s.handle}>@{u.username}</span>
                        <Button size="sm" variant={has ? "secondary" : "primary"} loading={busy === u.id + "give"} onClick={() => void act(u.id, "give", level)} data-testid="holder-give">
                          {has && tiered ? t("badges.holders.setLevel") : has ? t("badges.holders.has") : t("badges.holders.give")}
                        </Button>
                      </li>
                    );
                  })}
                </ul>
              ) : null}
            </section>
          ) : null}

          {failed ? (
            <LoadError message={t("moderation.loadFailed")} onRetry={load} />
          ) : holders === null ? (
            <Spinner label={t("chat.pins.loading")} />
          ) : (
            <>
              <section>
                <h3 className={s.title}>{t("badges.holders.heldBy")}</h3>
                {holders.length === 0 ? <p className={s.none}>{t("badges.holders.nobody")}</p> : <ul className={s.list}>{holders.map((h) => row(h, false))}</ul>}
                {holders.length < total ? (
                  <Button className={s.more} variant="secondary" size="sm" loading={more} onClick={() => void loadMore()}>
                    {t("badges.holders.more")}
                  </Button>
                ) : null}
              </section>
              {taken.length > 0 ? (
                <section>
                  <h3 className={s.title}>{t("badges.holders.takenTitle")}</h3>
                  <p className={s.hint}>{t("badges.holders.takenHint")}</p>
                  <ul className={s.list}>{taken.map((h) => row(h, true))}</ul>
                </section>
              ) : null}
            </>
          )}
        </div>
      ) : null}
    </Modal>
  );
}

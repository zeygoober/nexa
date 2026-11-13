import { personText } from "@/lib/personText";
import { PersonName } from "@/components/common/PersonName";
import { useEffect, useState } from "react";
import { moderation as modApi } from "@/api/moderation";
import type { BadgeDef } from "@/api/types";
import { BadgePill } from "@/components/badges/Badges";
import { Button } from "@/components/common/Button";
import { LoadError } from "@/components/common/LoadError";
import { Modal } from "@/components/common/Modal";
import { Spinner } from "@/components/common/Spinner";
import { defBadge, HAND_BADGES, OVERRIDABLE } from "@/lib/badges";
import { Select } from "@/components/common/Select";
import { describeError } from "@/lib/errors";
import { useUI } from "@/stores/ui";
import { t, Tr } from "@/i18n";
import r from "./RankModal.module.css";
import b from "./BadgesTab.module.css";
import { Check } from "lucide-react";

const NOBODY = { id: "0", flags: 0, plan: "" as const };

type Over = "auto" | "give" | "take";

/** A state of an earned badge from its override: missing follows the account, 0 is taken away, above 0 given at that level. */
const stateOf = (v: number | undefined): Over => (v === undefined ? "auto" : v === 0 ? "take" : "give");

const sameOverrides = (a: Record<string, number>, b: Record<string, number>) =>
  Object.keys(a).length === Object.keys(b).length && Object.keys(a).every((k) => a[k] === b[k]);

/**
 * The badges staff give someone: Contributor and every event badge, each
 * ticked when they hold it, then the earned badges, each following the
 * account or given or taken by hand until set back. Saving gives and takes
 * the difference, in one line of the log.
 */
export function GrantBadgesModal({ user, onClose }: { user: { id: string; display_name: string }; onClose: () => void }) {
  const toast = useUI((u) => u.toast);
  const [defs, setDefs] = useState<BadgeDef[] | null>(null);
  const [held, setHeld] = useState<string[] | null>(null);
  const [pick, setPick] = useState<Set<string>>(new Set());
  const [over, setOver] = useState<Record<string, number>>({});
  const [overWas, setOverWas] = useState<Record<string, number>>({});
  const [earned, setEarned] = useState<Record<string, number>>({});
  const [failed, setFailed] = useState(false);
  const [busy, setBusy] = useState(false);

  const load = () => {
    setFailed(false);
    Promise.all([modApi.badges(), modApi.userBadges(user.id)])
      .then(([all, mine]) => {
        setDefs(all.badges);
        setHeld(mine.granted);
        setPick(new Set(mine.granted));
        setOver(mine.overrides ?? {});
        setOverWas(mine.overrides ?? {});
        setEarned(mine.earned ?? {});
      })
      .catch(() => setFailed(true));
  };
  useEffect(load, [user.id]); // eslint-disable-line react-hooks/exhaustive-deps

  const options = [...HAND_BADGES.map((id) => ({ id })), ...(defs ?? []).map(defBadge)];
  const changed = held !== null && (held.length !== pick.size || held.some((b) => !pick.has(b)) || !sameOverrides(over, overWas));
  const setState = (k: string, st: Over) =>
    setOver((cur) => {
      const next = { ...cur };
      if (st === "auto") delete next[k];
      else if (st === "take") next[k] = 0;
      else next[k] = cur[k] || Math.max(1, earned[k] ?? 0);
      return next;
    });
  const earnedText = (k: string) => {
    const lv = earned[k] ?? 0;
    if (lv <= 0) return t("badges.staff.earnedNot");
    if (OVERRIDABLE[k]! > 1) return k === "years" ? t("badges.staff.earnedYears", { years: t("badges.years", { count: lv }) }) : t("badges.staff.earnedLevel", { level: lv });
    return t("badges.staff.earnedYes");
  };
  const flip = (id: string) =>
    setPick((cur) => {
      const next = new Set(cur);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  const save = async () => {
    setBusy(true);
    try {
      await modApi.setUserBadges(user.id, [...pick], over);
      toast(personText("badges.staff.givenDone", user.id, user.display_name), "success");
      onClose();
    } catch (err) {
      toast(describeError(err, t("moderation.failed")), "error");
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal
      open
      onClose={onClose}
      title={<Tr id="badges.staff.giveTitle" values={{ name: <PersonName user={user}>{user.display_name}</PersonName> }} />}
      width={500}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>{t("chat.editMessage.cancel")}</Button>
          <Button onClick={() => void save()} loading={busy} disabled={!changed} data-testid="badges-give-save">
            {t("badges.staff.save")}
          </Button>
        </>
      }
    >
      <p className={r.lede}>{t("badges.staff.giveBody")}</p>
      {failed ? (
        <LoadError message={t("moderation.loadFailed")} onRetry={load} />
      ) : held === null ? (
        <Spinner label={t("chat.pins.loading")} />
      ) : (
        <div className={r.ladder} role="group" aria-label={t("badges.staff.giveTitle", { name: user.display_name })}>
          {options.map((o) => {
            const on = pick.has(o.id);
            return (
              <label key={o.id} className={r.rung} data-on={on || undefined} data-testid={`give-${o.id}`}>
                <input type="checkbox" className={r.input} checked={on} onChange={() => flip(o.id)} />
                <span className={b.tick} data-on={on || undefined} aria-hidden="true">{on ? <Check size={14} strokeWidth={3} /> : null}</span>
                <span className={r.text}>
                  <span className={r.name}>
                    <BadgePill badge={o} user={NOBODY} still named />
                    {held.includes(o.id) ? <span className={r.now}>{t("badges.staff.held")}</span> : null}
                  </span>
                  {HAND_BADGES.includes(o.id) ? <span className={r.powers}>{t(`badges.staff.${o.id}Note`)}</span> : null}
                </span>
              </label>
            );
          })}
          {defs?.length === 0 ? <p className={r.powers}>{t("badges.staff.noneYet")}</p> : null}
          <h3 className={`${b.listTitle} ${b.ladderTitle}`}>{t("badges.staff.earned")}</h3>
          <p className={b.listHint}>{t("badges.staff.overrideHint")}</p>
          {Object.keys(OVERRIDABLE).map((k) => {
            const st = stateOf(over[k]);
            const top = OVERRIDABLE[k]!;
            const shown = st === "give" ? over[k]! : Math.max(1, earned[k] ?? 1);
            return (
              <div key={k} className={b.override} data-state={st} data-testid={`override-${k}`}>
                <span className={b.overrideText}>
                  <BadgePill badge={{ id: k, level: shown }} user={NOBODY} still named />
                  <span className={r.powers}>{earnedText(k)}</span>
                </span>
                <span className={b.overrideControls}>
                  <span className={b.segment} role="radiogroup" data-glide="" aria-label={t("badges.staff.overrideLabel")}>
                    {(["auto", "give", "take"] as const).map((v) => (
                      <button key={v} type="button" role="radio" aria-checked={st === v} className={b.segmentItem} onClick={() => setState(k, v)} data-testid={`override-${k}-${v}`}>
                        {t(`badges.staff.override.${v}`)}
                      </button>
                    ))}
                  </span>
                  {st === "give" && top > 1 ? (
                    <Select className={b.overrideLevel} value={over[k]} onChange={(e) => setOver((cur) => ({ ...cur, [k]: Number(e.target.value) }))} aria-label={t("badges.holders.levelLabel")}>
                      {Array.from({ length: top }, (_, i) => i + 1).map((n) => (
                        <option key={n} value={n}>{k === "years" ? t("badges.years", { count: n }) : t("badges.holders.level", { level: n })}</option>
                      ))}
                    </Select>
                  ) : null}
                </span>
              </div>
            );
          })}
        </div>
      )}
    </Modal>
  );
}

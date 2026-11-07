import { ArrowDown, ArrowUp, Eye, EyeOff } from "lucide-react";
import { useEffect } from "react";
import type { Badge, PublicUser } from "@/api/types";
import { BadgePill } from "@/components/badges/Badges";
import { Button } from "@/components/common/Button";
import { badgeLook, badgeName } from "@/lib/badges";
import { describeError } from "@/lib/errors";
import { useMyBadges } from "@/stores/myBadges";
import { useUI } from "@/stores/ui";
import { t } from "@/i18n";
import d from "./Designer.module.css";

/**
 * The designer's list of this account's badges, in the order the profile's
 * pill draws them. Each moves and hides, saved as it changes (as pictures and
 * the status are). A staff rank never hides, so staff are always known as
 * staff.
 */
export function BadgeEditor({ me }: { me: Pick<PublicUser, "id" | "flags" | "plan"> }) {
  const list = useMyBadges((b) => b.list);
  const failed = useMyBadges((b) => b.error);
  const load = useMyBadges((b) => b.load);
  const save = useMyBadges((b) => b.save);
  const toast = useUI((u) => u.toast);
  useEffect(() => {
    void load();
  }, [load]);

  if (failed && !list) {
    return (
      <div className={d.notice} role="status">
        <span>{t("badges.couldNotLoad")}</span>
        <Button size="sm" variant="secondary" onClick={() => void load()}>{t("invite.tryAgain")}</Button>
      </div>
    );
  }
  if (!list) return <p className={d.fieldHint}>{t("chat.pins.loading")}</p>;
  if (!list.length) return <p className={d.fieldHint} data-testid="badges-none">{t("badges.none")}</p>;

  const commit = (next: Badge[]) => save(next).catch((err) => toast(describeError(err, t("settings.privacy.couldNotSave")), "error"));
  const moved = (at: number, delta: number) => {
    const next = [...list];
    const [b] = next.splice(at, 1);
    next.splice(at + delta, 0, b!);
    return next;
  };
  const flipped = (id: string) => list.map((b) => (b.id === id ? { ...b, hidden: !b.hidden } : b));
  const shown = list.filter((b) => !b.hidden).length;

  return (
    <section className={d.badgeGroup} data-testid="badge-editor">
      <p className={d.badgeCount}>{t("badges.shownCount", { shown, total: list.length })}</p>
      <ol className={d.badgeRows}>
        {list.map((b, i) => {
          const name = badgeName(b, me);
          const locked = b.id === "staff";
          const why = locked ? t("badges.staffAlways") : (badgeLook(b, me)?.tip ?? "");
          return (
            <li key={b.id} className={d.badgeRow} data-hidden={b.hidden || undefined} data-testid={`badge-row-${b.id}`}>
              <span className={d.badgeMark}>
                <BadgePill badge={b} user={me} still />
              </span>
              <span className={d.badgeText}>
                <span className={d.badgeName} dir="auto">{name}</span>
                {b.hidden || why ? <span className={d.badgeWhy}>{b.hidden ? t("badges.hiddenWhy") : why}</span> : null}
              </span>
              <span className={d.widgetTools}>
                <button type="button" className={d.iconBtn} onClick={() => void commit(moved(i, -1))} disabled={i === 0} aria-label={t("badges.moveUp", { name })} title={t("reorder.moveUp")}>
                  <ArrowUp size={16} aria-hidden="true" />
                </button>
                <button type="button" className={d.iconBtn} onClick={() => void commit(moved(i, 1))} disabled={i === list.length - 1} aria-label={t("badges.moveDown", { name })} title={t("reorder.moveDown")}>
                  <ArrowDown size={16} aria-hidden="true" />
                </button>
                <button
                  type="button"
                  className={d.iconBtn}
                  onClick={() => void commit(flipped(b.id))}
                  disabled={locked}
                  aria-pressed={!b.hidden}
                  aria-label={b.hidden ? t("badges.show", { name }) : t("badges.hide", { name })}
                  title={locked ? t("badges.staffAlways") : b.hidden ? t("badges.showShort") : t("badges.hideShort")}
                  data-testid={`badge-toggle-${b.id}`}
                >
                  {b.hidden ? <EyeOff size={16} aria-hidden="true" /> : <Eye size={16} aria-hidden="true" />}
                </button>
              </span>
            </li>
          );
        })}
      </ol>
    </section>
  );
}

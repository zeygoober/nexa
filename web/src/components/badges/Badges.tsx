import { lazy, Suspense, type CSSProperties } from "react";
import type { Badge, PublicUser } from "@/api/types";
import { Tooltip } from "@/components/common/Tooltip";
import { badgeLook, fallbackBadges, hexColour } from "@/lib/badges";
import s from "./Badges.module.css";

const EventIcon = lazy(() => import("./EventIcon"));

type Who = Pick<PublicUser, "id" | "flags" | "plan">;

/**
 * One badge: its picture (assets/badges, written by tools/badge_art.py), or
 * for an event badge staff made, its icon on a disc in the colour they chose.
 * The tooltip gives its name and meaning; [named] puts the name beside it, for
 * lists (the designer, the staff panel).
 */
export function BadgePill({ badge, user, still, named }: { badge: Badge; user: Who; still?: boolean; named?: boolean }) {
  const look = badgeLook(badge, user);
  if (!look) return null;
  const text = look.tip ? `${look.label}: ${look.tip}` : look.label;
  const mark = look.art ? (
    <img className={s.badge} src={look.art} alt={still ? "" : text} draggable={false} tabIndex={still ? undefined : 0} data-testid={`badge-${badge.id}`} />
  ) : (
    <span
      className={`${s.badge} ${s.event}`}
      style={look.color !== undefined ? ({ "--tone": hexColour(look.color) } as CSSProperties) : undefined}
      tabIndex={still ? undefined : 0}
      role="img"
      aria-label={still ? undefined : text}
      aria-hidden={still ? true : undefined}
      data-testid={`badge-${badge.id}`}
    >
      <Suspense fallback={null}>
        <EventIcon name={look.iconName} size={12} />
      </Suspense>
    </span>
  );
  if (named) {
    return (
      <span className={s.named}>
        {mark}
        <span className={s.name} dir="auto">{look.label}</span>
      </span>
    );
  }
  return still ? mark : <Tooltip text={text} side="top">{mark}</Tooltip>;
}

/**
 * A profile's badges in its holder's order, all in one pill that sits on the
 * username line beside the pronouns. Until the list arrives, the staff rank
 * and plan come from the account itself.
 */
export function BadgeRow({ user, badges, className }: { user: Who; badges: Badge[] | undefined; className?: string }) {
  const list = (badges ?? fallbackBadges(user)).filter((b) => !b.hidden && badgeLook(b, user));
  if (!list.length) return null;
  return (
    <span className={[s.pill, className].filter(Boolean).join(" ")} data-testid="profile-badges">
      {list.map((b) => <BadgePill key={b.id} badge={b} user={user} />)}
    </span>
  );
}

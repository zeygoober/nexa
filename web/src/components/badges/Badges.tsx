import { lazy, Suspense, useId } from "react";
import type { Badge, PublicUser } from "@/api/types";
import { Tooltip } from "@/components/common/Tooltip";
import { badgeLook, fallbackBadges, hexColour, type BadgeLook } from "@/lib/badges";
import s from "./Badges.module.css";

const EventIcon = lazy(() => import("./EventIcon"));

type Who = Pick<PublicUser, "id" | "flags" | "plan">;

/** A colour staff chose, pulled towards the text colour so it reads on this theme (--badge-ink-mix). */
const ink = (rgb: number) => `color-mix(in srgb, ${hexColour(rgb)}, var(--text-strong) var(--badge-ink-mix))`;

/**
 * An event badge staff made: just its icon, in its colour (or a gradient to
 * its second), or its uploaded picture whole. Drawn at any [size], so the
 * Badges tab's preview is the badge itself.
 */
export function BadgeMark({ look, size = 20 }: { look: BadgeLook; size?: number }) {
  const id = useId().replace(/:/g, "");
  if (look.image) {
    return (
      <span className={s.mark} style={{ width: size, height: size }}>
        <img className={s.markImage} src={look.image} alt="" draggable={false} />
      </span>
    );
  }
  const from = look.color !== undefined ? ink(look.color) : "var(--brand)";
  const to = look.color2 !== undefined ? ink(look.color2) : null;
  return (
    <span className={s.mark} style={{ width: size, height: size }}>
      {to && (
        <svg className={s.defs} aria-hidden="true">
          {/* in the icon's own 24 box: a bounding box would lose a straight line's gradient */}
          <linearGradient id={`${id}i`} gradientUnits="userSpaceOnUse" x1={12} y1={2} x2={12} y2={22}>
            <stop offset={0} style={{ stopColor: from }} />
            <stop offset={1} style={{ stopColor: to }} />
          </linearGradient>
        </svg>
      )}
      <Suspense fallback={null}>
        <EventIcon name={look.iconName} size={size} color={to ? `url(#${id}i)` : from} />
      </Suspense>
    </span>
  );
}

/** What a badge's tooltip shows: the badge large, its name, and what it means. */
function BadgeCard({ look }: { look: BadgeLook }) {
  return (
    <span className={s.card}>
      {look.art ? <img className={s.cardArt} src={look.art} alt="" draggable={false} /> : <BadgeMark look={look} size={64} />}
      <span className={s.cardName} dir="auto">{look.label}</span>
      {look.tip ? <span className={s.cardTip} dir="auto">{look.tip}</span> : null}
    </span>
  );
}

/**
 * One badge: its picture (assets/badges, written by tools/badge_art.py), or
 * for an event badge staff made, its mark (BadgeMark). The tooltip gives its
 * name and meaning; [named] puts the name beside it, for lists (the designer,
 * the staff panel).
 */
export function BadgePill({ badge, user, still, named, size = 20 }: { badge: Badge; user: Who; still?: boolean; named?: boolean; size?: number }) {
  const look = badgeLook(badge, user);
  if (!look) return null;
  const text = look.tip ? `${look.label}: ${look.tip}` : look.label;
  const mark = look.art ? (
    <img className={s.badge} style={{ width: size, height: size }} src={look.art} alt={still ? "" : text} draggable={false} tabIndex={still ? undefined : 0} data-testid={`badge-${badge.id}`} />
  ) : (
    <span
      className={s.badge}
      style={{ width: size, height: size }}
      tabIndex={still ? undefined : 0}
      role="img"
      aria-label={still ? undefined : text}
      aria-hidden={still ? true : undefined}
      data-testid={`badge-${badge.id}`}
    >
      <BadgeMark look={look} size={size} />
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
  return still ? mark : <Tooltip text={text} side="top" card={<BadgeCard look={look} />}>{mark}</Tooltip>;
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
      {list.map((b) => <BadgePill key={b.id} badge={b} user={user} size={22} />)}
    </span>
  );
}

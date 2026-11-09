import { lazy, Suspense, useId, type CSSProperties } from "react";
import type { Badge, PublicUser } from "@/api/types";
import { Modal } from "@/components/common/Modal";
import { Tooltip } from "@/components/common/Tooltip";
import { t } from "@/i18n";
import { useUI } from "@/stores/ui";
import { badgeLook, badgeTones, fallbackBadges, type BadgeLook } from "@/lib/badges";
import s from "./Badges.module.css";

const EventIcon = lazy(() => import("./EventIcon"));

type Who = Pick<PublicUser, "id" | "flags" | "plan">;

/**
 * An event badge staff made, drawn as the badge art is: the icon in a short
 * gradient at the art's luminance with a tint inside its shapes, or its
 * uploaded picture whole. Drawn at any [size], so the Badges tab's preview is
 * the badge itself.
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
  const [from, to] = look.color !== undefined ? badgeTones(look.color, look.color2) : ["var(--brand)", null];
  return (
    <span className={s.mark} style={{ width: size, height: size, "--badge-tint": from } as CSSProperties} data-badge-icon={look.iconName}>
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

/** How many badges a profile shows before the rest go behind +x. */
const SHOWN_BADGES = 5;

/** Every badge a profile has, each with its name and meaning: what +x opens. */
export function AllBadges({ user, list, onClose }: { user: Who; list: Badge[]; onClose: () => void }) {
  return (
    <Modal open onClose={onClose} title={t("badges.all")} width={400}>
      <ul className={s.allList} data-testid="all-badges">
        {list.map((b) => {
          const look = badgeLook(b, user)!;
          return (
            <li key={b.id} className={s.allRow}>
              <BadgePill badge={b} user={user} still size={36} />
              <span className={s.allText}>
                <span className={s.allName} dir="auto">{look.label}</span>
                {look.tip ? <span className={s.allTip} dir="auto">{look.tip}</span> : null}
              </span>
            </li>
          );
        })}
      </ul>
    </Modal>
  );
}

/**
 * A profile's badges in its holder's order, all in one pill that sits on the
 * username line beside the pronouns: the first five, then +x for the rest
 * ([onMore] says it was pressed, so a popout can get out of the box's way).
 * Until the list arrives, the staff rank and plan come from the account itself.
 */
export function BadgeRow({ user, badges, className, onMore }: { user: Who; badges: Badge[] | undefined; className?: string; onMore?: (() => void) | undefined }) {
  const list = (badges ?? fallbackBadges(user)).filter((b) => !b.hidden && badgeLook(b, user));
  if (!list.length) return null;
  const rest = list.length - SHOWN_BADGES;
  return (
    <span className={[s.pill, className].filter(Boolean).join(" ")} data-testid="profile-badges">
      {list.slice(0, SHOWN_BADGES).map((b) => <BadgePill key={b.id} badge={b} user={user} size={22} />)}
      {rest > 0 ? (
        <button type="button" className={s.more} aria-label={t("badges.showAll", { count: list.length })} onClick={() => { onMore?.(); useUI.getState().openModal("allBadges", { user, list }); }} data-testid="profile-badges-more">+{rest}</button>
      ) : null}
    </span>
  );
}

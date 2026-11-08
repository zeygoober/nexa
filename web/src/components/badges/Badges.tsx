import { lazy, Suspense, useId } from "react";
import type { Badge, PublicUser } from "@/api/types";
import { Tooltip } from "@/components/common/Tooltip";
import { badgeLook, fallbackBadges, hexColour, SHAPE_ICONS, SHAPE_PATHS, type BadgeLook } from "@/lib/badges";
import s from "./Badges.module.css";

const EventIcon = lazy(() => import("./EventIcon"));

type Who = Pick<PublicUser, "id" | "flags" | "plan">;

/** The dark glyph, for light colours a white icon would vanish on. */
const DARK_GLYPH = "#1b1724";

/** One outline in the 24 by 24 box every badge is drawn in. */
function Outline({ shape, ...rest }: { shape: BadgeLook["shape"] } & React.SVGProps<SVGPathElement & SVGCircleElement & SVGRectElement>) {
  if (!shape || shape === "circle") return <circle cx={12} cy={12} r={11.5} {...rest} />;
  if (shape === "rounded") return <rect x={0.5} y={0.5} width={23} height={23} rx={6.5} {...rest} />;
  return <path d={SHAPE_PATHS[shape]} {...rest} />;
}

/**
 * An event badge staff made: its shape filled with its colour (or a gradient
 * to its second), lit from above and ringed, holding its icon in white or dark,
 * or its uploaded picture clipped to the shape. Drawn at any [size], so the
 * Badges tab's preview is the badge itself.
 */
export function BadgeMark({ look, size = 20 }: { look: BadgeLook; size?: number }) {
  const id = useId().replace(/:/g, "");
  const from = look.color !== undefined ? hexColour(look.color) : "var(--brand)";
  const to = look.color2 !== undefined ? hexColour(look.color2) : from;
  const fit = (look.shape && SHAPE_ICONS[look.shape]) || { size: 0.62, y: 0 };
  return (
    <span className={s.mark} style={{ width: size, height: size, color: look.glyph === "dark" ? DARK_GLYPH : "#fff" }}>
      <svg className={s.shape} viewBox="0 0 24 24" aria-hidden="true">
        <defs>
          <linearGradient id={`${id}f`} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0" stopColor={from} />
            <stop offset="1" stopColor={to} />
          </linearGradient>
          <linearGradient id={`${id}h`} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0" stopColor="#fff" stopOpacity={0.32} />
            <stop offset="0.55" stopColor="#fff" stopOpacity={0} />
          </linearGradient>
          <clipPath id={`${id}c`}>
            <Outline shape={look.shape} />
          </clipPath>
        </defs>
        {look.image ? (
          <image href={look.image} x={0} y={0} width={24} height={24} preserveAspectRatio="xMidYMid slice" clipPath={`url(#${id}c)`} />
        ) : (
          <>
            <Outline shape={look.shape} fill={`url(#${id}f)`} />
            <Outline shape={look.shape} fill={`url(#${id}h)`} />
          </>
        )}
        <Outline shape={look.shape} fill="none" stroke="rgb(0 0 0 / 0.22)" strokeWidth={1} vectorEffect="non-scaling-stroke" />
      </svg>
      {!look.image && (
        <Suspense fallback={null}>
          <span className={s.markIcon} style={{ transform: fit.y ? `translateY(${(fit.y * size).toFixed(1)}px)` : undefined }}>
            <EventIcon name={look.iconName} size={Math.round(size * fit.size)} />
          </span>
        </Suspense>
      )}
    </span>
  );
}

/**
 * One badge: its picture (assets/badges, written by tools/badge_art.py), or
 * for an event badge staff made, its mark (BadgeMark). The tooltip gives its
 * name and meaning; [named] puts the name beside it, for lists (the designer,
 * the staff panel).
 */
export function BadgePill({ badge, user, still, named }: { badge: Badge; user: Who; still?: boolean; named?: boolean }) {
  const look = badgeLook(badge, user);
  if (!look) return null;
  const text = look.tip ? `${look.label}: ${look.tip}` : look.label;
  const mark = look.art ? (
    <img className={s.badge} src={look.art} alt={still ? "" : text} draggable={false} tabIndex={still ? undefined : 0} data-testid={`badge-${badge.id}`} />
  ) : (
    <span
      className={s.badge}
      tabIndex={still ? undefined : 0}
      role="img"
      aria-label={still ? undefined : text}
      aria-hidden={still ? true : undefined}
      data-testid={`badge-${badge.id}`}
    >
      <BadgeMark look={look} />
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

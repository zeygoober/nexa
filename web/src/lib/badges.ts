import type { Badge, BadgeDef, BadgeShape, PublicUser } from "@/api/types";
import { t } from "@/i18n";
import { rankName, rankOf, rankTip } from "@/lib/staffRank";

/**
 * Profile badges (internal/store/badges.go; Android data/Badges.kt). The
 * server works out which an account holds and puts them in its order; this
 * names and pictures them, staff rank and plan included, all drawn
 * together in one pill (components/badges/Badges.tsx).
 */

/** The badges staff give by hand, beside the event badges they make. */
export const CONTRIBUTOR = "contributor";
export const EVENT_PREFIX = "event:";

/** Whether a badge is an event badge staff made. */
export const isEventBadge = (id: string) => id.startsWith(EVENT_PREFIX);

/** The icons the Badges tab offers for an event badge (Lucide names, which Android draws too). */
export const EVENT_ICONS = [
  "award", "trophy", "medal", "crown", "star", "sparkles", "heart", "flame",
  "zap", "rocket", "ghost", "skull", "candy", "snowflake", "tree-pine", "gift",
  "party-popper", "cake", "music", "gamepad-2", "palette", "camera", "paintbrush", "pen-tool",
  "flag", "shield", "gem", "sun", "moon", "cloud", "leaf", "flower-2",
  "bug", "code", "terminal", "megaphone", "mic", "headphones", "book-open", "graduation-cap",
] as const;

/** The colours the Badges tab offers first; any other can be picked. */
export const EVENT_COLOURS = [0xf0c04a, 0xff7a45, 0xff5c93, 0xb48cff, 0x6aa2ff, 0x4cc9b0, 0x5fd17a, 0xb9c3cf];

/** The badge art (tools/badge_art.py writes it; Android copies the same files). */
const ART = import.meta.glob<string>("../assets/badges/*.svg", { eager: true, query: "?url", import: "default" });
const art = (name: string) => ART[`../assets/badges/${name}.svg`];

/** How a badge looks: its picture (or, for an event badge, its icon and colour), its name and what it means. */
export interface BadgeLook {
  art?: string;
  /** An event badge's Lucide icon by name, loaded when shown, on a [shape] in [color] (to [color2], a gradient). */
  iconName?: string;
  color?: number;
  color2?: number;
  glyph?: "dark";
  shape?: BadgeShape;
  /** An event badge's uploaded picture, in place of the icon. */
  image?: string;
  label: string;
  tip: string;
}

type Who = Pick<PublicUser, "id" | "flags" | "plan">;

/** A tiered badge's level, held to what this app draws. */
const tier = (b: Badge, top: number) => Math.min(top, Math.max(1, b.level ?? 1));

/** What a Bug Hunter or Ideas badge needs at each level (store.ReportTiers). */
export const REPORT_TIERS = [1, 3, 10, 25, 50] as const;

/** The look of a badge; null for ids this app does not know, and for staff or a plan the account no longer has. */
export function badgeLook(b: Badge, user: Who): BadgeLook | null {
  switch (b.id) {
    case "staff": {
      const rank = rankOf(user.id, user.flags);
      return rank ? { art: art(`staff-${rank}`), label: rankName(rank), tip: rankTip(rank) } : null;
    }
    case "plan":
      if (user.plan === "pro") return { art: art("plan-pro"), label: t("support.plan.pro"), tip: t("support.badge.proTip") };
      if (user.plan === "supporter") return { art: art("plan-supporter"), label: t("support.plan.supporter"), tip: t("support.badge.supporterTip") };
      return null;
    case "og":
      return { art: art("og"), label: t("badges.og"), tip: t("badges.ogTip") };
    case "years": {
      const count = Math.max(1, b.level ?? 1);
      return { art: art("years"), label: t("badges.years", { count }), tip: t("badges.yearsTip", { count }) };
    }
    case "bug_hunter": {
      const level = tier(b, 5);
      return { art: art(`bug-${level}`), label: t(`badges.bug.l${level}`), tip: t(`badges.bugTip.l${level}`) };
    }
    case "ideas": {
      const level = tier(b, 5);
      return { art: art(`ideas-${level}`), label: t(`badges.ideas.l${level}`), tip: t(`badges.ideasTip.l${level}`) };
    }
    case "app_developer":
      return { art: art("app-developer"), label: t("badges.appDeveloper"), tip: t("badges.appDeveloperTip") };
    case "gift_giver": {
      const level = tier(b, 3);
      return { art: art(`gift-${level}`), label: t("badges.giftGiver"), tip: t(`badges.giftGiverTip.${GIFT_TONES[level - 1]!}`) };
    }
    case CONTRIBUTOR:
      return { art: art("contributor"), label: t("badges.contributor"), tip: t("badges.contributorTip") };
    default:
      // a badge staff made says what they wrote for it
      if (isEventBadge(b.id) && b.name) return { iconName: b.icon, color: b.color, color2: b.color2, glyph: b.glyph, shape: b.shape, image: b.image, label: b.name, tip: b.description ?? "" };
      return null;
  }
}

const GIFT_TONES = ["bronze", "silver", "gold"] as const;

/** A badge's name in a list (the designer, the staff panel), staff and plan included. */
export function badgeName(b: Badge, user: Who): string {
  if (b.id === "staff" && !rankOf(user.id, user.flags)) return t("badges.staff");
  return badgeLook(b, user)?.label ?? b.id;
}

/** What a profile shows before its badges arrive, or where there is no list: the staff rank and the plan. */
export function fallbackBadges(user: Who): Badge[] {
  const out: Badge[] = [];
  if (rankOf(user.id, user.flags)) out.push({ id: "staff" });
  if (user.plan === "supporter" || user.plan === "pro") out.push({ id: "plan" });
  return out;
}

/** A colour number as CSS. */
export const hexColour = (c: number) => `#${(c & 0xffffff).toString(16).padStart(6, "0")}`;

/** The badges staff can give or take against what an account earned, with their top level (store.OverridableBadges). */
export const OVERRIDABLE: Record<string, number> = { og: 1, years: 50, bug_hunter: 5, ideas: 5, app_developer: 1, gift_giver: 3 };

/** The outlines an event badge can take, as the Badges tab offers them. */
export const BADGE_SHAPES: BadgeShape[] = ["circle", "rounded", "shield", "hexagon", "star", "diamond", "seal", "ribbon"];

/** An event badge staff made, as a badge someone holds (for drawing it in the panel). */
export function defBadge(d: Pick<BadgeDef, "id" | "name" | "description" | "icon" | "color" | "color2" | "glyph" | "shape" | "image_id">): Badge {
  const b: Badge = { id: EVENT_PREFIX + d.id, name: d.name, description: d.description, icon: d.icon, color: d.color };
  if (d.color2 >= 0) b.color2 = d.color2;
  if (d.glyph === "dark") b.glyph = "dark";
  if (d.shape && d.shape !== "circle") b.shape = d.shape;
  if (d.image_id && d.image_id !== "0") b.image = `/media/badge/${d.image_id}`;
  return b;
}

/**
 * The outline of each shape in a 24 by 24 box (Android ui/profile/Badges.kt
 * draws the same points), so a badge reads alike at 20 px and at 64.
 */
export const SHAPE_PATHS: Record<Exclude<BadgeShape, "circle" | "rounded">, string> = {
  shield: "M12 0.6 L22.2 4.2 Q22.6 16.6 12 23.4 Q1.4 16.6 1.8 4.2 Z",
  hexagon: "M12 0.4 L22.3 6.2 L22.3 17.8 L12 23.6 L1.7 17.8 L1.7 6.2 Z",
  star: "M 11.24 1.25 Q 12 -0.05 12.76 1.25 L 16.17 7.11 L 22.8 8.55 Q 24.27 8.86 23.27 9.98 L 18.75 15.04 L 19.43 21.79 Q 19.58 23.29 18.21 22.68 L 12 19.95 L 5.79 22.68 Q 4.42 23.29 4.57 21.79 L 5.25 15.04 L 0.73 9.98 Q -0.27 8.86 1.2 8.55 L 7.83 7.11 Z",
  diamond: "M 10.73 1.47 Q 12 0.2 13.27 1.47 L 22.53 10.73 Q 23.8 12 22.53 13.27 L 13.27 22.53 Q 12 23.8 10.73 22.53 L 1.47 13.27 Q 0.2 12 1.47 10.73 Z",
  seal: "M 22.5 12 Q 24.85 15.44 21.09 17.25 Q 21.4 21.4 17.25 21.09 Q 15.44 24.85 12 22.5 Q 8.56 24.85 6.75 21.09 Q 2.6 21.4 2.91 17.25 Q -0.85 15.44 1.5 12 Q -0.85 8.56 2.91 6.75 Q 2.6 2.6 6.75 2.91 Q 8.56 -0.85 12 1.5 Q 15.44 -0.85 17.25 2.91 Q 21.4 2.6 21.09 6.75 Q 24.85 8.56 22.5 12 Z",
  ribbon: "M 5.2 0.6 L 18.8 0.6 Q 21 0.6 21 2.8 L 21 22.4 Q 21 23.8 19.8 23.1 L 12 18.6 L 4.2 23.1 Q 3 23.8 3 22.4 L 3 2.8 Q 3 0.6 5.2 0.6 Z",
};

/**
 * How big a shape's icon is, as a share of the badge, and how far it sits below
 * the middle (Android ui/profile/Badges.kt keeps the same numbers): a star and a
 * diamond narrow towards their points, and a ribbon's notch takes its lower part.
 */
export const SHAPE_ICONS: Partial<Record<BadgeShape, { size: number; y: number }>> = {
  star: { size: 0.46, y: 0.03 },
  diamond: { size: 0.5, y: 0 },
  seal: { size: 0.58, y: 0 },
  ribbon: { size: 0.52, y: -0.1 },
};

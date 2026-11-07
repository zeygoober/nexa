import type { Badge, PublicUser } from "@/api/types";
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
  /** An event badge's Lucide icon by name, loaded when shown, on a disc in [color]. */
  iconName?: string;
  color?: number;
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
      if (isEventBadge(b.id) && b.name) // a badge staff made says only its own name: what it is for is theirs to put in it
      return { iconName: b.icon, color: b.color, label: b.name, tip: "" };
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

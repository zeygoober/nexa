import type { Badge, BadgeDef, PublicUser } from "@/api/types";
import { t, translator } from "@/i18n";
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
/** A badge picture by its file name, for the Badge Index. */
export const badgeArt = art;

/** How a badge looks: its picture (or, for an event badge, its icon and colour), its name and what it means. */
export interface BadgeLook {
  art?: string;
  /** An event badge's Lucide icon by name, loaded when shown, in [color] (to [color2], a gradient). */
  iconName?: string;
  color?: number;
  color2?: number;
  /** An event badge's uploaded picture, in place of the icon. */
  image?: string;
  label: string;
  tip: string;
}

type Who = Pick<PublicUser, "id" | "flags" | "plan">;

/** The relative luminance of the top and bottom of every badge (tools/badge_art.py), so one reads like another on any card. */
const BADGE_TOP = 0.27;
const BADGE_BOTTOM = 0.2;

const linear = (c: number) => (c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4);
const luminance = (c: number[]) => 0.2126 * linear(c[0]!) + 0.7152 * linear(c[1]!) + 0.0722 * linear(c[2]!);

/** [rgb] moved to [target] luminance, towards black or white, so its hue stays. */
function badgeTone(rgb: number, target: number): string {
  const from = [(rgb >> 16) & 255, (rgb >> 8) & 255, rgb & 255].map((c) => c / 255);
  const end = luminance(from) > target ? 0 : 1;
  const at = (t: number) => from.map((c) => c + (end - c) * t);
  let low = 0;
  let high = 1;
  for (let i = 0; i < 20; i++) {
    const t = (low + high) / 2;
    if (luminance(at(t)) > target === (end === 0)) low = t;
    else high = t;
  }
  return `#${at(high).map((c) => Math.round(c * 255).toString(16).padStart(2, "0")).join("")}`;
}

/** The two ends of a staff-made badge: its colour, or its two, at the luminance every badge has. */
export function badgeTones(color: number, color2?: number): [string, string] {
  return [badgeTone(color, BADGE_TOP), badgeTone(color2 ?? color, BADGE_BOTTOM)];
}

/** A tiered badge's level, held to what this app draws. */
const tier = (b: Badge, top: number) => Math.min(top, Math.max(1, b.level ?? 1));

/** What a Bug Hunter or Ideas badge needs at each level (store.ReportTiers). */
export const REPORT_TIERS = [1, 3, 10, 25, 50] as const;

/** The look of a badge; null for ids this app does not know, and for staff or a plan the account no longer has. */
/** "32nd" in English; other languages' sentences take the plain number. */
function joinPlace(n: number): string {
  if (translator().locale !== "en") return String(n);
  const tens = n % 100;
  const end = tens >= 11 && tens <= 13 ? "th" : ({ 1: "st", 2: "nd", 3: "rd" } as Record<number, string>)[n % 10] ?? "th";
  return `${n}${end}`;
}

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
      return { art: art("og"), label: t("badges.og"), tip: b.count ? t("badges.ogTipPlace", { place: joinPlace(b.count) }) : t("badges.ogTip") };
    case "years": {
      const count = Math.max(1, b.level ?? 1);
      return { art: art("years"), label: t("badges.years", { count }), tip: t("badges.yearsTip", { count }) };
    }
    case "bug_hunter": {
      const level = tier(b, 5);
      return { art: art(`bug-${level}`), label: t(`badges.bug.l${level}`), tip: b.count ? t("badges.bugTipCount", { count: b.count }) : t(`badges.bugTip.l${level}`) };
    }
    case "ideas": {
      const level = tier(b, 5);
      return { art: art(`ideas-${level}`), label: t(`badges.ideas.l${level}`), tip: b.count ? t("badges.ideasTipCount", { count: b.count }) : t(`badges.ideasTip.l${level}`) };
    }
    case "app_developer": {
      const level = tier(b, 3);
      return { art: art(`bot-maker-${level}`), label: t(`badges.botMaker.l${level}`), tip: b.count ? t("badges.appDeveloperTipCount", { count: b.count }) : t("badges.botMakerTip", { count: BOT_MAKER_TIERS[level - 1]! }) };
    }
    case "gift_giver": {
      const level = tier(b, 3);
      return { art: art(`gift-${level}`), label: t("badges.giftGiver"), tip: b.count ? t("badges.giftGiverTipCount", { count: b.count }) : t(`badges.giftGiverTip.${GIFT_TONES[level - 1]!}`) };
    }
    // friends, hours and streaks stay private: these say the level reached, never the holder's number
    case "friendly": {
      const level = tier(b, 3);
      return { art: art(`friendly-${level}`), label: t(`badges.friendly.l${level}`), tip: t("badges.friendlyTip", { count: FRIENDLY_TIERS[level - 1]! }) };
    }
    case "host": {
      const level = tier(b, 3);
      return { art: art(`host-${level}`), label: t(`badges.host.l${level}`), tip: t("badges.hostTip", { count: HOST_TIERS[level - 1]! }) };
    }
    case "booster":
      return { art: art("booster"), label: t("badges.booster"), tip: t("badges.boosterTip") };
    case "regular": {
      const level = tier(b, 4);
      return { art: art(`regular-${level}`), label: t(`badges.regular.l${level}`), tip: t("badges.regularTip", { count: REGULAR_TIERS[level - 1]! }) };
    }
    case "open_mic": {
      const level = tier(b, 3);
      return { art: art(`open-mic-${level}`), label: t(`badges.openMic.l${level}`), tip: t("badges.openMicTip", { count: OPEN_MIC_TIERS[level - 1]! }) };
    }
    case CONTRIBUTOR:
      return { art: art("contributor"), label: t("badges.contributor"), tip: t("badges.contributorTip") };
    default:
      // a badge staff made says what they wrote for it
      if (isEventBadge(b.id) && b.name) return { iconName: b.icon, color: b.color, color2: b.color2, image: b.image, label: b.name, tip: b.description ?? "" };
      return null;
  }
}

const GIFT_TONES = ["bronze", "silver", "gold"] as const;

/** What each level needs (store's tiers of the same names). */
const BOT_MAKER_TIERS = [10, 100, 1000] as const;
const FRIENDLY_TIERS = [5, 25, 100] as const;
const HOST_TIERS = [10, 100, 1000] as const;
const REGULAR_TIERS = [7, 30, 100, 365] as const;
const OPEN_MIC_TIERS = [10, 100, 1000] as const;

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
export const OVERRIDABLE: Record<string, number> = { og: 1, years: 50, bug_hunter: 5, ideas: 5, app_developer: 3, gift_giver: 3, friendly: 3, host: 3, booster: 1, regular: 4, open_mic: 3 };

/** An event badge staff made, as a badge someone holds (for drawing it in the panel). */
export function defBadge(d: Pick<BadgeDef, "id" | "name" | "description" | "icon" | "color" | "color2" | "glyph" | "shape" | "image_id">): Badge {
  const b: Badge = { id: EVENT_PREFIX + d.id, name: d.name, description: d.description, icon: d.icon, color: d.color };
  if (d.color2 >= 0) b.color2 = d.color2;
  if (d.glyph === "dark") b.glyph = "dark";
  if (d.shape && d.shape !== "circle") b.shape = d.shape;
  if (d.image_id && d.image_id !== "0") b.image = `/media/badge/${d.image_id}`;
  return b;
}

import type { TradeLine } from "@/api/types";
import { BadgePill } from "@/components/badges/Badges";
import { badgeLook } from "@/lib/badges";
import { t } from "@/i18n";
import s from "./Trades.module.css";

/** Whoever draws a trade's badges: a trade has no use for the account's rank or plan, so nobody here has any. */
export const NOBODY = { id: "0", flags: 0, plan: "" as const };

/** A badge's name as a trade lists it; empty when this app cannot draw it. */
export function lineName(line: TradeLine): string {
  return line.badge ? (badgeLook(line.badge, NOBODY)?.label ?? "") : "";
}

/** One side of a trade as the card in the messages lists it: a picture, a name and how many. */
export function LineList({ lines }: { lines: TradeLine[] }) {
  if (!lines.length) return <p className={s.nothing}>{t("trades.nothing")}</p>;
  return (
    <ul className={s.lines}>
      {lines.map((l) => (
        <li key={l.key} className={s.line}>
          {l.badge ? <BadgePill badge={l.badge} user={NOBODY} still size={28} /> : <span className={s.blank} aria-hidden="true" />}
          <span className={s.lineName} dir="auto">{lineName(l) || l.key}</span>
          {l.qty > 1 ? <span className={s.lineQty} aria-label={t("badges.index.copies", { count: l.qty })}>×{l.qty}</span> : null}
        </li>
      ))}
    </ul>
  );
}

import { iconNames } from "lucide-react/dynamic";
import EventIcon from "@/components/badges/EventIcon";
import { t } from "@/i18n";
import b from "./BadgesTab.module.css";

/** The most icons a search shows at once, so a short query does not draw a thousand. */
const SHOWN = 120;

/**
 * Every Lucide icon whose name holds each word typed, best first: exact,
 * then starting with the query, then the rest. Loaded only once someone
 * searches, since the list of names is large.
 */
export default function BadgeIconSearch({ query, value, onPick }: { query: string; value: string; onPick: (name: string) => void }) {
  const words = query.trim().toLowerCase().split(/[\s-]+/).filter(Boolean);
  const q = words.join("-");
  const found = iconNames
    .filter((n) => words.every((w) => n.includes(w)))
    .sort((a, b2) => rank(a, q) - rank(b2, q) || a.length - b2.length || a.localeCompare(b2))
    .slice(0, SHOWN);
  if (found.length === 0) return <p className={b.iconsNone}>{t("badges.staff.iconsNone")}</p>;
  return (
    <>
      {found.map((n) => (
        <button key={n} type="button" role="radio" aria-checked={value === n} aria-label={n} title={n} className={b.icon} data-on={value === n || undefined} onClick={() => onPick(n)}>
          <EventIcon name={n} size={18} />
        </button>
      ))}
    </>
  );
}

const rank = (name: string, q: string) => (name === q ? 0 : name.startsWith(q) ? 1 : 2);

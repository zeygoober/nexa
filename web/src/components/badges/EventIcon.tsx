import { Award } from "lucide-react";
import { DynamicIcon, iconNames, type IconName } from "lucide-react/dynamic";
import "./iconBodies.css";

const known = new Set<string>(iconNames);

/**
 * An event badge's Lucide icon by name, fetched the first time one shows
 * (this module, with the list of every name, loads only then too). A name
 * this build does not know draws a plain award.
 */
export default function EventIcon({ name, size = 12, color }: { name: string | undefined; size?: number; color?: string }) {
  const line = { size, color, strokeWidth: 2.3, "aria-hidden": true } as const;
  if (!name || !known.has(name)) return <Award {...line} />;
  return <DynamicIcon name={name as IconName} {...line} fallback={() => <Award {...line} />} />;
}

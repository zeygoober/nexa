import { GEM_ART } from "@/lib/badges";

/** The Gem, drawn as the badge art is (tools/badge_art.py), wherever an amount of Gems is shown. */
export function GemIcon({ size = 16, className }: { size?: number; className?: string }) {
  return <img src={GEM_ART} width={size} height={size} alt="" aria-hidden="true" draggable={false} className={className} style={{ flex: "none" }} />;
}

import { api } from "./client";
import type { Badge, Snowflake, TradeLine } from "./types";

/** Gems (api/gems.go): earned by chatting, spent on capsules of collectibles, fused and traded. */

export type GemRarity = "common" | "rare" | "epic" | "huge";

/** "" earns as usual; `check` holds new Gems until a captcha; `report` waits on staff; `off` until `off_until`. */
export type GemState = "" | "check" | "report" | "off";

export interface GemWallet {
  gems: number;
  held: number;
  earned: number;
  last_earn: number;
  state: GemState;
  state_at: number;
  off_until: number;
}

export interface GemTeamMember {
  badge: Badge;
  /** What this badge adds to earning, in percent. */
  power: number;
}

export interface Collectible {
  id: Snowflake;
  name: string;
  rarity: GemRarity;
  /** Drawn at bronze. */
  badge: Badge;
}

export interface GemsView {
  wallet: GemWallet;
  team: GemTeamMember[];
  /** Earning in percent: 100 is one Gem a message. */
  multiplier: number;
  capsule_price: number;
  fuse_cost: number;
  /** The chance of each rarity, in hundredths of a percent (10000 is certain), as the server draws it. */
  odds: Partial<Record<GemRarity, number>>;
  collectibles: Collectible[];
  /** The collectibles this account holds, with copies. */
  collection: TradeLine[];
}

export const gems = {
  get(): Promise<GemsView> {
    return api.request("GET", "/gems");
  },
  /** Spends a capsule's price on one collectible, drawn at the odds shown. */
  openCapsule(): Promise<{ got: Collectible; key: string; gems: GemsView }> {
    return api.request("POST", "/gems/capsule");
  },
  /**
   * Puts 1 to `fuse_cost` copies of `key` in for one of the next material; each
   * copy is an equal share of the chance, so a full set always works. The server
   * rolls: `upgraded` says how it went, and the copies are spent either way.
   */
  fuse(key: string, copies: number): Promise<{ key: string; upgraded: boolean; gems: GemsView }> {
    return api.request("POST", "/gems/fuse", { key, copies });
  },
  /** Answers a farming check; the captcha is asked for by the client's solver. */
  confirm(): Promise<GemsView> {
    return api.request("POST", "/gems/confirm");
  },
  /** Everything kept about this account's Gems, for the data export: the wallet, the ledger and the timing summary. */
  record(): Promise<unknown> {
    return api.request("GET", "/gems/record");
  },
};

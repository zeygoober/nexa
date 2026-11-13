import { api } from "./client";
import type { Snowflake, Trade, TradeLine } from "./types";

/** Badge trades (api/badge_trades.go): a window two people share, each putting badges of their own in it. */
export const trades = {
  /** What this account can put in a trade, with how many copies of each. */
  mine(): Promise<TradeLine[]> {
    return api.request("GET", "/trades/holdings");
  },
  /** Starts a trade with someone, or gives the one already open between the two. */
  open(to: Snowflake): Promise<Trade> {
    return api.request("POST", "/trades", { to });
  },
  get(id: Snowflake): Promise<Trade> {
    return api.request("GET", `/trades/${id}`);
  },
  /** Replaces this account's side. Both Ready marks go. */
  offer(id: Snowflake, lines: { key: string; qty: number }[]): Promise<Trade> {
    return api.request("PUT", `/trades/${id}/offer`, { lines });
  },
  /** Marks this account ready, or not, for the offers at `version`; when both are, it happens. */
  ready(id: Snowflake, ready: boolean, version: number): Promise<Trade> {
    return api.request("PUT", `/trades/${id}/ready`, { ready, version });
  },
  /** Ends it before it happened: declined by the one it was opened with, cancelled by the one who opened it. */
  end(id: Snowflake): Promise<Trade> {
    return api.request("POST", `/trades/${id}/end`);
  },
};

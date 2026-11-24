import { api } from "./client";
import type { Snowflake } from "./types";

/** Quests (api/quests.go): three daily quests with a bonus for all three, and the Quest Tower. Paid in Gems when finished. */

export type QuestKind =
  | "messages" | "servers" | "call_minutes" | "call_size" | "friends" | "joins" | "capsules"
  | "fuses" | "materials" | "trades" | "gems" | "bugs" | "ideas" | "dailies";

export interface Quest {
  id: Snowflake;
  track: "daily" | "bonus" | "trial";
  /** Daily: 1 easy, 2 medium, 3 hard. Trial: the step. */
  level: number;
  kind: QuestKind;
  target: number;
  progress: number;
  reward: number;
  started_at: number;
  /** 0 while open. */
  done_at: number;
  /** What finishing paid; 0 while earning was off, and held Gems count. */
  paid: number;
}

export interface QuestBoard {
  /** The UTC day, in days since 1970. */
  day: number;
  /** When the daily quests change, in Unix milliseconds. */
  resets_at: number;
  daily: Quest[];
  bonus: Quest | null;
  trial: Quest | null;
  trial_cleared: number;
  reroll_left: boolean;
  skip_left: boolean;
}

export const quests = {
  get(): Promise<QuestBoard> {
    return api.request("GET", "/quests");
  },
  /** Swaps an open daily quest for another of the same difficulty, once a day. */
  reroll(id: Snowflake): Promise<QuestBoard> {
    return api.request("POST", `/quests/${id}/reroll`);
  },
  /** Swaps the Trial's quest for another at the same step, once a day. */
  skipTrial(): Promise<QuestBoard> {
    return api.request("POST", "/quests/trial/skip");
  },
};

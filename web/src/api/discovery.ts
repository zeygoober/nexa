import { api, captchaOptions } from "./client";
import type { DiscoveryCard, PublicUser, ServerView, Snowflake } from "./types";

export interface DiscoveryQuery {
  nsfw?: number;
  q?: string;
  category?: string;
  tag?: string;
  language?: string;
  min_members?: number;
  max_members?: number;
  sort?: "members" | "newest" | "created";
  offset?: number;
  limit?: number;
}

/** A bot its developer listed on Discover. */
export interface ListedApp {
  id: Snowflake;
  name: string;
  description: string;
  icon_id: Snowflake | null;
  avatar_id: Snowflake | null;
  tags: string[];
  servers: number;
  reads_all: boolean;
}

export const discovery = {
  bots(q: { q?: string; sort?: "popular" | "newest"; offset?: number; limit?: number }): Promise<{ bots: ListedApp[]; has_more: boolean; next_offset?: number }> {
    const query: Record<string, string> = {};
    for (const [k, v] of Object.entries(q)) if (v !== undefined && v !== "" && v !== 0) query[k] = String(v);
    return api.request("GET", "/discovery/bots", undefined, { query });
  },
  search(q: DiscoveryQuery = {}): Promise<{ servers: DiscoveryCard[]; has_more: boolean; next_offset?: number }> {
    const query: Record<string, string> = {};
    for (const [k, v] of Object.entries(q)) if (v !== undefined && v !== "" && v !== 0) query[k] = String(v);
    return api.request("GET", "/discovery", undefined, { query });
  },
  categories(showNSFW = false): Promise<{ categories: string[]; counts: Record<string, number>; total: number; tags: string[]; languages: string[] }> {
    return api.request("GET", "/discovery/categories", undefined, { query: showNSFW ? { nsfw: "1" } : {} });
  },
  server(id: Snowflake, nsfw = false): Promise<{ server: DiscoveryCard; emojis?: { id: Snowflake; blob_id: Snowflake; name: string; animated: boolean }[]; owner?: PublicUser }> {
    return api.request("GET", `/discovery/${id}`, undefined, { query: nsfw ? { nsfw: "1" } : {} });
  },
  join(id: Snowflake, captcha?: string | null, nsfw = false): Promise<{ server: ServerView; users: PublicUser[] }> {
    return api.request("POST", `/discovery/${id}/join`, undefined, { ...captchaOptions(captcha), query: nsfw ? { nsfw: "1" } : {} });
  },
};

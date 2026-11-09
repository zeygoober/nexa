import { api } from "./client";
import type { Message, ReadState, SendMessageRequest, Snowflake } from "./types";

export interface HistoryQuery {
  before?: Snowflake;
  after?: Snowflake;
  around?: Snowflake;
  limit?: number;
}

/** The most messages one bulk delete takes; the server refuses more. */
export const BULK_DELETE_MAX = 100;

const reactionPath = (channelId: Snowflake, mid: Snowflake, emoji: string): string =>
  `/channels/${channelId}/messages/${mid}/reactions/${encodeURIComponent(emoji)}`;

export const messages = {
  /** `opts.signal` lets a long walk through history be called off mid-page. */
  list(channelId: Snowflake, q: HistoryQuery = {}, opts?: { signal?: AbortSignal }): Promise<Message[]> {
    return api.request<Message[]>("GET", `/channels/${channelId}/messages`, undefined, {
      query: { before: q.before, after: q.after, around: q.around, limit: q.limit },
      ...(opts?.signal ? { signal: opts.signal } : {}),
    });
  },
  /** Particular messages of a channel, up to 100; the ones missing from the answer are gone. */
  byIds(channelId: Snowflake, ids: Snowflake[]): Promise<Message[]> {
    return api.request<Message[]>("GET", `/channels/${channelId}/messages`, undefined, { query: { ids: ids.join(",") } });
  },
  get(channelId: Snowflake, mid: Snowflake): Promise<Message> {
    return api.request<Message>("GET", `/channels/${channelId}/messages/${mid}`);
  },
  send(channelId: Snowflake, body: SendMessageRequest): Promise<Message> {
    return api.request<Message>("POST", `/channels/${channelId}/messages`, body);
  },
  edit(channelId: Snowflake, mid: Snowflake, body: Partial<SendMessageRequest>): Promise<Message> {
    return api.request<Message>("PATCH", `/channels/${channelId}/messages/${mid}`, body);
  },
  remove(channelId: Snowflake, mid: Snowflake): Promise<void> {
    return api.request<void>("DELETE", `/channels/${channelId}/messages/${mid}`);
  },
  /** Deletes up to BULK_DELETE_MAX messages of one channel; answers with the ids that went. */
  removeMany(channelId: Snowflake, ids: Snowflake[]): Promise<{ ids: Snowflake[] }> {
    return api.request<{ ids: Snowflake[] }>("POST", `/channels/${channelId}/messages/bulk-delete`, { ids });
  },
  ack(channelId: Snowflake, mid: Snowflake): Promise<ReadState> {
    return api.request<ReadState>("POST", `/channels/${channelId}/messages/${mid}/ack`);
  },
  setReactionRole(channelId: Snowflake, mid: Snowflake, emoji: string, roleId: Snowflake): Promise<unknown> {
    return api.request("PUT", `/channels/${channelId}/messages/${mid}/reaction-roles/${encodeURIComponent(emoji)}`, { role_id: roleId });
  },
  removeReactionRole(channelId: Snowflake, mid: Snowflake, emoji: string): Promise<unknown> {
    return api.request("DELETE", `/channels/${channelId}/messages/${mid}/reaction-roles/${encodeURIComponent(emoji)}`);
  },
  react(channelId: Snowflake, mid: Snowflake, emoji: string): Promise<void> {
    return api.request<void>("PUT", `${reactionPath(channelId, mid, emoji)}/@me`);
  },
  unreact(channelId: Snowflake, mid: Snowflake, emoji: string): Promise<void> {
    return api.request<void>("DELETE", `${reactionPath(channelId, mid, emoji)}/@me`);
  },
  reactors(channelId: Snowflake, mid: Snowflake, emoji: string, limit?: number): Promise<Snowflake[]> {
    return api.request<Snowflake[]>("GET", reactionPath(channelId, mid, emoji), undefined, { query: { limit } });
  },
};

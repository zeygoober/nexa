import { useThreads } from "@/stores/threads";
import type { Channel } from "@/api/types";
import { compareIds } from "@/lib/ids";
import type { ChannelRead } from "@/stores/readStates";
import { isUnread } from "@/stores/readStates";
import type { Settings } from "@/stores/settings";

export interface UnreadSummary {
  unread: boolean;
  mentions: number;
}

/** Whether notifications for a channel (and its server) are muted in settings. */
export function isMuted(settings: Settings, channelId: string, serverId: string | null | undefined): boolean {
  const { muted, perChannel } = settings.notifications;
  if (muted.includes(channelId)) return true;
  if (serverId && muted.includes(serverId)) return true;
  const level = perChannel[channelId] ?? (serverId ? perChannel[serverId] : undefined);
  return level === "none";
}

/**
 * The number a conversation's badge shows: every unread message from the
 * others, or only the mentions once it is muted.
 */
export function conversationCount(ch: Channel, rs: ChannelRead | undefined, settings: Settings): number {
  if (!rs || !isUnread(rs, ch.last_message_id)) return 0;
  if (isMuted(settings, ch.id, null)) return rs.mentions;
  return Math.max(rs.unread, rs.mentions);
}

/** DMs and groups with messages waiting, newest first, leaving out the one open. */
export function unreadConversations(channels: Record<string, Channel>, reads: Record<string, ChannelRead>, settings: Settings, openId?: string): { channel: Channel; count: number }[] {
  const out: { channel: Channel; count: number }[] = [];
  for (const ch of Object.values(channels)) {
    if ((ch.type !== "dm" && ch.type !== "group_dm") || ch.id === openId) continue;
    const count = conversationCount(ch, reads[ch.id], settings);
    if (count > 0) out.push({ channel: ch, count });
  }
  return out.sort((a, b) => compareIds(b.channel.last_message_id ?? b.channel.id, a.channel.last_message_id ?? a.channel.id));
}

/** A badge's text: the count, or 99+. */
export function badgeText(n: number): string {
  return n > 99 ? "99+" : String(n);
}

/** Whether a channel's unread state and mentions count towards its server's badge. A thread counts once this account is in it, as on Android. */
export function countsForServer(ch: Channel): boolean {
  if (ch.type === "text" || ch.type === "voice" || ch.type === "announcement") return true;
  return (ch.type === "thread" || ch.type === "private_thread") && !!useThreads.getState().joined[ch.id];
}

/** Aggregates unread state over a server's text channels and the chats of its voice channels, honouring mutes. */
export function serverUnread(serverId: string, channels: Record<string, Channel>, reads: Record<string, ChannelRead>, settings: Settings): UnreadSummary {
  let unread = false;
  let mentions = 0;
  const serverMuted = settings.notifications.muted.includes(serverId);
  for (const ch of Object.values(channels)) {
    if (ch.server_id !== serverId || !countsForServer(ch)) continue;
    const rs = reads[ch.id];
    mentions += rs?.mentions ?? 0;
    if (!serverMuted && !settings.notifications.muted.includes(ch.id) && isUnread(rs, ch.last_message_id)) unread = true;
  }
  return { unread, mentions };
}

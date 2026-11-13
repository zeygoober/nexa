import { ApiError } from "@/api/client";
import { servers as serversApi } from "@/api/servers";
import { t } from "@/i18n";
import { countsForServer } from "@/lib/unread";
import { useChannels } from "@/stores/channels";
import { isUnread, useReadStates } from "@/stores/readStates";
import { useServers } from "@/stores/servers";
import { useSettings } from "@/stores/settings";
import { useUI } from "@/stores/ui";

/** What a server's menus do, apart from the menus, so the rail's menus can share it. */

export function markServerRead(serverId: string): void {
  const channels = useChannels.getState().byId;
  const reads = useReadStates.getState();
  // every channel the server's badge counts, announcements and joined threads included, or their pings stay
  for (const ch of Object.values(channels)) {
    if (ch.server_id !== serverId || !countsForServer(ch) || !ch.last_message_id) continue;
    if (isUnread(reads.byChannel[ch.id], ch.last_message_id) || (reads.byChannel[ch.id]?.mentions ?? 0) > 0) void reads.ack(ch.id, ch.last_message_id);
  }
}

export function toggleMuted(id: string): void {
  const st = useSettings.getState();
  const muted = st.settings.notifications.muted;
  st.patch({ notifications: { ...st.settings.notifications, muted: muted.includes(id) ? muted.filter((x) => x !== id) : [...muted, id] } });
}

export async function leaveServer(serverId: string, nav: (p: string) => void): Promise<void> {
  try {
    await serversApi.leave(serverId);
    useServers.getState().remove(serverId);
    nav("/channels/@me");
  } catch (err) {
    useUI.getState().toast(err instanceof ApiError ? err.message : t("sidebar.serverMenu.couldNotLeaveTheServer"), "error");
  }
}

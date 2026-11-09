import { Suspense, startTransition, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Spinner } from "@/components/common/Spinner";
import { useIsMobile } from "@/hooks/useIsMobile";
import { readyPage, type ReadyPage } from "@/lib/lazyChunk";
import { P } from "@/lib/perms";
import { useAuth } from "@/stores/auth";
import { canI, useServers } from "@/stores/servers";
import { useChannels } from "@/stores/channels";
import { SettingsLayer, type NavGroup } from "./SettingsLayer";

import { t } from "@/i18n";
const OverviewPage = readyPage(() => import("./pages/ServerOverviewPage"));
const ServerTagPage = readyPage(() => import("./pages/ServerTagPage"));
const RolesPage = readyPage(() => import("./pages/RolesPage"));
const MembersPage = readyPage(() => import("./pages/MembersPage"));
const BansPage = readyPage(() => import("./pages/BansPage"));
const InvitesPage = readyPage(() => import("./pages/InvitesPage"));
const AuditLogPage = readyPage(() => import("./pages/AuditLogPage"));
const EncryptionPage = readyPage(() => import("./pages/EncryptionPage"));
const EmojiPage = readyPage(() => import("./pages/EmojiPage"));
const ServerStickersPage = readyPage(() => import("./pages/ServerStickersPage"));
const IntegrationsPage = readyPage(() => import("./pages/IntegrationsPage"));
const DeleteServerPage = readyPage(() => import("./pages/DeleteServerPage"));
const AutomodPage = readyPage(() => import("./pages/automod/AutomodPage"));
const DiscoverySettingsPage = readyPage(() => import("./pages/DiscoverySettingsPage"));
const SafetyPage = readyPage(() => import("./pages/SafetyPage"));

const PAGES: Record<string, ReadyPage<never>> = {
  overview: OverviewPage, tag: ServerTagPage, roles: RolesPage, members: MembersPage, bans: BansPage, invites: InvitesPage, audit: AuditLogPage,
  encryption: EncryptionPage, emoji: EmojiPage, stickers: ServerStickersPage, integrations: IntegrationsPage, delete: DeleteServerPage,
  automod: AutomodPage, discovery: DiscoverySettingsPage, safety: SafetyPage,
} as unknown as Record<string, ReadyPage<never>>;

export default function ServerSettingsModal({ onClose, serverId, page: initial }: { onClose: () => void; serverId: string; page?: string | undefined }) {
  const st = useServers((x) => x.byId[serverId]);
  const meId = useAuth((a) => a.me?.id);
  const nav = useNavigate();
  const owner = !!st && st.server.owner_id === meId;
  const pages: { id: string; label: string; ok: boolean; group: number; danger?: boolean }[] = [
    { id: "overview", label: t("server.channelSettings.overview"), ok: canI(st, P.MANAGE_SERVER), group: 0 },
    { id: "tag", label: t("serverTag.page.title"), ok: canI(st, P.MANAGE_SERVER), group: 0 },
    { id: "roles", label: t("server.members.roles"), ok: canI(st, P.MANAGE_ROLES), group: 0 },
    { id: "emoji", label: t("modals.customStatus.emoji"), ok: true, group: 0 },
    { id: "stickers", label: t("pickers.expressionPicker.stickers"), ok: true, group: 0 },
    // its apps for whoever manages the server, its webhooks for whoever manages them somewhere
    { id: "integrations", label: t("integrations.title"), ok: canI(st, P.MANAGE_SERVER) || canI(st, P.MANAGE_WEBHOOKS) || Object.values(useChannels.getState().byId).some((c) => c.server_id === serverId && c.type === "text" && canI(st, P.MANAGE_WEBHOOKS, c.id)), group: 0 },
    { id: "encryption", label: t("server.encryption.encryption"), ok: true, group: 0 },
    { id: "discovery", label: t("discovery.title"), ok: canI(st, P.MANAGE_SERVER), group: 0 },
    { id: "members", label: t("server.serverSettings.members"), ok: true, group: 1 },
    { id: "invites", label: t("server.invites.invites"), ok: canI(st, P.MANAGE_SERVER), group: 1 },
    { id: "bans", label: t("server.serverSettings.bans"), ok: canI(st, P.BAN_MEMBERS), group: 1 },
    { id: "automod", label: t("automod.title"), ok: canI(st, P.MANAGE_SERVER) || canI(st, P.VIEW_AUDIT_LOG), group: 1 },
    { id: "safety", label: t("safety.title"), ok: canI(st, P.MANAGE_SERVER), group: 1 },
    { id: "audit", label: t("server.auditLog.auditLog"), ok: canI(st, P.VIEW_AUDIT_LOG), group: 1 },
    { id: "delete", label: t("server.deleteServer.deleteServer"), ok: owner, group: 2, danger: true },
  ];
  const allowed = pages.filter((p) => p.ok);
  const [page, setShown] = useState(initial && allowed.some((p) => p.id === initial) ? initial : (allowed[0]?.id ?? "members"));
  const mobile = useIsMobile();
  // Beside the nav, another page keeps the one on screen until its file is
  // here, rather than blanking the panel to a spinner for the moment it takes.
  // A phone has just slid over to the page asked for, so the old page there
  // would be the wrong page under the new page's place, for as long as a slow
  // line takes.
  const setPage = (id: string) => (mobile ? setShown(id) : startTransition(() => setShown(id)));
  const [opening] = useState(page);
  useEffect(() => {
    if (!st) onClose();
  }, [st, onClose]);
  if (!st) return null;
  // the frame waits for the page it opens on, so both show at once (the dialog's place is held meanwhile);
  // a phone opened as a whole opens on the list, which has no such page to wait for
  if (!mobile || initial) PAGES[opening]?.ready();
  const groups: NavGroup[] = [
    { head: st.server.name, items: allowed.filter((p) => p.group === 0).map((p) => ({ id: p.id, label: p.label })) },
    { head: t("server.serverSettings.moderation"), items: allowed.filter((p) => p.group === 1).map((p) => ({ id: p.id, label: p.label })) },
  ];
  if (owner) groups.push({ head: "", items: [{ id: "delete", label: t("server.deleteServer.deleteServer"), danger: true }] });
  const body =
    page === "overview" ? <OverviewPage serverId={serverId} /> :
    page === "tag" ? <ServerTagPage serverId={serverId} /> :
    page === "roles" ? <RolesPage serverId={serverId} /> :
    page === "members" ? <MembersPage serverId={serverId} /> :
    page === "bans" ? <BansPage serverId={serverId} /> :
    page === "invites" ? <InvitesPage serverId={serverId} /> :
    page === "audit" ? <AuditLogPage serverId={serverId} /> :
    page === "automod" ? <AutomodPage serverId={serverId} /> :
    page === "discovery" ? <DiscoverySettingsPage serverId={serverId} /> :
    page === "safety" ? <SafetyPage serverId={serverId} go={setPage} /> :
    page === "emoji" ? <EmojiPage serverId={serverId} /> :
    page === "stickers" ? <ServerStickersPage serverId={serverId} /> :
    page === "integrations" ? <IntegrationsPage serverId={serverId} /> :
    page === "delete" ? <DeleteServerPage serverId={serverId} onDone={() => { onClose(); nav("/channels/@me"); }} /> :
    <EncryptionPage serverId={serverId} />;
  return (
    <SettingsLayer label={t("server.serverSettings.serverSettings")} groups={groups} page={page} setPage={setPage} onClose={onClose} startOnList={!initial}>
      {/* A blank panel on a slow link is indistinguishable from a crash. */}
      <Suspense fallback={<div style={{ display: "flex", justifyContent: "center", padding: 40 }}><Spinner label={t("chat.pins.loading")} /></div>}>{body}</Suspense>
    </SettingsLayer>
  );
}

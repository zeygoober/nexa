import { Lock, Plug, Webhook, X } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { moderation as modApi, type ManageChannel, type ManageMember, type ManageRule, type ManageView, type ModServer } from "@/api/moderation";
import type { Channel, Role } from "@/api/types";
import { Avatar } from "@/components/common/Avatar";
import { Button } from "@/components/common/Button";
import { ChannelGlyph } from "@/components/common/ChannelGlyph";
import { TextInput as Input } from "@/components/common/TextInput";
import { LoadError } from "@/components/common/LoadError";
import { Modal } from "@/components/common/Modal";
import { Spinner } from "@/components/common/Spinner";
import { hexColour } from "@/lib/badges";
import { personName } from "@/lib/moderation";
import { useAuth } from "@/stores/auth";
import { sameOverwrites } from "@/lib/perms";
import { useUI } from "@/stores/ui";
import { t } from "@/i18n";
import { ServerIcon } from "../ProfileSheet";
import { ManageActionModal, type ManageAsk, type ManageField, type ManageTarget } from "./ManageActionModal";
import m from "../Moderation.module.css";
import s from "./ManageServer.module.css";

type Tab = "overview" | "channels" | "roles" | "members" | "bans" | "invites" | "apps" | "automod";

const DAY = 24 * 3600 * 1000;
const CHANNEL_KINDS = ["text", "voice", "category", "announcement", "forum", "media", "link"];

const hexToNumber = (hex: unknown) => parseInt(String(hex).replace("#", ""), 16) || 0;

/**
 * Looking after a server for its owner's absence, for Nexa's owner only. Nothing
 * here reads a message or touches a key. Every change takes a reason, is written
 * to both logs under this account, and says first who will hear of it.
 */
export function ManageServerModal({ server, onClose, onChanged }: { server: ModServer; onClose: () => void; onChanged: () => void }) {
  const toast = useUI((u) => u.toast);
  const [view, setView] = useState<ManageView | null>(null);
  const [failed, setFailed] = useState(false);
  const [tab, setTab] = useState<Tab>("overview");
  const [ask, setAsk] = useState<ManageAsk | null>(null);
  const [query, setQuery] = useState("");
  const [members, setMembers] = useState<ManageMember[] | null>(null);
  const changed = useRef(false);

  // opening this writes a line in the log, so it is asked for only here
  const load = useCallback(() => {
    setFailed(false);
    modApi.serverManage(server.id).then(setView, () => setFailed(true));
  }, [server.id]);
  useEffect(() => load(), [load]);

  const search = useCallback(() => {
    modApi.manageMembers(server.id, query.trim()).then((r) => setMembers(r.members), () => setMembers([]));
  }, [server.id, query]);
  useEffect(() => {
    if (tab !== "members") return;
    const timer = setTimeout(search, query ? 250 : 0);
    return () => clearTimeout(timer);
  }, [tab, query, search]);

  const close = () => {
    if (changed.current) onChanged();
    onClose();
  };
  const done = (next: ManageView) => {
    changed.current = true;
    setAsk(null);
    if (next.gone) {
      toast(t("moderation.manage.gone"), "info");
      onChanged();
      onClose();
      return;
    }
    setView(next);
    if (tab === "members") search();
  };

  const title = t("moderation.manage.title", { name: server.name });
  const frame = (content: React.ReactNode) => (
    <Modal open onClose={close} bare width={720} ariaLabel={title}>
      <div className={[m.modal, s.frame].join(" ")} data-testid="manage-server">
        <div className={m.modalTop}>
          <ServerIcon id={server.id} iconId={view?.server?.icon_id ?? server.icon_id} name={server.name} size={40} />
          <div style={{ minWidth: 0 }}>
            <h2 className={m.modalTitle} dir="auto">{t("moderation.manage.title", { name: view?.server?.name ?? server.name })}</h2>
            {view?.owner ? (
              <div className={m.modalTarget}>
                <span>{t("moderation.manage.owner", { name: view.owner.username })}</span>
                {view.owner_gone ? <span className={s.tag} data-tone="danger">{t("moderation.manage.ownerGone")}</span> : null}
              </div>
            ) : null}
          </div>
          <button type="button" className={m.modalClose} onClick={close} aria-label={t("chat.attachments.close")}>
            <X size={20} />
          </button>
        </div>
        {content}
      </div>
      {ask ? <ManageActionModal ask={ask} serverId={server.id} onClose={() => setAsk(null)} onDone={done} /> : null}
    </Modal>
  );

  if (failed) return frame(<div className={s.center}><LoadError message={t("moderation.loadFailed")} onRetry={load} /></div>);
  if (!view || !view.server) return frame(<div className={s.center}><Spinner size={24} label={t("chat.pins.loading")} /></div>);
  return frame(<Body view={view} tab={tab} setTab={setTab} setAsk={setAsk} query={query} setQuery={setQuery} members={members} />);
}

function Body({ view, tab, setTab, setAsk, query, setQuery, members }: {
  view: ManageView;
  tab: Tab;
  setTab: (t: Tab) => void;
  setAsk: (a: ManageAsk) => void;
  query: string;
  setQuery: (q: string) => void;
  members: ManageMember[] | null;
}) {
  const srv = view.server!;
  const ownerName = view.owner ? personName(view.owner) : "";
  const everyoneId = srv.id;
  const seesOwner = view.owner_deleted ? t("moderation.manage.seesNoOwner") : t("moderation.manage.seesOwner", { name: ownerName });
  const sees = (...extra: string[]) => [seesOwner, ...extra];
  const say = (a: ManageAsk) => setAsk(a);

  const categories = useMemo(() => view.channels.filter((c) => c.type === "category").sort((a, b) => a.position - b.position), [view.channels]);
  const categoryOptions: [string, string][] = [["", t("moderation.manage.noCategory")], ...categories.map((c): [string, string] => [c.id, c.name])];
  const roleOrder = useMemo(() => [...view.roles].sort((a, b) => b.position - a.position), [view.roles]);
  const assignable = roleOrder.filter((r) => r.id !== everyoneId && !r.managed_by);

  const label = (key: string) => t(`moderation.manage.${key}`);
  const tabs: [Tab, string][] = [
    ["overview", t("server.channelSettings.overview")],
    ["channels", t("moderation.preview.channels")],
    ["roles", t("moderation.preview.roles")],
    ["members", t("moderation.preview.members")],
    ["bans", t("server.bans.bans")],
    ["invites", t("server.invites.invites")],
    ["apps", t("apps.menu.apps")],
    ["automod", t("automod.title")],
  ];

  const editServer = () =>
    say({
      action: "server_edit",
      sees: sees(),
      confirm: t("server.invites.vanitySave"),
      initial: { name: srv.name, description: srv.description ?? "", system_channel_id: srv.system_channel_id ?? "", default_notifications: String(srv.default_notifications ?? 0), nsfw: !!srv.nsfw },
      fields: [
        { kind: "text", key: "name", label: t("integrations.name"), max: 100, required: true },
        { kind: "area", key: "description", label: t("moderation.servers.description"), max: 1000 },
        { kind: "select", key: "system_channel_id", label: t("server.serverOverview.systemMessagesChannel"), options: [["", t("moderation.manage.none")], ...view.channels.filter((c) => c.type === "text" && !c.private).map((c): [string, string] => [c.id, c.name])] },
        { kind: "select", key: "default_notifications", label: t("server.serverOverview.defaultNotificationSettings"), options: [["0", t("server.serverOverview.allMessages")], ["1", t("server.serverOverview.onlyMentions")]] },
        { kind: "toggle", key: "nsfw", label: label("serverNsfw") },
      ],
      request: (v) => ({ data: { name: String(v.name).trim(), description: String(v.description), system_channel_id: v.system_channel_id || null, default_notifications: Number(v.default_notifications), nsfw: !!v.nsfw } }),
    });
  const plain = (action: string, opts: Partial<ManageAsk> & { request?: ManageAsk["request"] } = {}) =>
    say({ action, sees: sees(), danger: true, request: () => ({}), ...opts });

  const roleFields: ManageField[] = [
    { kind: "text", key: "name", label: t("server.roles.roleName"), max: 100, required: true },
    { kind: "colour", key: "color", label: t("badges.staff.colour") },
    { kind: "toggle", key: "hoist", label: label("hoist") },
    { kind: "toggle", key: "mentionable", label: label("mentionable") },
    { kind: "perms", key: "permissions", label: t("server.channelSettings.permissions") },
  ];
  const roleData = (v: Record<string, unknown>) => ({ name: String(v.name).trim(), color: hexToNumber(v.color), hoist: !!v.hoist, mentionable: !!v.mentionable, permissions: String(v.permissions) });
  const roleName = (r: Role) => (r.id === everyoneId ? "@everyone" : r.name);

  const channelRow = (c: ManageChannel) => {
    const parent = c.parent_id ? view.channels.find((x) => x.id === c.parent_id) : undefined;
    const targets: ManageTarget[] = [
      ...roleOrder.map((r): ManageTarget => {
        const o = c.overwrites.find((x) => x.target_id === r.id);
        return { id: r.id, type: "role", name: roleName(r), allow: o?.allow ?? "0", deny: o?.deny ?? "0" };
      }),
      ...c.overwrites.filter((o) => o.target_type === "member").map((o): ManageTarget => ({ id: o.target_id, type: "member", name: t("moderation.manage.memberEntry", { id: o.target_id }), allow: o.allow, deny: o.deny })),
    ];
    const edit = () =>
      say({
        action: "channel_edit",
        target: c.name,
        sees: sees(),
        confirm: t("server.invites.vanitySave"),
        initial: { name: c.name, topic: c.topic ?? "", nsfw: !!c.nsfw, slowmode_seconds: c.slowmode_seconds ?? 0, user_limit: c.user_limit ?? 0 },
        fields: [
          { kind: "text", key: "name", label: t("modals.createChannel.channelName"), max: 100, required: true },
          ...(!c.private && c.type !== "category" ? [{ kind: "area", key: "topic", label: label("topic"), max: 1024 } as ManageField] : []),
          ...(c.type !== "category" && c.type !== "link" ? [{ kind: "toggle", key: "nsfw", label: label("channelNsfw") } as ManageField] : []),
          ...(["text", "announcement", "forum", "media"].includes(c.type) ? [{ kind: "number", key: "slowmode_seconds", label: label("slowmode"), min: 0, max: 21600 } as ManageField] : []),
          ...(c.type === "voice" ? [{ kind: "number", key: "user_limit", label: label("userLimit"), min: 0, max: 99 } as ManageField] : []),
        ],
        request: (v) => ({
          channel_id: c.id,
          data: {
            name: String(v.name).trim(),
            ...(!c.private && c.type !== "category" ? { topic: String(v.topic) } : {}),
            ...(c.type !== "category" && c.type !== "link" ? { nsfw: !!v.nsfw } : {}),
            ...(["text", "announcement", "forum", "media"].includes(c.type) ? { slowmode_seconds: Number(v.slowmode_seconds) } : {}),
            ...(c.type === "voice" ? { user_limit: Number(v.user_limit) } : {}),
          },
        }),
      });
    return (
      <div key={c.id} className={s.row} data-testid={`manage-channel-${c.id}`}>
        <span className={s.rowLead}>{c.private ? <Lock size={16} aria-hidden="true" /> : <ChannelGlyph channel={c as unknown as Channel} size={18} />}</span>
        <div className={s.rowText}>
          <span className={s.rowName}>
            <bdi>{c.name}</bdi>
            {c.private ? <span className={s.tag}>{label("private")}</span> : null}
            {c.nsfw ? <span className={s.tag}>{t("nsfw.label")}</span> : null}
          </span>
        </div>
        <div className={s.rowActions}>
          <Button size="sm" variant="ghost" onClick={edit}>{label("edit")}</Button>
          {c.type !== "category" ? (
            <Button size="sm" variant="ghost" onClick={() => say({ action: "channel_move", target: c.name, sees: sees(), confirm: label("move"), initial: { parent_id: c.parent_id ?? "", position: c.position }, fields: [{ kind: "select", key: "parent_id", label: t("discovery.settings.category"), options: categoryOptions }, { kind: "number", key: "position", label: label("position"), min: 0, max: 500 }], request: (v) => ({ channel_id: c.id, data: { parent_id: v.parent_id || null, position: Number(v.position) } }) })}>
              {label("move")}
            </Button>
          ) : null}
          <Button size="sm" variant="ghost" data-testid={`manage-perms-${c.id}`} onClick={() => say({ action: "overwrite_set", target: c.name, sees: sees(), confirm: t("server.invites.vanitySave"), fields: [{ kind: "overwrite", channelType: c.type, targets }], request: (v) => { const who = targets.find((x) => x.id === v.target_id)!; return v.clear ? { action: "overwrite_remove", channel_id: c.id, target_id: who.id } : { channel_id: c.id, target_id: who.id, data: { type: who.type, allow: String(v.allow), deny: String(v.deny) } }; } })}>
            {t("server.channelSettings.permissions")}
          </Button>
          {parent && !sameOverwrites(c.overwrites, parent.overwrites) ? (
            <Button size="sm" variant="ghost" onClick={() => say({ action: "overwrite_sync", target: c.name, sees: sees(), request: () => ({ channel_id: c.id }) })}>{label("sync")}</Button>
          ) : null}
          <Button size="sm" variant="ghost" className={s.destroy} onClick={() => plain("channel_delete", { target: c.name, request: () => ({ channel_id: c.id }) })}>{label("delete")}</Button>
        </div>
      </div>
    );
  };

  const channelList = () => {
    const under = (id: string | null) => view.channels.filter((c) => c.type !== "category" && (c.parent_id ?? null) === id).sort((a, b) => a.position - b.position);
    const newChannel = () =>
      say({
        action: "channel_create",
        sees: sees(),
        initial: { type: "text", name: "", parent_id: "", url: "" },
        fields: [
          { kind: "select", key: "type", label: label("channelType"), options: CHANNEL_KINDS.map((k): [string, string] => [k, k === "category" ? t("discovery.settings.category") : t(`modals.createChannel.${k}`)]) },
          { kind: "text", key: "name", label: t("modals.createChannel.channelName"), max: 100, required: true },
          { kind: "select", key: "parent_id", label: t("discovery.settings.category"), options: categoryOptions },
          { kind: "text", key: "url", label: label("linkAddress"), max: 1024, when: (v) => v.type === "link" },
        ],
        request: (v) => ({ data: { type: v.type, name: String(v.name).trim(), ...(v.parent_id && v.type !== "category" ? { parent_id: v.parent_id } : {}), ...(v.type === "link" ? { url: String(v.url).trim() } : {}) } }),
      });
    return (
      <>
        <div className={s.head}>
          <h3 className={s.headTitle}>{t("moderation.preview.channels")}</h3>
          <div className={s.headActions}><Button size="sm" variant="secondary" onClick={newChannel} data-testid="manage-new-channel">{t("modals.createChannel.createChannel")}</Button></div>
        </div>
        <div className={s.list}>
          {view.channels.length === 0 ? <div className={s.empty}>{label("noChannels")}</div> : null}
          {under(null).map(channelRow)}
          {categories.map((cat) => (
            <div key={cat.id}>
              {channelRow(cat)}
              {under(cat.id).map(channelRow)}
            </div>
          ))}
        </div>
      </>
    );
  };

  const roleList = () => (
    <>
      <div className={s.head}>
        <h3 className={s.headTitle}>{t("moderation.preview.roles")}</h3>
        <div className={s.headActions}>
          <Button size="sm" variant="secondary" data-testid="manage-new-role" onClick={() => say({ action: "role_create", sees: sees(), initial: { name: "", color: "#99aab5", hoist: false, mentionable: false, permissions: "0" }, fields: roleFields, request: (v) => ({ data: roleData(v) }) })}>
            {t("server.roles.createRole")}
          </Button>
        </div>
      </div>
      <div className={s.list}>
        {roleOrder.map((r) => (
          <div key={r.id} className={s.row} data-testid={`manage-role-${r.id}`}>
            <span className={s.rowLead}><i className={s.dot} style={{ "--dot": r.color ? hexColour(r.color) : "var(--text-muted)" } as React.CSSProperties} /></span>
            <div className={s.rowText}><span className={s.rowName}><bdi>{roleName(r)}</bdi></span></div>
            <div className={s.rowActions}>
              <Button size="sm" variant="ghost" onClick={() => say({ action: "role_edit", target: roleName(r), sees: sees(), confirm: t("server.invites.vanitySave"), initial: { name: r.name, color: r.color ? hexColour(r.color) : "#000000", hoist: r.hoist, mentionable: r.mentionable, permissions: r.permissions }, fields: r.id === everyoneId ? roleFields.filter((f) => f.kind === "perms") : roleFields, request: (v) => ({ role_id: r.id, data: r.id === everyoneId ? { permissions: String(v.permissions) } : roleData(v) }) })}>
                {label("edit")}
              </Button>
              {r.id !== everyoneId && !r.managed_by ? <Button size="sm" variant="ghost" className={s.destroy} onClick={() => plain("role_delete", { target: r.name, request: () => ({ role_id: r.id }) })}>{label("delete")}</Button> : null}
            </div>
          </div>
        ))}
      </div>
    </>
  );

  const memberRow = (x: ManageMember) => {
    const name = personName(x.user);
    const mine = assignable.filter((r) => x.role_ids.includes(r.id));
    const others = assignable.filter((r) => !x.role_ids.includes(r.id));
    const isOwner = x.user.id === view.owner?.id;
    const isMe = x.user.id === useAuth.getState().me?.id;
    const pick = (action: string, list: Role[]) =>
      say({ action, target: name, sees: sees(), initial: { role_id: list[0]?.id ?? "" }, fields: [{ kind: "select", key: "role_id", label: label("role"), options: list.map((r): [string, string] => [r.id, r.name]) }], request: (v) => ({ user_id: x.user.id, role_id: String(v.role_id) }) });
    return (
      <div key={x.user.id} className={s.row} data-testid={`manage-member-${x.user.username}`}>
        <Avatar id={x.user.id} avatarId={x.user.avatar_id ?? null} name={name} size={32} />
        <div className={s.rowText} style={{ flexBasis: undefined }}>
          <span className={s.rowName}><bdi>{name}</bdi>{isOwner ? <span className={s.tag}>{label("ownerTag")}</span> : null}</span>
          <span className={s.rowSub}>@{x.user.username}{mine.length ? ` · ${mine.map((r) => r.name).join(", ")}` : ""}</span>
        </div>
        {isOwner || isMe ? null : (
          <div className={s.rowActions} data-flush="">
            {others.length ? <Button size="sm" variant="ghost" onClick={() => pick("member_role_add", others)}>{label("giveRole")}</Button> : null}
            {mine.length ? <Button size="sm" variant="ghost" onClick={() => pick("member_role_remove", mine)}>{label("takeRole")}</Button> : null}
            <Button size="sm" variant="ghost" className={s.destroy} onClick={() => plain("member_remove", { target: name, sees: sees(t("moderation.manage.seesRemoved", { name })), request: () => ({ user_id: x.user.id }) })}>{label("remove")}</Button>
            <Button size="sm" variant="ghost" className={s.destroy} onClick={() => plain("member_ban", { target: name, sees: sees(t("moderation.manage.seesBanned", { name })), request: () => ({ user_id: x.user.id }) })}>{label("ban")}</Button>
            {view.owner_gone ? <Button size="sm" variant="secondary" data-testid={`manage-transfer-${x.user.username}`} onClick={() => say({ action: "transfer", target: name, sees: [...(view.owner_deleted ? [] : [seesOwner]), t("moderation.manage.seesTransfer", { name })], danger: true, request: () => ({ user_id: x.user.id }) })}>{label("makeOwner")}</Button> : null}
          </div>
        )}
      </div>
    );
  };

  const ruleRow = (r: ManageRule) => {
    const toggle = () => say({ action: "automod_edit", target: r.name, sees: sees(), confirm: label(r.enabled ? "turnOff" : "turnOn"), request: () => ({ rule_id: r.id, data: { enabled: !r.enabled } }) });
    return (
      <div key={r.id} className={s.row} data-testid={`manage-rule-${r.id}`}>
        <div className={s.rowText} style={{ marginInlineStart: 2 }}>
          <span className={s.rowName}>
            <bdi>{r.name}</bdi>
            {!r.enabled ? <span className={s.tag}>{label("off")}</span> : null}
            {r.alerts ? <span className={s.tag}>{label("postsCaught")}</span> : null}
          </span>
          <span className={s.rowSub}>{t(`automod.kind.${r.kind}`)}</span>
        </div>
        <div className={s.rowActions} data-flush="">
          {!r.alerts ? (
            <Button size="sm" variant="ghost" onClick={() => say({ action: "automod_edit", target: r.name, sees: sees(), confirm: t("server.invites.vanitySave"), initial: { name: r.name, words: (r.words ?? []).join("\n") }, fields: [{ kind: "text", key: "name", label: t("integrations.name"), max: 100, required: true }, ...(r.kind === "words" ? [{ kind: "area", key: "words", label: label("ruleWords"), max: 20000 } as ManageField] : [])], request: (v) => ({ rule_id: r.id, data: { name: String(v.name).trim(), ...(r.kind === "words" ? { config: { words: String(v.words).split("\n").map((w) => w.trim()).filter(Boolean) } } : {}) } }) })}>
              {label("edit")}
            </Button>
          ) : null}
          {r.enabled || !r.alerts ? <Button size="sm" variant="ghost" onClick={toggle}>{label(r.enabled ? "turnOff" : "turnOn")}</Button> : null}
          <Button size="sm" variant="ghost" className={s.destroy} onClick={() => plain("automod_delete", { target: r.name, request: () => ({ rule_id: r.id }) })}>{label("delete")}</Button>
        </div>
      </div>
    );
  };

  const paused = !!srv.invites_paused_until && srv.invites_paused_until > Date.now();
  const empty = (key: string) => <div className={s.empty}>{label(key)}</div>;

  return (
    <>
      <div className={s.tabs} role="tablist" aria-label={t("moderation.manage.open")}>
        {tabs.map(([id, text]) => (
          <button key={id} type="button" role="tab" aria-selected={tab === id} data-selected={tab === id} className={m.chip} onClick={() => setTab(id)} data-testid={`manage-tab-${id}`}>
            {text}
          </button>
        ))}
      </div>
      <div className={s.body} role="tabpanel">
        {tab === "overview" ? (
          <>
            <p className={m.note}>{label("note")}</p>
            <div className={s.head}>
              <h3 className={s.headTitle}>{t("server.channelSettings.overview")}</h3>
              <div className={s.headActions}><Button size="sm" variant="secondary" onClick={editServer} data-testid="manage-edit-server">{t("moderation.manage.a.server_edit")}</Button></div>
            </div>
            <div className={s.list}>
              <div className={s.row}><div className={s.rowText}><span className={s.rowSub}>{t("integrations.name")}</span><span className={s.rowName}><bdi>{srv.name}</bdi></span></div></div>
              <div className={s.row}><div className={s.rowText}><span className={s.rowSub}>{t("moderation.servers.description")}</span><span className={s.rowName} style={{ fontWeight: 500 }}><bdi>{srv.description || label("none")}</bdi></span></div></div>
              {srv.icon_id ? <div className={s.row}><div className={s.rowText}><span className={s.rowName}>{t("moderation.manage.iconLabel")}</span></div><div className={s.rowActions} data-flush=""><Button size="sm" variant="ghost" className={s.destroy} onClick={() => plain("server_icon_clear")}>{label("remove")}</Button></div></div> : null}
              {srv.banner_id ? <div className={s.row}><div className={s.rowText}><span className={s.rowName}>{t("moderation.manage.bannerLabel")}</span></div><div className={s.rowActions} data-flush=""><Button size="sm" variant="ghost" className={s.destroy} onClick={() => plain("server_banner_clear")}>{label("remove")}</Button></div></div> : null}
            </div>
            <p className={s.hint}>{view.owner_gone ? label("transferHere") : label("transferHint")}</p>
          </>
        ) : null}
        {tab === "channels" ? channelList() : null}
        {tab === "roles" ? roleList() : null}
        {tab === "members" ? (
          <>
            <Input value={query} onChange={(e) => setQuery(e.target.value)} placeholder={t("server.channelSettings.searchMembers")} aria-label={t("server.channelSettings.searchMembers")} data-testid="manage-member-search" />
            <div className={s.list}>
              {members === null ? <div className={s.center} style={{ padding: 18 }}><Spinner size={22} label={t("chat.pins.loading")} /></div> : members.length === 0 ? empty("noMembers") : members.map(memberRow)}
            </div>
          </>
        ) : null}
        {tab === "bans" ? (
          <div className={s.list}>
            {view.bans.length === 0 ? empty("noBans") : view.bans.map((b) => {
              const name = personName(b.user);
              return (
                <div key={b.user.id} className={s.row}>
                  <Avatar id={b.user.id} avatarId={b.user.avatar_id ?? null} name={name} size={32} />
                  <div className={s.rowText}><span className={s.rowName}><bdi>{name}</bdi></span>{b.reason ? <span className={s.rowSub} dir="auto">{b.reason}</span> : null}</div>
                  <div className={s.rowActions} data-flush=""><Button size="sm" variant="ghost" onClick={() => say({ action: "member_unban", target: name, sees: sees(), request: () => ({ user_id: b.user.id }) })}>{label("unban")}</Button></div>
                </div>
              );
            })}
          </div>
        ) : null}
        {tab === "invites" ? (
          <>
            <div className={s.head}>
              <h3 className={s.headTitle}>{t("server.invites.invites")}</h3>
              <div className={s.headActions}>
                <Button size="sm" variant="ghost" onClick={() => say({ action: "server_edit", target: label(paused ? "resumeInvites" : "pauseInvites"), confirm: label(paused ? "resumeInvites" : "pauseInvites"), sees: sees(), request: () => ({ data: { invites_paused_until: paused ? null : Date.now() + DAY } }) })}>{label(paused ? "resumeInvites" : "pauseInvites")}</Button>
                {view.invites.length ? <Button size="sm" variant="ghost" className={s.destroy} onClick={() => plain("invites_revoke_all")}>{t("server.invites.revokeAll")}</Button> : null}
              </div>
            </div>
            <div className={s.list}>
              {view.vanity ? <div className={s.row}><div className={s.rowText}><span className={s.rowName}><code>{view.vanity}</code><span className={s.tag}>{label("vanity")}</span></span></div><div className={s.rowActions} data-flush=""><Button size="sm" variant="ghost" className={s.destroy} onClick={() => plain("vanity_clear", { target: view.vanity })}>{label("remove")}</Button></div></div> : null}
              {view.invites.map((i) => (
                <div key={i.code} className={s.row}>
                  <div className={s.rowText}><span className={s.rowName}><code>{i.code}</code></span><span className={s.rowSub}>{t("moderation.manage.uses", { count: i.uses })}</span></div>
                  <div className={s.rowActions} data-flush=""><Button size="sm" variant="ghost" className={s.destroy} onClick={() => plain("invite_revoke", { target: i.code, request: () => ({ code: i.code }) })}>{label("revoke")}</Button></div>
                </div>
              ))}
              {!view.vanity && view.invites.length === 0 ? empty("noInvites") : null}
            </div>
          </>
        ) : null}
        {tab === "apps" ? (
          <>
            <h3 className={s.headTitle}>{t("apps.menu.apps")}</h3>
            <div className={s.list}>
              {view.apps.length === 0 ? empty("noApps") : view.apps.map((a) => (
                <div key={a.id} className={s.row}>
                  <span className={s.rowLead}><Plug size={16} aria-hidden="true" /></span>
                  <div className={s.rowText}><span className={s.rowName}><bdi>{a.name}</bdi>{a.reads_all ? <span className={s.tag}>{label("readsAll")}</span> : null}</span></div>
                  <div className={s.rowActions} data-flush=""><Button size="sm" variant="ghost" className={s.destroy} onClick={() => plain("app_remove", { target: a.name, request: () => ({ app_id: a.id }) })}>{label("remove")}</Button></div>
                </div>
              ))}
            </div>
            <h3 className={s.headTitle}>{t("integrations.webhooks")}</h3>
            <div className={s.list}>
              {view.webhooks.length === 0 ? empty("noWebhooks") : view.webhooks.map((w) => (
                <div key={w.id} className={s.row}>
                  <span className={s.rowLead}><Webhook size={16} aria-hidden="true" /></span>
                  <div className={s.rowText}><span className={s.rowName}><bdi>{w.name}</bdi></span><span className={s.rowSub}>{view.channels.find((c) => c.id === w.channel_id)?.name ?? ""}</span></div>
                  <div className={s.rowActions} data-flush=""><Button size="sm" variant="ghost" className={s.destroy} onClick={() => plain("webhook_delete", { target: w.name, request: () => ({ webhook_id: w.id }) })}>{label("remove")}</Button></div>
                </div>
              ))}
            </div>
          </>
        ) : null}
        {tab === "automod" ? (
          <>
            <div className={s.head}>
              <h3 className={s.headTitle}>{t("automod.title")}</h3>
              <div className={s.headActions}>
                <Button size="sm" variant="secondary" data-testid="manage-new-rule" onClick={() => say({ action: "automod_create", sees: sees(), initial: { name: "", words: "" }, fields: [{ kind: "text", key: "name", label: t("integrations.name"), max: 100, required: true }, { kind: "area", key: "words", label: label("ruleWords"), max: 20000, required: true }], request: (v) => ({ data: { name: String(v.name).trim(), kind: "words", config: { words: String(v.words).split("\n").map((w) => w.trim()).filter(Boolean) }, actions: [{ type: "block" }] } }) })}>
                  {t("moderation.manage.a.automod_create")}
                </Button>
              </div>
            </div>
            <div className={s.list}>{view.automod.length === 0 ? empty("noRules") : view.automod.map(ruleRow)}</div>
          </>
        ) : null}
      </div>
    </>
  );
}

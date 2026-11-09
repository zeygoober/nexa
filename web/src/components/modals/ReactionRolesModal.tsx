import { Plus, SmilePlus, X } from "lucide-react";
import { useMemo, useState } from "react";
import { messages as messagesApi } from "@/api/messages";
import { emojiUrl } from "@/api/users";
import { Button } from "@/components/common/Button";
import { Modal } from "@/components/common/Modal";
import { Select } from "@/components/common/Select";
import { Twemoji } from "@/components/common/Twemoji";
import { customReaction, reactionKeyOf } from "@/components/chat/Reactions";
import { ExpressionPicker } from "@/components/pickers/ExpressionPicker";
import { t } from "@/i18n";
import { describeError } from "@/lib/errors";
import { has, P, REACTION_ROLE_UNSAFE } from "@/lib/perms";
import { useAuth } from "@/stores/auth";
import { useChannels } from "@/stores/channels";
import { useMessages } from "@/stores/messages";
import { memberHighest, rolesByPosition, useServers } from "@/stores/servers";
import { useUI } from "@/stores/ui";
import s from "./ReactionRolesModal.module.css";

function Glyph({ emoji }: { emoji: string }) {
  const custom = customReaction(emoji);
  return custom ? <img className={s.glyph} src={emojiUrl(custom.id, custom.animated, 48)} alt={custom.name} /> : <Twemoji text={emoji} className={s.glyph} />;
}

/** What each reaction on one message gives, for those who manage roles. */
export function ReactionRolesModal({ channelId, messageId, onClose }: { channelId: string; messageId: string; onClose: () => void }) {
  const msg = useMessages((m) => m.channels[channelId]?.byId[messageId]);
  const serverId = useChannels((c) => c.byId[channelId]?.server_id) ?? undefined;
  const server = useServers((x) => (serverId ? x.byId[serverId] : undefined));
  const meId = useAuth((a) => a.me?.id);
  const [emoji, setEmoji] = useState<string | null>(null);
  const [roleId, setRoleId] = useState("");
  const [pickerAt, setPickerAt] = useState<DOMRect | null>(null);
  const [busy, setBusy] = useState(false);

  const offered = useMemo(() => {
    if (!server || !meId) return [];
    const { highest, isOwner } = memberHighest(server, meId);
    return rolesByPosition(server).filter((r) => r.id !== server.server.id && !r.managed_by && (BigInt(r.permissions) & REACTION_ROLE_UNSAFE) === 0n && (isOwner || r.position < highest));
  }, [server, meId]);

  if (!msg || !server) return null;
  const list = msg.reaction_roles ?? [];

  const run = async (fn: () => Promise<unknown>) => {
    setBusy(true);
    try {
      await fn();
      return true;
    } catch (err) {
      useUI.getState().toast(describeError(err), "error");
      return false;
    } finally {
      setBusy(false);
    }
  };
  const add = async () => {
    if (!emoji || !roleId) return;
    if (await run(() => messagesApi.setReactionRole(channelId, messageId, emoji, roleId))) {
      setEmoji(null);
      setRoleId("");
    }
  };
  return (
    <Modal open onClose={onClose} title={t("chat.reactionRoles.title")} width={440}>
      <p className={s.hint}>{t("chat.reactionRoles.hint")}</p>
      {list.length === 0 ? (
        <p className={s.empty} data-testid="reaction-roles-empty">{t("chat.reactionRoles.empty")}</p>
      ) : (
        <ul className={s.list}>
          {list.map((rr) => {
            const role = server.roles[rr.role_id];
            if (!role) return null;
            return (
              <li key={rr.emoji} className={s.row} data-testid="reaction-role-row">
                <Glyph emoji={rr.emoji} />
                {role.color ? <span className={s.dot} style={{ background: `#${role.color.toString(16).padStart(6, "0")}` }} aria-hidden="true" /> : null}
                <span className={s.role}>{role.name}</span>
                <button type="button" className={s.remove} aria-label={t("chat.reactionRoles.remove", { role: role.name })} disabled={busy} onClick={() => void run(() => messagesApi.removeReactionRole(channelId, messageId, rr.emoji))} data-testid="reaction-role-remove"><X size={16} aria-hidden="true" /></button>
              </li>
            );
          })}
        </ul>
      )}
      {offered.length === 0 ? (
        <p className={s.hint}>{t("chat.reactionRoles.noRoles")}</p>
      ) : (
        <div className={s.add}>
          <button type="button" className={s.choose} aria-label={t("chat.reactionRoles.chooseEmoji")} onClick={(e) => setPickerAt(e.currentTarget.getBoundingClientRect())} data-testid="reaction-role-emoji">
            {emoji ? <Glyph emoji={emoji} /> : <SmilePlus size={20} aria-hidden="true" />}
          </button>
          <Select className={s.select} value={roleId} onChange={(e) => setRoleId(e.target.value)} aria-label={t("chat.reactionRoles.chooseRole")} data-testid="reaction-role-select">
            <option value="">{t("chat.reactionRoles.chooseRole")}</option>
            {offered.map((r) => <option key={r.id} value={r.id}>{r.name}</option>)}
          </Select>
          <Button type="button" loading={busy} disabled={!emoji || !roleId} onClick={() => void add()} data-testid="reaction-role-add"><Plus size={16} aria-hidden="true" /> {t("chat.reactionRoles.add")}</Button>
        </div>
      )}
      {pickerAt ? (
        <ExpressionPicker
          title={t("chat.reactionRoles.chooseEmoji")}
          anchor={pickerAt}
          serverId={serverId}
          onClose={() => setPickerAt(null)}
          onEmoji={(e) => { setEmoji(reactionKeyOf(e)); setPickerAt(null); }}
        />
      ) : null}
    </Modal>
  );
}

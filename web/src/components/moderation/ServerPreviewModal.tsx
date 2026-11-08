import { Lock, X } from "lucide-react";
import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { moderation as modApi, type ModServer, type ModServerPreview } from "@/api/moderation";
import type { Channel } from "@/api/types";
import { Avatar } from "@/components/common/Avatar";
import { ChannelGlyph } from "@/components/common/ChannelGlyph";
import { Modal } from "@/components/common/Modal";
import { Spinner } from "@/components/common/Spinner";
import { hexColour } from "@/lib/badges";
import { describeError } from "@/lib/errors";
import { personName } from "@/lib/moderation";
import { t } from "@/i18n";
import { ServerIcon } from "./ProfileSheet";
import m from "./Moderation.module.css";
import b from "./BadgesTab.module.css";

/**
 * A server's layout for the owner, without joining: the channels a new member
 * could see, its roles and its first members. Asking for it writes a line in
 * the log, so it is asked for only when this dialog opens, never with the page.
 */
export function ServerPreviewModal({ server, onClose }: { server: ModServer; onClose: () => void }) {
  const nav = useNavigate();
  const [data, setData] = useState<ModServerPreview | null>(null);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    let alive = true;
    modApi
      .serverPreview(server.id)
      .then((r) => alive && setData(r))
      .catch((err) => alive && setError(describeError(err, t("moderation.loadFailed"))));
    return () => {
      alive = false;
    };
  }, [server.id]);
  const title = t("moderation.preview.title", { name: server.name });
  return (
    <Modal open onClose={onClose} bare width={560} ariaLabel={title}>
      <div className={m.modal} data-testid="server-preview-modal">
        <div className={m.modalTop}>
          <ServerIcon id={server.id} iconId={server.icon_id} name={server.name} size={40} />
          <div style={{ minWidth: 0 }}>
            <h2 className={m.modalTitle} dir="auto">{title}</h2>
            <div className={m.modalTarget}>
              <span>{t("moderation.servers.members", { count: server.member_count })}</span>
            </div>
          </div>
          <button type="button" className={m.modalClose} onClick={onClose} aria-label={t("chat.attachments.close")}>
            <X size={20} />
          </button>
        </div>
        <div className={m.modalScroll}>
          <p className={m.note}>{t("moderation.preview.note")}</p>
          {error ? <p className={m.danger} role="alert">{error}</p> : null}
          {!data && !error ? (
            <div style={{ padding: 16, display: "grid", placeItems: "center" }}>
              <Spinner size={24} label={t("chat.pins.loading")} />
            </div>
          ) : null}
          {data ? (
            <>
              <h3 className={b.listTitle}>{t("moderation.preview.channels")}</h3>
              <div className={b.previewList}>
                {data.channels.map((c) =>
                  c.type === "category" ? (
                    <div key={c.id} className={b.previewCategory} dir="auto">{c.name}</div>
                  ) : (
                    <div key={c.id} className={b.previewChannel}>
                      <ChannelGlyph channel={c as unknown as Channel} size={18} />
                      <span dir="auto">{c.name}</span>
                    </div>
                  ),
                )}
                {data.channels.length === 0 ? <div className={[b.previewChannel, b.previewPrivate].join(" ")}><span>{t("moderation.preview.noChannels")}</span></div> : null}
                {data.private_channels > 0 ? (
                  <div className={[b.previewChannel, b.previewPrivate].join(" ")}>
                    <Lock size={16} aria-hidden="true" />
                    <span>{t("moderation.preview.private", { count: data.private_channels })}</span>
                  </div>
                ) : null}
              </div>
              {data.roles.length > 0 ? (
                <>
                  <h3 className={b.listTitle}>{t("moderation.preview.roles")}</h3>
                  <ul className={b.previewRoles}>
                    {data.roles.map((r) => (
                      <li key={r.id} className={b.previewRole}>
                        <i className={b.previewDot} style={{ "--dot": r.color ? hexColour(r.color) : "var(--text-muted)" } as React.CSSProperties} />
                        <span dir="auto">{r.name}</span>
                      </li>
                    ))}
                  </ul>
                </>
              ) : null}
              <h3 className={b.listTitle}>{t("moderation.preview.members")}</h3>
              {data.member_count > data.members.length ? <p className={b.previewHint}>{t("moderation.preview.firstMembers", { shown: data.members.length, total: data.member_count })}</p> : null}
              <div className={b.previewList}>
                {data.members.map((x) => {
                  const who = data.people[x.user_id];
                  const name = x.nickname || personName(who);
                  return (
                    <button key={x.user_id} type="button" className={b.previewMember} onClick={() => nav(`/channels/@admin/users/${x.user_id}`)}>
                      <Avatar id={x.user_id} avatarId={who?.avatar_id ?? null} name={name} size={32} />
                      <span className={b.recordText}>
                        <span className={b.previewMemberName} dir="auto">{name}</span>
                        <span>@{who?.username ?? ""}</span>
                      </span>
                    </button>
                  );
                })}
              </div>
            </>
          ) : null}
        </div>
      </div>
    </Modal>
  );
}

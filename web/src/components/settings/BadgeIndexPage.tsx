import { useEffect, useState } from "react";
import type { BadgeIndex } from "@/api/types";
import { users } from "@/api/users";
import { BadgeMark, BadgePill } from "@/components/badges/Badges";
import { Button } from "@/components/common/Button";
import { Spinner } from "@/components/common/Spinner";
import { t } from "@/i18n";
import { badgeArt } from "@/lib/badges";
import { can, useMyRank } from "@/lib/staffRank";
import { useAuth } from "@/stores/auth";
import chat from "@/components/chat/Chat.module.css";
import BadgeArrange from "./BadgeArrange";
import st from "./Settings.module.css";
import s from "./BadgeIndexPage.module.css";

/** A badge not collected: its shape, blacked out, and nothing that says what it is or how to get it. */
function Locked({ entry }: { entry: BadgeIndex["entries"][number] }) {
  return (
    <span className={s.tile} data-locked="" role="img" aria-label={t("badges.index.locked")} data-testid="badge-index-locked">
      {entry.art ? <img className={s.art} src={badgeArt(`${entry.art}-locked`)} alt="" draggable={false} /> : <span className={s.shape}><BadgeMark look={{ iconName: entry.icon, image: entry.image, label: "", tip: "" }} size={40} /></span>}
    </span>
  );
}

/** Settings, Badge Index: every badge there is, each level its own, the ones this account lacks blacked out. Staff ranks are not in it. */
export default function BadgeIndexPage() {
  const me = useAuth((a) => a.me);
  const [index, setIndex] = useState<BadgeIndex | null>(null);
  const [failed, setFailed] = useState(false);
  const [arranging, setArranging] = useState(false);
  const mine = useMyRank();
  const load = () => {
    setFailed(false);
    users.badgeIndex().then(setIndex, () => setFailed(true));
  };
  useEffect(load, []);
  return (
    <>
      <h2 className={st.title}>{t("badges.index.title")}</h2>
      {failed ? (
        <p className={s.note}>
          {t("badges.index.failed")}{" "}
          <button type="button" className={chat.loadRetry} onClick={load}>{t("chat.messageList.tryAgain")}</button>
        </p>
      ) : !index || !me ? (
        !failed && <div className={s.loading}><Spinner size={24} label={t("chat.pins.loading")} /></div>
      ) : arranging ? (
        <BadgeArrange
          me={me}
          onDone={(saved) => {
            setArranging(false);
            if (saved) load();
          }}
        />
      ) : (
        <>
          <div className={s.summary} data-testid="badge-index-summary">
            <div className={s.countRow}>
              <p className={s.count}>
                <strong>{index.collected}</strong> <span>{t("badges.index.of", { total: index.total })}</span> <span>{t("badges.index.collected")}</span>
              </p>
              {can(mine, "ownerTools") ? <Button variant="secondary" size="sm" onClick={() => setArranging(true)} data-testid="badge-index-arrange">{t("badges.index.arrange")}</Button> : null}
            </div>
            <div className={s.bar} aria-hidden="true"><div className={s.fill} style={{ width: `${index.total ? (index.collected / index.total) * 100 : 0}%` }} /></div>
          </div>
          <ul className={s.grid} data-testid="badge-index-grid">
            {index.entries.map((e, i) => (
              <li key={i}>
                {e.badge ? (
                  <span className={s.tile} data-testid="badge-index-collected"><BadgePill badge={e.badge} user={me} size={40} /></span>
                ) : <Locked entry={e} />}
              </li>
            ))}
          </ul>
        </>
      )}
    </>
  );
}

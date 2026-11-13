import { useEffect, useRef, useState } from "react";
import type { BadgeCategory, BadgeIndex } from "@/api/types";
import { users } from "@/api/users";
import { BadgeMark, BadgePill } from "@/components/badges/Badges";
import { Button } from "@/components/common/Button";
import { Spinner } from "@/components/common/Spinner";
import { t } from "@/i18n";
import { badgeArt } from "@/lib/badges";
import { CURIOUS_HOLD_MS, reportFound } from "@/lib/secretBadges";
import { can, useMyRank } from "@/lib/staffRank";
import { useAuth } from "@/stores/auth";
import chat from "@/components/chat/Chat.module.css";
import BadgeArrange from "./BadgeArrange";
import BadgeDescriptions from "./BadgeDescriptions";
import st from "./Settings.module.css";
import s from "./BadgeIndexPage.module.css";

/** The parts of the index, always in this order (BadgeArrange too). */
export const BADGE_SECTIONS: BadgeCategory[] = ["obtainable", "exclusive", "special"];

/** A badge not collected: its shape, blacked out, and nothing that says what it is or how to get it. */
function Locked({ entry, onFound }: { entry: BadgeIndex["entries"][number]; onFound: () => void }) {
  const held = useRef<number | undefined>(undefined);
  const letGo = () => window.clearTimeout(held.current);
  useEffect(() => letGo, []);
  // a padlock held still long enough is Curious; nothing shows while it is held
  const hold =
    entry.art === "secret"
      ? {
          onPointerDown: () => {
            letGo();
            held.current = window.setTimeout(() => {
              void reportFound("curious").then((isNew) => isNew && onFound());
            }, CURIOUS_HOLD_MS);
          },
          onPointerUp: letGo,
          onPointerLeave: letGo,
          onPointerCancel: letGo,
          onContextMenu: (e: React.MouseEvent) => e.preventDefault(),
        }
      : {};
  return (
    <span className={s.tile} data-locked="" role="img" aria-label={t("badges.index.locked")} data-testid="badge-index-locked" {...hold}>
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
  const [describing, setDescribing] = useState(false);
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
      ) : describing ? (
        <BadgeDescriptions
          me={me}
          onDone={() => {
            setDescribing(false);
            load();
          }}
        />
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
              {can(mine, "ownerTools") ? (
                <span className={s.ownerTools}>
                  <Button variant="secondary" size="sm" onClick={() => setDescribing(true)} data-testid="badge-index-describe">{t("badges.index.describe")}</Button>
                  <Button variant="secondary" size="sm" onClick={() => setArranging(true)} data-testid="badge-index-arrange">{t("badges.index.arrange")}</Button>
                </span>
              ) : null}
            </div>
            <div className={s.bar} aria-hidden="true"><div className={s.fill} style={{ width: `${index.total ? (index.collected / index.total) * 100 : 0}%` }} /></div>
          </div>
          {BADGE_SECTIONS.map((section) => {
            const entries = index.entries.filter((e) => e.category === section);
            // Special starts out empty and still says it is there
            if (!entries.length && section !== "special") return null;
            return (
              <section key={section} className={s.section} data-testid={`badge-index-${section}`}>
                <h3 className={`${st.h5} ${s.sectionTitle}`}>{t(`badges.index.${section}`)}</h3>
                {entries.length ? (
                  <ul className={s.grid} data-testid="badge-index-grid">
                    {entries.map((e, i) => (
                      <li key={i}>
                        {e.badge ? (
                          <span className={s.tile} data-testid="badge-index-collected">
                            <BadgePill badge={e.badge} user={me} size={40} />
                            {e.qty && e.qty > 1 ? <span className={s.qty} role="img" aria-label={t("badges.index.copies", { count: e.qty })} data-testid="badge-index-qty">×{e.qty}</span> : null}
                          </span>
                        ) : <Locked entry={e} onFound={load} />}
                      </li>
                    ))}
                  </ul>
                ) : <p className={s.note}>{t("badges.index.specialEmpty")}</p>}
              </section>
            );
          })}
        </>
      )}
    </>
  );
}

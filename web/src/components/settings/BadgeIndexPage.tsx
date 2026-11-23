import { useEffect, useState } from "react";
import type { BadgeCategory, BadgeIndex, PublicUser } from "@/api/types";
import { users } from "@/api/users";
import { BadgeCard, BadgeMark, BadgePill } from "@/components/badges/Badges";
import { Tooltip } from "@/components/common/Tooltip";
import { Button } from "@/components/common/Button";
import { Spinner } from "@/components/common/Spinner";
import { t } from "@/i18n";
import { badgeArt, badgeLook } from "@/lib/badges";
import { can, useMyRank } from "@/lib/staffRank";
import { useAuth } from "@/stores/auth";
import chat from "@/components/chat/Chat.module.css";
import BadgeArrange from "./BadgeArrange";
import BadgeDescriptions from "./BadgeDescriptions";
import st from "./Settings.module.css";
import s from "./BadgeIndexPage.module.css";

/** The parts of the index, always in this order (BadgeArrange too). */
export const BADGE_SECTIONS: BadgeCategory[] = ["obtainable", "collectible", "exclusive"];

type Entry = BadgeIndex["entries"][number];
type Who = Pick<PublicUser, "id" | "flags" | "plan">;

/** A badge not collected, blacked out: its picture's -locked file, or a badge staff made in the same solid shape. */
function LockedShape({ entry, size }: { entry: Entry; size: number }) {
  return entry.art ? (
    <img className={s.art} style={{ width: size, height: size }} src={badgeArt(`${entry.art}-locked`)} alt="" draggable={false} />
  ) : (
    <span className={s.shape} style={{ width: size, height: size }}>
      <BadgeMark look={{ iconName: entry.icon, image: entry.image, label: "", tip: "" }} size={size} />
    </span>
  );
}

/**
 * A badge not collected: its shape, blacked out. Hovered, focused or tapped it
 * opens the same card a collected one does, still black, with its name and how
 * to get it. A server from before previews sends neither, and the tile says only that it is locked.
 */
function Locked({ entry, me }: { entry: Entry; me: Who }) {
  const p = entry.preview;
  // a plan badge is drawn from the account's plan; here the plan is the one the tile is
  const look = p ? badgeLook(p, p.id === "plan" ? { ...me, plan: entry.art === "plan-pro" ? "pro" : "supporter" } : me) : null;
  if (!p || !look) {
    return (
      <span className={s.tile} data-locked="" role="img" aria-label={t("badges.index.locked")} data-testid="badge-index-locked">
        <LockedShape entry={entry} size={40} />
      </span>
    );
  }
  const said = look.tip ? `${look.label}: ${look.tip}` : look.label;
  return (
    <Tooltip className={s.cell} text={said} side="top" card={<BadgeCard look={look} exist={p.exist} locked={<LockedShape entry={entry} size={52} />} />}>
      <span className={s.tile} data-locked="" role="img" tabIndex={0} aria-label={`${t("badges.index.locked")}: ${said}`} data-testid="badge-index-locked">
        <LockedShape entry={entry} size={40} />
      </span>
    </Tooltip>
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
            if (!entries.length) return null;
            return (
              <section key={section} className={s.section} data-testid={`badge-index-${section}`}>
                <h3 className={`${st.h5} ${s.sectionTitle}`}>{t(`badges.index.${section}`)}</h3>
                <ul className={s.grid} data-testid="badge-index-grid">
                    {entries.map((e, i) => (
                      <li key={i}>
                        {e.badge ? (
                          <span className={s.tile} data-testid="badge-index-collected">
                            <BadgePill badge={e.badge} user={me} size={40} />
                            {e.qty && e.qty > 1 ? <span className={s.qty} role="img" aria-label={t("badges.index.copies", { count: e.qty })} data-testid="badge-index-qty">×{e.qty}</span> : null}
                          </span>
                        ) : <Locked entry={e} me={me} />}
                      </li>
                    ))}
                  </ul>
              </section>
            );
          })}
        </>
      )}
    </>
  );
}

import { RotateCw, UserX } from "lucide-react";
import { useEffect, useMemo } from "react";
import type { UnfurlResult } from "@/api/types";
import { Button } from "@/components/common/Button";
import v from "@/components/invite/Invite.module.css";
import { ProfileCard } from "@/components/profile/ProfileCard";
import { t } from "@/i18n";
import { MAX_PROFILE_CARDS, profileLinksIn, type ProfileLinkRef } from "@/lib/appLinks";
import { loadSharedProfile, useSharedProfile } from "@/lib/profileLinks";
import { fetchMissingUsers, useUsers } from "@/stores/users";
import s from "./ProfileCards.module.css";

/** One card per profile a message links to, even when the sender's device made no preview for it. */
function cardLinks(nodes: unknown, embeds: UnfurlResult[] | null | undefined): ProfileLinkRef[] {
  const out = profileLinksIn(nodes);
  for (const e of embeds ?? []) {
    if (!e || e.type !== "profile" || typeof e.username !== "string" || !/^[a-z0-9_.]{1,32}$/.test(e.username)) continue;
    if (!out.some((l) => l.username === e.username) && out.length < MAX_PROFILE_CARDS) out.push({ username: e.username, url: e.url });
  }
  return out;
}

/**
 * The profiles under a message, each drawn as the popout a name opens: their
 * banner, picture and names, a few lines of their own, what they are doing,
 * their first widget, a box to message them and the way to their full
 * profile. Each card asks the server itself, so it shows the profile as it is
 * now and a link sent without a preview gets one too.
 */
export function ProfileCards({ nodes, embeds }: { nodes: unknown; embeds: UnfurlResult[] | null | undefined }) {
  const links = useMemo(() => cardLinks(nodes, embeds), [nodes, embeds]);
  if (links.length === 0) return null;
  return (
    <div className={s.cards}>
      {links.map((l) => <ProfileLinkCard key={l.username} username={l.username} />)}
    </div>
  );
}

function ProfileLinkCard({ username }: { username: string }) {
  const entry = useSharedProfile(username);
  if (entry?.status === "missing") {
    return (
      <div className={v.view} data-variant="card" data-gone="true" role="group" aria-label={t("profileLink.missingTitle")} data-testid="profile-card" data-profile-state="missing">
        <div className={v.gone}>
          <span className={v.goneIcon} aria-hidden="true"><UserX size={22} /></span>
          <div className={v.goneText}>
            <p className={v.goneTitle}>{t("profileLink.missingTitle")}</p>
            <p className={v.goneBody}>{t("profileLink.missingBody")}</p>
          </div>
        </div>
      </div>
    );
  }
  if (entry?.status === "failed") {
    return (
      <div className={v.view} data-variant="card" data-gone="true" data-testid="profile-card" data-profile-state="failed">
        <div className={v.gone}>
          <div className={v.goneText}>
            <p className={v.goneTitle}>{t("profileLink.couldNotLoad")}</p>
            <p className={v.goneBody}>{t("invite.checkYourConnection")}</p>
          </div>
          <Button size="sm" variant="secondary" onClick={() => void loadSharedProfile(username, { force: true })}><RotateCw size={14} aria-hidden="true" /> {t("invite.tryAgain")}</Button>
        </div>
      </div>
    );
  }
  return <ProfileLinkPopout id={entry?.status === "ready" ? entry.user.id : null} />;
}

/**
 * The popout itself, once the link has said who it is and the app has the
 * whole of them: the card a link shows is what anyone may see, without the
 * key or status the popout's buttons need, so it is never put in their place.
 */
function ProfileLinkPopout({ id }: { id: string | null }) {
  const known = useUsers((u) => (id ? !!u.byId[id] : false));
  useEffect(() => {
    if (id && !known) fetchMissingUsers([id]);
  }, [id, known]);
  if (!id || !known) return <ProfileCardSkeleton />;
  return (
    <div className={s.popout} data-testid="profile-card" data-profile-state="ready">
      <ProfileCard userId={id} variant="popout" embedded />
    </div>
  );
}

/** The popout's shape while the server is asked, so nothing jumps when it answers. */
function ProfileCardSkeleton() {
  return (
    <div className={s.skeleton} aria-busy="true" data-testid="profile-card" data-profile-state="loading">
      <span className="visually-hidden">{t("profileLink.loading")}</span>
      <span className={s.skelBanner} />
      <span className={s.skelAvatar} />
      <span className={s.skelBar} data-w="name" />
      <span className={s.skelBar} data-w="user" />
    </div>
  );
}

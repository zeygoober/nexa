import { Button } from "@/components/common/Button";
import { confirmInviteAge } from "@/lib/nsfw";
import { loadInvitePreview } from "@/lib/invitePreviews";
import { Link2Off } from "lucide-react";
import { useState, type CSSProperties, type ReactNode } from "react";
import type { InvitePreview, UnfurlResult } from "@/api/types";
import { bannerUrl, emojiUrl, iconUrl } from "@/api/users";
import { Markdown, type RenderCtx } from "@/markdown";
import { t } from "@/i18n";
import { avatarColor, initials } from "@/lib/avatarColor";
import { useDominantColor } from "@/lib/dominantColor";
import s from "./Invite.module.css";

/**
 * A server's description is formatted. Read before joining, it knows none of
 * the server's people or channels, so a mention in it shows as plain words.
 */
const DESCRIPTION_CTX: RenderCtx = { users: {}, channels: {}, roles: {}, emojiUrl: (id, animated, css) => emojiUrl(id, animated, css), inert: false };

/** Where an invite leads, as the views draw it. */
export interface InviteTarget {
  ageGateCode?: string;
  kind: "server" | "group";
  /** The server or group; null while only a link card is known. */
  id: string | null;
  name: string;
  description: string;
  icon: string | null;
  banner: string | null;
  online: number | null;
  members: number | null;
  /** Instead of a picture and initials: what a gift card draws in the icon's place. */
  glyph?: ReactNode;
  /** The colour of the band, and of the icon behind a glyph. */
  tone?: string;
}

export type InviteVariant = "card" | "dialog" | "page";

/** How many pixels across the icon and the banner are drawn in each view, for the size asked of the server. */
const SIZES: Record<InviteVariant, { icon: number; banner: number }> = {
  card: { icon: 56, banner: 432 },
  dialog: { icon: 80, banner: 440 },
  page: { icon: 80, banner: 440 },
};

export function targetOfPreview(p: InvitePreview, variant: InviteVariant): InviteTarget | null {
  const px = SIZES[variant];
  if (p.age_gate) return { kind: "server", id: null, name: t("nsfw.warningTitle"), description: "", icon: null, banner: null, online: null, members: null, ageGateCode: p.code };
  if (p.server) {
    const sv = p.server;
    return { kind: "server", id: sv.id, name: sv.name, description: sv.description, icon: iconUrl(sv.icon_id, px.icon), banner: bannerUrl(sv.banner_id, px.banner), online: sv.online_count, members: sv.member_count };
  }
  if (p.group) {
    const g = p.group;
    return { kind: "group", id: g.id, name: g.name, description: "", icon: iconUrl(g.icon_id, px.icon), banner: null, online: null, members: g.member_count };
  }
  return null;
}

/** The id in one of Nexa's own picture addresses, and nothing from anywhere else. */
function mediaId(url: string | undefined, kind: "icon" | "banner"): string | null {
  if (!url) return null;
  try {
    const u = new URL(url, window.location.href);
    const m = new RegExp(`^/media/${kind}/(\\d{1,20})$`).exec(u.pathname);
    return m && u.host === window.location.host ? m[1]! : null;
  } catch {
    return null;
  }
}

/**
 * What a link card sealed into the message says about the invite, drawn until
 * the server answers. Its pictures are only ever Nexa's own: the
 * card came from the sender's device, and an address in it is not trusted.
 */
export function targetOfEmbed(e: UnfurlResult): InviteTarget {
  const px = SIZES.card;
  return {
    kind: "server",
    id: null,
    name: e.title || e.site_name || "",
    description: "",
    icon: iconUrl(mediaId(e.image, "icon"), px.icon),
    banner: bannerUrl(mediaId(e.banner, "banner"), px.banner),
    online: typeof e.online_count === "number" ? e.online_count : null,
    members: typeof e.member_count === "number" ? e.member_count : null,
  };
}

/**
 * The band across the top: the banner, or without one a strip of the colour
 * the server's icon is mostly made of, as a profile without a banner shows
 * its colour. With no icon either, the colour its initials are drawn on.
 */
function Band({ target }: { target: InviteTarget }) {
  const [failed, setFailed] = useState<string | null>(null);
  const hasBanner = !!target.banner && failed !== target.banner;
  const fromIcon = useDominantColor(hasBanner ? null : target.icon);
  if (hasBanner) {
    return (
      <div className={s.band} data-kind="banner">
        <img className={s.bandImg} src={target.banner!} alt="" draggable={false} onError={() => setFailed(target.banner)} />
      </div>
    );
  }
  const color = target.tone ?? fromIcon ?? (target.id && !target.icon ? avatarColor(target.id) : undefined);
  return <div className={s.band} data-kind="plain" style={color ? ({ "--band-color": color } as CSSProperties) : undefined} />;
}

function Icon({ target }: { target: InviteTarget }) {
  const [failed, setFailed] = useState<string | null>(null);
  const color = target.tone ?? (target.id ? avatarColor(target.id) : undefined);
  return (
    <span className={s.icon}>
      {target.glyph ? (
        <span className={s.initials} style={{ "--icon-color": color, "--icon-text": "#fff" } as CSSProperties}>{target.glyph}</span>
      ) : target.icon && failed !== target.icon ? (
        <img src={target.icon} alt="" draggable={false} onError={() => setFailed(target.icon)} />
      ) : (
        <span className={s.initials} style={color ? ({ "--icon-color": color, "--icon-text": "#fff" } as CSSProperties) : undefined}>{initials(target.name)}</span>
      )}
    </span>
  );
}

function Counts({ target }: { target: InviteTarget }) {
  if (target.online === null && target.members === null) return null;
  return (
    <div className={s.counts}>
      {target.online !== null ? (
        <span className={s.count}><i className={s.dot} data-online="true" aria-hidden="true" />{t("invite.onlineCount", { count: target.online })}</span>
      ) : null}
      {target.members !== null ? (
        <span className={s.count}><i className={s.dot} aria-hidden="true" />{target.kind === "group" ? t("invite.inThisGroupCount", { count: target.members }) : t("invite.membersCount", { count: target.members })}</span>
      ) : null}
    </div>
  );
}

interface ViewProps {
  target: InviteTarget;
  variant: InviteVariant;
  /** A line above the name: who sent the invite. */
  lead?: ReactNode;
  headingId?: string;
  /** The one thing to do: join, or open what is already joined. */
  actions: ReactNode;
  /** Anything under the actions: what the link's key does, how groups are kept private. */
  children?: ReactNode;
}

/**
 * An invite, the same three ways: a card in a message, the dialog a link
 * opens in the app, and the page a visitor lands on. The server's banner
 * runs across the top with its icon over the banner's lower edge; then its
 * name, who is here, what it is about, and the button: beside the name on a
 * card, under everything in the dialog and on the page.
 */
export function InviteView({ target, variant, lead, headingId, actions, children }: ViewProps) {
  const Heading = variant === "page" ? "h1" : variant === "dialog" ? "h2" : "p";
  if (target.ageGateCode) return <InviteAgeGate code={target.ageGateCode} variant={variant} />;
  return (
    <div className={s.view} data-variant={variant}>
      <Band target={target} />
      <div className={s.top}>
        <Icon target={target} />
      </div>
      <div className={s.body}>
        <div className={s.main}>
          {lead ? <p className={s.lead}>{lead}</p> : null}
          {/* the line follows the page's direction, as the counts under it do; the name keeps its own inside it */}
          <Heading id={headingId} className={s.name}><bdi>{target.name}</bdi></Heading>
          <Counts target={target} />
        </div>
        {variant === "card" ? <div className={s.cardActions}>{actions}</div> : null}
        {target.description ? (
          <div className={s.desc} dir="auto">
            <Markdown source={target.description} ctx={DESCRIPTION_CTX} />
          </div>
        ) : null}
        {variant !== "card" ? <div className={s.actions}>{actions}</div> : null}
        {children}
      </div>
    </div>
  );
}

/** The same frame while the server is asked, so nothing jumps when it answers. */
export function InviteSkeleton({ variant }: { variant: InviteVariant }) {
  return (
    <div className={s.view} data-variant={variant} data-loading="true" aria-busy="true">
      <span className="visually-hidden">{t("invite.loading")}</span>
      <div className={s.band} data-kind="plain" />
      <div className={s.top}>
        <span className={s.icon}><span className={s.initials} /></span>
      </div>
      <div className={s.body}>
        <div className={s.main}>
          <span className={s.bar} data-w="name" />
          <span className={s.bar} data-w="counts" />
        </div>
      </div>
    </div>
  );
}

/** An invite that no longer leads anywhere: expired, used up, or deleted. */
export function InviteGone({ variant, children }: { variant: InviteVariant; children?: ReactNode }) {
  const Heading = variant === "page" ? "h1" : variant === "dialog" ? "h2" : "p";
  return (
    <div className={s.view} data-variant={variant} data-gone="true">
      <div className={s.gone}>
        <span className={s.goneIcon} aria-hidden="true"><Link2Off size={variant === "card" ? 22 : 28} /></span>
        <div className={s.goneText}>
          <Heading className={s.goneTitle}>{t("invite.invalidInvite")}</Heading>
          <p className={s.goneBody}>{t("invite.thisInviteIsInvalidOrHasExpired")} {t("invite.askForANewOne")}</p>
        </div>
      </div>
      {children ? <div className={s.goneMore}>{children}</div> : null}
    </div>
  );
}

function InviteAgeGate({ code, variant }: { code: string; variant: InviteVariant }) {
 const Heading = variant === "page" ? "h1" : variant === "dialog" ? "h2" : "p";
 const [busy, setBusy] = useState(false);
 const [failed, setFailed] = useState(false);
 const confirm = async () => {
  setBusy(true);
  setFailed(false);
  confirmInviteAge(code);
  await loadInvitePreview(code, { force: true });
  setBusy(false);
  setFailed(true);
 };
 return <div className={s.view} data-variant={variant} data-testid="nsfw-invite-warning">
  <div className={s.gone}><div className={s.goneText}>
   <Heading className={s.goneTitle}>{t("nsfw.warningTitle")}</Heading>
   <p className={s.goneBody}>{t("nsfw.warningBody")}</p>
  </div></div>
  <div className={s.goneMore}>
   <Button block loading={busy} onClick={() => void confirm()}>{t("nsfw.confirm")}</Button>
   {failed ? <p className={s.error} role="alert">{t("invite.checkYourConnection")}</p> : null}
  </div>
 </div>;
}

import { Check, Gift, RotateCw } from "lucide-react";
import { useMemo, useState, type ReactNode } from "react";
import { Button } from "@/components/common/Button";
import { InviteSkeleton, InviteView, type InviteTarget } from "@/components/invite/InviteView";
import s from "@/components/invite/Invite.module.css";
import { t } from "@/i18n";
import { giftLinksIn, type GiftLinkRef } from "@/lib/appLinks";
import { copyText } from "@/lib/clipboard";
import { describeError } from "@/lib/errors";
import { claimGiftPreview, giftTitle, loadGiftPreview, SHORT_NAME_GIFT, useGiftPreview } from "@/lib/giftPreviews";
import { useUI } from "@/stores/ui";
import { useNavigate } from "react-router-dom";
import type { GiftLinkView } from "@/api/types";

/** A card under a message for each gift link in it, in the frame an invite is drawn in. */
export function GiftCards({ nodes }: { nodes: unknown }) {
  const links = useMemo(() => giftLinksIn(nodes), [nodes]);
  if (links.length === 0) return null;
  return (
    <div className={s.cards}>
      {links.map((l) => <GiftCard key={l.code} link={l} />)}
    </div>
  );
}

function Gone({ state, title, body, children }: { state: string; title: string; body: string; children?: ReactNode }) {
  return (
    <div className={s.view} data-variant="card" data-gone="true" data-testid="gift-card" data-gift-state={state}>
      <div className={s.gone}>
        <span className={s.goneIcon} aria-hidden="true"><Gift size={22} /></span>
        <div className={s.goneText}>
          <p className={s.goneTitle}>{title}</p>
          <p className={s.goneBody}>{body}</p>
        </div>
        {children}
      </div>
    </div>
  );
}

function colourOf(gift: GiftLinkView): string {
  return gift.plan === SHORT_NAME_GIFT ? "var(--brand)" : `var(--tier-${gift.plan})`;
}

function GiftCard({ link }: { link: GiftLinkRef }) {
  const entry = useGiftPreview(link.code);
  const nav = useNavigate();
  const [busy, setBusy] = useState(false);
  if (entry?.status === "invalid") return <Gone state="invalid" title={t("chat.giftCard.goneTitle")} body={t("chat.giftCard.goneBody")} />;
  if (entry?.status === "failed") {
    return (
      <Gone state="failed" title={t("chat.giftCard.failedTitle")} body={t("invite.checkYourConnection")}>
        <Button size="sm" variant="secondary" onClick={() => loadGiftPreview(link.code, true)}><RotateCw size={14} aria-hidden="true" /> {t("invite.tryAgain")}</Button>
      </Gone>
    );
  }
  if (entry?.status !== "ready") return <div data-testid="gift-card" data-gift-state="loading"><InviteSkeleton variant="card" /></div>;

  const { gift } = entry;
  const claim = async () => {
    setBusy(true);
    try {
      await claimGiftPreview(link.code);
    } catch (err) {
      useUI.getState().toast(describeError(err), "error");
      loadGiftPreview(link.code, true);
    } finally {
      setBusy(false);
    }
  };
  const seeIt = () => {
    if (gift.plan === SHORT_NAME_GIFT) useUI.getState().openModal("settings", { page: "account", anchor: "username" });
    else nav("/channels/@me/support");
  };

  let lead: ReactNode;
  let action: ReactNode = null;
  let tone = colourOf(gift);
  let done = false;
  if (gift.claimed_by_me) {
    lead = t("support.giftLink.yoursNow");
    tone = "var(--status-online)";
    done = true;
    action = <Button size="sm" variant="secondary" onClick={seeIt} data-testid="gift-card-see">{gift.plan === SHORT_NAME_GIFT ? t("support.giftLink.chooseName") : t("support.giftLink.seePlan")}</Button>;
  } else if (gift.mine) {
    lead = gift.claimed ? t("chat.giftCard.mineClaimed") : t("chat.giftCard.mineWaiting");
    if (!gift.claimed) action = <Button size="sm" variant="secondary" onClick={() => void copyText(link.url, t("support.giftLink.copied"))} data-testid="gift-card-copy">{t("support.giftLink.copy")}</Button>;
  } else if (gift.claimed) {
    lead = t("chat.giftCard.taken");
    tone = "var(--bg-active)";
  } else {
    lead = t("chat.giftCard.from", { name: gift.from_name || t("support.giftLink.someone") });
    action = <Button size="sm" variant="positive" loading={busy} onClick={() => void claim()} data-testid="gift-card-claim">{t("support.giftLink.claim")}</Button>;
  }
  const taken = gift.claimed && !gift.claimed_by_me && !gift.mine;
  const glyph = done ? <Check size={26} aria-hidden="true" /> : <Gift size={26} aria-hidden="true" style={taken ? { color: "var(--text-muted)" } : undefined} />;
  const target: InviteTarget = { kind: "server", id: null, name: giftTitle(gift.plan, gift.days), description: "", icon: null, banner: null, online: null, members: null, glyph, tone };
  return (
    <div role="group" aria-label={target.name} data-testid="gift-card" data-gift-state={taken ? "taken" : "ready"}>
      <InviteView target={target} variant="card" lead={lead} actions={action} />
    </div>
  );
}

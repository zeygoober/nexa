import { personText } from "@/lib/personText";
import { Check, Gift, ShieldAlert } from "lucide-react";
import { useEffect, useState, type ReactNode } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { billing as billingApi } from "@/api/billing";
import type { GiftLinkView } from "@/api/types";
import { Button } from "@/components/common/Button";
import { FullScreenSpinner } from "@/components/common/Spinner";
import { RequireAuth } from "@/components/shell/RequireAuth";
import { SHORT_NAME_GIFT, giftLinkUrl, giftTitle } from "@/components/support/SupportPage";
import { copyText } from "@/lib/clipboard";
import { describeError } from "@/lib/errors";
import { useAuth } from "@/stores/auth";
import { useBilling } from "@/stores/billing";
import { useUI } from "@/stores/ui";
import { AuthLayout, usePageTitle } from "./AuthLayout";
import s from "./Auth.module.css";

import { t } from "@/i18n";

/**
 * What a gift link opens (`/gift/<code>`): the plan someone bought as a link,
 * and Claim Gift for whoever opens it first. Signing in comes first.
 */
export default function GiftPage() {
  usePageTitle(t("support.giftLink.pageTitle"));
  return (
    <RequireAuth>
      <Claim />
    </RequireAuth>
  );
}

type Step = { at: "loading" } | { at: "ready"; gift: GiftLinkView } | { at: "failed"; message: string };

function Claim() {
  const { code = "" } = useParams();
  const nav = useNavigate();
  const [step, setStep] = useState<Step>({ at: "loading" });
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    billingApi.giftLinkInfo(code).then(
      (gift) => setStep({ at: "ready", gift }),
      (err) => setStep({ at: "failed", message: describeError(err) }),
    );
  }, [code]);

  const claim = async () => {
    setBusy(true);
    try {
      const gift = await billingApi.claimGift(code);
      setStep({ at: "ready", gift });
      void useBilling.getState().load();
      if (gift.plan === SHORT_NAME_GIFT) {
        const me = useAuth.getState().me;
        if (me) useAuth.getState().setMe({ ...me, short_name_credits: (me.short_name_credits ?? 0) + 1 });
      }
    } catch (err) {
      setStep({ at: "failed", message: describeError(err) });
    } finally {
      setBusy(false);
    }
  };

  if (step.at === "loading") return <FullScreenSpinner />;
  const home = <Button block size="lg" variant="secondary" onClick={() => nav("/channels/@me", { replace: true })}>{t("qrLogin.openNexa")}</Button>;
  if (step.at === "failed") {
    return (
      <AuthLayout title={t("support.giftLink.pageTitle")} subtitle={step.message} mark={<div className={s.mark} data-tone="warn"><ShieldAlert size={28} /></div>}>
        {home}
      </AuthLayout>
    );
  }
  const { gift } = step;
  const title = giftTitle(gift.plan, gift.days);
  const mark = (icon: ReactNode) => <div className={s.mark} data-tone={gift.claimed_by_me ? "good" : undefined} style={{ ["--mark" as string]: gift.claimed_by_me ? undefined : gift.plan === SHORT_NAME_GIFT ? "var(--brand)" : `var(--tier-${gift.plan || "supporter"})` }}>{icon}</div>;
  if (gift.claimed_by_me) {
    return (
      <AuthLayout title={t("support.giftLink.yoursNow")} subtitle={gift.plan === SHORT_NAME_GIFT ? t("support.giftLink.shortNameBody") : t("support.giftLink.yoursNowBody", { gift: title })} mark={mark(<Check size={28} />)}>
        <div className={s.form}>
          {gift.plan === SHORT_NAME_GIFT ? (
            <Button block size="lg" onClick={() => { nav("/channels/@me", { replace: true }); useUI.getState().openModal("settings", { page: "account", anchor: "username" }); }} data-testid="gift-choose-name">{t("support.giftLink.chooseName")}</Button>
          ) : (
            <Button block size="lg" onClick={() => nav("/channels/@me/support", { replace: true })} data-testid="gift-see-plan">{t("support.giftLink.seePlan")}</Button>
          )}
          {home}
        </div>
      </AuthLayout>
    );
  }
  if (gift.mine) {
    return (
      <AuthLayout title={title} subtitle={gift.claimed ? t("support.giftLink.mineClaimed") : t("support.giftLink.mine")} mark={mark(<Gift size={28} />)}>
        <div className={s.form}>
          {gift.claimed ? null : <Button block size="lg" onClick={() => void copyText(giftLinkUrl(gift.code), t("support.giftLink.copied"))}>{t("support.giftLink.copy")}</Button>}
          {home}
        </div>
      </AuthLayout>
    );
  }
  if (gift.claimed) {
    return (
      <AuthLayout title={title} subtitle={t("support.giftLink.taken")} mark={mark(<Gift size={28} />)}>
        {home}
      </AuthLayout>
    );
  }
  return (
    <AuthLayout title={title} subtitle={personText("support.giftLink.fromName", gift.from_id, gift.from_name || t("support.giftLink.someone"))} mark={mark(<Gift size={28} />)}>
      <div className={s.form}>
        <Button block size="lg" loading={busy} onClick={() => void claim()} data-testid="gift-claim">{t("support.giftLink.claim")}</Button>
        {home}
      </div>
    </AuthLayout>
  );
}

import { personText } from "@/lib/personText";
import { Check, Copy, Gift, Link2, Trash2 } from "lucide-react";
import { useCallback, useEffect, useState } from "react";
import { moderation as modApi } from "@/api/moderation";
import type { GiftLink } from "@/api/types";
import { Button } from "@/components/common/Button";
import { LoadError } from "@/components/common/LoadError";
import { Spinner } from "@/components/common/Spinner";
import { giftLinkUrl, giftTitle } from "@/components/support/SupportPage";
import { copyAndSay } from "@/lib/copyText";
import { describeError } from "@/lib/errors";
import { formatRelative } from "@/lib/time";
import { useUI } from "@/stores/ui";
import { t } from "@/i18n";
import m from "./Moderation.module.css";
import b from "./BadgesTab.module.css";

type Plan = "supporter" | "pro";
type Period = "month" | "year";

function Choice<T extends string>({ label, value, options, onPick }: { label: string; value: T; options: [T, string][]; onPick: (v: T) => void }) {
  return (
    <div>
      <span className={b.label}>{label}</span>
      <span className={b.segment} role="radiogroup" data-glide="" aria-label={label}>
        {options.map(([v, text]) => (
          <button key={v} type="button" role="radio" aria-checked={value === v} className={b.segmentItem} onClick={() => onPick(v)} data-testid={`gift-${v}`}>
            {text}
          </button>
        ))}
      </span>
    </div>
  );
}

/**
 * Gift links the owner makes without paying: pick a plan and a length, and the
 * link goes on the clipboard to be sent like an invite. Each works once, for
 * whoever claims it first, and can be taken back until then.
 */
export function GiftLinksTab() {
  const toast = useUI((u) => u.toast);
  const openModal = useUI((u) => u.openModal);
  const [links, setLinks] = useState<GiftLink[] | null>(null);
  const [failed, setFailed] = useState(false);
  const [plan, setPlan] = useState<Plan>("supporter");
  const [period, setPeriod] = useState<Period>("month");
  const [busy, setBusy] = useState(false);

  const load = useCallback(() => {
    setFailed(false);
    modApi.giftLinks().then((r) => setLinks(r.links), () => setFailed(true));
  }, []);
  useEffect(() => load(), [load]);

  const create = async () => {
    setBusy(true);
    try {
      const g = await modApi.createGiftLink(plan, period);
      setLinks((cur) => [g, ...(cur ?? [])]);
      copyAndSay(giftLinkUrl(g.code), t("moderation.gifts.created"));
    } catch (err) {
      toast(describeError(err, t("moderation.failed")), "error");
    } finally {
      setBusy(false);
    }
  };
  const revoke = (g: GiftLink) =>
    openModal("confirm", {
      title: t("moderation.gifts.revokeTitle"),
      body: t("moderation.gifts.revokeBody"),
      confirmLabel: t("moderation.gifts.revoke"),
      danger: true,
      onConfirm: async () => {
        try {
          await modApi.revokeGiftLink(g.code);
          setLinks((cur) => cur?.filter((x) => x.code !== g.code) ?? cur);
          toast(t("moderation.gifts.revoked"), "success");
        } catch (err) {
          toast(describeError(err, t("moderation.failed")), "error");
        }
      },
    });

  return (
    <div className={m.scroll} data-testid="gift-links-tab">
      <div className={m.annLayout}>
        <section className={m.panel} aria-labelledby="gift-form">
          <h2 className={m.panelTitle} id="gift-form">
            <Gift size={18} aria-hidden="true" />
            {t("moderation.gifts.new")}
          </h2>
          <p className={m.panelHint}>{t("moderation.gifts.hint")}</p>
          <div className={m.annForm}>
            <div className={b.pair}>
              <Choice label={t("moderation.gifts.plan")} value={plan} onPick={setPlan} options={[["supporter", t("support.plan.supporter")], ["pro", t("support.plan.pro")]]} />
              <Choice label={t("moderation.gifts.length")} value={period} onPick={setPeriod} options={[["month", t("moderation.gifts.month")], ["year", t("moderation.gifts.year")]]} />
            </div>
            <div>
              <Button onClick={() => void create()} loading={busy} data-testid="gift-create">
                <Link2 size={15} aria-hidden="true" /> {t("moderation.gifts.create")}
              </Button>
            </div>
          </div>
        </section>
        <section className={b.lists} aria-labelledby="gift-list">
          <div>
            <h2 className={b.listTitle} id="gift-list">{t("moderation.gifts.list")}</h2>
            {failed ? (
              <LoadError message={t("moderation.loadFailed")} onRetry={load} />
            ) : !links ? (
              <div className={m.loadingRow}><Spinner /></div>
            ) : links.length === 0 ? (
              <p className={b.empty}>{t("moderation.gifts.none")}</p>
            ) : (
              <ul className={b.rows}>
                {links.map((g) => (
                  <li key={g.code} className={b.row} data-testid="gift-row">
                    <span className={b.giftMark} data-claimed={g.claimed || undefined} aria-hidden="true">{g.claimed ? <Check size={16} /> : <Link2 size={16} />}</span>
                    <span className={b.recordText}>
                      <strong>{giftTitle(g.plan, g.days)}</strong>
                      <span>
                        {g.claimed ? personText("support.giftLink.claimedBy", g.claimed_by, g.claimed_name || t("support.giftLink.someone")) : t("support.giftLink.waiting")}
                        {" · "}
                        {formatRelative(g.created_at)}
                      </span>
                    </span>
                    {g.claimed ? null : (
                      <span className={b.tools}>
                        <Button size="sm" variant="secondary" onClick={() => copyAndSay(giftLinkUrl(g.code), t("support.giftLink.copied"))} data-testid="gift-copy">
                          <Copy size={15} aria-hidden="true" /> {t("support.giftLink.copy")}
                        </Button>
                        <Button size="sm" variant="ghost" className={m.dangerGhost} onClick={() => revoke(g)} data-testid="gift-revoke">
                          <Trash2 size={15} aria-hidden="true" /> {t("moderation.gifts.revoke")}
                        </Button>
                      </span>
                    )}
                  </li>
                ))}
              </ul>
            )}
          </div>
        </section>
      </div>
    </div>
  );
}

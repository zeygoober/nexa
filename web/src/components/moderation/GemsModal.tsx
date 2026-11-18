import { PersonName } from "@/components/common/PersonName";
import { ArrowRight } from "lucide-react";
import { useEffect, useState } from "react";
import { moderation as modApi, type ModGems } from "@/api/moderation";
import { Button } from "@/components/common/Button";
import { Modal } from "@/components/common/Modal";
import { Spinner } from "@/components/common/Spinner";
import { Field, TextArea, TextInput } from "@/components/common/TextInput";
import { GemIcon } from "@/components/gems/GemIcon";
import { describeError } from "@/lib/errors";
import { personText } from "@/lib/personText";
import { useUI } from "@/stores/ui";
import { t, Tr } from "@/i18n";
import m from "./Moderation.module.css";
import g from "./GemsModal.module.css";

/** The most given or taken at once (store.MaxTradeGems). */
const MAX = 1_000_000;
const REASON_MAX = 300;

const fmt = (n: number) => new Intl.NumberFormat(document.documentElement.lang || undefined).format(n);

/**
 * The owner's Give or Take Gems: what the person can spend, the change typed,
 * and what they will have after it, in the same place. Held Gems are left to
 * the check, and taking stops at what they can spend.
 */
export function GemsModal({ user, onClose }: { user: { id: string; display_name: string }; onClose: () => void }) {
  const toast = useUI((u) => u.toast);
  const [wallet, setWallet] = useState<ModGems | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [mode, setMode] = useState<"give" | "take">("give");
  const [amount, setAmount] = useState("");
  const [reason, setReason] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    let alive = true;
    modApi
      .gems(user.id)
      .then((w) => alive && setWallet(w))
      .catch((err) => alive && setError(describeError(err, t("moderation.loadFailed"))));
    return () => {
      alive = false;
    };
  }, [user.id]);

  const n = /^\d+$/.test(amount.trim()) ? Number(amount.trim()) : 0;
  const tooMany = !!wallet && mode === "take" && n > wallet.gems;
  const outOfRange = amount.trim() !== "" && (n < 1 || n > MAX);
  const ok = !!wallet && n >= 1 && n <= MAX && !tooMany && reason.length <= REASON_MAX;
  const after = wallet && ok ? wallet.gems + (mode === "give" ? n : -n) : null;

  const save = async () => {
    if (!ok) return;
    setBusy(true);
    try {
      const w = await modApi.changeGems(user.id, mode === "give" ? n : -n, reason.trim());
      toast(personText("moderation.gems.done", user.id, user.display_name, { gems: t("gems.count", { count: w.gems }) }), "success");
      onClose();
    } catch (err) {
      toast(describeError(err, t("moderation.failed")), "error");
      setBusy(false);
    }
  };

  return (
    <Modal
      open
      onClose={onClose}
      title={<Tr id="moderation.gems.title" values={{ name: <PersonName user={user}>{user.display_name}</PersonName> }} />}
      width={440}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>{t("chat.editMessage.cancel")}</Button>
          <Button variant={mode === "give" ? "positive" : "danger"} onClick={() => void save()} loading={busy} disabled={!ok} data-testid="gems-save">
            {t(mode === "give" ? "moderation.gems.giveButton" : "moderation.gems.takeButton")}
          </Button>
        </>
      }
    >
      {error ? <p className={m.danger} role="alert">{error}</p> : null}
      {!wallet && !error ? (
        <div className={g.loading}>
          <Spinner size={24} label={t("chat.pins.loading")} />
        </div>
      ) : null}
      {wallet ? (
        <div className={g.body}>
          <div className={g.balance} data-testid="gems-balance">
            <span className={g.figures} aria-live="polite">
              <GemIcon size={26} />
              <span className={g.figure} data-was={after !== null || undefined}>{fmt(wallet.gems)}</span>
              {after !== null ? (
                <>
                  <ArrowRight size={18} className={`${g.arrow} ${m.flip}`} aria-hidden="true" />
                  <span className={g.figure} data-tone={mode} data-testid="gems-after">{fmt(after)}</span>
                </>
              ) : null}
            </span>
            <span className={g.caption}>{t(after !== null ? "moderation.gems.after" : "moderation.gems.now")}</span>
          </div>
          {wallet.held > 0 || wallet.state === "check" || wallet.state === "report" ? <p className={m.note}>{t("moderation.gems.held", { count: wallet.held })}</p> : null}
          <div className={g.modes} role="radiogroup" aria-label={t("moderation.gems.menu")} data-glide="">
            {(["give", "take"] as const).map((k) => (
              <button key={k} type="button" role="radio" aria-checked={mode === k} className={g.mode} onClick={() => setMode(k)} data-testid={`gems-${k}`}>
                {t(k === "give" ? "moderation.gems.give" : "moderation.gems.take")}
              </button>
            ))}
          </div>
          <Field
            label={t("moderation.gems.amount")}
            error={tooMany ? t("moderation.gems.tooMany", { count: wallet.gems }) : outOfRange ? t("moderation.gems.range") : null}
          >
            <TextInput
              inputMode="numeric"
              autoFocus
              value={amount}
              invalid={tooMany || outOfRange}
              onChange={(e) => setAmount(e.target.value.replace(/[^\d]/g, "").slice(0, 7))}
              onKeyDown={(e) => {
                if (e.key === "Enter") void save();
              }}
              data-testid="gems-amount"
            />
          </Field>
          <Field label={t("moderation.gems.reason")} hint={t("moderation.gems.reasonHint")}>
            <TextArea value={reason} maxLength={REASON_MAX} rows={2} onChange={(e) => setReason(e.target.value)} data-testid="gems-reason" />
          </Field>
        </div>
      ) : null}
    </Modal>
  );
}

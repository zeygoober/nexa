import { PersonName } from "@/components/common/PersonName";
import { ArrowRight } from "lucide-react";
import { useEffect, useState } from "react";
import { moderation as modApi, type ModCollectable } from "@/api/moderation";
import { BadgePill } from "@/components/badges/Badges";
import { Button } from "@/components/common/Button";
import { Modal } from "@/components/common/Modal";
import { Spinner } from "@/components/common/Spinner";
import { Field, TextArea, TextInput } from "@/components/common/TextInput";
import { describeError } from "@/lib/errors";
import { personText } from "@/lib/personText";
import { useUI } from "@/stores/ui";
import { t, Tr } from "@/i18n";
import m from "./Moderation.module.css";
import g from "./GemsModal.module.css";
import c from "./CollectablesModal.module.css";

/** The most given or taken at once (store.MaxOwnerCopies). */
const MAX = 100;
const REASON_MAX = 300;

const keyOf = (id: string, material: number) => `event:${id}-${material}`;

/**
 * The owner's Give or Take Collectables: pick a collectable, then a material
 * with the copies held of each in sight, then how many. Laid out as the Gems
 * dialog is, so the two owner tools work alike.
 */
export function CollectablesModal({ user, onClose }: { user: { id: string; display_name: string }; onClose: () => void }) {
  const toast = useUI((u) => u.toast);
  const [sets, setSets] = useState<ModCollectable[] | null>(null);
  const [held, setHeld] = useState<Record<string, number>>({});
  const [error, setError] = useState<string | null>(null);
  const [pick, setPick] = useState<string | null>(null);
  const [material, setMaterial] = useState(1);
  const [mode, setMode] = useState<"give" | "take">("give");
  const [amount, setAmount] = useState("");
  const [reason, setReason] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    let alive = true;
    modApi
      .collectables(user.id)
      .then((r) => {
        if (!alive) return;
        setSets(r.collectables);
        setHeld(r.held);
        setPick(r.collectables[0]?.id ?? null);
      })
      .catch((err) => alive && setError(describeError(err, t("moderation.loadFailed"))));
    return () => {
      alive = false;
    };
  }, [user.id]);

  const who = { id: user.id, flags: 0, plan: "" as const };
  const set = sets?.find((s) => s.id === pick) ?? null;
  const key = set ? keyOf(set.id, material) : "";
  const have = held[key] ?? 0;
  const n = /^\d+$/.test(amount.trim()) ? Number(amount.trim()) : 0;
  const tooMany = mode === "take" && n > have;
  const outOfRange = amount.trim() !== "" && (n < 1 || n > MAX);
  const ok = !!set && n >= 1 && n <= MAX && !tooMany && reason.length <= REASON_MAX;
  const after = ok ? have + (mode === "give" ? n : -n) : null;
  const total = (id: string) => [1, 2, 3, 4, 5].reduce((sum, k) => sum + (held[keyOf(id, k)] ?? 0), 0);

  const save = async () => {
    if (!ok || !set || busy) return;
    setBusy(true);
    try {
      const r = await modApi.changeCollectable(user.id, key, mode === "give" ? n : -n, reason.trim());
      toast(
        personText("moderation.collectables.done", user.id, user.display_name, {
          copies: t("badges.index.copies", { count: r.copies }),
          badge: set.name,
          material: t(`gems.material.${material}`),
        }),
        "success",
      );
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
      title={<Tr id="moderation.collectables.title" values={{ name: <PersonName user={user}>{user.display_name}</PersonName> }} />}
      width={440}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>{t("chat.editMessage.cancel")}</Button>
          <Button variant={mode === "give" ? "positive" : "danger"} onClick={() => void save()} loading={busy} disabled={!ok} data-testid="collectables-save">
            {t(mode === "give" ? "moderation.collectables.giveButton" : "moderation.collectables.takeButton")}
          </Button>
        </>
      }
    >
      {error ? <p className={m.danger} role="alert">{error}</p> : null}
      {!sets && !error ? (
        <div className={g.loading}>
          <Spinner size={24} label={t("chat.pins.loading")} />
        </div>
      ) : null}
      {sets && !sets.length ? <p className={m.note}>{t("moderation.collectables.none")}</p> : null}
      {sets && set ? (
        <div className={g.body}>
          <div className={c.list} role="radiogroup" aria-label={t("moderation.collectables.pick")} data-testid="collectables-list">
            {sets.map((s) => {
              const count = total(s.id);
              return (
                <button
                  key={s.id}
                  type="button"
                  role="radio"
                  aria-checked={s.id === pick}
                  className={c.row}
                  onClick={() => setPick(s.id)}
                >
                  {s.materials[0] ? <BadgePill badge={s.materials[0]} user={who} size={28} still /> : null}
                  <span className={c.name} dir="auto">{s.name}</span>
                  <span className={c.rarity} data-rarity={s.rarity}>
                    <i aria-hidden="true" />
                    {t(`gems.rarity.${s.rarity}`)}
                  </span>
                  <span className={c.count}>{count ? t("badges.index.copies", { count }) : ""}</span>
                </button>
              );
            })}
          </div>
          <div className={c.materials} role="radiogroup" aria-label={t("moderation.collectables.material")}>
            {set.materials.map((b, i) => {
              const k = i + 1;
              const q = held[keyOf(set.id, k)] ?? 0;
              const name = t(`gems.material.${k}`);
              return (
                <button
                  key={k}
                  type="button"
                  role="radio"
                  aria-checked={material === k}
                  aria-label={q ? `${name}, ${t("badges.index.copies", { count: q })}` : name}
                  className={c.material}
                  onClick={() => setMaterial(k)}
                  data-testid={`collectables-material-${k}`}
                >
                  <span className={c.tile}>
                    <BadgePill badge={b} user={who} size={36} still />
                    {q ? <span className={c.qty} aria-hidden="true">×{q}</span> : null}
                  </span>
                  <span className={c.word}>{name}</span>
                </button>
              );
            })}
          </div>
          <div className={g.balance} data-testid="collectables-balance">
            <span className={g.figures} aria-live="polite">
              {set.materials[material - 1] ? <BadgePill badge={set.materials[material - 1]!} user={who} size={26} still /> : null}
              <span className={g.figure} data-was={after !== null || undefined}>{have}</span>
              {after !== null ? (
                <>
                  <ArrowRight size={18} className={`${g.arrow} ${m.flip}`} aria-hidden="true" />
                  <span className={g.figure} data-tone={mode} data-testid="collectables-after">{after}</span>
                </>
              ) : null}
            </span>
            <span className={g.caption}>{t(after !== null ? "moderation.collectables.after" : "moderation.collectables.now")}</span>
          </div>
          <div className={g.modes} role="radiogroup" aria-label={t("moderation.collectables.menu")} data-glide="">
            {(["give", "take"] as const).map((k) => (
              <button key={k} type="button" role="radio" aria-checked={mode === k} className={g.mode} onClick={() => setMode(k)} data-testid={`collectables-${k}`}>
                {t(k === "give" ? "moderation.gems.give" : "moderation.gems.take")}
              </button>
            ))}
          </div>
          <Field
            label={t("moderation.gems.amount")}
            error={tooMany ? t("moderation.collectables.tooMany", { count: have }) : outOfRange ? t("moderation.collectables.range") : null}
          >
            <TextInput
              inputMode="numeric"
              value={amount}
              invalid={tooMany || outOfRange}
              onChange={(e) => setAmount(e.target.value.replace(/[^\d]/g, "").slice(0, 3))}
              onKeyDown={(e) => {
                if (e.key === "Enter") void save();
              }}
              data-testid="collectables-amount"
            />
          </Field>
          <Field label={t("moderation.gems.reason")} hint={t("moderation.gems.reasonHint")}>
            <TextArea value={reason} maxLength={REASON_MAX} rows={2} onChange={(e) => setReason(e.target.value)} data-testid="collectables-reason" />
          </Field>
        </div>
      ) : null}
    </Modal>
  );
}

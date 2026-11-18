import { useEffect, useRef, useState } from "react";
import { gems as gemsApi, type Collectible, type GemRarity } from "@/api/gems";
import type { TradeLine } from "@/api/types";
import { BadgePill } from "@/components/badges/Badges";
import { Button } from "@/components/common/Button";
import { GemIcon } from "@/components/gems/GemIcon";
import { FuseMachine, materialOf } from "@/components/gems/FuseMachine";
import { Spinner } from "@/components/common/Spinner";
import chat from "@/components/chat/Chat.module.css";
import { t } from "@/i18n";
import { describeError } from "@/lib/errors";
import { capsuleBuild, capsuleOpen, capsuleWait, primeGemSounds } from "@/lib/gemSounds";
import { CAPSULE_ART } from "@/lib/badges";
import { GEM_RARITIES } from "@/lib/gems";
import { motionOff } from "@/lib/motion";
import { formatShortDate } from "@/lib/time";
import { useAuth } from "@/stores/auth";
import { useGems } from "@/stores/gems";
import { useUI } from "@/stores/ui";
import st from "./Settings.module.css";
import s from "./GemsPage.module.css";

const TEAM_SIZE = 5;
const MATERIALS = 5;

const fmt = (n: number, digits = 0) => new Intl.NumberFormat(document.documentElement.lang || undefined, { maximumFractionDigits: digits }).format(n);

/**
 * A capsule's life on the page: `opening` while the server draws, `reveal` while
 * it wobbles and its glow flares to the rarity that came out, `pop` while its
 * halves part and the collectible rises, `got` once it has.
 */
type Phase = "idle" | "opening" | "reveal" | "pop" | "got";

export function RarityTag({ rarity }: { rarity: GemRarity }) {
  return (
    <span className={s.rarity} data-rarity={rarity}>
      <i aria-hidden="true" />
      {t(`gems.rarity.${rarity}`)}
    </span>
  );
}

/** Settings, Gems: the balance, what the team adds, the capsule and its odds, and the collection with fusing. */
export default function GemsPage() {
  const me = useAuth((a) => a.me);
  const { view, error, load, setView } = useGems();
  const toast = useUI((u) => u.toast);
  const [phase, setPhase] = useState<Phase>("idle");
  const [got, setGot] = useState<Collectible | null>(null);
  const [fusing, setFusing] = useState<TradeLine | null>(null);
  const [confirming, setConfirming] = useState(false);
  useEffect(() => {
    void load();
  }, [load]);

  // the capsule's sounds follow its phases: a rattle while drawing, the build through the reveal, the split once it opens
  const was = useRef<Phase>("idle");
  useEffect(() => {
    const before = was.current;
    was.current = phase;
    if (phase === "opening") {
      const p = capsuleWait();
      return () => p.stop();
    }
    if (!got) return;
    if (phase === "reveal") {
      const p = capsuleBuild(got.rarity);
      return () => p.stop();
    }
    // pop after the build, or got straight from a skip or under Reduced motion
    if ((phase === "pop" && before === "reveal") || (phase === "got" && (before === "reveal" || before === "opening"))) capsuleOpen(got.rarity);
  }, [phase, got]);

  const open = async () => {
    primeGemSounds("capsule");
    setPhase("opening");
    setGot(null);
    try {
      const res = await gemsApi.openCapsule();
      setView(res.gems);
      setGot(res.got);
      // the draw is already made; what follows only shows it, and a press skips it
      setPhase(motionOff() ? "got" : "reveal");
    } catch (err) {
      setPhase("idle");
      toast(describeError(err, t("gems.capsule.failed")), "error");
    }
  };
  const onCapsuleAnimation = (name: string) => {
    if (name.includes("build")) setPhase("pop");
    else if (name.includes("splitBottom")) setPhase("got");
  };

  const confirm = async () => {
    setConfirming(true);
    try {
      setView(await gemsApi.confirm());
      const cur = useAuth.getState().me;
      if (cur) useAuth.getState().setMe({ ...cur, gem_state: undefined });
      toast(t("gems.hold.confirmed"), "success");
    } catch (err) {
      toast(describeError(err, t("gems.hold.confirmFailed")), "error");
    } finally {
      setConfirming(false);
    }
  };

  if (error && !view) {
    return (
      <>
        <h2 className={st.title}>{t("gems.title")}</h2>
        <p className={s.note}>
          {t("gems.failed")}{" "}
          <button type="button" className={chat.loadRetry} onClick={() => void load()}>{t("chat.messageList.tryAgain")}</button>
        </p>
      </>
    );
  }
  if (!view || !me) {
    return (
      <>
        <h2 className={st.title}>{t("gems.title")}</h2>
        <div className={s.loading}><Spinner size={24} label={t("chat.pins.loading")} /></div>
      </>
    );
  }

  const { wallet } = view;
  const held = wallet.state === "check" || wallet.state === "report";
  const short = Math.max(0, view.capsule_price - wallet.gems);
  const empty = view.collectibles.length === 0;
  const team = [...view.team, ...Array<null>(Math.max(0, TEAM_SIZE - view.team.length)).fill(null)].slice(0, TEAM_SIZE);

  return (
    <>
      <h2 className={st.title}>{t("gems.title")}</h2>
      <div className={s.wallet} data-testid="gems-summary">
        <GemIcon size={44} />
        <p className={s.count}>
          <strong data-testid="gems-balance">{fmt(wallet.gems)}</strong> <span>{t("gems.unit")}</span>
        </p>
        <span className={s.mult} data-testid="gems-multiplier">{t("gems.multiplier", { value: fmt(view.multiplier / 100, 2) })}</span>
      </div>
      <p className={s.note}>{t("gems.hint")}</p>
      {wallet.state ? (
        <div className={s.hold} role="status" data-testid="gems-hold">
          {wallet.state === "check" ? (
            <>
              <span>{t("gems.hold.check", { count: wallet.held })}</span>
              <Button size="sm" loading={confirming} onClick={() => void confirm()} data-testid="gems-confirm">{t("gems.hold.confirm")}</Button>
            </>
          ) : wallet.state === "report" ? (
            <span>{t("gems.hold.report", { count: wallet.held })}</span>
          ) : (
            <span>{t("gems.hold.off", { date: formatShortDate(wallet.off_until) })}</span>
          )}
        </div>
      ) : null}

      <section className={s.section} data-testid="gems-capsule">
        <h3 className={`${st.h5} ${s.sectionTitle}`}>{t("gems.capsule.title")}</h3>
        <div className={s.machine} data-rarity={(phase === "got" || phase === "pop") && got ? got.rarity : undefined}>
          <div className={s.draw}>
            <div className={s.stage} data-phase={phase} data-reveal={phase === "reveal" || phase === "pop" ? got?.rarity : undefined} aria-live="polite">
              <span className={s.flare} aria-hidden="true" />
              {phase !== "got" ? (
                <button
                  type="button"
                  className={s.capsule}
                  disabled={phase !== "reveal"}
                  tabIndex={phase === "reveal" ? 0 : -1}
                  aria-label={phase === "reveal" ? t("gems.skip") : undefined}
                  onClick={() => setPhase("got")}
                  onAnimationEnd={(e) => onCapsuleAnimation(e.animationName)}
                  data-testid="gems-capsule-art"
                >
                  <img className={s.half} data-half="top" src={CAPSULE_ART} alt="" draggable={false} />
                  <img className={s.half} data-half="bottom" src={CAPSULE_ART} alt="" draggable={false} />
                </button>
              ) : null}
              {(phase === "pop" || phase === "got") && got ? (
                <div className={s.result} data-testid="gems-result">
                  <span className={s.resultBadge}><BadgePill badge={got.badge} user={me} size={88} /></span>
                  <strong className={s.resultName}>{got.name}</strong>
                  <span className={s.resultMeta}><RarityTag rarity={got.rarity} /><span>{t("gems.material.1")}</span></span>
                </div>
              ) : null}
            </div>
            <div className={s.buy}>
              {empty ? (
                <p className={s.note}>{t("gems.capsule.empty")}</p>
              ) : (
                <>
                  <Button loading={phase === "opening" || phase === "reveal" || phase === "pop"} disabled={short > 0 || held} onClick={() => void open()} data-testid="gems-open">
                    {phase === "got" ? t("gems.capsule.again") : t("gems.capsule.open")}
                    <span className={s.price} aria-hidden="true">
                      <GemIcon size={18} />
                      {fmt(view.capsule_price)}
                    </span>
                    <span className="visually-hidden">{t("gems.count", { count: view.capsule_price })}</span>
                  </Button>
                  {short > 0 && !held ? <span className={s.short}>{t("gems.capsule.need", { count: short })}</span> : null}
                </>
              )}
            </div>
          </div>
          <dl className={s.odds} data-testid="gems-odds">
            {GEM_RARITIES.filter((r) => view.odds[r]).map((r) => (
              <div key={r} className={s.odd} data-rarity={r}>
                <dt><RarityTag rarity={r} /></dt>
                <dd>{fmt((view.odds[r] ?? 0) / 100, 2)}%</dd>
              </div>
            ))}
            {empty ? <p className={s.note}>{t("gems.capsule.noOdds")}</p> : null}
          </dl>
        </div>
      </section>

      <section className={s.section} data-testid="gems-collection">
        <h3 className={`${st.h5} ${s.sectionTitle}`}>{t("gems.collection.title")}</h3>
        {view.collection.length ? (
          <ul className={s.grid}>
            {view.collection.map((l) => {
              const m = materialOf(l.key);
              const top = m >= MATERIALS;
              return (
                <li key={l.key} className={s.item}>
                  <span className={s.tile}>
                    {l.badge ? <BadgePill badge={l.badge} user={me} size={48} /> : null}
                    {l.qty > 1 ? <span className={s.qty} role="img" aria-label={t("badges.index.copies", { count: l.qty })}>×{l.qty}</span> : null}
                  </span>
                  {top ? (
                    <span className={s.top}>{t("gems.material.5")}</span>
                  ) : (
                    <Button variant="secondary" size="sm" onClick={() => setFusing(l)} aria-label={t("gems.fusing.open", { material: t(`gems.material.${m + 1}`) })} data-testid="gems-fuse">
                      {t("gems.fuse")}
                    </Button>
                  )}
                </li>
              );
            })}
          </ul>
        ) : (
          <p className={s.note}>{t("gems.collection.empty")}</p>
        )}
        {view.collection.length ? <p className={s.note}>{t("gems.collection.chance", { percent: Math.round(100 / view.fuse_cost), count: view.fuse_cost })}</p> : null}
        {fusing ? (
          <FuseMachine
            line={fusing}
            view={view}
            name={view.collectibles.find((c) => fusing.key.startsWith(`event:${c.id}-`))?.name ?? ""}
            onView={setView}
            onClose={() => setFusing(null)}
          />
        ) : null}
      </section>

      <section className={s.section} data-testid="gems-team">
        <h3 className={`${st.h5} ${s.sectionTitle}`}>{t("gems.team.title")}</h3>
        <ul className={s.team}>
          {team.map((m, i) => (
            <li key={i} className={s.member}>
              {m ? (
                <>
                  <span className={s.tile}><BadgePill badge={m.badge} user={me} size={40} /></span>
                  <span className={s.power} data-none={m.power === 0 || undefined}>{t("gems.team.power", { value: fmt(m.power) })}</span>
                </>
              ) : (
                <>
                  <span className={s.tile} data-empty="" aria-label={t("gems.team.emptySlot")} role="img" />
                  <span className={s.power} data-none="">{" "}</span>
                </>
              )}
            </li>
          ))}
        </ul>
        <p className={s.note}>{t("gems.team.hint")}</p>
      </section>
    </>
  );
}

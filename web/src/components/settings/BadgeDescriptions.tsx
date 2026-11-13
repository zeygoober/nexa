import { useEffect, useState } from "react";
import { moderation } from "@/api/moderation";
import type { BadgeArrangement, BadgeCategory, PublicUser } from "@/api/types";
import { BadgeMark } from "@/components/badges/Badges";
import { Button } from "@/components/common/Button";
import { Modal } from "@/components/common/Modal";
import { Spinner } from "@/components/common/Spinner";
import { Field, TextArea } from "@/components/common/TextInput";
import { t } from "@/i18n";
import { isEventBadge } from "@/lib/badges";
import { useUI } from "@/stores/ui";
import { lookOf } from "./BadgeArrange";
import { BADGE_SECTIONS } from "./BadgeIndexPage";
import s from "./BadgeIndexPage.module.css";

type Item = BadgeArrangement["entries"][number];

/** The longest tooltip (store.MaxBadgeTip). */
const MAX_TIP = 120;

/** Whether the owner's own words stand in for the built-in tooltip. */
const changed = (i: Item) => !isEventBadge(i.badge.id) && !!i.badge.description;

/** The badge as it shows with no words of the owner's, for the built-in text. */
const asBuiltIn = (i: Item): Item => ({ ...i, badge: { ...i.badge, description: undefined } });

function Mark({ item, me }: { item: Item; me: PublicUser }) {
  const look = lookOf(item, me);
  if (!look) return null;
  return look.art ? <img className={s.art} src={look.art} alt="" draggable={false} /> : <BadgeMark look={look} size={40} />;
}

/**
 * The owner writing what a badge's card says, for everyone and in every
 * language (Android `Describe` in BadgeIndexPage.kt). Built-in badges keep the
 * words in badge_tips; an event badge's go to its own description.
 */
export default function BadgeDescriptions({ me, onDone }: { me: PublicUser; onDone: () => void }) {
  const toast = useUI((u) => u.toast);
  const [items, setItems] = useState<Item[] | null>(null);
  const [chosen, setChosen] = useState<BadgeCategory | null>(null);
  const [open, setOpen] = useState<Item | null>(null);
  const [text, setText] = useState("");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    moderation.badgeArrangement().then(
      (a) => setItems(a.entries),
      () => {
        toast(t("badges.index.failed"), "error");
        onDone();
      },
    );
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const edit = (i: Item) => {
    setOpen(i);
    setText(isEventBadge(i.badge.id) || changed(i) ? i.badge.description ?? "" : "");
  };

  const length = [...text.trim()].length;
  const save = () => {
    if (!open || length > MAX_TIP) return;
    const words = text.trim();
    setSaving(true);
    moderation.setBadgeTip(open.key, words).then(
      () => {
        setItems((list) => list?.map((i) => (i.key === open.key ? { ...i, badge: { ...i.badge, description: words || undefined } } : i)) ?? null);
        toast(t("badges.index.describeSaved"), "success");
        setSaving(false);
        setOpen(null);
      },
      () => {
        toast(t("badges.index.describeFailed"), "error");
        setSaving(false);
      },
    );
  };

  if (!items) return <div className={s.loading}><Spinner size={24} label={t("chat.pins.loading")} /></div>;
  const parts = BADGE_SECTIONS.filter((p) => items.some((i) => i.category === p));
  const part = chosen && parts.includes(chosen) ? chosen : parts[0];
  const builtIn = open && !isEventBadge(open.badge.id) ? lookOf(asBuiltIn(open), me) : null;
  // the card as it will read once saved
  const preview = open && lookOf({ ...open, badge: { ...open.badge, description: text.trim() || (isEventBadge(open.badge.id) ? "" : undefined) } }, me);
  return (
    <>
      <div className={s.arrangeHead}>
        <h3 className={s.arrangeTitle}>{t("badges.index.describeTitle")}</h3>
        <p className={s.hint}>{t("badges.index.describeHint")}</p>
        <div className={s.actions}>
          <Button size="sm" onClick={onDone} data-testid="badge-describe-done">{t("badges.index.describeDone")}</Button>
        </div>
      </div>
      {parts.length > 1 ? (
        <div className={s.parts} role="radiogroup" aria-label={t("badges.index.describeTitle")} data-glide="">
          {parts.map((p) => (
            <button key={p} type="button" role="radio" aria-checked={p === part} className={s.part} onClick={() => setChosen(p)} data-testid={`badge-describe-${p}`}>
              {t(`badges.index.${p}`)}
            </button>
          ))}
        </div>
      ) : null}
      <ul className={s.grid} data-testid="badge-describe-grid">
        {items.filter((i) => i.category === part).map((item) => (
          <li key={item.key}>
            <button
              type="button"
              className={`${s.tile} ${s.pickable}`}
              aria-label={`${lookOf(item, me)?.label ?? item.key}${changed(item) ? `, ${t("badges.index.describeChanged")}` : ""}`}
              onClick={() => edit(item)}
              data-key={item.key}
              data-testid="badge-describe-tile"
            >
              <Mark item={item} me={me} />
              {changed(item) ? <span className={s.changedDot} aria-hidden="true" /> : null}
            </button>
          </li>
        ))}
      </ul>
      <Modal
        open={!!open}
        onClose={() => !saving && setOpen(null)}
        title={t("badges.index.describeEdit")}
        footer={
          <>
            {open && !isEventBadge(open.badge.id) ? (
              <Button variant="secondary" disabled={saving || !text} onClick={() => setText("")} data-testid="badge-describe-reset">{t("badges.index.describeReset")}</Button>
            ) : null}
            <Button variant="secondary" disabled={saving} onClick={() => setOpen(null)}>{t("moderation.cancel")}</Button>
            <Button onClick={save} loading={saving} disabled={length > MAX_TIP} data-testid="badge-describe-save">{t("chat.editMessage.save")}</Button>
          </>
        }
      >
        {open && preview ? (
          <div className={s.editor}>
            <div className={s.card} aria-hidden="true">
              <Mark item={open} me={me} />
              <span className={s.cardText}>
                <span className={s.cardName}>{preview.label}</span>
                <span className={s.cardTip}>{preview.tip}</span>
              </span>
            </div>
            <Field label={t("badges.index.describeField")} hint={`${length}/${MAX_TIP}`} error={length > MAX_TIP ? t("badges.index.describeLong") : undefined}>
              <TextArea
                value={text}
                onChange={(e) => setText(e.target.value)}
                placeholder={builtIn?.tip || t("badges.staff.descriptionPlaceholder")}
                rows={3}
                invalid={length > MAX_TIP}
                data-testid="badge-describe-text"
              />
            </Field>
            {builtIn ? <p className={s.hint}>{t("badges.index.describeEmptyHint")}</p> : null}
          </div>
        ) : null}
      </Modal>
    </>
  );
}

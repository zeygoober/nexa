import { Award, ImagePlus, PenLine, Pipette, Plus, Search, Trash2, Users, X } from "lucide-react";
import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState, type CSSProperties } from "react";
import { moderation as modApi } from "@/api/moderation";
import type { Badge, BadgeDef, BadgeShape } from "@/api/types";
import { BadgeMark, BadgePill } from "@/components/badges/Badges";
import { Button } from "@/components/common/Button";
import { ColourInput } from "@/components/common/ColourInput";
import { ImageCropModal } from "@/components/common/ImageCropModal";
import { LoadError } from "@/components/common/LoadError";
import { Spinner } from "@/components/common/Spinner";
import { Field, TextInput } from "@/components/common/TextInput";
import { Toggle } from "@/components/common/Toggle";
import { CONTRIBUTOR, defBadge, EVENT_COLOURS, EVENT_ICONS, EVENT_PREFIX, hexColour, OVERRIDABLE, type BadgeLook } from "@/lib/badges";
import { describeError } from "@/lib/errors";
import { useModEvents } from "@/stores/moderation";
import { useUI } from "@/stores/ui";
import { t } from "@/i18n";
import { BadgeHoldersModal } from "./BadgeHoldersModal";
import m from "./Moderation.module.css";
import b from "./BadgesTab.module.css";

const EventIcon = lazy(() => import("@/components/badges/EventIcon"));
const IconSearch = lazy(() => import("./BadgeIconSearch"));

const MAX_NAME = 32;
const MAX_DESCRIPTION = 120;
const NOBODY = { id: "0", flags: 0, plan: "" as const };
/** Who the Earned list draws staff ranks and plans as, so their pictures show. */
const SAMPLE = { id: "0", flags: 1, plan: "pro" as const };

type Mode = "icon" | "picture";

/** A badge being held, as the holders dialog needs it: what it is and how to draw it. */
export interface HoldersOf {
  badge: Badge;
  label: string;
  /** The highest level staff can give it at (1 for untiered); 0 when staff cannot give or take it. */
  top: number;
}

/**
 * Every badge on Nexa: those staff make for an event (a name, a description
 * shown as its tooltip, an icon in one colour or two, or a picture),
 * Contributor, and the earned ones. Each opens its holders, where it is given
 * and taken; deleting one staff made takes it from everyone.
 */
export function BadgesTab() {
  const toast = useUI((u) => u.toast);
  const openModal = useUI((u) => u.openModal);
  const [list, setList] = useState<BadgeDef[] | null>(null);
  const [contributors, setContributors] = useState(0);
  const [failed, setFailed] = useState(false);
  const [editing, setEditing] = useState<BadgeDef | null>(null);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [icon, setIcon] = useState<string>(EVENT_ICONS[0]);
  const [colour, setColour] = useState(EVENT_COLOURS[0]!);
  const [colour2, setColour2] = useState<number | null>(null);
  const [glyph, setGlyph] = useState<"light" | "dark">("light");
  const [shape, setShape] = useState<BadgeShape>("circle");
  const [image, setImage] = useState<{ id: string; url: string } | null>(null);
  const [mode, setMode] = useState<Mode>("icon");
  const [query, setQuery] = useState("");
  const [crop, setCrop] = useState<File | null>(null);
  const [uploading, setUploading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [holders, setHolders] = useState<HoldersOf | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  const load = useCallback(() => {
    setFailed(false);
    modApi
      .badges()
      .then((r) => {
        setList(r.badges);
        setContributors(r.contributors);
      })
      .catch(() => setFailed(true));
  }, []);
  useEffect(() => load(), [load]);
  // another member of staff gave, made or deleted one
  useModEvents((e) => {
    if (e.type === "log" && e.entry.action.startsWith("badge_")) load();
  });

  const startEdit = (d: BadgeDef | null) => {
    setEditing(d);
    setName(d?.name ?? "");
    setDescription(d?.description ?? "");
    setIcon(d?.icon ?? EVENT_ICONS[0]);
    setColour(d?.color ?? EVENT_COLOURS[0]!);
    setColour2(d && d.color2 >= 0 ? d.color2 : null);
    setGlyph(d?.glyph === "dark" ? "dark" : "light");
    setShape(d?.shape ?? "circle");
    const pic = d?.image_id && d.image_id !== "0" ? { id: d.image_id, url: `/media/badge/${d.image_id}` } : null;
    setImage(pic);
    setMode(pic ? "picture" : "icon");
    setQuery("");
  };
  const nameLength = [...name.trim()].length;
  const descLength = [...description.trim()].length;
  const ready = nameLength > 0 && nameLength <= MAX_NAME && descLength <= MAX_DESCRIPTION && (mode === "icon" || image !== null) && !uploading;

  const upload = async (blob: Blob, area: { x: number; y: number; w: number; h: number } | null) => {
    setUploading(true);
    try {
      const file = new File([blob], "badge", { type: blob.type });
      const r = await modApi.uploadBadgeImage(file, area ?? undefined);
      setImage({ id: r.id, url: r.image });
      setMode("picture");
    } catch (err) {
      toast(describeError(err, t("moderation.failed")), "error");
    } finally {
      setUploading(false);
    }
  };

  const submit = async () => {
    setBusy(true);
    try {
      const body = {
        name: name.trim(),
        description: description.trim(),
        icon,
        color: colour,
        color2: colour2 ?? -1,
        glyph,
        shape,
        image_id: mode === "picture" && image ? image.id : "0",
      };
      if (editing) {
        const d = await modApi.editBadge(editing.id, body);
        setList((cur) => cur?.map((x) => (x.id === d.id ? { ...d, holders: x.holders } : x)) ?? cur);
        toast(t("badges.staff.saved"), "success");
      } else {
        const d = await modApi.createBadge(body);
        setList((cur) => [{ ...d, holders: 0 }, ...(cur ?? [])]);
        toast(t("badges.staff.created"), "success");
      }
      startEdit(null);
    } catch (err) {
      toast(describeError(err, t("moderation.failed")), "error");
    } finally {
      setBusy(false);
    }
  };

  const remove = (d: BadgeDef) =>
    openModal("confirm", {
      title: t("badges.staff.deleteTitle"),
      body: t("badges.staff.deleteBody", { name: d.name }),
      confirmLabel: t("badges.staff.delete"),
      danger: true,
      onConfirm: async () => {
        try {
          await modApi.deleteBadge(d.id);
          setList((cur) => cur?.filter((x) => x.id !== d.id) ?? cur);
          if (editing?.id === d.id) startEdit(null);
          toast(t("badges.staff.deleted"), "success");
        } catch (err) {
          toast(describeError(err, t("moderation.failed")), "error");
        }
      },
    });

  const shownName = name.trim() || t("badges.staff.namePlaceholder");
  const look: BadgeLook = {
    iconName: icon,
    color: colour,
    color2: colour2 ?? undefined,
    image: mode === "picture" ? image?.url : undefined,
    label: shownName,
    tip: description.trim(),
  };
  const glow = hexColour(colour2 ?? colour);

  const earned = useMemo(
    () =>
      Object.keys(OVERRIDABLE).map((id) => ({ badge: { id, level: 1 } as Badge, top: OVERRIDABLE[id]! })),
    [],
  );

  const holdersButton = (of: HoldersOf, count?: number) => (
    <Button size="sm" variant="secondary" onClick={() => setHolders(of)} data-testid={`badge-holders-${of.badge.id}`}>
      <Users size={15} aria-hidden="true" />
      {count === undefined ? t("badges.staff.holdersButton") : t("badges.staff.holders", { count })}
    </Button>
  );

  return (
    <div className={m.scroll} data-testid="badges-tab">
      <div className={m.annLayout}>
        <section className={m.panel} aria-labelledby="badge-form">
          <h2 className={m.panelTitle} id="badge-form">
            <Award size={18} aria-hidden="true" />
            {editing ? t("badges.staff.edit") : t("badges.staff.new")}
          </h2>
          <p className={m.panelHint}>{t("badges.staff.hint")}</p>
          <div className={m.annForm}>
            <div className={b.stage} style={{ "--glow": glow } as CSSProperties} aria-hidden="true">
              <Suspense fallback={<span className={b.stageMark} />}>
                <BadgeMark look={look} size={64} />
              </Suspense>
              <span className={b.stageText}>
                <span className={b.stageName}>{shownName}</span>
                <span className={b.stageDesc} data-empty={!description.trim() || undefined}>
                  {description.trim() || t("badges.staff.descriptionPlaceholder")}
                </span>
              </span>
              <span className={b.stageSmall}>
                <BadgePill badge={previewBadge(look)} user={NOBODY} still />
              </span>
            </div>

            <Field label={t("badges.staff.name")} hint={nameLength > MAX_NAME - 8 ? `${nameLength}/${MAX_NAME}` : undefined}>
              <TextInput value={name} onChange={(e) => setName(e.target.value)} placeholder={t("badges.staff.namePlaceholder")} maxLength={MAX_NAME + 8} data-testid="badge-name" />
            </Field>
            <Field label={t("badges.staff.description")} hint={`${descLength}/${MAX_DESCRIPTION}`}>
              <TextInput
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                placeholder={t("badges.staff.descriptionPlaceholder")}
                maxLength={MAX_DESCRIPTION + 8}
                data-testid="badge-description"
              />
            </Field>

            <div>
              <span className={b.label} id="badge-colour-label">{colour2 === null ? t("badges.staff.colour") : t("badges.staff.firstColour")}</span>
              <Swatches value={colour} onChange={setColour} labelledBy="badge-colour-label" />
            </div>
            <Toggle
              checked={colour2 !== null}
              onChange={(on) => setColour2(on ? (EVENT_COLOURS.find((c) => c !== colour) ?? 0xffffff) : null)}
              label={t("badges.staff.gradient")}
              description={t("badges.staff.gradientHint")}
            />
            {colour2 !== null ? (
              <div>
                <span className={b.label} id="badge-colour2-label">{t("badges.staff.secondColour")}</span>
                <Swatches value={colour2} onChange={setColour2} labelledBy="badge-colour2-label" />
              </div>
            ) : null}

            <div className={b.pair}>
              <div>
                <span className={b.label} id="badge-mode-label">{t("badges.staff.look")}</span>
                <div className={b.segment} role="radiogroup" aria-labelledby="badge-mode-label" data-glide="">
                  {(["icon", "picture"] as const).map((v) => (
                    <button key={v} type="button" role="radio" aria-checked={mode === v} className={b.segmentItem} onClick={() => setMode(v)} data-testid={`badge-mode-${v}`}>
                      {v === "icon" ? t("badges.staff.lookIcon") : t("badges.staff.lookPicture")}
                    </button>
                  ))}
                </div>
              </div>
            </div>

            {mode === "icon" ? (
              <div>
                <span className={b.label} id="badge-icon-label">{t("badges.staff.icon")}</span>
                <label className={b.search}>
                  <Search size={15} aria-hidden="true" />
                  <input
                    value={query}
                    onChange={(e) => setQuery(e.target.value)}
                    placeholder={t("badges.staff.iconSearch")}
                    aria-label={t("badges.staff.iconSearch")}
                    data-testid="badge-icon-search"
                  />
                  {query ? (
                    <button type="button" onClick={() => setQuery("")} aria-label={t("badges.staff.clearSearch")}>
                      <X size={14} aria-hidden="true" />
                    </button>
                  ) : null}
                </label>
                <div className={b.icons} role="radiogroup" aria-labelledby="badge-icon-label">
                  <Suspense fallback={<Spinner />}>
                    {query.trim() ? (
                      <IconSearch query={query} value={icon} onPick={setIcon} />
                    ) : (
                      [...new Set<string>([...(EVENT_ICONS as readonly string[]).includes(icon) ? [] : [icon], ...EVENT_ICONS])].map((n) => (
                        <button key={n} type="button" role="radio" aria-checked={icon === n} aria-label={n} title={n} className={b.icon} data-on={icon === n || undefined} onClick={() => setIcon(n)}>
                          <EventIcon name={n} size={18} />
                        </button>
                      ))
                    )}
                  </Suspense>
                </div>
              </div>
            ) : (
              <div className={b.picture}>
                {image ? <img src={image.url} alt="" className={b.pictureThumb} /> : <span className={b.pictureEmpty}><ImagePlus size={20} aria-hidden="true" /></span>}
                <span className={b.pictureText}>
                  <Button size="sm" variant="secondary" loading={uploading} onClick={() => fileRef.current?.click()} data-testid="badge-upload">
                    <ImagePlus size={15} aria-hidden="true" />
                    {image ? t("badges.staff.replacePicture") : t("badges.staff.uploadPicture")}
                  </Button>
                  <span className={b.pictureHint}>{t("badges.staff.pictureHint")}</span>
                </span>
                <input
                  ref={fileRef}
                  type="file"
                  accept="image/png,image/jpeg,image/gif,image/webp"
                  hidden
                  onChange={(e) => {
                    const f = e.target.files?.[0];
                    e.target.value = "";
                    if (f) setCrop(f);
                  }}
                />
              </div>
            )}

            <div className={m.annFooter}>
              {editing ? (
                <Button size="sm" variant="ghost" onClick={() => startEdit(null)}>{t("chat.editMessage.cancel")}</Button>
              ) : null}
              <span className={m.grow} />
              <Button size="sm" disabled={!ready} loading={busy} onClick={() => void submit()} data-testid="badge-save">
                {editing ? t("badges.staff.save") : <><Plus size={15} aria-hidden="true" /> {t("badges.staff.create")}</>}
              </Button>
            </div>
          </div>
        </section>

        <div className={b.lists}>
          {failed && !list ? (
            <LoadError message={t("moderation.loadFailed")} onRetry={load} />
          ) : list === null ? (
            <Spinner label={t("chat.pins.loading")} />
          ) : (
            <>
              <section aria-labelledby="badge-made">
                <h2 className={b.listTitle} id="badge-made">{t("badges.staff.madeByStaff")}</h2>
                <ul className={b.rows}>
                  {list.map((d) => {
                    const badge = defBadge(d);
                    return (
                      <li key={d.id} className={b.row} data-editing={editing?.id === d.id || undefined} data-testid="badge-def">
                        <BadgePill badge={badge} user={NOBODY} still named />
                        {d.description ? <span className={b.desc}>{d.description}</span> : <span className={m.grow} />}
                        <span className={b.tools}>
                          {holdersButton({ badge, label: d.name, top: 1 }, d.holders)}
                          <Button size="sm" variant="ghost" onClick={() => startEdit(d)} aria-label={t("badges.staff.editNamed", { name: d.name })} title={t("badges.staff.edit")}>
                            <PenLine size={15} aria-hidden="true" />
                          </Button>
                          <Button size="sm" variant="ghost" onClick={() => remove(d)} aria-label={t("badges.staff.deleteNamed", { name: d.name })} title={t("badges.staff.delete")}>
                            <Trash2 size={15} aria-hidden="true" />
                          </Button>
                        </span>
                      </li>
                    );
                  })}
                  {list.length === 0 ? <li className={b.empty}>{t("badges.staff.none")}</li> : null}
                </ul>
              </section>

              <section aria-labelledby="badge-hand">
                <h2 className={b.listTitle} id="badge-hand">{t("badges.staff.givenByHand")}</h2>
                <ul className={b.rows}>
                  <li className={b.row} data-testid="badge-def-contributor">
                    <BadgePill badge={{ id: CONTRIBUTOR }} user={NOBODY} still named />
                    <span className={b.desc}>{t("badges.staff.contributorNote")}</span>
                    <span className={b.tools}>{holdersButton({ badge: { id: CONTRIBUTOR }, label: t("badges.contributor"), top: 1 }, contributors)}</span>
                  </li>
                </ul>
              </section>

              <section aria-labelledby="badge-earned">
                <h2 className={b.listTitle} id="badge-earned">{t("badges.staff.earned")}</h2>
                <p className={b.listHint}>{t("badges.staff.earnedHint")}</p>
                <ul className={b.rows}>
                  {earned.map(({ badge, top }) => (
                    <li key={badge.id} className={b.row} data-testid={`badge-earned-${badge.id}`}>
                      <BadgePill badge={badge} user={NOBODY} still named />
                      <span className={m.grow} />
                      <span className={b.tools}>{holdersButton({ badge, label: earnedLabel(badge), top })}</span>
                    </li>
                  ))}
                  {(["staff", "plan"] as const).map((id) => (
                    <li key={id} className={b.row} data-testid={`badge-earned-${id}`}>
                      <span className={b.namedRow}>
                        <BadgePill badge={{ id }} user={SAMPLE} still />
                        <span className={b.rowName}>{id === "staff" ? t("badges.staff.ranks") : t("badges.staff.plans")}</span>
                      </span>
                      <span className={b.desc}>{t("badges.staff.followsAccount")}</span>
                      <span className={b.tools}>
                        {holdersButton({ badge: { id }, label: id === "staff" ? t("badges.staff.ranks") : t("badges.staff.plans"), top: 0 })}
                      </span>
                    </li>
                  ))}
                </ul>
              </section>
            </>
          )}
        </div>
      </div>

      {crop ? (
        <ImageCropModal
          file={crop}
          aspect={1}
          maxSize={256}
          serverCrop
          title={t("badges.staff.cropTitle")}
          onClose={() => setCrop(null)}
          onDone={({ blob, crop: area }) => {
            setCrop(null);
            void upload(blob, area ?? null);
          }}
        />
      ) : null}
      {holders ? <BadgeHoldersModal of={holders} onClose={() => setHolders(null)} onChanged={load} /> : null}
    </div>
  );
}

/** An earned badge's name in the holders dialog's title, without its level. */
function earnedLabel(badge: Badge): string {
  switch (badge.id) {
    case "og": return t("badges.og");
    case "years": return t("badges.staff.yearsName");
    case "bug_hunter": return t("badges.staff.bugName");
    case "ideas": return t("badges.staff.ideasName");
    case "app_developer": return t("badges.appDeveloper");
    case "gift_giver": return t("badges.giftGiver");
    case "friendly": return t("badges.family.friendly");
    case "host": return t("badges.family.host");
    case "booster": return t("badges.booster");
    case "regular": return t("badges.family.regular");
    case "open_mic": return t("badges.family.openMic");
    default: return badge.id;
  }
}

/** The badge being made as a badge someone holds, for the small pill beside the big preview. */
function previewBadge(look: BadgeLook): Badge {
  return {
    id: `${EVENT_PREFIX}0`,
    name: look.label,
    icon: look.iconName,
    color: look.color,
    color2: look.color2,
    image: look.image,
  };
}

/** The colour swatches, with any other colour from the picker. */
function Swatches({ value, onChange, labelledBy }: { value: number; onChange: (c: number) => void; labelledBy: string }) {
  return (
    <div className={b.colours} role="radiogroup" aria-labelledby={labelledBy}>
      {EVENT_COLOURS.map((c) => (
        <button key={c} type="button" role="radio" aria-checked={value === c} aria-label={hexColour(c)} className={b.swatch} style={{ "--swatch": hexColour(c) } as CSSProperties} data-on={value === c || undefined} onClick={() => onChange(c)} />
      ))}
      {/* any other colour: a pipette over the picker's own cover, so it is not taken for one more swatch */}
      <span className={b.custom} data-on={!EVENT_COLOURS.includes(value) || undefined} style={{ "--swatch": hexColour(value) } as CSSProperties}>
        <Pipette size={15} aria-hidden="true" />
        <ColourInput value={hexColour(value)} onChange={(v) => onChange(parseInt(v.slice(1, 7), 16))} label={t("badges.staff.customColour")} />
      </span>
    </div>
  );
}

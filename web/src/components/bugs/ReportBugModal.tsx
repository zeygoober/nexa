import { AlertCircle, ChevronDown, CircleCheck, ImagePlus, Play, Plus, ShieldAlert, Upload, X } from "lucide-react";
import { useEffect, useId, useMemo, useRef, useState, type ClipboardEvent, type DragEvent } from "react";
import { useNavigate } from "react-router-dom";
import { ApiError } from "@/api/client";
import { bugs as bugsApi, type BugKind, type MemberBug } from "@/api/bugs";
import { Button } from "@/components/common/Button";
import { Modal } from "@/components/common/Modal";
import { TextArea, TextInput } from "@/components/common/TextInput";
import { Toggle } from "@/components/common/Toggle";
import { BUG_ACCEPT, BUG_LIMITS, bugContext, bugFileProblem, bugNumber, contextRows, isBugVideo, readDeviceFacts } from "@/lib/bugs";
import { describeError } from "@/lib/errors";
import { formatBytes, formatShortDuration } from "@/lib/time";
import { useSettings } from "@/stores/settings";
import { t, useLocale } from "@/i18n";
import s from "./Bugs.module.css";

/** One file on its way into a report: shown from this device while it goes up. */
interface Pending {
  key: string;
  file: File;
  preview: string;
  video: boolean;
  /** Seconds, once the browser has read a video's length. */
  duration?: number;
  progress: number;
  state: "uploading" | "done" | "failed";
  /** The server's id for it, once it is up. */
  id?: string;
  error?: string;
  abort: AbortController;
}

/** What was written before the dialog closed without sending, kept for the rest of the session: a bug report and a suggestion each have their own. */
const drafts: Record<BugKind, { subject: string; description: string }> = { bug: { subject: "", description: "" }, suggestion: { subject: "", description: "" } };

/** Where the dialog was opened from, which the report can say. */
export type BugFrom = "dm" | "group" | "friends" | "settings" | "nexa";

/** The words for a failed upload or send that the server named by code. */
function problemText(err: unknown, fallback: string): string {
  if (err instanceof ApiError) {
    switch (err.code) {
      case "image_too_large":
        return t("bugs.form.imageTooLarge");
      case "video_too_large":
      case "too_large":
        return t("bugs.form.videoTooLarge");
      case "unsupported_type":
        return t("bugs.form.fileType");
      case "bad_dimensions":
        return t("bugs.form.imageTooBig");
      case "files_too_large":
        return t("bugs.form.filesTooLarge");
      case "attachment_gone":
        return t("bugs.form.fileGone");
      case "rate_limited":
        return t("bugs.form.rateLimited");
    }
  }
  return describeError(err, fallback);
}

let seq = 0;

/**
 * Report a Bug: what went wrong in the reporter's own words, the screenshots
 * or recording that show it (picked, pasted or dropped anywhere on the
 * dialog), and, unless they turn it off, the facts about their app and device
 * that help reproduce it, each of which they can read before sending. Files go
 * up as they are added, so sending is quick. It says plainly that reports are
 * not end-to-end encrypted, and once sent, where the answer will come.
 *
 * Suggest a Feature is the same dialog for an idea: its own words, pictures
 * or a recording if they help, and no device details, which an idea has no
 * use for. It goes to the team as a suggestion, answered the same way.
 */
export default function ReportBugModal({ onClose, from, kind = "bug" }: { onClose: () => void; from?: BugFrom | undefined; kind?: BugKind | undefined }) {
  const idea = kind === "suggestion";
  // the words that differ between the two: a key under bugs.form or suggestions.form
  const w = (key: string, values?: Record<string, string | number>) => t(`${idea ? "suggestions" : "bugs"}.form.${key}`, values);
  const draft = drafts[kind];
  const nav = useNavigate();
  const locale = useLocale();
  const settings = useSettings((st) => st.settings);
  const [subject, setSubject] = useState(draft.subject);
  const [description, setDescription] = useState(draft.description);
  const [files, setFiles] = useState<Pending[]>([]);
  const [fileErrors, setFileErrors] = useState<string[]>([]);
  const [withContext, setWithContext] = useState(!idea);
  const [showFacts, setShowFacts] = useState(false);
  const [tried, setTried] = useState(false);
  const [sending, setSending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [sent, setSent] = useState<MemberBug | null>(null);
  const [dragging, setDragging] = useState(false);
  const picker = useRef<HTMLInputElement>(null);
  const descriptionRef = useRef<HTMLTextAreaElement>(null);
  const dragDepth = useRef(0);
  const filesRef = useRef(files);
  filesRef.current = files;
  const ids = useId();
  const touch = typeof window !== "undefined" && !!window.matchMedia?.("(pointer: coarse)").matches;

  // what the report will say about this device: read once, as the dialog opens
  const context = useMemo(
    () =>
      bugContext(
        readDeviceFacts({
          language: locale,
          theme: settings.theme,
          customCss: (settings.customCss.snippetOn && settings.customCss.snippet.trim() !== "") || settings.customCss.links.some((l) => l.enabled),
          plugins: settings.plugins.filter((p) => p.enabled).length,
          openedFrom: from ?? "",
        }),
      ),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [],
  );
  const facts = useMemo(() => contextRows(context), [context]);

  useEffect(() => {
    if (!sent) drafts[kind] = { subject, description };
  }, [subject, description, sent, kind]);

  // files still going up stop, and their previews are let go, when the dialog does
  useEffect(
    () => () => {
      for (const f of filesRef.current) {
        if (f.state === "uploading") f.abort.abort();
        URL.revokeObjectURL(f.preview);
      }
    },
    [],
  );

  const update = (key: string, patch: Partial<Pending>) => setFiles((cur) => cur.map((f) => (f.key === key ? { ...f, ...patch } : f)));

  const upload = (p: Pending) => {
    bugsApi
      .upload(p.file, p.file.name, { signal: p.abort.signal, onProgress: (loaded, total) => update(p.key, { progress: total ? loaded / total : 0 }) })
      .then((r) => update(p.key, { state: "done", id: r.id, progress: 1 }))
      .catch((err: unknown) => {
        if (p.abort.signal.aborted) return;
        update(p.key, { state: "failed", error: problemText(err, t("bugs.form.uploadFailed")) });
      });
  };

  const add = (list: File[]) => {
    const problems: string[] = [];
    const next: Pending[] = [];
    let count = filesRef.current.length;
    let bytes = filesRef.current.reduce((n, f) => n + f.file.size, 0);
    for (const raw of list) {
      // a pasted screenshot comes without a name worth keeping
      const file = raw.name && raw.name !== "image.png" ? raw : new File([raw], `${t("bugs.file.screenshot")}.${(raw.type.split("/")[1] ?? "png").replace("jpeg", "jpg")}`, { type: raw.type });
      const problem = bugFileProblem(file, { count, bytes });
      if (problem) {
        problems.push(`${file.name}: ${problem}`);
        continue;
      }
      count += 1;
      bytes += file.size;
      const p: Pending = { key: `f${++seq}`, file, preview: URL.createObjectURL(file), video: isBugVideo(file.type), progress: 0, state: "uploading", abort: new AbortController() };
      next.push(p);
    }
    setFileErrors(problems);
    if (!next.length) return;
    setFiles((cur) => [...cur, ...next]);
    for (const p of next) upload(p);
  };

  const remove = (p: Pending) => {
    if (p.state === "uploading") p.abort.abort();
    URL.revokeObjectURL(p.preview);
    setFiles((cur) => cur.filter((f) => f.key !== p.key));
    setFileErrors([]);
  };

  const retry = (p: Pending) => {
    const again: Pending = { ...p, state: "uploading", progress: 0, error: undefined, abort: new AbortController() };
    setFiles((cur) => cur.map((f) => (f.key === p.key ? again : f)));
    upload(again);
  };

  const onPaste = (e: ClipboardEvent<HTMLDivElement>) => {
    const pasted = [...e.clipboardData.files];
    if (!pasted.length) return;
    e.preventDefault();
    add(pasted);
  };
  const hasFiles = (e: DragEvent) => [...e.dataTransfer.types].includes("Files");
  const onDragEnter = (e: DragEvent<HTMLDivElement>) => {
    if (!hasFiles(e)) return;
    e.preventDefault();
    dragDepth.current += 1;
    setDragging(true);
  };
  const onDragOver = (e: DragEvent<HTMLDivElement>) => {
    if (!hasFiles(e)) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = "copy";
  };
  const onDragLeave = (e: DragEvent<HTMLDivElement>) => {
    if (!hasFiles(e)) return;
    dragDepth.current = Math.max(0, dragDepth.current - 1);
    if (dragDepth.current === 0) setDragging(false);
  };
  const onDrop = (e: DragEvent<HTMLDivElement>) => {
    if (!hasFiles(e)) return;
    e.preventDefault();
    dragDepth.current = 0;
    setDragging(false);
    add([...e.dataTransfer.files]);
  };

  const subjectLen = [...subject.trim()].length;
  const descriptionLen = [...description.trim()].length;
  const subjectProblem = subjectLen < BUG_LIMITS.subjectMin ? t("bugs.form.subjectNeeded") : subjectLen > BUG_LIMITS.subjectMax ? t("bugs.form.subjectLong") : null;
  const descriptionProblem = descriptionLen < BUG_LIMITS.descriptionMin ? w("descriptionNeeded") : descriptionLen > BUG_LIMITS.descriptionMax ? t("bugs.form.descriptionLong") : null;
  const uploading = files.some((f) => f.state === "uploading");
  const failed = files.some((f) => f.state === "failed");

  const send = async () => {
    setTried(true);
    setError(null);
    if (subjectProblem || descriptionProblem || sending) return;
    if (uploading) return setError(t("bugs.form.stillUploading"));
    if (failed) return setError(t("bugs.form.removeFailed"));
    setSending(true);
    try {
      const r = await bugsApi.create({
        kind,
        subject: subject.trim(),
        description: description.trim(),
        context: withContext ? context : undefined,
        attachments: files.map((f) => ({ id: f.id!, name: f.file.name })),
      });
      drafts[kind] = { subject: "", description: "" };
      setSent(r.report);
    } catch (err) {
      setError(problemText(err, w("sendFailed")));
    } finally {
      setSending(false);
    }
  };

  const openThread = () => {
    if (!sent) return;
    onClose();
    nav(`/channels/@me/nexa/bugs/${sent.id}`);
  };

  if (sent) {
    return (
      <Modal open onClose={onClose} title={w("title")} width={520} footer={
        <>
          <Button variant="secondary" onClick={onClose} data-testid="bug-done">
            {t("bugs.form.done")}
          </Button>
          <Button onClick={openThread} data-testid="bug-open-thread">
            {t("bugs.form.openThread")}
          </Button>
        </>
      }>
        <div className={s.sent} role="status" data-testid="bug-sent">
          <span className={s.sentIcon} aria-hidden="true">
            <CircleCheck size={24} />
          </span>
          <h3 className={s.sentTitle}>{w("sentTitle", { number: bugNumber(sent.number) })}</h3>
          <p className={s.sentText}>{w("sentText")}</p>
        </div>
      </Modal>
    );
  }

  const subjectId = `${ids}-subject`;
  const descriptionId = `${ids}-description`;
  return (
    <Modal
      open
      onClose={onClose}
      title={w("title")}
      width={580}
      // a click beside the dialog does not throw away files on their way up
      closeOnOverlay={files.length === 0}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t("bugs.form.cancel")}
          </Button>
          <Button onClick={() => void send()} loading={sending} data-testid="bug-send">
            {w("send")}
          </Button>
        </>
      }
    >
      <div className={s.form} onPaste={onPaste} onDragEnter={onDragEnter} onDragOver={onDragOver} onDragLeave={onDragLeave} onDrop={onDrop} data-testid="bug-form">
        <p className={s.lead}>{w("lead")}</p>

        <div className={s.field}>
          <div className={s.fieldHead}>
            <label className={s.fieldLabel} htmlFor={subjectId}>
              {t("bugs.form.subject")}
            </label>
            {subjectLen > BUG_LIMITS.subjectMax - 20 ? (
              <span className={s.fieldCount} data-over={subjectLen > BUG_LIMITS.subjectMax || undefined}>
                {BUG_LIMITS.subjectMax - subjectLen}
              </span>
            ) : null}
          </div>
          <TextInput
            id={subjectId}
            value={subject}
            onChange={(e) => setSubject(e.target.value)}
            placeholder={w("subjectPlaceholder")}
            maxLength={BUG_LIMITS.subjectMax + 20}
            invalid={tried && !!subjectProblem}
            aria-invalid={tried && !!subjectProblem}
            aria-describedby={tried && subjectProblem ? `${subjectId}-error` : undefined}
            dir="auto"
            enterKeyHint="next"
            onKeyDown={(e) => {
              // the subject is one line: Enter moves on to the description rather than sending half a report
              if (e.key === "Enter" && !e.nativeEvent.isComposing) {
                e.preventDefault();
                descriptionRef.current?.focus();
              }
            }}
            data-testid="bug-subject"
          />
          {tried && subjectProblem ? (
            <p className={s.fieldError} id={`${subjectId}-error`}>
              {subjectProblem}
            </p>
          ) : null}
        </div>

        <div className={s.field}>
          <div className={s.fieldHead}>
            <label className={s.fieldLabel} htmlFor={descriptionId}>
              {w("description")}
            </label>
            {descriptionLen > BUG_LIMITS.descriptionMax - 300 ? (
              <span className={s.fieldCount} data-over={descriptionLen > BUG_LIMITS.descriptionMax || undefined}>
                {BUG_LIMITS.descriptionMax - descriptionLen}
              </span>
            ) : null}
          </div>
          <TextArea
            ref={descriptionRef}
            id={descriptionId}
            className={s.description}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder={w("descriptionPlaceholder")}
            invalid={tried && !!descriptionProblem}
            aria-invalid={tried && !!descriptionProblem}
            aria-describedby={`${descriptionId}-hint`}
            rows={5}
            dir="auto"
            data-testid="bug-description"
          />
          {tried && descriptionProblem ? (
            <p className={s.fieldError} id={`${descriptionId}-hint`}>
              {descriptionProblem}
            </p>
          ) : (
            <p className={s.hint} id={`${descriptionId}-hint`}>
              {w("descriptionHint")}
            </p>
          )}
        </div>

        <div className={s.field}>
          <span className={s.fieldLabel} id={`${ids}-files`}>
            {w("files")}
          </span>
          <input ref={picker} type="file" accept={BUG_ACCEPT} multiple hidden onChange={(e) => {
            add([...(e.target.files ?? [])]);
            e.target.value = "";
          }} data-testid="bug-file-input" />
          {files.length === 0 ? (
            <button type="button" className={s.drop} onClick={() => picker.current?.click()} aria-describedby={`${ids}-files-sub`} data-testid="bug-add-files">
              <span className={s.dropIcon} aria-hidden="true">
                <ImagePlus size={20} />
              </span>
              <span className={s.dropText}>
                <span className={s.dropTitle}>{w("addFiles")}</span>
                <span className={s.dropSub} id={`${ids}-files-sub`}>
                  {touch ? t("bugs.form.filesLimits") : t("bugs.form.filesHint")}
                </span>
              </span>
            </button>
          ) : (
            <ul className={s.files} aria-labelledby={`${ids}-files`} style={{ listStyle: "none", margin: 0, padding: 0 }}>
              {files.map((f) => (
                <li key={f.key} className={s.tile} data-state={f.state} title={`${f.file.name} (${formatBytes(f.file.size)})`} data-testid="bug-file">
                  {f.video ? (
                    <video src={f.preview} muted playsInline preload="metadata" onLoadedMetadata={(e) => update(f.key, { duration: e.currentTarget.duration })} aria-hidden="true" />
                  ) : (
                    <img src={f.preview} alt="" />
                  )}
                  {f.video ? (
                    <span className={s.tileBadge}>
                      <Play size={10} aria-hidden="true" />
                      {f.duration && Number.isFinite(f.duration) ? formatShortDuration(f.duration) : t("bugs.file.video")}
                    </span>
                  ) : null}
                  {f.state === "uploading" ? (
                    <span className={s.progress} role="progressbar" aria-label={t("bugs.form.uploading", { name: f.file.name })} aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(f.progress * 100)}>
                      <span style={{ width: `${Math.round(f.progress * 100)}%` }} />
                    </span>
                  ) : null}
                  {f.state === "failed" ? (
                    <button type="button" className={s.tileFailed} onClick={() => retry(f)} aria-label={t("bugs.form.retryFile", { name: f.file.name })} title={f.error}>
                      <AlertCircle size={22} aria-hidden="true" />
                    </button>
                  ) : null}
                  <button type="button" className={s.tileRemove} onClick={() => remove(f)} aria-label={t("bugs.form.removeFile", { name: f.file.name })} data-testid="bug-file-remove">
                    <X size={14} aria-hidden="true" />
                  </button>
                </li>
              ))}
              {files.length < BUG_LIMITS.files ? (
                <li>
                  <button type="button" className={s.addTile} onClick={() => picker.current?.click()} aria-label={t("bugs.form.addMore")} data-testid="bug-add-more">
                    <Plus size={20} aria-hidden="true" />
                    <span aria-hidden="true">{t("bugs.form.add")}</span>
                  </button>
                </li>
              ) : null}
            </ul>
          )}
          {fileErrors.length || failed ? (
            <ul className={s.fileErrors} role="alert">
              {fileErrors.map((e) => (
                <li key={e} className={s.fieldError}>
                  {e}
                </li>
              ))}
              {files
                .filter((f) => f.state === "failed")
                .map((f) => (
                  <li key={f.key} className={s.fieldError}>
                    {`${f.file.name}: ${f.error ?? t("bugs.form.uploadFailed")}`}
                  </li>
                ))}
            </ul>
          ) : null}
        </div>

        {idea ? null : (
          <div className={s.details}>
            <Toggle checked={withContext} onChange={setWithContext} label={t("bugs.form.includeDetails")} description={t("bugs.form.includeDetailsHint")} />
            {withContext ? (
              <>
                <button type="button" className={s.disclose} aria-expanded={showFacts} aria-controls={`${ids}-facts`} onClick={() => setShowFacts((v) => !v)} data-testid="bug-show-details">
                  {showFacts ? t("bugs.form.hideDetails") : t("bugs.form.showDetails")}
                  <ChevronDown size={15} aria-hidden="true" />
                </button>
                {showFacts ? (
                  <dl className={s.facts} id={`${ids}-facts`} data-testid="bug-facts">
                    {facts.map((f) => (
                      <FactRow key={f.key} label={f.label} value={f.value} ltr={f.key !== "language" && f.key !== "opened_from" && f.key !== "device" && f.key !== "app" && f.key !== "custom_css"} />
                    ))}
                  </dl>
                ) : null}
              </>
            ) : null}
          </div>
        )}

        <p className={s.privacy}>
          <ShieldAlert size={16} aria-hidden="true" />
          {w("notEncrypted")}
        </p>
        {error ? (
          <p className={s.formError} role="alert" data-testid="bug-error">
            {error}
          </p>
        ) : null}

        {dragging ? (
          <div className={s.dropping} aria-hidden="true">
            <Upload size={28} />
            {t("bugs.form.dropHere")}
          </div>
        ) : null}
      </div>
    </Modal>
  );
}

function FactRow({ label, value, ltr }: { label: string; value: string; ltr: boolean }) {
  return (
    <>
      <dt>{label}</dt>
      <dd {...(ltr ? { "data-ltr": "" } : {})}>{value}</dd>
    </>
  );
}

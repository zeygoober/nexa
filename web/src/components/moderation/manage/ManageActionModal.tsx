import { X } from "lucide-react";
import { useState } from "react";
import { moderation as modApi, type ManageRequest, type ManageView } from "@/api/moderation";
import { Button } from "@/components/common/Button";
import { ColourInput } from "@/components/common/ColourInput";
import { TextArea, TextInput as Input } from "@/components/common/TextInput";
import { Modal } from "@/components/common/Modal";
import { Select } from "@/components/common/Select";
import { Toggle } from "@/components/common/Toggle";
import { OverwriteGrid, RoleGrid } from "@/components/server/PermissionGrid";
import type { ChannelType } from "@/api/types";
import { describeError } from "@/lib/errors";
import { parsePerm, permString } from "@/lib/perms";
import { useUI } from "@/stores/ui";
import { t } from "@/i18n";
import m from "../Moderation.module.css";
import s from "./ManageServer.module.css";

const MAX_REASON = 1000;

export interface ManageTarget {
  id: string;
  type: "role" | "member";
  name: string;
  allow: string;
  deny: string;
}

export type ManageField =
  | { kind: "text" | "area"; key: string; label: string; max?: number; required?: boolean; when?: (v: ManageValues) => boolean }
  | { kind: "number"; key: string; label: string; min: number; max: number }
  | { kind: "select"; key: string; label: string; options: [string, string][] }
  | { kind: "toggle"; key: string; label: string }
  | { kind: "colour"; key: string; label: string }
  | { kind: "perms"; key: string; label: string }
  | { kind: "overwrite"; channelType: string; targets: ManageTarget[] };

export type ManageValues = Record<string, string | number | boolean>;

/** One owner's action, as the dialog asks for it: what it is, what it needs and who will hear of it. */
export interface ManageAsk {
  action: string;
  target?: string;
  confirm?: string;
  danger?: boolean;
  sees: string[];
  fields?: ManageField[];
  initial?: ManageValues;
  request: (v: ManageValues) => Omit<ManageRequest, "reason" | "action"> & { action?: string };
}

export function ManageActionModal({ ask, serverId, onClose, onDone }: { ask: ManageAsk; serverId: string; onClose: () => void; onDone: (view: ManageView) => void }) {
  const toast = useUI((u) => u.toast);
  const overwrite = ask.fields?.find((f): f is Extract<ManageField, { kind: "overwrite" }> => f.kind === "overwrite");
  const firstTarget = overwrite?.targets[0];
  const [values, setValues] = useState<ManageValues>({
    ...ask.initial,
    ...(firstTarget ? { target_id: firstTarget.id, allow: firstTarget.allow, deny: firstTarget.deny, clear: false } : {}),
  });
  const [reason, setReason] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const put = (patch: ManageValues) => setValues((v) => ({ ...v, ...patch }));
  const title = t(`moderation.manage.a.${ask.action}`);
  const missing = ask.fields?.some((f) => (f.kind === "text" || f.kind === "area") && f.required && !String(values[f.key] ?? "").trim());
  const shortReason = [...reason.trim()].length < 3;
  const longReason = [...reason].length > MAX_REASON;

  const submit = async () => {
    setBusy(true);
    setError(null);
    try {
      const { action, ...rest } = ask.request(values);
      const view = await modApi.manage(serverId, { action: action ?? ask.action, reason: reason.trim(), ...rest });
      toast(t("moderation.manage.done"), "success");
      onDone(view);
    } catch (err) {
      setError(describeError(err, t("moderation.failed")));
      setBusy(false);
    }
  };

  const fieldOf = (f: ManageField) => {
    switch (f.kind) {
      case "text":
        return (
          <label key={f.key} className={m.section}>
            <span className={m.sectionLabel}>{f.label}</span>
            <Input value={String(values[f.key] ?? "")} maxLength={f.max} onChange={(e) => put({ [f.key]: e.target.value })} data-testid={`manage-field-${f.key}`} />
          </label>
        );
      case "area":
        return (
          <label key={f.key} className={m.section}>
            <span className={m.sectionLabel}>{f.label}</span>
            <TextArea className={m.reasonBox} value={String(values[f.key] ?? "")} maxLength={f.max} onChange={(e) => put({ [f.key]: e.target.value })} data-testid={`manage-field-${f.key}`} />
          </label>
        );
      case "number":
        return (
          <label key={f.key} className={m.section}>
            <span className={m.sectionLabel}>{f.label}</span>
            <Input type="number" min={f.min} max={f.max} value={String(values[f.key] ?? 0)} onChange={(e) => put({ [f.key]: Math.min(f.max, Math.max(f.min, Number(e.target.value) || 0)) })} data-testid={`manage-field-${f.key}`} />
          </label>
        );
      case "select":
        return (
          <label key={f.key} className={m.section}>
            <span className={m.sectionLabel}>{f.label}</span>
            <Select value={String(values[f.key] ?? "")} onChange={(e) => put({ [f.key]: e.target.value })} data-testid={`manage-field-${f.key}`}>
              {f.options.map(([v, text]) => (
                <option key={v} value={v}>{text}</option>
              ))}
            </Select>
          </label>
        );
      case "toggle":
        return <Toggle key={f.key} label={f.label} checked={!!values[f.key]} onChange={(on) => put({ [f.key]: on })} />;
      case "colour":
        return (
          <div key={f.key} className={m.section}>
            <span className={m.sectionLabel}>{f.label}</span>
            <ColourInput shown label={f.label} value={String(values[f.key] ?? "#000000")} onChange={(v) => put({ [f.key]: v })} />
          </div>
        );
      case "perms":
        return (
          <div key={f.key} className={m.section}>
            <span className={m.sectionLabel}>{f.label}</span>
            <div className={s.grid}>
              <RoleGrid value={parsePerm(String(values[f.key] ?? "0"))} editable onChange={(v) => put({ [f.key]: permString(v) })} />
            </div>
          </div>
        );
      case "overwrite": {
        const current = f.targets.find((x) => x.id === values.target_id);
        return (
          <div key="overwrite" className={m.section}>
            <label className={m.section}>
              <span className={m.sectionLabel}>{t("moderation.manage.appliesTo")}</span>
              <Select
                value={String(values.target_id ?? "")}
                onChange={(e) => {
                  const next = f.targets.find((x) => x.id === e.target.value);
                  if (next) put({ target_id: next.id, allow: next.allow, deny: next.deny, clear: false });
                }}
                data-testid="manage-field-target"
              >
                {f.targets.map((x) => (
                  <option key={x.id} value={x.id}>{x.name}</option>
                ))}
              </Select>
            </label>
            {current && (current.allow !== "0" || current.deny !== "0") ? <Toggle label={t("moderation.manage.clearEntry")} checked={!!values.clear} onChange={(on) => put({ clear: on })} /> : null}
            {!values.clear ? (
              <div className={s.grid}>
                <OverwriteGrid allow={parsePerm(String(values.allow))} deny={parsePerm(String(values.deny))} editable channelType={f.channelType as ChannelType} onChange={(allow, deny) => put({ allow: permString(allow), deny: permString(deny) })} />
              </div>
            ) : null}
          </div>
        );
      }
    }
  };

  return (
    <Modal open onClose={onClose} bare width={480} ariaLabel={title}>
      <div className={m.modal} data-testid="manage-action">
        <div className={m.modalTop}>
          <div style={{ minWidth: 0 }}>
            <h2 className={m.modalTitle}>{title}</h2>
            {ask.target ? <div className={m.modalTarget}><bdi style={{ overflowWrap: "anywhere" }}>{ask.target}</bdi></div> : null}
          </div>
          <button type="button" className={m.modalClose} onClick={onClose} aria-label={t("chat.attachments.close")}>
            <X size={20} />
          </button>
        </div>
        <div className={m.modalScroll}>
          {ask.fields?.filter((f) => !("when" in f) || !f.when || f.when(values)).map(fieldOf)}
          <div className={m.section}>
            <label className={m.section}>
              <span className={m.sectionLabel}>{t("moderation.form.reason")}</span>
              <TextArea className={m.reasonBox} value={reason} onChange={(e) => setReason(e.target.value)} placeholder={t("moderation.manage.reasonPlaceholder")} maxLength={MAX_REASON + 50} data-testid="manage-reason" />
            </label>
            <div className={m.reasonMeta}>
              <span />
              {shortReason ? <span data-short="">{t("moderation.form.reasonTooShort", { min: 3 })}</span> : <span>{[...reason].length}/{MAX_REASON}</span>}
            </div>
          </div>
          <div className={m.note}>
            <strong className={s.seesLabel}>{t("moderation.manage.sees")}</strong>
            {ask.sees.map((line) => (
              <div key={line}>{line}</div>
            ))}
          </div>
          {error ? <p className={m.danger} role="alert" style={{ margin: 0 }}>{error}</p> : null}
        </div>
        <div className={m.modalFoot}>
          <span className={m.grow} />
          <Button variant="ghost" onClick={onClose}>{t("bugs.form.cancel")}</Button>
          <Button variant={ask.danger ? "danger" : "primary"} loading={busy} disabled={shortReason || longReason || !!missing} onClick={submit} data-testid="manage-confirm">
            {ask.confirm ?? title}
          </Button>
        </div>
      </div>
    </Modal>
  );
}

import { PersonName } from "@/components/common/PersonName";
import { useMediaAnimation } from "@/lib/mediaAnimation";
import { useState } from "react";
import { ApiError } from "@/api/client";
import { bannerUrl, users as usersApi } from "@/api/users";
import { Avatar } from "@/components/common/Avatar";
import { profileTheme } from "@/lib/avatarColor";
import { Button } from "@/components/common/Button";
import { Modal } from "@/components/common/Modal";
import { Field, TextInput } from "@/components/common/TextInput";
import { RecoveryKeyPanel, usernameProblem } from "@/pages/AuthLayout";
import { useAuth } from "@/stores/auth";
import { useUI } from "@/stores/ui";
import { DeleteAccountDialog } from "./DeleteAccountDialog";
import s from "./Settings.module.css";

import { t } from "@/i18n";
import { statusOf, usePresence } from "@/stores/presence";
type Dialog = null | "username" | "displayName" | "password" | "recovery" | "delete";

/** `onEditProfile` opens the Profiles page beside it, in the same settings. */
export default function AccountPage({ onEditProfile }: { onEditProfile: () => void }) {
  useMediaAnimation();
  const me = useAuth((a) => a.me);
  // the status the account actually shows, Do Not Disturb or Invisible included
  const myStatus = usePresence((p) => statusOf(me ? p.byUser[me.id] : undefined));
  const setMe = useAuth((a) => a.setMe);
  const changePassword = useAuth((a) => a.changePassword);
  const newRecoveryKey = useAuth((a) => a.newRecoveryKey);
  const toast = useUI((u) => u.toast);
  const [dialog, setDialog] = useState<Dialog>(null);
  const [value, setValue] = useState("");
  const [pw, setPw] = useState("");
  const [pw2, setPw2] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [recovery, setRecovery] = useState<string | null>(null);
  if (!me) return null;

  const open = (d: Dialog, initial = "") => {
    setDialog(d);
    setValue(initial);
    setPw("");
    setPw2("");
    setError(null);
    setRecovery(null);
  };
  const close = () => setDialog(null);
  const run = async (fn: () => Promise<void>) => {
    setBusy(true);
    setError(null);
    try {
      await fn();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : err instanceof Error ? err.message : t("modals.confirm.somethingWentWrong"));
    } finally {
      setBusy(false);
    }
  };
  const saveName = (field: "username" | "display_name") =>
    run(async () => {
      const v = value.trim();
      if (field === "username") {
        const next = v.toLowerCase();
        const p = usernameProblem(next, next === me.username || (me.short_name_credits ?? 0) > 0);
        if (p) throw new Error(p);
      } else if (!v) throw new Error(t("settings.account.displayNameCannotBeEmpty"));
      setMe(await usersApi.patchMe(field === "username" ? { username: v.toLowerCase() } : { display_name: v }));
      toast(t("settings.account.saved"), "success");
      close();
    });
  const banner = bannerUrl(me.banner_id, 600);
  const submitName = () => {
    if (!busy) void saveName(dialog === "username" ? "username" : "display_name");
  };
  const passwordReady = value.length > 0 && pw.length >= 8 && pw === pw2;
  const submitPassword = () => {
    if (!passwordReady || busy) return;
    void run(async () => {
      await changePassword(value, pw);
      toast(t("settings.account.passwordChangedOtherDevicesWereLogged"), "success");
      close();
    });
  };
  const generate = () => {
    if (!value || busy) return;
    void run(async () => setRecovery(await newRecoveryKey(value)));
  };

  return (
    <>
      <h2 data-setting="account" className={s.title}>{t("settings.account.myAccount")}</h2>
      <div className={s.card}>
        {/* The banner everyone else sees on the profile card: its colours, or
            without them the avatar's own colour. Here it used to fall back to
            the app's accent at full strength, a third of the page of raw purple
            that no other screen showed for this account. */}
        <div className={s.cardBanner} style={banner ? { backgroundImage: `url(${banner})` } : { background: profileTheme(me.accent_color, me.accent_color_2, me.id) }} />
        <div className={s.cardHead}>
          <div className={s.cardAvatar}><Avatar id={me.id} avatarId={me.avatar_id} name={me.display_name} size={80} status={myStatus} /></div>
          <div className={s.cardText}>
            <div className={s.cardName}><PersonName user={me}>{me.display_name}</PersonName></div>
            <div className={s.cardUser}>{me.username}</div>
          </div>
          <Button size="sm" onClick={onEditProfile}>{t("settings.account.editUserProfile")}</Button>
        </div>
        <div className={s.rows}>
          <div className={s.row}>
            <div><div className={s.rowLabel}>{t("settings.account.displayName")}</div><div className={s.rowValue}><PersonName user={me}>{me.display_name}</PersonName></div></div>
            <Button size="sm" variant="secondary" onClick={() => open("displayName", me.display_name)}>{t("pickers.stickerPicker.edit")}</Button>
          </div>
          <div className={s.row}>
            <div><div className={s.rowLabel}>{t("settings.account.username")}</div><div className={s.rowValue}>{me.username}</div></div>
            <Button size="sm" variant="secondary" onClick={() => open("username", me.username)}>{t("pickers.stickerPicker.edit")}</Button>
          </div>
        </div>
      </div>

      <div className={s.sep} />
      <h3 data-setting="password" className={s.h5}>{t("settings.account.passwordAndAuthentication")}</h3>
      <p className={s.hint}>{t("settings.account.yourPasswordProtectsYourAccountAnd")}</p>
      <div style={{ marginTop: 12, display: "flex", gap: 8, flexWrap: "wrap" }}>
        <Button onClick={() => open("password")}>{t("settings.account.changePassword")}</Button>
        <Button variant="secondary" onClick={() => open("recovery")}>{me.has_recovery ? t("settings.account.generateANewRecoveryKey") : t("settings.account.setUpARecoveryKey")}</Button>
      </div>
      <p className={s.hint}>{me.has_recovery ? t("settings.account.aRecoveryKeyIsSetGenerating") : t("settings.account.withoutARecoveryKeyAForgotten")}</p>

      <div className={s.sep} />
      <h3 data-setting="deleteAccount" className={s.h5}>{t("settings.account.accountRemoval")}</h3>
      <div className={s.dangerBox}>
        <span className={s.hint} style={{ margin: 0 }}>{t("settings.account.deletingYourAccountIsPermanentYour")}</span>
        <Button variant="danger" onClick={() => open("delete")}>{t("settings.account.deleteAccount")}</Button>
      </div>

      {dialog === "displayName" || dialog === "username" ? (
        <Modal open onClose={close} title={dialog === "username" ? t("settings.account.changeYourUsername") : t("settings.account.changeYourDisplayName")} onSubmit={submitName} footer={<><Button variant="ghost" onClick={close}>{t("chat.editMessage.cancel")}</Button><Button loading={busy} onClick={submitName}>{t("modals.serverNotifications.done")}</Button></>}>
          <Field label={dialog === "username" ? t("settings.account.username") : t("settings.account.displayName")} error={error} hint={dialog === "username" && (me.short_name_credits ?? 0) > 0 ? t("settings.account.shortNameHint") : undefined}>
            <TextInput value={value} onChange={(e) => setValue(e.target.value)} autoFocus maxLength={32} autoCapitalize={dialog === "username" ? "none" : undefined} enterKeyHint="done" />
          </Field>
        </Modal>
      ) : null}

      {dialog === "password" ? (
        <Modal open onClose={close} title={t("settings.account.updateYourPassword")} onSubmit={submitPassword} footer={<><Button variant="ghost" onClick={close}>{t("chat.editMessage.cancel")}</Button><Button loading={busy} disabled={!passwordReady} onClick={submitPassword}>{t("modals.serverNotifications.done")}</Button></>}>
          <div className={s.form}>
            {error ? <p className={s.hint} style={{ color: "var(--text-danger)" }} role="alert">{error}</p> : null}
            <Field label={t("settings.account.currentPassword")} required><TextInput type="password" reveal value={value} onChange={(e) => setValue(e.target.value)} autoComplete="current-password" autoFocus /></Field>
            <Field label={t("settings.account.newPassword")} required hint={t("settings.account.atLeast8Characters")}><TextInput type="password" reveal value={pw} onChange={(e) => setPw(e.target.value)} autoComplete="new-password" /></Field>
            <Field label={t("settings.account.confirmNewPassword")} required error={pw2 && pw2 !== pw ? t("settings.account.passwordsDoNotMatch") : null}><TextInput type="password" reveal value={pw2} onChange={(e) => setPw2(e.target.value)} autoComplete="new-password" enterKeyHint="done" /></Field>
          </div>
        </Modal>
      ) : null}

      {dialog === "recovery" ? (
        // Once the key is on screen the old one has stopped working, so only the
        // panel's own Done puts it away: Escape, a tap outside and the close
        // button each used to lose the only copy there would ever be.
        <Modal
          open
          onClose={recovery ? () => undefined : close}
          closeOnOverlay={!recovery}
          closeButton={!recovery}
          title={recovery ? t("register.saveYourRecoveryKey") : t("settings.account.recoveryKey")}
          onSubmit={recovery ? undefined : generate}
          footer={recovery ? undefined : <><Button variant="ghost" onClick={close}>{t("chat.editMessage.cancel")}</Button><Button loading={busy} disabled={!value} onClick={generate}>{t("settings.account.generate")}</Button></>}
        >
          {recovery ? (
            <div className={s.form}>
              <RecoveryKeyPanel value={recovery} username={me.username} warning={t("settings.account.newKeyWarning")} confirm={t("register.iHaveSavedMyRecoveryKey")} doneLabel={t("modals.serverNotifications.done")} onDone={close} />
            </div>
          ) : (
            <div className={s.form}>
              {error ? <p className={s.hint} style={{ color: "var(--text-danger)" }} role="alert">{error}</p> : null}
              <p className={s.hint}>{t("settings.account.confirmYourPasswordToGenerateA")}</p>
              <Field label={t("chat.lockedChannel.password")} required><TextInput type="password" reveal value={value} onChange={(e) => setValue(e.target.value)} autoComplete="current-password" autoFocus enterKeyHint="done" /></Field>
            </div>
          )}
        </Modal>
      ) : null}

      {dialog === "delete" ? <DeleteAccountDialog onClose={close} /> : null}
    </>
  );
}

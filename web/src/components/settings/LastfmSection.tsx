import { Unplug } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { ApiError } from "@/api/client";
import { connections } from "@/api/connections";
import { ActivityCard } from "@/components/activity/ActivityCard";
import { Button } from "@/components/common/Button";
import { Field, TextInput } from "@/components/common/TextInput";
import { t } from "@/i18n";
import { describeError } from "@/lib/errors";
import { useAuth } from "@/stores/auth";
import { useLastfm } from "@/stores/lastfm";
import { usePresence } from "@/stores/presence";
import { useUI } from "@/stores/ui";
import a from "./Activity.module.css";
import st from "./Settings.module.css";

const AUDIOSCROBBLER = "https://www.last.fm/api";

function linkError(err: unknown): string {
  if (err instanceof ApiError && err.code === "lastfm_not_found") return t("settings.activity.lastfmNotFound");
  if (err instanceof ApiError && err.code === "lastfm_private") return t("settings.activity.lastfmPrivate");
  return describeError(err, t("settings.activity.lastfmFailed"));
}

/** Settings, Activity: showing what a Last.fm account is playing. Not there unless this server has a key. */
export function LastfmSection({ share }: { share: boolean }) {
  const state = useLastfm((x) => x.state);
  const [name, setName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    void useLastfm.getState().load();
  }, []);
  if (!state?.available) return null;
  const linked = state.linked;

  const connect = async () => {
    setBusy(true);
    setError(null);
    try {
      useLastfm.getState().set(await connections.linkLastfm(name.trim(), share));
      setName("");
    } catch (err) {
      setError(linkError(err));
    } finally {
      setBusy(false);
    }
  };
  const disconnect = async () => {
    setBusy(true);
    try {
      useLastfm.getState().set(await connections.unlinkLastfm());
      useUI.getState().toast(t("settings.activity.lastfmUnlinked"), "success");
    } catch (err) {
      useUI.getState().toast(describeError(err, t("settings.activity.lastfmFailed")), "error");
    } finally {
      setBusy(false);
    }
  };
  const credit = (
    <p className={st.hint}>
      <a href={AUDIOSCROBBLER} target="_blank" rel="noreferrer noopener">{t("settings.activity.lastfmPowered")}</a>
    </p>
  );
  return (
    <>
      <h3 data-setting="lastfm" className={st.h5}>{t("settings.activity.lastfm")}</h3>
      {!linked ? (
        <form className={a.connect} onSubmit={(e) => { e.preventDefault(); if (name.trim() && !busy) void connect(); }}>
          <p className={a.lead}>{t("settings.activity.lastfmIntro")}</p>
          <p className={st.hint}>{t("settings.activity.lastfmHow")}</p>
          <Field label={t("settings.activity.lastfmUsername")} error={error}>
            <TextInput value={name} onChange={(e) => setName(e.target.value)} maxLength={15} autoComplete="off" spellCheck={false} data-testid="lastfm-username" />
          </Field>
          <div>
            <Button type="submit" loading={busy} disabled={!name.trim()} data-testid="lastfm-connect">{t("settings.activity.lastfmConnect")}</Button>
          </div>
          {credit}
        </form>
      ) : (
        <div className={st.form}>
          <div className={a.account} data-testid="lastfm-account">
            <div className={a.names}>
              <a className={a.name} href={linked.url} target="_blank" rel="noreferrer noopener">{linked.username}</a>
              <span className={a.handle}>{t("settings.activity.lastfm")}</span>
            </div>
            <Button variant="secondary" size="sm" loading={busy} onClick={() => void disconnect()} data-testid="lastfm-unlink">
              <Unplug size={16} /> {t("settings.activity.disconnect")}
            </Button>
          </div>
          <LastfmNow />
          <p className={st.hint}>{t("settings.activity.lastfmPrivacy")}</p>
          {credit}
        </div>
      )}
    </>
  );
}

function LastfmNow() {
  const meId = useAuth((x) => x.me?.id);
  const acts = usePresence((p) => (meId ? p.byUser[meId]?.activities : undefined));
  const playing = useMemo(() => (acts ?? []).filter((x) => x.source === "lastfm"), [acts]);
  if (playing.length === 0) return null;
  return (
    <div className={a.now}>
      <p className={a.sub}>{t("settings.activity.showingNow")}</p>
      {playing.map((x) => <ActivityCard key={x.created_at} activity={x} />)}
    </div>
  );
}

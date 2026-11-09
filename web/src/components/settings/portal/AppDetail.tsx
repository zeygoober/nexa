import { useMediaAnimation } from "@/lib/mediaAnimation";
import { ChevronLeft, ImagePlus, Plus, Trash2, X } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { apps as appsApi, type AppCommand, type AppPatch, type IntegrationTypes, type OwnedApp } from "@/api/apps";
import type { PublicUser } from "@/api/types";
import { avatarUrl, iconUrl } from "@/api/users";
import { AppIcon } from "@/components/apps/AppIcon";
import { PermissionPicker } from "@/components/apps/PermissionPicker";
import { ReadingGlass } from "@/components/apps/ReadingGlass";
import { HookTags } from "@/components/chat/webhook/HookHeader";
import { Avatar } from "@/components/common/Avatar";
import { Button } from "@/components/common/Button";
import { ImageCropModal } from "@/components/common/ImageCropModal";
import { Select } from "@/components/common/Select";
import { Field, TextArea, TextInput } from "@/components/common/TextInput";
import { Toggle } from "@/components/common/Toggle";
import { ConfirmModal } from "@/components/modals/ConfirmModal";
import { SaveBar } from "@/components/server/SettingsLayer";
import { t } from "@/i18n";
import { commandDescription, commandLeaves, optionName } from "@/lib/apps";
import { describeError } from "@/lib/errors";
import { P, parsePerm, permString } from "@/lib/perms";
import { useUI } from "@/stores/ui";
import { useUsers } from "@/stores/users";
import d from "../Designer.module.css";
import st from "../Settings.module.css";
import { serverBase } from "./DeveloperPortalPage";
import { CopyButton, IdChip, PasswordConfirm, SecretReveal } from "./portalParts";
import s from "./Portal.module.css";

type Tab = "general" | "bot" | "install" | "oauth" | "commands";

/** What has been changed on an app and not saved yet: its own fields, and its bot's. */
interface Draft {
  app: AppPatch;
  bot: { username?: string; avatar?: string | null };
}

const EMPTY: Draft = { app: {}, bot: {} };

function blobToDataURL(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const r = new FileReader();
    r.onload = () => resolve(String(r.result));
    r.onerror = () => reject(r.error ?? new Error("read failed"));
    r.readAsDataURL(blob);
  });
}

/** The OAuth2 scopes an app can ask for, with what each lets it do, for the link builder. */
export function scopeList(): { id: string; label: string }[] {
  return [
    { id: "bot", label: t("apps.scope.bot") },
    { id: "applications.commands", label: t("apps.scope.commands") },
    { id: "identify", label: t("apps.scope.identify") },
    { id: "email", label: t("apps.scope.email") },
    { id: "guilds", label: t("apps.scope.guilds") },
    { id: "guilds.join", label: t("apps.scope.guildsJoin") },
    { id: "guilds.members.read", label: t("apps.scope.membersRead") },
    { id: "connections", label: t("apps.scope.connections") },
    { id: "applications.commands.permissions.update", label: t("apps.scope.commandPermissions") },
    { id: "webhook.incoming", label: t("apps.scope.webhook") },
  ];
}

/** The link that adds an app, as its settings make it. */
export function installLink(app: Pick<OwnedApp, "protocol_id">, opts: { scopes?: string[]; permissions?: string; integrationType?: number; redirect?: string } = {}): string {
  const q = new URLSearchParams({ client_id: app.protocol_id });
  if (opts.permissions && opts.permissions !== "0") q.set("permissions", opts.permissions);
  if (opts.integrationType !== undefined) q.set("integration_type", String(opts.integrationType));
  if (opts.scopes?.length) q.set("scope", opts.scopes.join(" "));
  if (opts.redirect) {
    q.set("redirect_uri", opts.redirect);
    q.set("response_type", "code");
  }
  return `${serverBase()}/oauth2/authorize?${q.toString().replace(/\+/g, "%20")}`;
}

/**
 * One app in the Developer Portal: who it is at the top (its picture, name
 * and the id libraries know it by), then its settings by tab. Changes wait in
 * a draft until saved; a new token, a new secret and deleting the app ask for
 * the account's password first.
 */
export function AppDetail({ app, onBack, onChange, onDeleted }: { app: OwnedApp; onBack: () => void; onChange: (app: OwnedApp) => void; onDeleted: () => void }) {
  const [tab, setTab] = useState<Tab>("general");
  const [draft, setDraft] = useState<Draft>(EMPTY);
  const [busy, setBusy] = useState(false);
  const toast = useUI((u) => u.toast);
  const dirty = Object.keys(draft.app).length > 0 || Object.keys(draft.bot).length > 0;
  const setApp = (patch: AppPatch) => setDraft((cur) => ({ ...cur, app: { ...cur.app, ...patch } }));
  const setBot = (patch: Draft["bot"]) => setDraft((cur) => ({ ...cur, bot: { ...cur.bot, ...patch } }));
  const save = async () => {
    setBusy(true);
    try {
      let next = app;
      if (Object.keys(draft.app).length) next = await appsApi.update(app.id, draft.app);
      if (Object.keys(draft.bot).length) {
        const bot = await appsApi.updateBot(app.id, draft.bot);
        next = { ...next, bot };
        useUsers.getState().upsert([bot]);
      }
      onChange(next);
      setDraft(EMPTY);
      toast(t("apps.portal.saved"), "success");
    } catch (err) {
      toast(describeError(err, t("apps.portal.saveFailed")), "error");
    } finally {
      setBusy(false);
    }
  };
  const tabs: { id: Tab; label: string }[] = [
    { id: "general", label: t("apps.portal.tab.general") },
    { id: "bot", label: t("apps.portal.tab.bot") },
    { id: "install", label: t("apps.portal.tab.install") },
    { id: "oauth", label: t("apps.portal.tab.oauth") },
    { id: "commands", label: t("apps.portal.tab.commands") },
  ];
  const name = draft.app.name ?? app.name;
  return (
    <>
      <button type="button" className={s.back} onClick={onBack} data-testid="portal-back">
        <ChevronLeft size={18} aria-hidden="true" /> {t("apps.portal.allApps")}
      </button>
      <div className={s.head}>
        <span className={s.headIcon}>
          <AppIcon app={app} size={64} square />
        </span>
        <div className={s.headText}>
          <h2 className={s.headName}>
            <span dir="auto">{name}</span>
            <HookTags app />
          </h2>
          <div className={s.ids}>
            <IdChip label={t("apps.portal.applicationId")} value={app.protocol_id} testId="app-id" />
          </div>
        </div>
      </div>
      <div className={d.scopeTabs} role="tablist" aria-label={name}>
        {tabs.map((x) => (
          <button key={x.id} type="button" role="tab" className={d.scopeTab} aria-selected={tab === x.id} onClick={() => setTab(x.id)} data-testid={`portal-tab-${x.id}`}>
            {x.label}
          </button>
        ))}
      </div>
      {tab === "general" ? <GeneralTab app={app} draft={draft} setApp={setApp} onDeleted={onDeleted} /> : null}
      {tab === "bot" ? <BotTab app={app} draft={draft} setApp={setApp} setBot={setBot} /> : null}
      {tab === "install" ? <InstallTab app={app} draft={draft} setApp={setApp} /> : null}
      {tab === "oauth" ? <OAuthTab app={app} draft={draft} setApp={setApp} onChange={onChange} /> : null}
      {tab === "commands" ? <CommandsTab app={app} /> : null}
      <SaveBar dirty={dirty} busy={busy} onReset={() => setDraft(EMPTY)} onSave={() => void save()} />
    </>
  );
}

interface TabProps {
  app: OwnedApp;
  draft: Draft;
  setApp: (patch: AppPatch) => void;
}

/** Picks a square picture, cropped, as image data; `null` when it is taken away. */
function PicturePick({ shown, round, onPick, onRemove, canRemove, label }: { shown: React.ReactNode; round?: boolean; onPick: (data: string) => void; onRemove: () => void; canRemove: boolean; label: string }) {
  const input = useRef<HTMLInputElement>(null);
  const [crop, setCrop] = useState<File | null>(null);
  return (
    <div className={s.iconRow}>
      <span className={s.iconPick} style={round ? { borderRadius: "50%" } : undefined}>{shown}</span>
      <div className={s.iconBtns}>
        <Button size="sm" variant="secondary" onClick={() => input.current?.click()}>
          <ImagePlus size={15} aria-hidden="true" /> {label}
        </Button>
        {canRemove ? (
          <Button size="sm" variant="ghost" onClick={onRemove}>{t("apps.portal.removeImage")}</Button>
        ) : null}
      </div>
      <input
        ref={input}
        type="file"
        hidden
        accept="image/png,image/jpeg,image/gif,image/webp"
        onChange={(e) => {
          const f = e.target.files?.[0];
          if (f) setCrop(f);
          e.target.value = "";
        }}
      />
      {crop ? (
        <ImageCropModal
          file={crop}
          aspect={1}
          round={!!round}
          maxSize={512}
          title={t("apps.portal.cropImage")}
          onClose={() => setCrop(null)}
          onDone={({ blob }) => {
            setCrop(null);
            void blobToDataURL(blob).then(onPick);
          }}
        />
      ) : null}
    </div>
  );
}

function GeneralTab({ app, draft, setApp, onDeleted }: TabProps & { onDeleted: () => void }) {
  useMediaAnimation();
  const [deleting, setDeleting] = useState(false);
  const [tag, setTag] = useState("");
  const v = draft.app;
  const icon = v.icon !== undefined ? v.icon : undefined;
  const tags = v.tags ?? app.tags;
  const types = v.event_webhooks_types ?? app.event_webhooks_types;
  const addTag = () => {
    const x = tag.trim();
    if (!x || tags.length >= 5 || tags.includes(x)) return setTag("");
    setApp({ tags: [...tags, x.slice(0, 20)] });
    setTag("");
  };
  const toggleType = (type: string) => setApp({ event_webhooks_types: types.includes(type) ? types.filter((x) => x !== type) : [...types, type] });
  return (
    <div className={st.form}>
      <PicturePick
        label={t("apps.portal.uploadIcon")}
        shown={icon ? <img src={icon} alt="" /> : icon === null || !app.icon_id ? <AppIcon app={{ ...app, icon_id: null }} size={96} square /> : <img src={iconUrl(app.icon_id, 128) ?? ""} alt="" />}
        canRemove={icon === undefined ? !!app.icon_id : icon !== null}
        onPick={(data) => setApp({ icon: data })}
        onRemove={() => setApp({ icon: app.icon_id ? null : undefined })}
      />
      <Field label={t("apps.portal.name")} required hint={t("apps.portal.nameHint")}>
        <TextInput value={v.name ?? app.name} maxLength={32} onChange={(e) => setApp({ name: e.target.value })} data-testid="app-field-name" />
      </Field>
      <Field label={t("apps.portal.description")} hint={t("apps.portal.descriptionHint")}>
        <TextArea value={v.description ?? app.description} maxLength={400} rows={3} onChange={(e) => setApp({ description: e.target.value })} />
      </Field>
      <Field label={t("apps.portal.tags")} hint={t("apps.portal.tagsHint")} as="div">
        <div className={s.tags}>
          {tags.map((x) => (
            <span key={x} className={s.tag}>
              <span>{x}</span>
              <button type="button" aria-label={t("apps.portal.removeTag", { tag: x })} onClick={() => setApp({ tags: tags.filter((y) => y !== x) })}>
                <X size={13} />
              </button>
            </span>
          ))}
          {tags.length < 5 ? (
            <input
              value={tag}
              maxLength={20}
              placeholder={t("apps.portal.addTag")}
              aria-label={t("apps.portal.addTag")}
              onChange={(e) => setTag(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" || e.key === ",") {
                  e.preventDefault();
                  addTag();
                }
              }}
              onBlur={addTag}
            />
          ) : null}
        </div>
      </Field>
      <h3 className={st.h5}>{t("apps.portal.interactions")}</h3>
      <Field label={t("apps.portal.endpoint")} hint={t("apps.portal.endpointHint")}>
        <TextInput className={s.mono} value={v.interactions_url ?? app.interactions_url} placeholder="https://" inputMode="url" onChange={(e) => setApp({ interactions_url: e.target.value })} data-testid="app-endpoint" />
      </Field>
      <div className={s.ids}>
        <IdChip label={t("apps.portal.publicKey")} value={app.verify_key} />
      </div>
      <h3 className={st.h5}>{t("apps.portal.links")}</h3>
      <div className={s.twoFields}>
        <Field label={t("apps.portal.terms")}>
          <TextInput value={v.terms_url ?? app.terms_url} placeholder="https://" inputMode="url" onChange={(e) => setApp({ terms_url: e.target.value })} />
        </Field>
        <Field label={t("apps.portal.privacy")}>
          <TextInput value={v.privacy_url ?? app.privacy_url} placeholder="https://" inputMode="url" onChange={(e) => setApp({ privacy_url: e.target.value })} />
        </Field>
      </div>
      <h3 className={st.h5}>{t("apps.portal.events")}</h3>
      <p className={st.hint} style={{ marginTop: -6 }}>{t("apps.portal.eventsHint")}</p>
      <Field label={t("apps.portal.eventsUrl")}>
        <TextInput className={s.mono} value={v.event_webhooks_url ?? app.event_webhooks_url} placeholder="https://" inputMode="url" onChange={(e) => setApp({ event_webhooks_url: e.target.value })} />
      </Field>
      <Toggle label={t("apps.portal.eventsOn")} checked={v.event_webhooks_enabled ?? app.event_webhooks_enabled} onChange={(on) => setApp({ event_webhooks_enabled: on })} />
      <div className={s.checks}>
        <label className={s.check}>
          <input type="checkbox" checked={types.includes("APPLICATION_AUTHORIZED")} onChange={() => toggleType("APPLICATION_AUTHORIZED")} />
          {t("apps.portal.eventAdded")}
        </label>
        <label className={s.check}>
          <input type="checkbox" checked={types.includes("APPLICATION_DEAUTHORIZED")} onChange={() => toggleType("APPLICATION_DEAUTHORIZED")} />
          {t("apps.portal.eventRemoved")}
        </label>
      </div>
      <h3 className={st.h5}>{t("apps.portal.dangerZone")}</h3>
      <div className={st.dangerBox}>
        <div className={s.rowText}>
          <span className={s.rowTitle}>{t("apps.portal.deleteApp")}</span>
          <span className={s.rowHint}>{t("apps.portal.deleteHint")}</span>
        </div>
        <Button variant="danger" onClick={() => setDeleting(true)} data-testid="app-delete">
          <Trash2 size={15} aria-hidden="true" /> {t("apps.portal.deleteApp")}
        </Button>
      </div>
      {deleting ? (
        <PasswordConfirm
          title={t("apps.portal.deleteNamed", { name: app.name })}
          body={t("apps.portal.deleteBody")}
          confirm={t("apps.portal.deleteApp")}
          danger
          run={async (key) => {
            await appsApi.remove(app.id, key);
            onDeleted();
          }}
          onClose={() => setDeleting(false)}
        />
      ) : null}
    </div>
  );
}

function BotTab({ app, draft, setApp, setBot }: TabProps & { setBot: (patch: Draft["bot"]) => void }) {
  useMediaAnimation();
  const [resetting, setResetting] = useState(false);
  const [token, setToken] = useState<string | null>(null);
  const v = draft.app;
  const intents = { ...app.intents, ...v.intents };
  const avatar = draft.bot.avatar;
  const bot: PublicUser = app.bot;
  const name = v.name ?? app.name;
  return (
    <div className={st.form}>
      <PicturePick
        round
        label={t("apps.portal.uploadAvatar")}
        shown={avatar ? <img src={avatar} alt="" /> : avatar === null || !bot.avatar_id ? <Avatar id={bot.id} avatarId={null} name={name} size={96} /> : <img src={avatarUrl(bot.avatar_id, 128) ?? ""} alt="" />}
        canRemove={avatar === undefined ? !!bot.avatar_id : avatar !== null}
        onPick={(data) => setBot({ avatar: data })}
        onRemove={() => setBot({ avatar: bot.avatar_id ? null : undefined })}
      />
      <Field label={t("apps.portal.username")} hint={t("apps.portal.usernameHint")}>
        <TextInput value={draft.bot.username ?? bot.username} maxLength={32} autoCapitalize="none" spellCheck={false} onChange={(e) => setBot({ username: e.target.value.toLowerCase() })} />
      </Field>
      <h3 className={st.h5}>{t("apps.portal.token")}</h3>
      {token ? (
        <SecretReveal value={token} note={t("apps.portal.tokenNote")} onDone={() => setToken(null)} testId="bot-token" />
      ) : (
        <div className={s.rowLine}>
          <div className={s.rowText}>
            <span className={s.rowHint}>{t("apps.portal.tokenHint")}</span>
          </div>
          <Button variant="secondary" onClick={() => setResetting(true)} data-testid="token-reset">{t("apps.portal.resetToken")}</Button>
        </div>
      )}
      <h3 className={st.h5}>{t("apps.portal.whoCanAdd")}</h3>
      <Toggle label={t("apps.portal.public")} description={t("apps.portal.publicHint")} checked={v.bot_public ?? app.bot_public} onChange={(on) => setApp({ bot_public: on })} />
      <Toggle
        label={t("apps.portal.discovery")}
        description={app.discovery_hidden ? t("apps.portal.discoveryHidden") : (v.bot_public ?? app.bot_public) ? t("apps.portal.discoveryHint") : t("apps.portal.discoveryNeedsPublic")}
        checked={(v.discoverable ?? app.discoverable) && (v.bot_public ?? app.bot_public) && !app.discovery_hidden}
        disabled={app.discovery_hidden || !(v.bot_public ?? app.bot_public)}
        onChange={(on) => setApp({ discoverable: on })}
      />
      <Toggle label={t("apps.portal.codeGrant")} description={t("apps.portal.codeGrantHint")} checked={v.bot_require_code_grant ?? app.bot_require_code_grant} onChange={(on) => setApp({ bot_require_code_grant: on })} />
      <h3 className={st.h5}>{t("apps.portal.privileged")}</h3>
      <Toggle label={t("apps.portal.presence")} description={t("apps.portal.presenceHint")} checked={intents.presence} onChange={(on) => setApp({ intents: { ...v.intents, presence: on } })} />
      <Toggle label={t("apps.portal.members")} description={t("apps.portal.membersHint")} checked={intents.members} onChange={(on) => setApp({ intents: { ...v.intents, members: on } })} />
      <h3 className={st.h5}>{t("apps.glass.title", { app: name })}</h3>
      <ReadingGlass name={name} everything={intents.message_content} onChange={(on) => setApp({ intents: { ...v.intents, message_content: on } })} />
      {resetting ? (
        <PasswordConfirm
          title={t("apps.portal.resetToken")}
          body={t("apps.portal.resetTokenBody")}
          confirm={t("apps.portal.resetToken")}
          run={async (key) => {
            const res = await appsApi.resetToken(app.id, key);
            setToken(res.token);
          }}
          onClose={() => setResetting(false)}
        />
      ) : null}
    </div>
  );
}

function InstallTab({ app, draft, setApp }: TabProps) {
  const types: IntegrationTypes = draft.app.integration_types ?? app.integration_types;
  const guild = types["0"];
  const user = types["1"];
  const guildParams = guild?.oauth2_install_params ?? app.install_params ?? { scopes: ["bot", "applications.commands"], permissions: "0" };
  const setTypes = (next: IntegrationTypes) => {
    // the older single setting follows the server one, for the libraries and links that read it
    const g = next["0"]?.oauth2_install_params;
    setApp({ integration_types: next, ...(g ? { install_params: g } : {}) });
  };
  const toggle = (k: "0" | "1") => {
    const next = { ...types };
    if (next[k]) delete next[k];
    else next[k] = k === "0" ? { oauth2_install_params: guildParams } : { oauth2_install_params: { scopes: ["applications.commands"], permissions: "0" } };
    if (!next["0"] && !next["1"]) return;
    setTypes(next);
  };
  const setGuild = (patch: Partial<{ scopes: string[]; permissions: string }>) => setTypes({ ...types, "0": { oauth2_install_params: { ...guildParams, ...patch } } });
  const scopes = guildParams.scopes;
  const toggleScope = (sc: string) => setGuild({ scopes: scopes.includes(sc) ? scopes.filter((x) => x !== sc) : [...scopes, sc] });
  return (
    <div className={st.form}>
      <h3 className={st.h5} style={{ marginTop: 0 }}>{t("apps.portal.whereAdded")}</h3>
      <label className={st.radio} data-on={!!guild || undefined}>
        <input type="checkbox" checked={!!guild} onChange={() => toggle("0")} data-testid="install-guild" />
        <span className={st.radioText}>
          <span className={st.radioLabel}>{t("apps.portal.installServers")}</span>
          <span className={st.radioDesc}>{t("apps.portal.installServersHint")}</span>
        </span>
      </label>
      <label className={st.radio} data-on={!!user || undefined}>
        <input type="checkbox" checked={!!user} onChange={() => toggle("1")} data-testid="install-user" />
        <span className={st.radioText}>
          <span className={st.radioLabel}>{t("apps.portal.installAccounts")}</span>
          <span className={st.radioDesc}>{t("apps.portal.installAccountsHint")}</span>
        </span>
      </label>
      <h3 className={st.h5}>{t("apps.portal.installLink")}</h3>
      <p className={st.hint} style={{ marginTop: -6 }}>{t("apps.portal.installLinkHint")}</p>
      <div className={s.generated}>
        <code>{installLink(app)}</code>
        <CopyButton value={installLink(app)} label={t("apps.portal.copyLink")} done={t("apps.portal.linkCopied")} />
      </div>
      {guild ? (
        <>
          <h3 className={st.h5}>{t("apps.portal.serverDefaults")}</h3>
          <div className={s.checks}>
            {["bot", "applications.commands"].map((sc) => (
              <label key={sc} className={s.check}>
                <input type="checkbox" checked={scopes.includes(sc)} onChange={() => toggleScope(sc)} />
                {sc === "bot" ? t("apps.scope.bot") : t("apps.scope.commands")} <code>{sc}</code>
              </label>
            ))}
          </div>
          {scopes.includes("bot") ? (
            <>
              <p className={st.hint}>{t("apps.portal.permissionsHint")}</p>
              <PermissionPicker value={parsePerm(guildParams.permissions)} onChange={(p) => setGuild({ permissions: permString(p) })} />
            </>
          ) : null}
        </>
      ) : null}
      <h3 className={st.h5}>{t("apps.portal.customInstall")}</h3>
      <Field label={t("apps.portal.customInstallUrl")} hint={t("apps.portal.customInstallHint")}>
        <TextInput className={s.mono} value={draft.app.custom_install_url ?? app.custom_install_url} placeholder="https://" inputMode="url" onChange={(e) => setApp({ custom_install_url: e.target.value })} />
      </Field>
    </div>
  );
}

function OAuthTab({ app, draft, setApp, onChange }: TabProps & { onChange: (app: OwnedApp) => void }) {
  const [resetting, setResetting] = useState(false);
  const [secret, setSecret] = useState<string | null>(null);
  const redirects = draft.app.redirect_uris ?? app.redirect_uris;
  const [scopes, setScopes] = useState<string[]>(["bot", "applications.commands"]);
  const [perms, setPerms] = useState<bigint>(0n);
  const [redirect, setRedirect] = useState("");
  const [userInstall, setUserInstall] = useState(false);
  const canUser = !!app.integration_types["1"];
  const needsRedirect = scopes.some((x) => x !== "bot" && x !== "applications.commands");
  const link = useMemo(
    () => installLink(app, { scopes, permissions: scopes.includes("bot") ? permString(perms) : undefined, integrationType: userInstall ? 1 : undefined, redirect: needsRedirect ? redirect || redirects[0] : undefined }),
    [app, scopes, perms, userInstall, needsRedirect, redirect, redirects],
  );
  useEffect(() => {
    if (userInstall) setScopes((cur) => cur.filter((x) => x !== "bot" && x !== "webhook.incoming"));
  }, [userInstall]);
  return (
    <div className={st.form}>
      <div className={s.ids}>
        <IdChip label={t("apps.portal.clientId")} value={app.protocol_id} />
      </div>
      <h3 className={st.h5}>{t("apps.portal.clientSecret")}</h3>
      {secret ? (
        <SecretReveal value={secret} note={t("apps.portal.secretNote")} onDone={() => setSecret(null)} testId="client-secret" />
      ) : (
        <div className={s.rowLine}>
          <div className={s.rowText}>
            <span className={s.rowHint}>{app.has_secret ? t("apps.portal.secretSet") : t("apps.portal.secretNone")}</span>
          </div>
          <Button variant="secondary" onClick={() => setResetting(true)}>{app.has_secret ? t("apps.portal.resetSecret") : t("apps.portal.makeSecret")}</Button>
        </div>
      )}
      <h3 className={st.h5}>{t("apps.portal.redirects")}</h3>
      <p className={st.hint} style={{ marginTop: -6 }}>{t("apps.portal.redirectsHint")}</p>
      <div className={s.urlList}>
        {redirects.map((u, i) => (
          <div key={i} className={s.urlRow}>
            <TextInput className={s.mono} value={u} placeholder="https://" inputMode="url" aria-label={t("apps.portal.redirectN", { n: i + 1 })} onChange={(e) => setApp({ redirect_uris: redirects.map((x, j) => (j === i ? e.target.value : x)) })} />
            <Button size="sm" variant="ghost" aria-label={t("apps.portal.removeRedirect")} onClick={() => setApp({ redirect_uris: redirects.filter((_, j) => j !== i) })}>
              <Trash2 size={15} />
            </Button>
          </div>
        ))}
        {redirects.length < 10 ? (
          <div>
            <Button size="sm" variant="secondary" onClick={() => setApp({ redirect_uris: [...redirects, ""] })}>
              <Plus size={15} aria-hidden="true" /> {t("apps.portal.addRedirect")}
            </Button>
          </div>
        ) : null}
      </div>
      <h3 className={st.h5}>{t("apps.portal.linkBuilder")}</h3>
      <p className={st.hint} style={{ marginTop: -6 }}>{t("apps.portal.linkBuilderHint")}</p>
      {canUser ? (
        <Toggle label={t("apps.portal.forAccounts")} checked={userInstall} onChange={setUserInstall} />
      ) : null}
      <div className={s.checks}>
        {scopeList()
          .filter((x) => !(userInstall && (x.id === "bot" || x.id === "webhook.incoming")))
          .map((x) => (
            <label key={x.id} className={s.check} title={x.id}>
              <input type="checkbox" checked={scopes.includes(x.id)} onChange={() => setScopes((cur) => (cur.includes(x.id) ? cur.filter((y) => y !== x.id) : [...cur, x.id]))} />
              {x.label}
            </label>
          ))}
      </div>
      {needsRedirect ? (
        <Field label={t("apps.portal.redirect")} error={redirects.filter(Boolean).length ? null : t("apps.portal.redirectNeeded")}>
          <Select value={redirect || redirects[0] || ""} onChange={(e) => setRedirect(e.target.value)}>
            {redirects.filter(Boolean).map((u) => (
              <option key={u} value={u}>{u}</option>
            ))}
          </Select>
        </Field>
      ) : null}
      {scopes.includes("bot") ? <PermissionPicker value={perms} onChange={setPerms} /> : null}
      <div className={s.generated}>
        <code data-testid="built-link">{link}</code>
        <CopyButton value={link} label={t("apps.portal.copyLink")} done={t("apps.portal.linkCopied")} />
      </div>
      {resetting ? (
        <PasswordConfirm
          title={app.has_secret ? t("apps.portal.resetSecret") : t("apps.portal.makeSecret")}
          body={app.has_secret ? t("apps.portal.resetSecretBody") : t("apps.portal.makeSecretBody")}
          confirm={app.has_secret ? t("apps.portal.resetSecret") : t("apps.portal.makeSecret")}
          run={async (key) => {
            const res = await appsApi.resetSecret(app.id, key);
            setSecret(res.secret);
            onChange({ ...app, has_secret: true });
          }}
          onClose={() => setResetting(false)}
        />
      ) : null}
    </div>
  );
}

/**
 * What an app's bot registered: its commands everywhere, as people are offered
 * them. The owner can take one away, or all of them for the bot to register
 * again from nothing.
 */
function CommandsTab({ app }: { app: OwnedApp }) {
  const [list, setList] = useState<AppCommand[] | null>(null);
  const [failed, setFailed] = useState(false);
  const [removing, setRemoving] = useState<AppCommand | "all" | null>(null);
  const toast = useUI((u) => u.toast);
  useEffect(() => {
    let alive = true;
    appsApi
      .commands(app.id)
      .then((l) => alive && setList(l))
      .catch(() => alive && setFailed(true));
    return () => {
      alive = false;
    };
  }, [app.id]);
  if (failed) return <p className={st.hint}>{t("apps.portal.commandsFailed")}</p>;
  if (!list) return null;
  if (!list.length) return <p className={st.hint}>{t("apps.portal.noCommands")}</p>;
  const shownName = (c: AppCommand) => (c.type === 1 ? `/${c.name}` : c.name);
  const remove = async () => {
    if (!removing) return;
    if (removing === "all") {
      await appsApi.deleteAllCommands(app.id);
      setList([]);
      toast(t("apps.portal.commandsDeleted"), "success");
    } else {
      await appsApi.deleteCommand(app.id, removing.id);
      setList((l) => l?.filter((x) => x.id !== removing.id) ?? null);
      toast(t("apps.portal.commandDeleted", { name: shownName(removing) }), "success");
    }
  };
  const del = (c: AppCommand) => (
    <button type="button" className={s.cmdDelete} onClick={() => setRemoving(c)} aria-label={t("apps.portal.deleteCommandNamed", { name: shownName(c) })} title={t("apps.portal.deleteCommand")} data-testid="command-delete">
      <Trash2 size={16} aria-hidden="true" />
    </button>
  );
  return (
    <div className={s.list}>
      <div className={s.cmdHead}>
        <p className={st.hint} style={{ margin: 0 }}>{t("apps.portal.commandsHint")}</p>
        <Button size="sm" variant="danger" onClick={() => setRemoving("all")} data-testid="commands-delete-all">
          {t("apps.portal.deleteAllCommands")}
        </Button>
      </div>
      {list.flatMap((c) =>
        c.type === 1
          ? commandLeaves(c).map((leaf) => (
              <div key={c.id + leaf.path.join(" ")} className={s.cmd}>
                <div className={s.cmdTop}>
                  <span className={s.cmdName}>/{leaf.label}</span>
                  <span className={s.cmdKind}>{t("apps.portal.kindSlash")}</span>
                  {c.guild_id ? <span className={s.cmdWhere}>{t("apps.portal.onlyIn", { server: c.guild_name || c.guild_id })}</span> : null}
                  {del(c)}
                </div>
                {leaf.description ? <span className={s.cmdDesc}>{leaf.description}</span> : null}
                {leaf.options.length ? (
                  <div className={s.cmdOpts}>
                    {leaf.options.map((o) => (
                      <span key={o.name} className={s.cmdOpt} data-required={o.required || undefined}>{optionName(o)}{o.required ? "" : "?"}</span>
                    ))}
                  </div>
                ) : null}
              </div>
            ))
          : [
              <div key={c.id} className={s.cmd}>
                <div className={s.cmdTop}>
                  <span className={s.cmdName}>{c.name}</span>
                  <span className={s.cmdKind}>{c.type === 2 ? t("apps.portal.kindUser") : t("apps.portal.kindMessage")}</span>
                  {c.guild_id ? <span className={s.cmdWhere}>{t("apps.portal.onlyIn", { server: c.guild_name || c.guild_id })}</span> : null}
                  {del(c)}
                </div>
                {commandDescription(c) ? <span className={s.cmdDesc}>{commandDescription(c)}</span> : null}
              </div>,
            ],
      )}
      {removing ? (
        <ConfirmModal
          onClose={() => setRemoving(null)}
          title={removing === "all" ? t("apps.portal.deleteAllCommands") : t("apps.portal.deleteCommand")}
          body={removing === "all" ? t("apps.portal.deleteAllCommandsBody") : t("apps.portal.deleteCommandBody", { name: shownName(removing) })}
          confirmLabel={t("apps.portal.delete")}
          danger
          onConfirm={remove}
        />
      ) : null}
    </div>
  );
}

/** Whether a set of permissions asks for Administrator, which the consent page says plainly. */
export function asksAdmin(perms: string): boolean {
  return (parsePerm(perms) & P.ADMINISTRATOR) !== 0n;
}

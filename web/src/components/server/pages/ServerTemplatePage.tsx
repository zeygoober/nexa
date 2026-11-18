import { Copy, RefreshCw, Trash2 } from "lucide-react";
import { useEffect, useState, type FormEvent } from "react";
import { servers as serversApi, type ServerTemplate } from "@/api/servers";
import { Button } from "@/components/common/Button";
import { LoadError } from "@/components/common/LoadError";
import { Spinner } from "@/components/common/Spinner";
import { Field, TextArea, TextInput } from "@/components/common/TextInput";
import { copyAndSay } from "@/lib/copyText";
import { describeError } from "@/lib/errors";
import { formatTimestamp } from "@/lib/time";
import { useServers } from "@/stores/servers";
import { useUI } from "@/stores/ui";
import { t } from "@/i18n";
import st from "@/components/settings/Settings.module.css";
import s from "../ServerSettings.module.css";

/** What the server allows (maxTemplateDesc, cleanServerName). */
const DESC_MAX = 120;
const NAME_MAX = 100;

/**
 * Server Settings, Server Template: one link that lets anyone make a new
 * server with this one's channels, roles and permissions. It holds the server
 * as it was when made or last synced, never its messages or members. Laid out
 * as the vanity link is: the address in one well, what it has done under it.
 */
export default function ServerTemplatePage({ serverId }: { serverId: string }) {
  const serverName = useServers((x) => x.byId[serverId]?.server.name ?? "");
  const toast = useUI((u) => u.toast);
  const openModal = useUI((u) => u.openModal);
  const [tpl, setTpl] = useState<ServerTemplate | null | undefined>(undefined);
  const [failed, setFailed] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const [name, setName] = useState("");
  const [desc, setDesc] = useState("");
  const [busy, setBusy] = useState<"save" | "sync" | null>(null);

  const take = (next: ServerTemplate | null) => {
    setTpl(next);
    setName(next?.name ?? serverName);
    setDesc(next?.description ?? "");
  };

  useEffect(() => {
    let alive = true;
    serversApi
      .template(serverId)
      .then((next) => {
        if (!alive) return;
        take(next);
        setFailed(false);
      })
      .catch(() => alive && setFailed(true));
    return () => {
      alive = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [serverId, attempt]);

  const changed = !!tpl && (name.trim() !== tpl.name || desc.trim() !== tpl.description);

  const save = async (e: FormEvent) => {
    e.preventDefault();
    if (busy || !name.trim() || (tpl && !changed)) return;
    setBusy("save");
    try {
      const next = await serversApi.saveTemplate(serverId, { name: name.trim(), description: desc.trim() });
      toast(t(tpl ? "server.template.saved" : "server.template.created"), "success");
      take(next);
    } catch (err) {
      toast(describeError(err, t("server.template.failed")), "error");
    } finally {
      setBusy(null);
    }
  };

  const sync = async () => {
    if (busy) return;
    setBusy("sync");
    try {
      take(await serversApi.syncTemplate(serverId));
      toast(t("server.template.synced"), "success");
    } catch (err) {
      toast(describeError(err, t("server.template.failed")), "error");
    } finally {
      setBusy(null);
    }
  };

  const remove = () => {
    if (!tpl) return;
    openModal("confirm", {
      title: t("server.template.deleteTitle"),
      body: t("server.template.deleteBody"),
      confirmLabel: t("server.template.delete"),
      danger: true,
      onConfirm: async () => {
        try {
          await serversApi.deleteTemplate(serverId);
          take(null);
          toast(t("server.template.deleted"), "success");
        } catch (err) {
          toast(describeError(err, t("server.template.failed")), "error");
        }
      },
    });
  };

  const host = window.location.host;
  return (
    <>
      <h2 className={st.title}>{t("server.template.title")}</h2>
      <p className={st.hint} style={{ marginTop: -12, marginBottom: 20 }}>{t("server.template.intro")}</p>
      {failed ? (
        <LoadError message={t("server.template.loadFailed")} onRetry={() => setAttempt((n) => n + 1)} />
      ) : tpl === undefined ? (
        <div style={{ display: "flex", justifyContent: "center", padding: 40 }}>
          <Spinner label={t("chat.pins.loading")} />
        </div>
      ) : (
        <>
          {tpl ? (
            <section data-testid="server-template-link">
              <div className={s.vanity}>
                <div className={s.vanityField} dir="ltr">
                  <span className={s.vanityHost}>
                    <span dir="ltr">{host}/template/</span>
                  </span>
                  <span className={s.templateCode} data-testid="server-template-code">{tpl.code}</span>
                </div>
                <Button onClick={() => copyAndSay(tpl.url, t("server.invites.linkCopied"))} data-testid="server-template-copy">
                  <Copy size={14} aria-hidden="true" /> {t("server.invites.copyLink")}
                </Button>
              </div>
              <div className={s.vanityFoot}>
                <span className={s.vanityUses}>{t("server.template.used", { count: tpl.usage_count })}</span>
                <span className={s.vanityUses}>{t("server.template.syncedAt", { when: formatTimestamp(tpl.synced_at) })}</span>
                <p className={st.hint} style={{ margin: 0 }}>{t("server.template.syncHint")}</p>
                <div className={s.joinBtns}>
                  <Button size="sm" variant="secondary" onClick={() => void sync()} loading={busy === "sync"} data-testid="server-template-sync">
                    <RefreshCw size={14} aria-hidden="true" /> {t("server.template.sync")}
                  </Button>
                  <Button size="sm" variant="ghost" onClick={remove} data-testid="server-template-delete">
                    <Trash2 size={14} aria-hidden="true" /> {t("server.template.delete")}
                  </Button>
                </div>
              </div>
            </section>
          ) : null}
          <form onSubmit={(e) => void save(e)} data-testid="server-template-form">
            {tpl ? <h3 className={st.h5}>{t("server.template.details")}</h3> : null}
            <div className={st.form} style={{ maxWidth: 560 }}>
              <Field label={t("server.template.name")} required>
                <TextInput value={name} maxLength={NAME_MAX} onChange={(e) => setName(e.target.value)} data-testid="server-template-name" />
              </Field>
              <Field label={t("server.template.description")} hint={t("server.template.descriptionHint")}>
                <TextArea value={desc} maxLength={DESC_MAX} rows={2} onChange={(e) => setDesc(e.target.value)} data-testid="server-template-description" />
              </Field>
              <div>
                <Button type="submit" loading={busy === "save"} disabled={!name.trim() || (!!tpl && !changed)} data-testid="server-template-save">
                  {t(tpl ? "server.template.save" : "server.template.create")}
                </Button>
              </div>
            </div>
          </form>
        </>
      )}
    </>
  );
}

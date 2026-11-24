import { useEffect, useState } from "react";
import { api } from "@/api/client";
import type { Message } from "@/api/types";
import { HistoryReviews, ReviewContent } from "./HistoryReviews";
import { Button } from "@/components/common/Button";
import { LoadError } from "@/components/common/LoadError";
import { t } from "@/i18n";
import { readOwnerCopy } from "@/lib/ownerMessages";
import { useKeys } from "@/stores/keys";
import m from "./Moderation.module.css";
import s from "./TradesTab.module.css";

export function OwnerMessagesTab() {
  const [before, setBefore] = useState("");
  const [attempt, retry] = useState(0);
  const [page, setPage] = useState<{ row: Message; payload: Awaited<ReturnType<typeof readOwnerCopy>> | null }[]>([]);
  const [status, setStatus] = useState<"busy" | "ready" | "failed">("busy");
  const identity = useKeys(state => state.identity);
  useEffect(() => {
    const controller = new AbortController();
    setPage([]);
    setStatus("busy");
    if (!identity) return;
    void api.request<Message[]>("GET", "/moderation/messages", undefined, { signal: controller.signal, query: { before } })
      .then(rows => Promise.all(rows.map(async row => ({ row, payload: await readOwnerCopy(row).catch(() => null) }))))
      .then(rows => { if (!controller.signal.aborted) { setPage(rows); setStatus("ready"); } })
      .catch(() => { if (!controller.signal.aborted) setStatus("failed"); });
    return () => controller.abort();
  }, [before, attempt, identity]);
  return <div className={m.scroll}><div className={s.page}>
    <HistoryReviews owner />
    {status === "busy" && <p role="status">{t("chat.pins.loading")}</p>}
    {status === "failed" && <LoadError message={t("moderation.loadFailed")} onRetry={() => retry(n => n + 1)} />}
    {status === "ready" && !page.length && <p>{t("moderation.noMatches")}</p>}
    <ol className={s.list}>{page.map(({row, payload}) => <li key={row.id} className={s.trade}>
      <small>{t("ownerMessages.source", { author: row.author_id, channel: row.channel_id })} · {new Date(row.created_at).toLocaleString()}</small>
      {payload ? <ReviewContent row={row} payload={payload}/> : <p>{t("ownerMessages.cannotOpen")}</p>}
    </li>)}</ol>
    {page.length === 50 && <Button variant="secondary" onClick={() => setBefore(page[page.length-1]!.row.id)}>{t("moderation.trades.loadMore")}</Button>}
    {before && <Button variant="ghost" onClick={() => setBefore("")}>{t("ownerMessages.newest")}</Button>}
  </div></div>;
}

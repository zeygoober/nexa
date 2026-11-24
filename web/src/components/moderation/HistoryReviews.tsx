import { useEffect, useState } from "react";
import { api } from "@/api/client";
import type { Message } from "@/api/types";
import { Button } from "@/components/common/Button";
import { Attachments } from "@/components/chat/Attachments";
import type { MessagePayload } from "@/crypto/message";
import { decryptForStore } from "@/lib/decrypt";
import { attachOwnerCopy, readOwnerCopy, type HistoryReview } from "@/lib/ownerMessages";
import { useAuth } from "@/stores/auth";
import { useKeys } from "@/stores/keys";
import styles from "./HistoryReviews.module.css";
import s from "./TradesTab.module.css";

type Entry = { row: Message; payload: MessagePayload };
const ownPath = "/moderation/history-reviews";
const participantPath = "/users/@me/history-reviews";

export function ReviewContent({row, payload}: Entry) {
  return <div>
    <small>Message {row.id} · Author {row.author_id} · {new Date(row.created_at).toLocaleString()}</small>
    <p className={styles.body}>{[payload.fwd?.comment,payload.t,payload.sticker?.name,payload.poll?.question,...(payload.poll?.answers.map(a=>a.text)??[])].filter(Boolean).join("\n")}</p>
    {payload.att?.length ? <Attachments list={payload.att}/> : null}
    {payload.embeds?.map((e,i)=><pre key={i} className={styles.body}>{JSON.stringify(e,null,2)}</pre>)}
  </div>;
}

function ReviewCard({review:r, owner, refresh}: {review:HistoryReview;owner:boolean;refresh:()=>void}) {
  const me = useAuth(a=>a.me?.id);
  const [entries,setEntries]=useState<Entry[]|null>(null);
  const [busy,setBusy]=useState(false);
  const [error,setError]=useState("");
  async function act(action:"preview"|"approve"|"withdraw"|"share"|"view") {
    if (!me || busy) return;
    setBusy(true); setError("");
    try {
      if (action==="view" || action==="preview") {
        const rows = action==="view"
          ? await api.request<Message[]>("GET",ownPath+"/"+r.id+"/messages")
          : await api.request<Message[]>("POST",participantPath+"/"+r.id+"/preview");
        const opened=await Promise.all(rows.map(async row=>{
          const body=action==="view"?await readOwnerCopy(row):(await decryptForStore(row)).payload;
          if(!body) throw new Error("This device could not open every selected message.");
          const payload={...body};
          if(!r.include_files) delete payload.att;
          return {row,payload};
        }));
        setEntries(opened);
      } else {
        let boxes:Record<string,string>|undefined;
        if(action==="share"){
          if(!entries || entries.length!==r.items.length) throw new Error("Inspect every message before sharing.");
          boxes={};
          for(const {row,payload} of entries){
            if(!row.ciphertext) throw new Error("Missing message.");
            const env={...row.ciphertext};
            // Consent belongs to the signed-in participant. Authorship in the box
            // belongs to the original message and may be the other participant.
            await attachOwnerCopy(env,r.owner_id,row.author_id,row.channel_id,payload,me);
            boxes[row.id]=env.owner!.box;
          }
        }
        await api.request("POST",participantPath+"/"+r.id+"/"+action,boxes);
        setEntries(null);refresh();
      }
    }catch(e){setError(e instanceof Error?e.message:"Request failed.");}finally{setBusy(false);}
  }
  const approved=me===r.first_id?r.first_approved:r.second_approved;
  return <section className={s.trade}>
    <strong>{r.reason}</strong>
    <p>Channel {r.channel_id} · Receiving owner {r.owner_id} · {r.items.length} messages · {r.include_files?"With attachments":"Without attachments"}</p>
    <p>Access expires {new Date(r.expires_at).toLocaleString()}. Only the receiving owner can open a submitted copy. Withdrawal ends access here. Downloaded copies cannot be recalled.</p>
    <p>{r.status} · {Number(r.first_approved)+Number(r.second_approved)} of 2 approvals</p>
    {entries?.map(e=><ReviewContent key={e.row.id} {...e}/>)}
    {owner && r.status==="shared" && <Button disabled={busy} onClick={()=>void act("view")}>Read approved messages</Button>}
    {!owner && r.status==="pending" && <Button disabled={busy} onClick={()=>void act("preview")}>Inspect selection</Button>}
    {!owner && entries && r.status==="pending" && !approved && <Button disabled={busy} onClick={()=>void act("approve")}>Approve this selection</Button>}
    {!owner && entries && r.status==="pending" && r.first_approved && r.second_approved && <Button disabled={busy} onClick={()=>void act("share")}>Send approved copy to owner</Button>}
    {!owner && r.status!=="cancelled" && <Button variant="ghost" disabled={busy} onClick={()=>void act("withdraw")}>{approved?"Withdraw approval":"Decline request"}</Button>}
    {error && <p role="alert">{error}</p>}
  </section>;
}

export function HistoryReviews({owner=false}:{owner?:boolean}) {
  const [version,refresh]=useState(0);
  const [before,setBefore]=useState("");
  const [list,setList]=useState<HistoryReview[]>([]);
  const [error,setError]=useState("");
  const [busy,setBusy]=useState(false);
  const identity=useKeys(k=>k.identity);
  useEffect(()=>{
    if(!identity) return;
    const c=new AbortController();
    setError("");setList([]);
    void api.request<HistoryReview[]>("GET",owner?ownPath:participantPath,undefined,{signal:c.signal,query:{before}})
      .then(r=>{if(!c.signal.aborted)setList(r);}).catch(e=>{if(!c.signal.aborted)setError(e.message);});
    return ()=>c.abort();
  },[owner,version,identity,before]);
  if(!identity) return null;
  return <div className={s.page}>
    {owner && <details className={styles.request}><summary>Request historical messages</summary>
      <form className={styles.form} onSubmit={async ev=>{
        ev.preventDefault();const form=ev.currentTarget;const d=new FormData(form);setBusy(true);setError("");
        try{const ids=String(d.get("ids")??"").trim().split(/[\s,]+/).filter(Boolean);
          await api.request("POST",ownPath,{channel_id:d.get("channel"),message_ids:ids,reason:d.get("reason"),include_files:d.has("files")});
          form.reset();setBefore("");refresh(v=>v+1);
        }catch(e){setError(e instanceof Error?e.message:"Request failed.");}finally{setBusy(false);}
      }}>
        <p>Both people in a direct message must approve the selection. Long selections are divided into batches of 50 for approval.</p>
        <label>DM channel ID <input name="channel" required inputMode="numeric" pattern="[0-9]+"/></label>
        <label>Message IDs (leave empty for all earlier messages in this DM) <textarea name="ids"/></label>
        <label>Investigation reason <input name="reason" required maxLength={500}/></label>
        <label><input type="checkbox" name="files"/> Include the selected messages’ attachments</label>
        <Button type="submit" disabled={busy}>Request approval</Button>
      </form>
    </details>}
    {!!list.length && <h3>Historical message requests</h3>}
    {list.map(r=><ReviewCard key={r.id+"-"+version} review={r} owner={owner} refresh={()=>refresh(v=>v+1)}/>)}
    {list.length===50 && <Button onClick={()=>setBefore(list[list.length-1]!.id)}>Older requests</Button>}
    {before && <Button onClick={()=>setBefore("")}>Newest requests</Button>}
    {(list.length>0||error) && <Button variant="ghost" onClick={()=>refresh(v=>v+1)}>Refresh requests</Button>}
    {error && <p role="alert">{error}</p>}
  </div>;
}

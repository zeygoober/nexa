import { b64, utf8 } from "@/crypto/bytes";
import { open, seal } from "@/crypto/keys";
import { sanitizePayload, type Envelope, type MessagePayload } from "@/crypto/message";
import type { Message } from "@/api/types";
import { termsAccepted } from "./legal";
import { sealingKeyOf, useKeys } from "@/stores/keys";

/** Only the newly composed payload enters the box. Never pass conversation or identity keys here. */
export async function attachOwnerCopy(env: Envelope, owner: string | undefined, author: string, channel: string, payload: MessagePayload, approvingAccount = author) {
  if (!owner || owner === "0" || !termsAccepted(approvingAccount)) throw new Error("Owner message notice required");
  const key = await sealingKeyOf(owner);
  if (!key) throw new Error("Owner identity key changed");
  const content = utf8.enc(JSON.stringify({ author, channel, iv: env.iv, payload }));
  env.owner = { id: owner, box: b64.enc(await seal(key, content)) };
}

export async function readOwnerCopy(m: Message): Promise<MessagePayload> {
  if (m.webhook && !m.ciphertext) {
    const { decryptForStore } = await import("./decrypt");
    const msg = await decryptForStore(m);
    if (!msg.payload) throw new Error("Invalid app message");
    return { ...msg.payload, embeds: msg.hook?.embeds };
  }
  const identity = useKeys.getState().identity;
  const env = m.ciphertext;
  if (!identity || !env?.owner || m.owner_recipient !== env.owner.id) throw new Error("Missing owner copy");
  const data = await open(identity.privateKey, identity.publicKey, b64.dec(env.owner.box));
  const row = JSON.parse(utf8.dec(data));
  if (row.author !== m.author_id || row.channel !== m.channel_id || row.iv !== env.iv || !row.payload || typeof row.payload !== "object" || Array.isArray(row.payload)) throw new Error("Invalid owner copy");
  return sanitizePayload(row.payload);
}

export interface HistoryReview {
  id: string;
  channel_id: string;
  owner_id: string;
  first_id: string;
  second_id: string;
  reason: string;
  expires_at: number;
  include_files: boolean;
  items: {id: string; digest: string}[];
  first_approved: boolean;
  second_approved: boolean;
  status: "pending" | "shared" | "cancelled";
}

import { useEffect } from "react";
import { create } from "zustand";
import { ApiError } from "@/api/client";
import { expressions as expressionsApi } from "@/api/expressions";
import type { StickerPack } from "@/api/types";

/**
 * Sticker and emoji pack links in chats (https://nexa.pm/stickers/pack/ID),
 * drawn as cards (components/chat/PackCards). What a card shows is asked of
 * the server, so it is the pack as it is now, and whether this account has it.
 */
export type SharedPackEntry =
  | { status: "loading" }
  | { status: "ready"; pack: StickerPack; at: number }
  /** private, or gone */
  | { status: "missing"; at: number }
  | { status: "failed"; at: number };

/** A card drawn again after this long asks again, showing what it had meanwhile. */
const FRESH_MS = 5 * 60_000;
/** A failed request (offline, busy) is tried again after this long. */
const RETRY_MS = 30_000;

export const useSharedPacks = create<{ byId: Record<string, SharedPackEntry> }>()(() => ({ byId: {} }));

const inflight = new Map<string, Promise<void>>();

function put(id: string, entry: SharedPackEntry): void {
  useSharedPacks.setState((s) => ({ byId: { ...s.byId, [id]: entry } }));
}

/** Asks the server about a pack a link leads to, unless this device already knows or is already asking. */
export function loadSharedPack(id: string, opts: { force?: boolean } = {}): Promise<void> {
  const running = inflight.get(id);
  if (running) return running;
  const cur = useSharedPacks.getState().byId[id];
  const now = Date.now();
  if (!opts.force && cur && (cur.status === "loading" || now - cur.at <= (cur.status === "failed" ? RETRY_MS : FRESH_MS))) return Promise.resolve();
  // what was shown stays while it is asked again
  if (!cur || cur.status !== "ready") put(id, { status: "loading" });
  const job = expressionsApi
    .pack(id)
    .then((pack) => put(id, { status: "ready", pack, at: Date.now() }))
    .catch((err) => put(id, err instanceof ApiError && err.status === 404 ? { status: "missing", at: Date.now() } : { status: "failed", at: Date.now() }))
    .finally(() => inflight.delete(id));
  inflight.set(id, job);
  return job;
}

/** A pack as the app last heard of it, asked for when it is first wanted or has gone stale. */
export function useSharedPack(id: string): SharedPackEntry | undefined {
  const entry = useSharedPacks((s) => s.byId[id]);
  useEffect(() => {
    void loadSharedPack(id);
  }, [id]);
  return entry;
}

/** The pack as it is now that it has been added to (or taken from) this account: the card says so at once. */
export function setSharedPack(pack: StickerPack): void {
  put(pack.id, { status: "ready", pack, at: Date.now() });
}

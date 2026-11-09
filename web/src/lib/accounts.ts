/**
 * The accounts this browser is signed in to, for switching between them from
 * the status menu.
 *
 * Only one account runs at a time: the one whose token is TOKEN_KEY. The
 * others keep their tokens here and their keys where they always were (the
 * key store and the passcode lock are both per account), so switching is
 * putting another token in TOKEN_KEY and opening the app again. Each account
 * signs in once, on its own; nothing here can sign one in for another.
 */
import { create } from "zustand";
import { api } from "@/api/client";
import type { MeUser } from "@/api/types";
import { keystore } from "@/crypto/keystore";
import { clearLock } from "@/lib/appLock";
import { storage } from "@/lib/storage";
import { wipeSearchIndex } from "@/search/keys";
import { logoutHooks, TOKEN_KEY, useAuth } from "@/stores/auth";

/** As many accounts as the menu shows without growing past the screen. */
export const MAX_ACCOUNTS = 5;

const KEY = "ec.accounts";

export interface SavedAccount {
  id: string;
  username: string;
  display_name: string;
  avatar_id: string | null;
  name_style?: unknown;
  token: string;
}

function read(): SavedAccount[] {
  try {
    const v = JSON.parse(storage.get(KEY) ?? "[]");
    return Array.isArray(v) ? v.filter((a) => a && typeof a.id === "string" && typeof a.token === "string") : [];
  } catch {
    return [];
  }
}

function write(list: SavedAccount[]): void {
  if (list.length) storage.set(KEY, JSON.stringify(list));
  else storage.remove(KEY);
  useAccounts.setState({ list });
}

export const useAccounts = create<{ list: SavedAccount[] }>()(() => ({ list: read() }));

/** Keeps the account signed in now in the list, with its name and picture as they are. */
export function rememberAccount(me: MeUser, token: string): void {
  const list = read();
  const entry: SavedAccount = { id: me.id, username: me.username, display_name: me.display_name, avatar_id: me.avatar_id ?? null, name_style: me.name_style ?? null, token };
  const at = list.findIndex((a) => a.id === me.id);
  if (at >= 0) list[at] = entry;
  else list.push(entry);
  write(list);
}

/** Drops an account from the list, without touching its session. */
export function forgetAccount(id: string): void {
  write(read().filter((a) => a.id !== id));
}

/** Drops whichever account held a token the server no longer takes. */
export function forgetToken(token: string): void {
  write(read().filter((a) => a.token !== token));
}

/** Whether another account can be added. */
export function canAddAccount(): boolean {
  return read().length < MAX_ACCOUNTS;
}

/** How long one step of leaving an account may take before switching goes on without it. */
const LEAVE_STEP_MS = 4000;
/** How long to wait for the page to go before asking again: longer than a slow load, which asking again restarts. */
const GO_AGAIN_MS = 15000;

/** Ends this account's session here (the call, the connection) without signing it out. */
async function leaveQuietly(): Promise<void> {
  for (const hook of logoutHooks) {
    try {
      // a step stuck on a bad connection must not leave the page emptied and going nowhere
      await Promise.race([hook(), new Promise((done) => setTimeout(done, LEAVE_STEP_MS))]);
    } catch {
      /* switching goes ahead regardless */
    }
  }
}

/**
 * Leaves this page for [path]. On a touch screen a dialog closing steps back
 * through the history (shell/backLayers.ts), and that step can cancel a
 * navigation made while it travels, which left the page emptied by the switch
 * on screen for good. So a step back asks again, and so, much later, does the clock.
 */
function leaveFor(path: string): void {
  const again = () => location.assign(path);
  addEventListener("popstate", again);
  setInterval(again, GO_AGAIN_MS);
  again();
}

const placeKey = (id: string) => `ec.place:${id}`;

/** Where an account was in the app when it was last switched away from, to go back to. */
export function placeOf(id: string): string {
  const p = storage.get(placeKey(id));
  return p && p.startsWith("/channels/") ? p : "/channels/@me";
}

/** Opens the app as another saved account, where that account was last. */
export async function switchAccount(id: string, go: (path: string) => void = leaveFor): Promise<void> {
  const to = read().find((a) => a.id === id);
  const here = useAuth.getState().me?.id;
  if (!to || to.id === here) return;
  if (here && location.pathname.startsWith("/channels/")) storage.set(placeKey(here), location.pathname);
  await leaveQuietly();
  storage.set(TOKEN_KEY, to.token);
  go(placeOf(to.id));
}

/**
 * Signs out an account that is not the one open: its session on the server,
 * then everything this browser kept for it.
 */
export async function removeAccount(id: string): Promise<void> {
  const acct = read().find((a) => a.id === id);
  if (!acct || id === useAuth.getState().me?.id) return;
  forgetAccount(id);
  try {
    // its own token, not the one the client sends for the account open here
    await fetch(api.url("/auth/logout"), { method: "POST", headers: { Authorization: `Bearer ${acct.token}` } });
  } catch {
    /* already signed out there */
  }
  clearLock(id);
  void wipeSearchIndex(id);
  try {
    await keystore.clear(id);
  } catch {
    /* gone with the next sign-out of the key store */
  }
}

/** After the account open here signed out: the next one on the list, if any, to go on with. */
export function nextAccount(): SavedAccount | null {
  return read()[0] ?? null;
}

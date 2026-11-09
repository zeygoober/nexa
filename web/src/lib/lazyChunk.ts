/**
 * Lazily loaded pages, with a way back from a deploy.
 *
 * A page that is already open holds the file names its build was compiled
 * with. Publishing a new build replaces those files, so the next screen it
 * tries to open is a fetch for something that is no longer there. Rather than
 * showing that as a crash, the page reloads once and comes back on the new
 * build. Old files are also kept on the server for a week, so this is the
 * fallback rather than the usual path.
 */

import { lazy, type ComponentType, type LazyExoticComponent } from "react";
import { failedPage } from "@/components/common/FailedPage";

/* eslint-disable @typescript-eslint/no-explicit-any */

const RELOAD_FLAG = "ec.chunkReload";

function reloadedAlready(): boolean {
  try {
    return sessionStorage.getItem(RELOAD_FLAG) !== null;
  } catch {
    return false;
  }
}

function markReloaded(): void {
  try {
    sessionStorage.setItem(RELOAD_FLAG, String(Date.now()));
  } catch {
    /* private window: one attempt is all we get */
  }
}

/** Clears the guard once the app has run for a while on the new build. */
export function markChunksHealthy(): void {
  setTimeout(() => {
    try {
      sessionStorage.removeItem(RELOAD_FLAG);
    } catch {
      /* nothing to clear */
    }
  }, 20_000);
}

/** Resolves when the device says it is online, straight away if it already does. */
function online(): Promise<void> {
  if (typeof navigator === "undefined" || navigator.onLine !== false) return Promise.resolve();
  return new Promise((resolve) => window.addEventListener("online", () => resolve(), { once: true }));
}

/**
 * Loads a page's file, trying again a few times when the network is what
 * failed. Reloading without a connection would not bring the app back, only
 * the browser's offline page, and lose everything on screen.
 */
async function loadPatiently<T>(load: () => Promise<T>): Promise<T> {
  for (let attempt = 0; ; attempt++) {
    try {
      return await load();
    } catch (err) {
      // Online, a couple of quick retries cover a blip; after that the file is
      // most likely gone with an old build, and reloading is the way back.
      // Offline, it keeps waiting for the connection instead.
      if (attempt >= 2 && navigator.onLine !== false) throw err;
      await online();
      await new Promise((r) => setTimeout(r, Math.min(4000, 1000 * 2 ** Math.min(attempt, 2))));
    }
  }
}

/**
 * Loads a file of the build, and when it is gone with an old build, reloads
 * the page onto the new one (once) instead of failing.
 */
export async function loadChunk<T>(load: () => Promise<T>): Promise<T> {
  try {
    return await loadPatiently(load);
  } catch (err) {
    if (reloadedAlready()) throw err;
    markReloaded();
    window.location.reload();
    // the page is on its way out; never resolve, so nothing renders an error
    return await new Promise<T>(() => undefined);
  }
}

/** React.lazy that survives the file it wanted being replaced by a deploy. */
export function lazyPage<T extends ComponentType<any>>(load: () => Promise<{ default: T }>): LazyExoticComponent<T> {
  return lazy(() => loadChunk(load));
}

/** A lazy page whose owner can wait for it too (`ready()`, called while rendering). */
export type ReadyPage<T extends ComponentType<any>> = LazyExoticComponent<T> & { ready(): void };

/**
 * lazyPage for a page of a dialog, plus `ready()`, which suspends whoever calls
 * it until the page's file is here. A dialog calls it for the page it opens on,
 * so its frame and that page arrive together, rather than the frame first with
 * a spinner in it and then, a moment later, the page.
 *
 * On a bad connection a file not arriving is ordinary, so it never rejects
 * (React keeps a rejected page for good, and the app-wide boundary would
 * restart everything): the page becomes a Try Again in place.
 */
export function readyPage<T extends ComponentType<any>>(load: () => Promise<{ default: T }>): ReadyPage<T> {
  let settled = false;
  let pending: Promise<{ default: T }> | null = null;
  const once = () =>
    (pending ??= loadOrOffer(load).then((m) => {
      settled = true;
      return m;
    }));
  const page = lazy(once) as ReadyPage<T>;
  page.ready = () => {
    if (!settled) throw once();
  };
  return page;
}

/** lazyPage for a whole dialog: a file that does not arrive shows Try Again over what is open. */
export function lazyDialog<T extends ComponentType<any>>(load: () => Promise<{ default: T }>): LazyExoticComponent<T> {
  return lazy(() => loadOrOffer(load, true));
}

/** Only pressing Try Again may reload onto a new build: reaching this far usually means the signal, not a deploy. */
function loadOrOffer<T extends ComponentType<any>>(load: () => Promise<{ default: T }>, dialog = false): Promise<{ default: T }> {
  return loadPatiently(load).catch(() => ({ default: failedPage(() => loadChunk(load), dialog) }));
}

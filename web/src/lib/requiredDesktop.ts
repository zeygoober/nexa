/**
 * The oldest desktop app Nexa still works in. A copy older than this shows
 * nothing but the update (components/shell/RequiredUpdate.tsx) until it is
 * replaced: it updates itself where it can, and otherwise offers the
 * installer. Raise it only together with a desktop release that is at least
 * this version, since the gate holds only once the server publishes one
 * (a copy is never shut out with nothing to update to).
 */
import { useEffect, useState } from "react";
import { desktop, type DesktopFile, type DesktopRelease } from "@/api/desktop";
import { desktopBridge } from "./desktopBridge";
import { compareVersions } from "./macReplacement";
import { currentPlatform, pickDownload, type DesktopTarget } from "./platform";

export const MANUAL_SIGNATURE_VERSION = "1.2.12";
export const REQUIRED_DESKTOP = MANUAL_SIGNATURE_VERSION;

export interface RequiredUpdate {
  /** The version this copy is. */
  running: string;
  /** The trust-anchor transition requires an installer, never the old updater. */
  manual: boolean;
  /** The version published, which it has to move to. */
  version: string;
  /** The installer for this computer, as an address the browser can open. */
  url: string | null;
}

/** What this copy has to update to, or null when it may go on as it is. */
export function requiredUpdate(running: string | null, rel: DesktopRelease | null, file: DesktopFile | null, origin: string): RequiredUpdate | null {
  if (!running || !rel) return null;
  if (compareVersions(running, REQUIRED_DESKTOP) >= 0 || compareVersions(rel.version, REQUIRED_DESKTOP) < 0) return null;
  return { running, manual: compareVersions(running, MANUAL_SIGNATURE_VERSION) < 0, version: rel.version, url: file ? new URL(desktop.fileUrl(file.name), origin).toString() : null };
}

const OS: Record<string, DesktopTarget["os"]> = { win32: "windows", darwin: "mac", linux: "linux" };

/** How often a copy too old looks again whether there is a release to move to. */
const EVERY_MS = 5 * 60_000;

/**
 * The update this desktop app must take before Nexa opens, or null. Outside
 * the desktop app, and in a copy new enough, it asks nothing.
 */
export function useRequiredUpdate(): RequiredUpdate | null {
  const [need, setNeed] = useState<RequiredUpdate | null>(null);
  useEffect(() => {
    const bridge = desktopBridge();
    if (!bridge || compareVersions(bridge.version, REQUIRED_DESKTOP) >= 0) return;
    let alive = true;
    const look = () =>
      Promise.all([desktop.latest(), currentPlatform()])
        .then(([rel, target]) => {
          // the app knows which system it runs on better than the user agent says
          const file = rel ? pickDownload(rel, { os: OS[bridge.platform] ?? target.os, arch: target.arch }) : null;
          if (alive) setNeed(requiredUpdate(bridge.version, rel, file, window.location.origin));
        })
        .catch(() => undefined);
    void look();
    const timer = setInterval(() => void look(), EVERY_MS);
    return () => {
      alive = false;
      clearInterval(timer);
    };
  }, []);
  return need;
}

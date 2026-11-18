/**
 * What the page may use: the microphone, camera, screen, notifications and
 * clipboard, for Nexa's own origin and nothing else. On macOS the
 * system asks the person the first time the microphone or camera is wanted.
 */
import { app, desktopCapturer, session, shell, systemPreferences, type BrowserWindow, type DesktopCapturerSource } from "electron";
import { execFile } from "node:child_process";
import { randomUUID } from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { IPC, type DisplaySourceRequest } from "../shared/bridge";
import { NO_SOUND, WINDOWS_PROCESS_LOOPBACK_BUILD, windowAudioDevice } from "./appAudio";
import { isAppUrl, originOf, ORIGIN, testHook } from "./config";
import { askForDisplaySource } from "./displayPicker";

type Log = (msg: string, extra?: Record<string, unknown>) => void;

/**
 * `clipboard-read` is deliberately not here.
 *
 * The app writes to the clipboard in about fifteen places and reads from it in
 * none — every call site is a "copy" action. Granting the read silently, which
 * a browser would prompt for, bought nothing and would have let a compromised
 * renderer take whatever was on the clipboard. On a machine where this app
 * itself puts the account recovery key and the two-factor backup codes there,
 * and where a password manager's output passes through all day, that is not a
 * capability to hand over for free.
 */
const ALLOWED = new Set(["media", "notifications", "clipboard-sanitized-write", "fullscreen", "display-capture", "speaker-selection", "pointerLock"]);

/**
 * Players the app frames from another site, and the one thing each may ask:
 * YouTube's player plays straight from YouTube (from 1.2.13, since the web app
 * stopped playing it through Nexa's server) and needs fullscreen for its own
 * button. Only inside Nexa's own page.
 */
const FRAMED: Record<string, Set<string>> = { "https://www.youtube-nocookie.com": new Set(["fullscreen"]) };

function allowed(origin: string, permission: string, appPage: boolean): boolean {
  if (origin === ORIGIN) return ALLOWED.has(permission);
  return appPage && !!FRAMED[origin]?.has(permission);
}

export function lockPermissions(): void {
  const ses = session.defaultSession;
  ses.setPermissionRequestHandler((wc, permission, callback, details) => {
    const origin = originOf((details as { requestingUrl?: string }).requestingUrl ?? wc.getURL());
    if (!allowed(origin, permission, isAppUrl(wc.getURL()))) {
      callback(false);
      return;
    }
    if (permission === "media" && process.platform === "darwin") {
      const kinds = (details as { mediaTypes?: string[] }).mediaTypes ?? [];
      void (async () => {
        const mic = kinds.includes("audio") ? await systemPreferences.askForMediaAccess("microphone") : true;
        const cam = kinds.includes("video") ? await systemPreferences.askForMediaAccess("camera") : true;
        callback(mic && cam);
      })();
      return;
    }
    callback(true);
  });
  ses.setPermissionCheckHandler((wc, permission, requestingOrigin, details) => {
    // Electron hands the origin over as a URL with a trailing slash ("https://host/"), and
    // empty for some checks; compared as it came, every check failed and notifications stayed off
    const origin = originOf(requestingOrigin) || originOf(details.requestingUrl) || originOf(details.embeddingOrigin) || originOf(wc?.getURL());
    return allowed(origin, permission, originOf(details.embeddingOrigin) === ORIGIN || (!!wc && isAppUrl(wc.getURL())));
  });
  // nothing is ever loaded from anywhere else as a page, and downloads keep the system's own save flow
  ses.on("will-download", (_e, item) => {
    if (!isAppUrl(item.getURL()) && !item.getURL().startsWith("blob:") && !item.getURL().startsWith("data:")) {
      item.cancel();
      return;
    }
    // Always offer a location, including downloads initiated by the worker.
    item.setSaveDialogOptions({ defaultPath: path.join(app.getPath("downloads"), path.basename(item.getFilename())) });
  });
}

/**
 * Requests waiting on the page — only the "this computer will not let the app
 * record" explainer, which the page shows and dismisses. Choosing a source is
 * not in here: it belongs to the picker window, which the page cannot reach.
 */
const pending = new Map<string, { resolve: () => void }>();

/**
 * Sound that would not start on this computer this session. Chromium fails
 * the whole share when its sound source cannot start, so after the page says
 * so the next try uses the next safest kind: one program's sound, then the
 * computer's without this app, then all of it.
 */
let noProgramAudio = false;
let noOwnExclusion = false;
/** The last share answered with sound, and which kind it was, for the page to say it failed. */
let lastAnswer: { sourceId: string; sound: ShareSound; at: number } | null = null;
/** A share that failed for its sound and is being tried again: answered with the same source, without the picker. */
let retry: { sourceId: string; until: number } | null = null;

/**
 * The page's share failed to start after sound was chosen. Returns whether
 * trying again makes sense, in which case the next request (within half a
 * minute) shares the same source with safer sound, without asking again.
 */
export function shareAudioFailed(log: Log): boolean {
  const last = lastAnswer;
  lastAnswer = null;
  if (!last || Date.now() - last.at > 60_000) return false;
  if (last.sound.kind === "program") noProgramAudio = true;
  else if (last.sound.kind === "own") noOwnExclusion = true;
  else return false;
  log("share audio failed to start; falling back", { device: last.sound.device });
  retry = { sourceId: last.sourceId, until: Date.now() + 30_000 };
  return true;
}

/**
 * Whether this computer will actually let the app record the screen.
 *
 * macOS keeps that decision in its own database, keyed by the app. A system
 * update, or a new build in the same place, can leave an entry that reads as
 * granted in System Settings while the recording it allows is the old one:
 * the app is asked again, granting changes nothing, and only removing the
 * entry (resetScreenAccess) and switching the app on again, or restarting
 * after granting, puts it right.
 *
 * Asking the system for its answer is not enough on its own, because the
 * stale case answers "granted". A capture with nothing in it is the tell: the
 * sources come back with empty pictures when the decision is not really ours.
 */
export interface ScreenAccess {
  /** What the system says, where it says anything: "granted", "denied", "not-determined", "restricted" or "unknown". */
  status: string;
  /** Whether a share would actually carry a picture. */
  usable: boolean;
  /** True when the system says yes but nothing can be captured — the stale case. */
  stale: boolean;
}

export async function screenAccess(): Promise<ScreenAccess> {
  if (process.platform !== "darwin") return { status: "granted", usable: true, stale: false };
  const status = systemPreferences.getMediaAccessStatus("screen");
  let captured = false;
  try {
    const sources = await desktopCapturer.getSources({ types: ["screen"], thumbnailSize: { width: 32, height: 18 } });
    captured = sources.some((src) => !src.thumbnail.isEmpty());
  } catch {
    captured = false;
  }
  return { status, usable: captured, stale: status === "granted" && !captured };
}

/** The identifier macOS knows this copy of the app by, from its Info.plist as electron-builder writes it (XML). */
function bundleId(): string | null {
  try {
    const plist = fs.readFileSync(path.resolve(app.getPath("exe"), "..", "..", "Info.plist"), "utf8");
    return /<key>CFBundleIdentifier<\/key>\s*<string>([^<]+)<\/string>/.exec(plist)?.[1]?.trim() ?? null;
  } catch {
    return null;
  }
}

/**
 * Has macOS forget its decision about this app recording the screen, for the
 * stale case above: the entry reads as on in System Settings but belongs to
 * an older copy of the app. Asking again then puts Nexa back in the list, and
 * System Settings opens there for the person to switch it on. Returns whether
 * macOS took the reset.
 */
export async function resetScreenAccess(): Promise<boolean> {
  if (process.platform !== "darwin") return false;
  const id = bundleId();
  if (!id) return false;
  const reset = await new Promise<boolean>((resolve) => execFile("/usr/bin/tccutil", ["reset", "ScreenCapture", id], { timeout: 10_000 }, (err) => resolve(!err)));
  if (!reset) return false;
  await desktopCapturer.getSources({ types: ["screen"], thumbnailSize: { width: 1, height: 1 } }).catch(() => []);
  openScreenSettings();
  return true;
}

/** Opens the Screen Recording list in System Settings. */
export function openScreenSettings(): void {
  if (process.platform === "darwin") {
    void shell.openExternal("x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture");
  } else if (process.platform === "win32") {
    void shell.openExternal("ms-settings:privacy-general");
  }
}

/** Quits and opens the app again, which is what a new recording decision needs. */
export function restartApp(): void {
  app.relaunch();
  app.exit(0);
}

/** The audio device a share asks Chromium for: one program, everything but this app, or everything. */
interface ShareSound {
  device: string;
  kind: "program" | "own" | "all" | "none";
}

/** The sound a share of this source carries. */
async function audioDevice(source: DesktopCapturerSource, mode: ShareAudioMode, log: Log): Promise<ShareSound> {
  // a window shares just its program's sound where the system can capture one program,
  // and none at all when that program is a voice changer playing the sharer's voice back
  const program = await windowAudioDevice(source.id, log);
  if (program === NO_SOUND) return { device: NO_SOUND, kind: "none" };
  if (mode === "all" || noOwnExclusion) return { device: "loopback", kind: "all" };
  if (program && !noProgramAudio) return { device: program, kind: "program" };
  return { device: "loopbackWithoutChrome", kind: "own" };
}

/**
 * Answers the page's getDisplayMedia: the page shows its picker with the
 * screens and windows sent here, and exactly the one chosen is shared (on
 * Wayland the portal asks instead). macOS's own picker is not used: in
 * Electron it is experimental, and a window picked there could end up as the
 * whole screen.
 *
 * Sound comes from system loopback when the share asked for it, leaving this
 * app's own playback out wherever the system can (see shareAudioMode). A
 * window carries just its own program's sound where the system can capture
 * one program (see appAudio.ts).
 */
export function handleDisplayMedia(getWindow: () => BrowserWindow | null, log: Log = () => {}): void {
  const wayland = process.platform === "linux" && (process.env.XDG_SESSION_TYPE === "wayland" || !!process.env.WAYLAND_DISPLAY);
  session.defaultSession.setDisplayMediaRequestHandler(
    (request, callback) => {
      void (async () => {
        if (!isAppUrl(request.securityOrigin) && originOf(request.frame?.url) !== ORIGIN) {
          callback({});
          return;
        }
        try {
          if (wayland) {
            // the portal asks the person itself, and hands back just the one they chose
            const [source] = await desktopCapturer.getSources({ types: ["screen", "window"] });
            callback(source ? { video: source } : {});
            return;
          }
          const sources = await desktopCapturer.getSources({ types: ["screen", "window"], thumbnailSize: { width: 320, height: 180 }, fetchWindowIcons: true });
          const win = getWindow();
          if (!win) {
            callback({});
            return;
          }
          // Nothing to offer means the system is not letting this app record,
          // whatever its settings say. The page is told so it can explain it
          // rather than showing an empty picker or blaming the sound.
          const access = sources.length === 0 || sources.every((src) => src.thumbnail.isEmpty()) ? await screenAccess() : null;
          if (access && !access.usable) {
            const requestId = randomUUID();
            win.webContents.send(IPC.displaySources, { requestId, audio: false, sources: [], permission: { status: access.status, stale: access.stale } } satisfies DisplaySourceRequest);
            pending.set(requestId, { resolve: () => callback({}) });
            if (!win.isVisible()) win.show();
            setTimeout(() => {
              if (pending.delete(requestId)) callback({});
            }, 120_000);
            return;
          }
          if (sources.length === 0) {
            callback({});
            return;
          }
          const mode = shareAudioMode();
          const audio = request.audioRequested && mode !== "none";
          lastAnswer = null;
          const answerWith = async (source: DesktopCapturerSource | undefined, withAudio: boolean) => {
            // only the share being answered now can be the one whose sound fails; a cancelled or silent one is none
            lastAnswer = null;
            if (!source) {
              callback({});
              return;
            }
            if (!withAudio || !audio) {
              callback({ video: source });
              return;
            }
            const sound = await audioDevice(source, mode, log);
            if (sound.kind === "none") {
              log("share", { source: source.id, audio: "none (self-monitoring program)" });
              callback({ video: source });
              return;
            }
            lastAnswer = { sourceId: source.id, sound, at: Date.now() };
            log("share", { source: source.id, audio: sound.device });
            // Electron hands any device name over to Chromium; its typings only list "loopback" and "loopbackWithMute"
            callback({ video: source, audio: sound.device as "loopback" });
          };
          // the same share again, after its sound failed to start: no second trip through the picker
          const again = retry && Date.now() < retry.until ? sources.find((s) => s.id === retry!.sourceId) : undefined;
          retry = null;
          if (again) {
            await answerWith(again, true);
            return;
          }
          // The page does not get to answer this. See displayPicker.ts.
          const answer = await askForDisplaySource(
            win,
            sources.map((s) => ({
              id: s.id,
              name: s.name,
              kind: s.id.startsWith("screen:") ? "screen" : "window",
              thumbnail: s.thumbnail.isEmpty() ? "" : s.thumbnail.toDataURL(),
              ...(s.appIcon && !s.appIcon.isEmpty() ? { icon: s.appIcon.toDataURL() } : {}),
            })),
            { audio },
          );
          await answerWith(answer ? sources.find((s) => s.id === answer.sourceId) : undefined, answer?.withAudio === true);
        } catch {
          callback({});
        }
      })();
    },
  );
}

/**
 * What sound a screen share can carry on this computer.
 *
 * "own": everything the computer plays except this app, so nobody in the call
 * hears themselves come back through the share. Chromium's
 * loopbackWithoutChrome capture does this with Windows' process loopback
 * (excluding this app's process tree) and with ScreenCaptureKit on macOS.
 * It is asked for by name: asking for plain "loopback" with the page's
 * restrictOwnAudio constraint is meant to get the same, but the constraint
 * never reaches Electron's handler, so the call leaked into every share.
 * Windows 10 2004 (build 19041) and later have the process loopback it is
 * built on: OBS captures single programs from that build, Vesktop users
 * report it working on 19044, Microsoft documents 20348. A share whose
 * sound will not start is tried again with plain loopback (shareAudioFailed).
 * macOS 13 and later, measured on this build's Mac (the app's own sound at
 * silence, other programs still heard).
 *
 * "all": the whole system's sound, this app included, where the exclusion is
 * not there (Windows 10 before 2004). Sharing sound still works; the call may echo.
 *
 * "none": no system sound can be shared (Linux, macOS before 13).
 */
export type ShareAudioMode = "own" | "all" | "none";

export function shareAudioMode(): ShareAudioMode {
  const forced = testHook("EVILION_SHARE_AUDIO_MODE");
  if (forced === "own" || forced === "all" || forced === "none") return forced;
  const [major = 0, , build = 0] = os.release().split(".").map(Number);
  if (process.platform === "win32") return build >= WINDOWS_PROCESS_LOOPBACK_BUILD ? "own" : "all";
  // Darwin 22 is macOS 13
  if (process.platform === "darwin") return major >= 22 ? "own" : "none";
  return "none";
}

/** The page chose a source (null: the person cancelled). */
/**
 * Dismisses the explainer the page is showing. It takes no source and cannot
 * start a share: the arguments are gone deliberately, so that no future edit
 * can make the page the thing that answers a capture request again.
 */
export function chooseDisplaySource(requestId: string): void {
  pending.get(requestId)?.resolve();
}

/**
 * Bug reports in words and numbers, in one place: what each status is called
 * and how it looks, what a report may carry, and the facts about this app and
 * device that the reporter can choose to send with it.
 */
import type { BugContext, BugCounts, BugFile, BugKind, BugStatus, MemberBug, StaffBug } from "@/api/bugs";
import type { CaseCounts } from "@/api/moderation";
import { desktopBridge } from "@/lib/desktopBridge";
import { t } from "@/i18n";

/** The limits the server holds a report to, checked here first so nothing is sent to be refused. */
export const BUG_LIMITS = {
  subjectMin: 3,
  subjectMax: 120,
  descriptionMin: 10,
  descriptionMax: 4000,
  files: 5,
  imageBytes: 10 * 1024 * 1024,
  videoBytes: 50 * 1024 * 1024,
  totalBytes: 100 * 1024 * 1024,
} as const;

export const BUG_IMAGE_TYPES = ["image/png", "image/jpeg", "image/gif", "image/webp"];
export const BUG_VIDEO_TYPES = ["video/mp4", "video/quicktime", "video/webm"];
/** What the file picker offers. */
export const BUG_ACCEPT = [...BUG_IMAGE_TYPES, ...BUG_VIDEO_TYPES].join(",");

export function isBugVideo(mime: string): boolean {
  return mime.startsWith("video/");
}

/**
 * Why a file cannot be added to a report, or null when it can. `taken` is
 * what the report already holds: how many files and how many bytes.
 */
export function bugFileProblem(file: { type: string; size: number }, taken: { count: number; bytes: number }): string | null {
  const type = file.type.toLowerCase();
  const video = BUG_VIDEO_TYPES.includes(type);
  if (!video && !BUG_IMAGE_TYPES.includes(type)) return t("bugs.form.fileType");
  if (taken.count >= BUG_LIMITS.files) return t("bugs.form.tooManyFiles", { count: BUG_LIMITS.files });
  if (!video && file.size > BUG_LIMITS.imageBytes) return t("bugs.form.imageTooLarge");
  if (video && file.size > BUG_LIMITS.videoBytes) return t("bugs.form.videoTooLarge");
  if (taken.bytes + file.size > BUG_LIMITS.totalBytes) return t("bugs.form.filesTooLarge");
  return null;
}

// --- statuses ------------------------------------------------------------------

/** How a status is coloured: the panel's tag tones, and none for a finished report. */
export type BugTone = "info" | "warn" | "done" | "";

/**
 * What a status is called. Suggestions share the statuses with bug reports,
 * in their own words: in progress is Planned, resolved is Done, and won't
 * fix is Won't Do.
 */
export function bugStatusLabel(s: BugStatus, kind: BugKind = "bug"): string {
  const idea = kind === "suggestion";
  switch (s) {
    case "open":
      return t("bugs.status.open");
    case "in_progress":
      return idea ? t("suggestions.status.planned") : t("bugs.status.inProgress");
    case "resolved":
      return idea ? t("suggestions.status.done") : t("bugs.status.resolved");
    case "wont_fix":
      return idea ? t("suggestions.status.wontDo") : t("bugs.status.wontFix");
    default:
      return t("bugs.status.closed");
  }
}

/** One sentence on what a status means, for the menu staff pick it from. */
export function bugStatusHint(s: BugStatus, kind: BugKind = "bug"): string {
  const idea = kind === "suggestion";
  switch (s) {
    case "open":
      return t("bugs.status.openHint");
    case "in_progress":
      return idea ? t("suggestions.status.plannedHint") : t("bugs.status.inProgressHint");
    case "resolved":
      return idea ? t("suggestions.status.doneHint") : t("bugs.status.resolvedHint");
    case "wont_fix":
      return idea ? t("suggestions.status.wontDoHint") : t("bugs.status.wontFixHint");
    default:
      return idea ? t("suggestions.status.closedHint") : t("bugs.status.closedHint");
  }
}

export function bugStatusTone(s: BugStatus): BugTone {
  switch (s) {
    case "open":
      return "info";
    case "in_progress":
      return "warn";
    case "resolved":
      return "done";
    default:
      return "";
  }
}

/** Whether a report is finished with: fixed, not going to be, or closed. */
export function bugFinished(s: BugStatus): boolean {
  return s === "resolved" || s === "wont_fix" || s === "closed";
}

/** A report's kind, for one sent before suggestions existed too. */
export function bugKind(r: { kind?: BugKind | undefined }): BugKind {
  return r.kind === "suggestion" ? "suggestion" : "bug";
}

/** "#12", as reports are referred to. */
export function bugNumber(n: number): string {
  return t("bugs.number", { number: n });
}

// --- counts --------------------------------------------------------------------

/** What waits on the staff: cases the member wrote in last, and reports waiting for an answer. */
export function staffWaiting(cases: CaseCounts | null | undefined, reports: BugCounts | null | undefined): { cases: number; reports: number; total: number } {
  const c = cases?.needs_reply ?? 0;
  const r = reports?.waiting ?? 0;
  return { cases: c, reports: r, total: c + r };
}

// --- files ---------------------------------------------------------------------

/** A file's own name, or what it is when it came without one (a pasted screenshot). */
export function bugFileName(f: Pick<BugFile, "name" | "mime">): string {
  if (f.name) return f.name;
  return isBugVideo(f.mime) ? t("bugs.file.recording") : t("bugs.file.screenshot");
}

// --- the device ----------------------------------------------------------------

/** "Chrome 128", from a user agent: the browser people would name, and its major version. */
export function describeBrowser(ua: string): string {
  // Edge, Opera and Samsung's browser say Chrome too, and Chrome says Safari
  const tests: [RegExp, string][] = [
    [/Edg(?:e|A|iOS)?\/(\d+)/, "Edge"],
    [/OPR\/(\d+)/, "Opera"],
    [/SamsungBrowser\/(\d+)/, "Samsung Internet"],
    [/(?:Firefox|FxiOS)\/(\d+)/, "Firefox"],
    [/(?:CriOS|Chrome)\/(\d+)/, "Chrome"],
    [/Version\/(\d+)[\d.]*(?: Mobile\/\S+)? Safari\//, "Safari"],
  ];
  for (const [re, name] of tests) {
    const m = ua.match(re);
    if (m) return `${name} ${m[1]}`;
  }
  return "";
}

/** "Android 14", "iOS 17", "Windows": the system a user agent names. */
export function describeSystem(ua: string, touchMac = false): string {
  let m: RegExpMatchArray | null;
  // an iPad asks for desktop pages and says it is a Mac; only its touch screen gives it away
  if (/iPad/.test(ua) || (touchMac && /Macintosh/.test(ua))) return "iPadOS";
  if ((m = ua.match(/(?:iPhone|iPod).*? OS (\d+)/))) return `iOS ${m[1]}`;
  if ((m = ua.match(/Android (\d+)/))) return `Android ${m[1]}`;
  if (/CrOS/.test(ua)) return "ChromeOS";
  if (/Windows/.test(ua)) return "Windows";
  if (/Mac OS X|Macintosh/.test(ua)) return "macOS";
  if (/Linux/.test(ua)) return "Linux";
  return "";
}

const DESKTOP_SYSTEMS: Record<string, string> = { win32: "Windows", darwin: "macOS", linux: "Linux" };

export interface DeviceFacts {
  ua: string;
  touch: boolean;
  screen: { w: number; h: number; dpr: number };
  window: { w: number; h: number };
  language: string;
  theme: string;
  customCss: boolean;
  plugins: number;
  build: string;
  desktop: { version: string; platform: string } | null;
  openedFrom: string;
}

/**
 * The context a report carries when the reporter leaves it on: which app and
 * build, the browser and system, the screen and window, the language and
 * look. Never anything from a conversation, and never a key.
 */
export function bugContext(f: DeviceFacts): BugContext {
  const shortSide = Math.min(f.screen.w, f.screen.h);
  const out: BugContext = {
    app: f.desktop ? "desktop" : "web",
    build: f.build,
    browser: describeBrowser(f.ua),
    os: f.desktop ? (DESKTOP_SYSTEMS[f.desktop.platform] ?? f.desktop.platform) : describeSystem(f.ua, f.touch),
    device: f.touch ? (shortSide < 600 ? "phone" : "tablet") : "computer",
    screen: `${f.screen.w} × ${f.screen.h}, ${Math.round(f.screen.dpr * 100) / 100}x`,
    window: `${f.window.w} × ${f.window.h}`,
    language: f.language,
    theme: f.theme.startsWith("custom-") ? "custom" : f.theme,
    custom_css: f.customCss ? "on" : "off",
    plugins: String(f.plugins),
    opened_from: f.openedFrom,
  };
  if (f.desktop?.version) out.app_version = f.desktop.version;
  // nothing empty goes: the server would drop it anyway
  for (const k of Object.keys(out) as (keyof BugContext)[]) if (!out[k]) delete out[k];
  return out;
}

/** The facts about this device, as bugContext wants them. */
export function readDeviceFacts(extra: { language: string; theme: string; customCss: boolean; plugins: number; openedFrom: string }): DeviceFacts {
  const bridge = desktopBridge();
  let build = "";
  try {
    build = typeof __BUILD_ID__ === "string" ? __BUILD_ID__ : "";
  } catch {
    build = "";
  }
  const touch = typeof window !== "undefined" && !!window.matchMedia?.("(pointer: coarse)").matches;
  return {
    ua: typeof navigator !== "undefined" ? navigator.userAgent : "",
    touch,
    screen: typeof screen !== "undefined" ? { w: screen.width, h: screen.height, dpr: window.devicePixelRatio || 1 } : { w: 0, h: 0, dpr: 1 },
    window: typeof window !== "undefined" ? { w: window.innerWidth, h: window.innerHeight } : { w: 0, h: 0 },
    build,
    desktop: bridge ? { version: bridge.version, platform: bridge.platform } : null,
    ...extra,
  };
}

/** One fact of a report's context, as the dialog and the panel list it. */
export interface ContextRow {
  key: keyof BugContext;
  label: string;
  value: string;
}

const CONTEXT_ORDER: (keyof BugContext)[] = ["app", "app_version", "build", "server", "browser", "os", "device", "screen", "window", "language", "theme", "custom_css", "plugins", "opened_from"];

/** A report's context in reading order, with labels and its values put into words. */
export function contextRows(c: BugContext): ContextRow[] {
  const rows: ContextRow[] = [];
  for (const key of CONTEXT_ORDER) {
    const raw = c[key];
    if (!raw) continue;
    rows.push({ key, label: contextLabel(key), value: contextValue(key, raw) });
  }
  return rows;
}

function contextLabel(key: keyof BugContext): string {
  switch (key) {
    case "app":
      return t("bugs.context.app");
    case "app_version":
      return t("bugs.context.appVersion");
    case "build":
      return t("bugs.context.build");
    case "server":
      return t("bugs.context.server");
    case "browser":
      return t("bugs.context.browser");
    case "os":
      return t("bugs.context.system");
    case "device":
      return t("bugs.context.device");
    case "screen":
      return t("bugs.context.screen");
    case "window":
      return t("bugs.context.window");
    case "language":
      return t("bugs.context.language");
    case "theme":
      return t("bugs.context.theme");
    case "custom_css":
      return t("bugs.context.customCss");
    case "plugins":
      return t("bugs.context.plugins");
    default:
      return t("bugs.context.openedFrom");
  }
}

function contextValue(key: keyof BugContext, v: string): string {
  switch (key) {
    case "app":
      return v === "desktop" ? t("bugs.context.appDesktop") : v === "web" ? t("bugs.context.appWeb") : v === "android" ? t("bugs.context.appAndroid") : v;
    case "device":
      return v === "phone" ? t("bugs.context.phone") : v === "tablet" ? t("bugs.context.tablet") : v === "computer" ? t("bugs.context.computer") : v;
    case "custom_css":
      return v === "on" ? t("bugs.context.on") : v === "off" ? t("bugs.context.off") : v;
    case "theme":
      return v === "custom" ? t("bugs.context.customTheme") : v;
    case "opened_from":
      return openedFromLabel(v);
    default:
      return v;
  }
}

function openedFromLabel(v: string): string {
  switch (v) {
    case "dm":
      return t("bugs.from.dm");
    case "group":
      return t("bugs.from.group");
    case "friends":
      return t("bugs.from.friends");
    case "settings":
      return t("bugs.from.settings");
    case "nexa":
      return t("bugs.from.nexa");
    default:
      return v;
  }
}

// --- alerts --------------------------------------------------------------------

/** Something new about a report, worth a sound or a notification: for the reporter, or for staff. */
export type BugAlert = { kind: "reply" | "status"; report: MemberBug } | { kind: "staff"; report: StaffBug; isNew: boolean };

/** Listeners for BugAlert, which notifications register (lib/notify.ts). */
export const bugAlertHooks: ((a: BugAlert) => void)[] = [];

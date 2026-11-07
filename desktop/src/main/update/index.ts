/**
 * Keeping the app up to date from Nexa's own server.
 *
 * Windows (NSIS) and the Linux AppImage use electron-updater; the Mac build
 * uses its own updater (see mac.ts); a .deb can only be told there is a new
 * version. Checks run shortly after start and then every minute, so a new
 * version is announced (the page shows a banner) soon after it is published;
 * downloads happen in the background and install when the person presses
 * Update. A check that fails waits longer before the next one, so a broken
 * download is not fetched again every minute.
 *
 * Nothing installs on its own when the app quits: on Windows that ran the
 * installer in the background after a quit or at shutdown, which closed a copy
 * opened again meanwhile (the app seemed to restart by itself), and a shutdown
 * could cut it off halfway. A downloaded update stays in the cache, so the next
 * start offers it again at once. While one waits to be installed the checks go
 * on, so a newer release published meanwhile replaces it instead of being
 * installed old and then updated again straight after.
 */
import { app } from "electron";
import { EventEmitter } from "node:events";
import type { UpdateState } from "../../shared/bridge";
import { compareVersions, parseManifest } from "./manifest";
import { MAC_FEED, canReplace, installMacUpdate, stageMacUpdate, type MacUpdate } from "./mac";
import { testHook } from "../config";
import { fetchRelease } from "./releaseSignature";

const FIRST_CHECK_MS = 10_000;
const EVERY_MS = 60_000;
/** The longest a run of failed checks waits before trying again. */
const MAX_BACKOFF_MS = 15 * 60_000;
/** A check or download that has moved on for this long is given up and tried again. */
const STALL_MS = 3 * 60_000;

type AutoUpdater = typeof import("electron-updater").autoUpdater;

export declare interface Updater {
  on(event: "state", listener: (s: UpdateState) => void): this;
}

export class Updater extends EventEmitter {
  state: UpdateState = { status: "idle" };
  private timer: ReturnType<typeof setInterval> | null = null;
  private auto: AutoUpdater | null = null;
  private mac: MacUpdate | null = null;
  private busy = false;
  /** Failed checks in a row, and when the next background check may run. */
  private failures = 0;
  private nextAt = 0;
  /** Counts checks, so one given up as stalled no longer changes anything when it ends. */
  private runs = 0;
  /** When the check under way last showed signs of life. */
  private activity = 0;
  /** Stops the download under way, if it can be stopped. */
  private cancel: (() => void) | null = null;
  private restarting = false;

  constructor(
    /** Where releases are published, e.g. https://nexa.pm/desktop/ */
    private readonly feed: string,
    /** The download page, for builds that cannot update themselves. */
    private readonly downloadPage: string,
    private readonly quitForUpdate: (install: () => void) => void,
    private readonly log: (msg: string, extra?: Record<string, unknown>) => void,
  ) {
    super();
  }

  private get kind(): "auto" | "mac" | "manual" {
    if (process.platform === "darwin") return "mac";
    if (process.platform === "win32" || (process.platform === "linux" && process.env.APPIMAGE)) return "auto";
    return "manual";
  }

  start(): void {
    if (!app.isPackaged && !testHook("EVILION_UPDATE_TEST")) return;
    setTimeout(() => void this.check({ quiet: true }), FIRST_CHECK_MS);
    this.timer = setInterval(() => {
      if (this.busy && Date.now() - this.activity > STALL_MS) this.giveUp();
      if (Date.now() >= this.nextAt) void this.check({ quiet: true });
    }, EVERY_MS);
  }

  stop(): void {
    if (this.timer) clearInterval(this.timer);
    this.timer = null;
  }

  private set(next: UpdateState): void {
    this.activity = Date.now();
    this.state = { ...next, lastCheck: next.lastCheck ?? this.state.lastCheck };
    this.emit("state", this.state);
  }

  /**
   * Looks for a newer version. The minute-by-minute checks are quiet: they do
   * not flash "Checking…" in the settings page, and while an update is
   * already being downloaded or waits to be installed they leave it alone.
   * One asked for from the settings page or a menu always runs.
   */
  async check({ quiet = false }: { quiet?: boolean } = {}): Promise<void> {
    if (this.busy || this.restarting) return;
    this.busy = true;
    const run = ++this.runs;
    this.activity = Date.now();
    // an update already downloaded is replaced only by a newer release
    const staged = this.state.status === "ready" ? (this.state.version ?? null) : null;
    if (!staged && (!quiet || this.state.status === "error")) this.set({ status: "checking", lastCheck: Date.now() });
    try {
      if (staged) {
        const latest = await this.published();
        if (run !== this.runs || !latest || compareVersions(latest, staged) <= 0) return;
        this.log("a newer update replaces the downloaded one", { staged, latest });
      }
      if (this.kind === "auto") await this.checkAuto(run);
      else if (this.kind === "mac") await this.checkMac(run);
      else await this.checkManual();
      if (run !== this.runs) return;
      this.failures = 0;
      this.nextAt = 0;
    } catch (err) {
      if (run !== this.runs) return;
      this.failed(err instanceof Error ? err.message : String(err));
    } finally {
      if (run === this.runs) {
        this.busy = false;
        this.cancel = null;
      }
    }
  }

  private failed(error: string): void {
    this.log("update check failed", { err: error });
    this.failures++;
    this.nextAt = Date.now() + Math.min(MAX_BACKOFF_MS, EVERY_MS * 2 ** this.failures);
    this.set({ status: "error", error, lastCheck: Date.now() });
  }

  /**
   * A check or download that stopped moving: without this the updater stayed
   * busy for good, so the app never looked for an update again until it was
   * restarted, and the page showed a download that never finished.
   */
  private giveUp(): void {
    this.runs++;
    this.busy = false;
    const cancel = this.cancel;
    this.cancel = null;
    try {
      cancel?.();
    } catch {
      // already over
    }
    this.failed("the update stopped downloading");
  }

  /** The newest version published for this build. */
  private async published(): Promise<string | null> {
    const feed = this.kind === "mac" ? MAC_FEED : process.platform === "win32" ? "signed-latest.yml" : "signed-latest-linux.yml";
    const body = await fetchRelease(this.feed, feed);
    return body === null ? null : parseManifest(body).version || null;
  }

  private async checkAuto(run: number): Promise<void> {
    if (!this.auto) {
      const { autoUpdater } = await import("electron-updater");
      const { SignedReleaseProvider } = await import("./provider");
      autoUpdater.setFeedURL({ provider: "custom", updateProvider: SignedReleaseProvider, url: this.feed });
      autoUpdater.autoDownload = true;
      autoUpdater.autoInstallOnAppQuit = false;
      autoUpdater.logger = null;
      autoUpdater.on("download-progress", (p) => this.set({ status: "downloading", version: this.state.version, progress: p.percent / 100 }));
      autoUpdater.on("update-downloaded", (info) => this.set({ status: "ready", version: info.version, lastCheck: Date.now() }));
      // the check's own failure is told by check(); these are the download running on, and an installer that would not start
      autoUpdater.on("error", (err) => {
        if (this.restarting) {
          this.restarting = false;
          this.start();
        } else if (this.state.status !== "downloading") return;
        this.set({ status: "error", error: err.message, lastCheck: Date.now() });
      });
      this.auto = autoUpdater;
    }
    const result = await this.auto.checkForUpdates();
    if (run !== this.runs) return;
    const version = result?.updateInfo.version;
    if (!version || compareVersions(version, app.getVersion()) <= 0) {
      this.set({ status: "idle", lastCheck: Date.now() });
      return;
    }
    this.set({ status: "downloading", version, progress: 0 });
    this.cancel = () => result?.cancellationToken?.cancel();
    // the download runs on; "update-downloaded" marks it ready
    await result?.downloadPromise;
  }

  private async checkMac(run: number): Promise<void> {
    const newer = await this.newerThanRunning(MAC_FEED);
    if (run !== this.runs) return;
    if (!newer) {
      this.set({ status: "idle", lastCheck: Date.now() });
      return;
    }
    if (!canReplace()) {
      this.set({ status: "manual", version: newer, downloadUrl: this.downloadPage, lastCheck: Date.now() });
      return;
    }
    this.set({ status: "downloading", version: newer, progress: 0 });
    const staged = await stageMacUpdate(this.feed, (progress) => {
      if (run === this.runs) this.set({ status: "downloading", version: newer, progress });
    });
    if (run !== this.runs) return;
    if (!staged) {
      this.set({ status: "idle", lastCheck: Date.now() });
      return;
    }
    this.mac = staged;
    this.set({ status: "ready", version: staged.version, lastCheck: Date.now() });
  }

  private async checkManual(): Promise<void> {
    const newer = await this.newerThanRunning("signed-latest-linux.yml");
    this.set(newer ? { status: "manual", version: newer, downloadUrl: this.downloadPage, lastCheck: Date.now() } : { status: "idle", lastCheck: Date.now() });
  }

  private async newerThanRunning(manifest: string): Promise<string | null> {
    const body = await fetchRelease(this.feed, manifest);
    if (body === null) return null;
    const m = parseManifest(body);
    return m.version && compareVersions(m.version, app.getVersion()) > 0 ? m.version : null;
  }

  /** Restarts into the downloaded update. */
  restart(): void {
    // once: a second press, or a check finishing meanwhile, must not start another install
    if (this.state.status !== "ready" || this.restarting) return;
    this.restarting = true;
    this.stop();
    if (this.kind === "mac" && this.mac) {
      const staged = this.mac;
      this.quitForUpdate(() => installMacUpdate(staged));
      return;
    }
    if (this.auto) {
      const auto = this.auto;
      this.quitForUpdate(() => auto.quitAndInstall(true, true));
      return;
    }
    this.restarting = false;
  }
}

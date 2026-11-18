import { Play, Radio } from "lucide-react";
import { useState } from "react";
import { t } from "@/i18n";
import { isDemo } from "@/lib/demoMode";
import { formatDuration } from "@/lib/time";
import s from "./YouTubePlayer.module.css";

/**
 * A YouTube video, played straight from YouTube's privacy-enhanced host. It
 * went through this server until 2026-11-18, when YouTube's bot check on the
 * proxy's address stopped every video, so YouTube now sees the viewer's
 * address while they watch (and only once they press play). The frame keeps
 * its own origin, which the player needs to run, and is sandboxed so it cannot
 * navigate the app; the referrer is only Nexa's origin, which YouTube asks of
 * an embedder.
 */
export function YouTubeFrame({ id, title, start }: { id: string; title: string; start?: number | undefined }) {
  const at = start && start > 0 ? `&start=${Math.floor(start)}` : "";
  return (
    <iframe
      className={s.frame}
      src={`https://www.youtube-nocookie.com/embed/${encodeURIComponent(id)}?autoplay=1&rel=0&playsinline=1${at}`}
      title={title}
      allow="autoplay; encrypted-media; picture-in-picture; fullscreen"
      allowFullScreen
      sandbox="allow-scripts allow-same-origin allow-presentation allow-popups allow-popups-to-escape-sandbox"
      referrerPolicy="strict-origin-when-cross-origin"
      data-testid="youtube-frame"
    />
  );
}

/** The 11 characters YouTube names a video by. */
export function isYouTubeID(id: string | undefined): id is string {
  return !!id && /^[A-Za-z0-9_-]{11}$/.test(id);
}

interface VideoProps {
  id: string;
  title: string;
  /** Its picture, already a proxy address. */
  poster: string | null | undefined;
  start?: number | undefined;
  duration?: number | undefined;
  live?: "now" | "upcoming" | undefined;
  w?: number | undefined;
  h?: number | undefined;
}

/**
 * A YouTube video's picture, with how long it runs and whether it is live,
 * that becomes the player when pressed. Nothing of YouTube's loads before
 * then; the picture comes through this server's proxy like every other.
 * The homepage demo plays nothing and asks for an account instead.
 */
export function YouTubeVideo({ id, title, poster, start, duration, live, w, h }: VideoProps) {
  const [play, setPlay] = useState(false);
  // a Short stands up; everything else is the 16:9 YouTube plays at
  const tall = !!w && !!h && h > w;
  return (
    <div className={s.video} data-tall={tall || undefined} data-testid="youtube-video">
      {play ? (
        <YouTubeFrame id={id} title={title} start={start} />
      ) : (
        <button type="button" className={s.poster} onClick={() => (isDemo ? window.dispatchEvent(new Event("nexa-demo-account")) : setPlay(true))} aria-label={t("chat.embeds.playVideoNamed", { name: title })}>
          {poster ? <img src={poster} alt="" loading="lazy" referrerPolicy="no-referrer" draggable={false} onError={(ev) => (ev.currentTarget.style.visibility = "hidden")} /> : null}
          <span className={s.play} aria-hidden="true">
            <Play size={26} fill="currentColor" />
          </span>
          {live === "now" ? (
            <span className={s.badge} data-live="true">
              <Radio size={13} aria-hidden="true" /> {t("chat.embeds.live")}
            </span>
          ) : live === "upcoming" ? (
            <span className={s.badge}>{t("chat.embeds.upcoming")}</span>
          ) : duration && duration > 0 ? (
            <span className={s.badge}>{formatDuration(duration)}</span>
          ) : null}
        </button>
      )}
    </div>
  );
}

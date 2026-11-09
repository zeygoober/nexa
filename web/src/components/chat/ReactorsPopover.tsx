import { PersonName } from "@/components/common/PersonName";
import { useMediaAnimation } from "@/lib/mediaAnimation";
import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { Avatar } from "@/components/common/Avatar";
import { Spinner } from "@/components/common/Spinner";
import { Twemoji } from "@/components/common/Twemoji";
import { portalRoot } from "@/hooks/usePortalRoot";
import { emojiUrl } from "@/api/users";
import { useUsers, displayName } from "@/stores/users";
import { useLayerPresence } from "./useLayerPresence";
import s from "./Reactions.module.css";

import { t } from "@/i18n";

/** How many reactors are listed; the server sends at most this many. */
export const REACTORS_SHOWN = 20;

/**
 * Who reacted with one emoji, in a small card by the reaction: a long press or
 * a right click on it, or the menu key on it from the keyboard. It was a
 * tooltip filled in on mouse-over, which no phone and no keyboard ever saw.
 */
export function ReactorsPopover({ anchor, emoji, custom, label, count, gives, ids, onClose }: {
  anchor: DOMRect;
  emoji: string;
  custom: { id: string; animated: boolean } | null;
  label: string;
  count: number;
  /** The role this reaction gives, if it gives one. */
  gives?: string;
  /** Who reacted, as far as the server says; null while it is being asked. */
  ids: string[] | null;
  onClose: () => void;
}) {
  useMediaAnimation();
  const users = useUsers((u) => u.byId);
  const ref = useRef<HTMLDivElement>(null);
  const [at, setAt] = useState<{ left: number; top: number } | null>(null);
  useLayerPresence();

  useLayoutEffect(() => {
    const box = ref.current?.getBoundingClientRect();
    if (!box) return;
    const vw = window.innerWidth;
    const vh = window.innerHeight;
    let top = anchor.top - box.height - 8;
    if (top < 8) top = anchor.bottom + 8;
    const left = Math.max(8, Math.min(anchor.left + anchor.width / 2 - box.width / 2, vw - box.width - 8));
    setAt({ left, top: Math.max(8, Math.min(top, vh - box.height - 8)) });
  }, [anchor, ids]);

  // focus in, back to the reaction after; Escape, a press elsewhere or a scroll closes it
  useEffect(() => {
    const opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    ref.current?.focus({ preventScroll: true });
    const down = (e: PointerEvent) => {
      if (!ref.current?.contains(e.target as Node)) onClose();
    };
    const key = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return;
      e.stopPropagation();
      onClose();
    };
    const scroll = (e: Event) => {
      if (!ref.current?.contains(e.target as Node)) onClose();
    };
    document.addEventListener("pointerdown", down, true);
    document.addEventListener("keydown", key, true);
    window.addEventListener("scroll", scroll, true);
    window.addEventListener("resize", onClose);
    return () => {
      document.removeEventListener("pointerdown", down, true);
      document.removeEventListener("keydown", key, true);
      window.removeEventListener("scroll", scroll, true);
      window.removeEventListener("resize", onClose);
      if (opener?.isConnected) opener.focus({ preventScroll: true });
    };
  }, [onClose]);

  const more = ids ? Math.max(0, count - ids.length) : 0;
  return createPortal(
    <div
      ref={ref}
      className={s.popover}
      role="dialog"
      aria-label={t("chat.reactions.whoReacted", { emoji: label || emoji })}
      tabIndex={-1}
      data-modal-layer=""
      data-testid="reactors"
      style={at ? { left: at.left, top: at.top } : { left: -9999, top: -9999 }}
    >
      <div className={s.head}>
        {custom ? <img className={s.headEmoji} src={emojiUrl(custom.id, custom.animated, 48)} alt="" /> : <Twemoji text={emoji} className={s.headEmoji} fallbackClassName={s.headEmojiText} title="" />}
        <span className={s.headLabel}>{label}</span>
        <span className={s.headCount}>{count}</span>
      </div>
      {gives ? <p className={s.gives}>{t("chat.reactionRoles.gives", { role: gives })}</p> : null}
      {ids === null ? (
        <div className={s.pending}><Spinner size={18} /></div>
      ) : (
        <ul className={s.list}>
          {ids.map((id) => (
            <li key={id} className={s.person}>
              <Avatar id={id} avatarId={users[id]?.avatar_id ?? null} name={displayName(users[id])} size={24} />
              <PersonName userId={id} className={s.name} dir="auto">{displayName(users[id])}</PersonName>
            </li>
          ))}
          {more > 0 ? <li className={s.more}>{t("chat.reactions.andMore", { count: more })}</li> : null}
        </ul>
      )}
    </div>,
    portalRoot(),
  );
}

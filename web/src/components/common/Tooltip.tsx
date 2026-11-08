import { cloneElement, isValidElement, useCallback, useEffect, useId, useRef, useState, type ReactElement, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { portalRoot } from "@/hooks/usePortalRoot";
import { luminance, mixOk, readableOn, toHex } from "@/lib/color";
import { surfaceOf } from "@/markdown/colors";
import s from "./Tooltip.module.css";

type Side = "top" | "right" | "bottom" | "left";

interface Props {
  text: string;
  /** The keys that do the same, shown quietly after the words: "Ctrl+Shift+M". */
  keys?: string;
  side?: Side;
  /** Drawn instead of the words, which stay the trigger's name for a screen reader. On a touch screen a card opens on a tap. */
  card?: ReactNode;
  children: ReactNode;
  className?: string;
  delay?: number;
}

/** Touch devices report a hover that never ends, which strands tooltips on screen. */
export function coarsePointer(): boolean {
  return typeof window !== "undefined" && !!window.matchMedia?.("(pointer: coarse)").matches;
}

const GAP = 8;

/** The ink a bubble is mixed toward: nearly black, in Nexa's own violet. */
const INK = "#120d1d";
/** What a bubble over a panel that is black already is mixed toward instead. */
const GLOW = "#b9aee0";

/**
 * A tooltip's colours: the panel it came from, taken most of the way to a
 * deep ink. The bubble stands out crisply from its panel yet still belongs to
 * it, violet over Nexa's own panels and rose over a rose profile card, and a
 * panel that is black already gets a lighter step of itself instead. The
 * words are whichever of black and white reads on it, which is nearly always
 * white.
 *
 * It used to be a shade one step darker than a light panel, which on the
 * light theme's white and pale panels came out as a pale grey box.
 */
export function tooltipPaint(el: Element): { bg: string; fg: string } {
  const under = toHex(surfaceOf(el).bg);
  const l = luminance(under);
  const bg = l < 0.01 ? mixOk(under, GLOW, 0.2) : mixOk(under, INK, l > 0.18 ? 0.84 : 0.58);
  return { bg, fg: readableOn(bg) };
}

/**
 * Places the bubble beside the trigger, nudged back on screen if it would not
 * fit, with where its arrow points. Left and right are the reading direction's:
 * in a right-to-left language the server rail sits on the right edge, and its
 * names, asked for on the "right", came out on top of the rail itself.
 */
function place(anchor: DOMRect, box: DOMRect, wanted: Side): { left: number; top: number; side: Side; arrow: number } {
  const vw = window.innerWidth;
  const vh = window.innerHeight;
  let side = wanted;
  if ((side === "left" || side === "right") && document.documentElement.dir === "rtl") side = side === "left" ? "right" : "left";
  // and a side with no room for it is swapped for the other one
  if (side === "right" && anchor.right + GAP + box.width > vw - 4 && anchor.left - GAP - box.width >= 4) side = "left";
  else if (side === "left" && anchor.left - GAP - box.width < 4 && anchor.right + GAP + box.width <= vw - 4) side = "right";
  let left = anchor.left + anchor.width / 2 - box.width / 2;
  let top = anchor.top - box.height - GAP;
  if (side === "bottom") top = anchor.bottom + GAP;
  if (side === "left" || side === "right") {
    top = anchor.top + anchor.height / 2 - box.height / 2;
    left = side === "left" ? anchor.left - box.width - GAP : anchor.right + GAP;
  }
  // a bubble above the very top of the window flips below, and the reverse
  let placed = side;
  if (side === "top" && top < 4) {
    top = anchor.bottom + GAP;
    placed = "bottom";
  } else if (side === "bottom" && top + box.height > vh - 4) {
    top = anchor.top - box.height - GAP;
    placed = "top";
  }
  left = Math.max(4, Math.min(left, vw - box.width - 4));
  top = Math.max(4, Math.min(top, vh - box.height - 4));
  // the arrow stays on the trigger's middle even when the bubble was pushed along the edge
  const along = placed === "top" || placed === "bottom" ? anchor.left + anchor.width / 2 - left : anchor.top + anchor.height / 2 - top;
  const length = placed === "top" || placed === "bottom" ? box.width : box.height;
  return { left, top, side: placed, arrow: Math.max(10, Math.min(length - 10, along)) };
}

/**
 * A hover tooltip drawn straight onto the page rather than inside whatever it
 * labels. A message row clips what paints outside it, so a bubble rendered in
 * place there is simply never seen.
 */
export function Tooltip({ text, keys, side = "top", card, children, className, delay = 250 }: Props) {
  const wrap = useRef<HTMLSpanElement>(null);
  const bubble = useRef<HTMLSpanElement>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [open, setOpen] = useState(false);
  const [at, setAt] = useState<{ left: number; top: number; side: Side; arrow: number } | null>(null);
  const [paint, setPaint] = useState<{ bg: string; fg: string } | null>(null);
  const tipId = useId();

  const clear = useCallback(() => {
    if (timer.current) clearTimeout(timer.current);
    timer.current = null;
  }, []);
  useEffect(() => clear, [clear]);

  // measured after it is in the DOM, so the bubble's own size is known
  useEffect(() => {
    if (!open) return;
    const anchor = wrap.current?.getBoundingClientRect();
    const box = bubble.current?.getBoundingClientRect();
    if (anchor && box) setAt(place(anchor, box, side));
    if (wrap.current) setPaint(tooltipPaint(wrap.current));
  }, [open, side, text, keys]);

  const show = () => {
    if (coarsePointer()) return;
    clear();
    timer.current = setTimeout(() => setOpen(true), delay);
  };
  const hide = useCallback(() => {
    if (timer.current) clearTimeout(timer.current);
    timer.current = null;
    setOpen(false);
    setAt(null);
  }, []);

  // WCAG 1.4.13: content that appears on hover or focus has to be dismissable
  // without moving either. Every other handler here is a pointer or focus one.
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") hide();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [open, hide]);

  // A card that a tap opened goes with the next press anywhere else, or a scroll, as nothing hovers there to end it.
  const tapped = !!card && coarsePointer();
  useEffect(() => {
    if (!open || !tapped) return;
    const away = (e: Event) => {
      if (!wrap.current?.contains(e.target as Node)) hide();
    };
    document.addEventListener("pointerdown", away, true);
    window.addEventListener("scroll", hide, true);
    return () => {
      document.removeEventListener("pointerdown", away, true);
      window.removeEventListener("scroll", hide, true);
    };
  }, [open, tapped, hide]);

  // The bubble is an orphan in a portal unless something points at it. Where
  // the trigger has no name of its own the tooltip *is* its name; where it
  // already says the same thing, saying it twice is worse than not linking it.
  const only = isValidElement(children) ? (children as ReactElement<Record<string, unknown>>) : null;
  const named = typeof only?.props["aria-label"] === "string" || typeof only?.props["title"] === "string";
  const sameText = typeof only?.props["aria-label"] === "string" && (only.props["aria-label"] as string).trim() === text.trim();
  const child = !only
    ? children
    : !named
      ? cloneElement(only, { "aria-label": text, "aria-describedby": open ? tipId : undefined })
      : sameText
        ? only
        : cloneElement(only, { "aria-describedby": open ? [only.props["aria-describedby"], tipId].filter(Boolean).join(" ") : only.props["aria-describedby"] });

  return (
    <span
      ref={wrap}
      className={[s.wrap, className ?? ""].join(" ")}
      onMouseEnter={show}
      onMouseLeave={hide}
      onFocus={show}
      onBlur={hide}
      onPointerDown={tapped ? undefined : hide}
      onClick={tapped ? () => (open ? hide() : setOpen(true)) : hide}
    >
      {child}
      {open
        ? createPortal(
            <span
              ref={bubble}
              id={tipId}
              className={s.tip}
              role="tooltip"
              data-side={at?.side ?? side}
              data-card={card ? "" : undefined}
              style={
                {
                  ...(at ? { left: at.left, top: at.top, "--arrow": `${at.arrow}px` } : { left: -9999, top: -9999 }),
                  ...(paint ? { "--tooltip-bg": paint.bg, "--tooltip-text": paint.fg } : {}),
                } as React.CSSProperties
              }
            >
              {card ?? (
                <span className={s.words}>
                  {text}
                  {keys ? <span className={s.keys} dir="ltr">{keys}</span> : null}
                </span>
              )}
            </span>,
            // tooltips come and go too often to each follow full screen; they look where to mount as they open
            portalRoot(),
          )
        : null}
    </span>
  );
}

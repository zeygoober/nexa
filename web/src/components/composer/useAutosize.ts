import { useLayoutEffect, type RefObject } from "react";

/** What decides how tall a textarea's text lays out. */
const COPIED = [
  "box-sizing",
  "width",
  "padding-top",
  "padding-right",
  "padding-bottom",
  "padding-left",
  "border-top-width",
  "border-right-width",
  "border-bottom-width",
  "border-left-width",
  "border-top-style",
  "border-right-style",
  "border-bottom-style",
  "border-left-style",
  "font-family",
  "font-size",
  "font-weight",
  "font-style",
  "font-variant",
  "font-stretch",
  "font-feature-settings",
  "font-kerning",
  "letter-spacing",
  "word-spacing",
  "line-height",
  "text-indent",
  "text-transform",
  "tab-size",
  "white-space",
  "word-break",
  "overflow-wrap",
  "direction",
] as const;

let mirror: HTMLTextAreaElement | null = null;

/**
 * The height a textarea's text needs, measured on a hidden copy. Shrinking
 * the real box to measure it made every scroller around it clamp its position
 * for a moment and never give it back: typing in a message box several lines
 * tall threw the conversation up by those lines.
 */
function neededHeight(el: HTMLTextAreaElement): number {
  if (!mirror || !mirror.isConnected) {
    mirror = document.createElement("textarea");
    mirror.setAttribute("aria-hidden", "true");
    mirror.tabIndex = -1;
    mirror.rows = 1;
    const st = mirror.style;
    st.position = "fixed";
    st.top = "0";
    st.left = "-10000px";
    st.visibility = "hidden";
    st.pointerEvents = "none";
    st.overflow = "hidden";
    st.height = "0";
    st.minHeight = "0";
    st.maxHeight = "none";
    st.zIndex = "-1";
    document.body.appendChild(mirror);
  }
  const cs = getComputedStyle(el);
  for (const p of COPIED) mirror.style.setProperty(p, cs.getPropertyValue(p));
  // Only what is typed decides the height. Measured with it, a placeholder
  // too long for a phone ("Message @someone with a long name") wrapped and
  // made the empty box two lines tall; the placeholder keeps to one line.
  mirror.placeholder = "";
  mirror.value = el.value;
  const border = cs.boxSizing === "border-box" ? parseFloat(cs.borderTopWidth) + parseFloat(cs.borderBottomWidth) : 0;
  return mirror.scrollHeight + (border || 0);
}

/** Grows a textarea with its content up to maxPx. */
export function useAutosize(ref: RefObject<HTMLTextAreaElement | null>, value: string, maxPx: number): void {
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    // An empty box is one line, which `rows` already says. Measured instead,
    // WebKit answers 0 for a textarea that has never held text, and a sheet
    // still loading gives the wrong line: the box came up with no room for its
    // placeholder and nothing to click until something was typed.
    if (!value) {
      if (el.style.height) el.style.height = "";
      if (el.style.overflowY !== "hidden") el.style.overflowY = "hidden";
      return;
    }
    const needed = neededHeight(el);
    const h = Math.min(maxPx, needed);
    const next = `${h}px`;
    if (el.style.height !== next) el.style.height = next;
    const overflow = needed > maxPx ? "auto" : "hidden";
    if (el.style.overflowY !== overflow) el.style.overflowY = overflow;
  }, [ref, value, maxPx]);
}

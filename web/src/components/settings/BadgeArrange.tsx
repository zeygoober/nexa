import { useEffect, useLayoutEffect, useRef, useState, type KeyboardEvent, type PointerEvent as ReactPointerEvent } from "react";
import { moderation } from "@/api/moderation";
import type { BadgeArrangement, PublicUser } from "@/api/types";
import { BadgeMark } from "@/components/badges/Badges";
import { Button } from "@/components/common/Button";
import { Spinner } from "@/components/common/Spinner";
import { t } from "@/i18n";
import { badgeLook } from "@/lib/badges";
import { motionOff } from "@/lib/motion";
import { useUI } from "@/stores/ui";
import s from "./BadgeIndexPage.module.css";

type Item = BadgeArrangement["entries"][number];

/** The grid's gap (BadgeIndexPage.module.css). */
const GAP = 10;
/** How long a finger stays put before a tile lifts, and how far it may stray first; further is a scroll. */
const HOLD_MS = 250;
const STRAY = 8;

/** Which plan's badge a plan entry is drawn as, since every plan shows here at once. */
function lookOf(item: Item, me: PublicUser) {
  return badgeLook(item.badge, item.key.startsWith("plan-") ? { ...me, plan: item.key.slice(5) as PublicUser["plan"] } : me);
}

/** The badge in colour with no card of its own, since a press here picks it up. */
function Mark({ look }: { look: NonNullable<ReturnType<typeof lookOf>> }) {
  return look.art ? <img className={s.art} src={look.art} alt="" draggable={false} /> : <BadgeMark look={look} size={40} />;
}

/** The nearest thing that can scroll, for a swipe that starts on a tile. */
function scrollerOf(el: HTMLElement): HTMLElement | null {
  for (let at = el.parentElement; at; at = at.parentElement) {
    if (at.scrollHeight > at.clientHeight && /auto|scroll/.test(getComputedStyle(at).overflowY)) return at;
  }
  return null;
}

/** The owner arranging the Badge Index for everyone, in place of the summary (Android `Arrange` in BadgeIndexPage.kt). */
export default function BadgeArrange({ me, onDone }: { me: PublicUser; onDone: (saved: boolean) => void }) {
  const toast = useUI((u) => u.toast);
  const [items, setItems] = useState<Item[] | null>(null);
  const [aToZ, setAToZ] = useState<string[]>([]);
  const [changed, setChanged] = useState(false);
  const [toAToZ, setToAToZ] = useState(false);
  const [saving, setSaving] = useState(false);
  const [held, setHeld] = useState<{ key: string; x: number; y: number; size: number } | null>(null);
  const grid = useRef<HTMLUListElement>(null);
  const list = useRef<Item[]>([]);
  const grab = useRef<{ key: string; dx: number; dy: number; size: number } | null>(null);
  const letGo = useRef<(() => void) | null>(null);
  const refocus = useRef<string | null>(null);

  useEffect(() => {
    moderation.badgeArrangement().then(
      (a) => {
        list.current = a.entries;
        setItems(a.entries);
        setAToZ(a.a_to_z);
      },
      () => {
        toast(t("badges.index.failed"), "error");
        onDone(false);
      },
    );
    return () => letGo.current?.();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useLayoutEffect(() => {
    if (!refocus.current) return;
    grid.current?.querySelector<HTMLElement>(`[data-key="${refocus.current}"]`)?.focus();
    refocus.current = null;
  }, [items]);

  const shape = () => {
    const box = grid.current!.getBoundingClientRect();
    const first = grid.current!.querySelector("button")!.getBoundingClientRect().width;
    const columns = Math.max(1, Math.round((box.width + GAP) / (first + GAP)));
    return { box, step: first + GAP, columns, rtl: getComputedStyle(grid.current!).direction === "rtl" };
  };

  const moveTo = (key: string, to: number) => {
    const from = list.current.findIndex((i) => i.key === key);
    const at = Math.min(Math.max(to, 0), list.current.length - 1);
    if (from < 0 || from === at) return;
    const next = list.current.slice();
    next.splice(at, 0, ...next.splice(from, 1));
    list.current = next;
    setItems(next);
    setChanged(true);
    setToAToZ(false);
  };

  const follow = (x: number, y: number) => {
    const g = grab.current;
    if (!g) return;
    const { box, step, columns, rtl } = shape();
    setHeld({ key: g.key, x: x - g.dx - box.left, y: y - g.dy - box.top, size: g.size });
    const column = Math.min(Math.max(Math.floor((x - box.left + GAP / 2) / step), 0), columns - 1);
    const row = Math.max(Math.floor((y - box.top + GAP / 2) / step), 0);
    moveTo(g.key, row * columns + (rtl ? columns - 1 - column : column));
  };

  const press = (key: string, e: ReactPointerEvent<HTMLButtonElement>) => {
    if ((e.pointerType === "mouse" && e.button !== 0) || letGo.current || saving) return;
    const tile = e.currentTarget.getBoundingClientRect();
    const scroller = scrollerOf(e.currentTarget);
    let swipedTo: number | null = null;
    const id = e.pointerId;
    const from = { x: e.clientX, y: e.clientY };
    const lift = () => {
      grab.current = { key, dx: from.x - tile.left, dy: from.y - tile.top, size: tile.width };
      follow(from.x, from.y);
    };
    const timer = e.pointerType === "mouse" ? 0 : window.setTimeout(lift, HOLD_MS);
    const move = (ev: PointerEvent) => {
      if (ev.pointerId !== id) return;
      if (grab.current) follow(ev.clientX, ev.clientY);
      else if (swipedTo !== null) {
        if (scroller) scroller.scrollTop += swipedTo - ev.clientY;
        swipedTo = ev.clientY;
      } else if (Math.hypot(ev.clientX - from.x, ev.clientY - from.y) > STRAY) {
        window.clearTimeout(timer);
        swipedTo = ev.clientY;
      }
    };
    const up = (ev: PointerEvent) => {
      if (ev.pointerId === id) end();
    };
    const end = () => {
      window.clearTimeout(timer);
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
      window.removeEventListener("pointercancel", up);
      grab.current = null;
      letGo.current = null;
      setHeld(null);
    };
    letGo.current = end;
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
    window.addEventListener("pointercancel", up);
    if (e.pointerType === "mouse") lift();
  };

  const nudge = (key: string, e: KeyboardEvent) => {
    const { columns, rtl } = shape();
    const by = { ArrowLeft: rtl ? 1 : -1, ArrowRight: rtl ? -1 : 1, ArrowUp: -columns, ArrowDown: columns }[e.key];
    if (!by) return;
    e.preventDefault();
    refocus.current = key;
    moveTo(key, list.current.findIndex((i) => i.key === key) + by);
  };

  const useAToZ = () => {
    const byKey = new Map(list.current.map((i) => [i.key, i]));
    const next = aToZ.map((k) => byKey.get(k)).filter((i): i is Item => !!i);
    list.current = next;
    setItems(next);
    setChanged(true);
    setToAToZ(true);
  };

  const save = () => {
    setSaving(true);
    moderation.setBadgeOrder(toAToZ ? [] : list.current.map((i) => i.key)).then(
      () => {
        toast(t("badges.index.arranged"), "success");
        onDone(true);
      },
      () => {
        toast(t("badges.index.arrangeFailed"), "error");
        setSaving(false);
      },
    );
  };

  if (!items) return <div className={s.loading}><Spinner size={24} label={t("chat.pins.loading")} /></div>;
  const inHand = held && items.find((i) => i.key === held.key);
  const inHandLook = inHand && lookOf(inHand, me);
  return (
    <>
      <div className={s.arrangeHead}>
        <h3 className={s.arrangeTitle}>{t("badges.index.arrangeTitle")}</h3>
        <p className={s.hint}>{t("badges.index.arrangeHint")}</p>
        <div className={s.actions}>
          <Button size="sm" disabled={!changed} loading={saving} onClick={save} data-testid="badge-arrange-save">{t("badges.staff.save")}</Button>
          <Button size="sm" variant="secondary" disabled={saving} onClick={() => onDone(false)}>{t("moderation.cancel")}</Button>
          <Button size="sm" variant="secondary" disabled={saving || toAToZ} onClick={useAToZ} data-testid="badge-arrange-aToZ">{t("badges.index.useAlphabetical")}</Button>
        </div>
      </div>
      <ul className={`${s.grid} ${s.arranging}`} ref={grid} data-no-swipe="" data-testid="badge-arrange-grid">
        {items.map((item) => {
          const look = lookOf(item, me);
          const left = held?.key === item.key;
          return (
            <li key={item.key}>
              <button
                type="button"
                className={`${s.tile} ${s.movable}`}
                data-key={item.key}
                data-left={left || undefined}
                aria-label={look?.label}
                onPointerDown={(e) => press(item.key, e)}
                onMouseDown={(e) => e.preventDefault()}
                onContextMenu={(e) => e.preventDefault()}
                onKeyDown={(e) => nudge(item.key, e)}
              >
                {look && !left ? <Mark look={look} /> : null}
              </button>
            </li>
          );
        })}
        {held && inHandLook ? (
          <li className={s.inHand} aria-hidden="true" data-still={motionOff() || undefined} style={{ left: held.x, top: held.y, width: held.size, height: held.size }}>
            <Mark look={inHandLook} />
          </li>
        ) : null}
      </ul>
    </>
  );
}

import { Bug, Lightbulb } from "lucide-react";
import { useLayoutEffect, useRef, useState, type RefObject } from "react";
import { Tooltip } from "@/components/common/Tooltip";
import type { ContextMenuItem } from "@/stores/ui";
import { useUI } from "@/stores/ui";
import { t } from "@/i18n";
import type { BugFrom } from "./ReportBugModal";
import s from "./Bugs.module.css";

/** Opens the Report a Bug dialog, saying where it was opened from. */
export function openReportBug(from: BugFrom): void {
  useUI.getState().openModal("reportBug", { from });
}

/** Opens Suggest a Feature: the same dialog, for an idea rather than a problem. */
export function openSuggest(from: BugFrom): void {
  useUI.getState().openModal("reportBug", { from, kind: "suggestion" });
}

/** The same, as an item for a menu (a phone's header keeps its buttons in one). */
export function reportBugMenuItem(from: BugFrom): ContextMenuItem {
  return { id: "report-bug", label: t("bugs.report"), onSelect: () => openReportBug(from) };
}

/**
 * Whether the conversation header around `el` is narrower than `min`. A tablet
 * held upright has a header of buttons with barely room for the name; the bug
 * button steps out of it there rather than squeezing the name to nothing, and
 * the dialog stays a tap away from Friends, Nexa and Settings.
 */
function useCramped(el: RefObject<HTMLElement | null>, min: number | undefined): boolean {
  const [cramped, setCramped] = useState(false);
  useLayoutEffect(() => {
    if (!min) return;
    const header = el.current?.closest<HTMLElement>("[data-chat-header]");
    if (!header || typeof ResizeObserver === "undefined") return;
    const measure = () => setCramped(header.clientWidth < min);
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(header);
    return () => ro.disconnect();
  }, [el, min]);
  return cramped;
}

/**
 * The bug button in a header: an icon where the header has others like it,
 * or the icon with its name where there is room for it and it is the header's
 * only action (`labelled`). `minHeaderWidth` lets it leave a conversation
 * header too narrow to hold it.
 */
export function ReportBugButton({ from, className, wrapClassName, labelled = false, minHeaderWidth }: { from: BugFrom; className?: string | undefined; wrapClassName?: string | undefined; labelled?: boolean; minHeaderWidth?: number }) {
  const label = t("bugs.report");
  const probe = useRef<HTMLSpanElement>(null);
  const cramped = useCramped(probe, minHeaderWidth);
  if (labelled) {
    return (
      <button type="button" className={s.headerButton} onClick={() => openReportBug(from)} aria-label={label} data-testid="report-bug">
        <Bug size={18} aria-hidden="true" />
        <span>{label}</span>
      </button>
    );
  }
  return (
    // no box of its own: the header lays the button out as if it were not wrapped
    <span ref={probe} style={{ display: cramped ? "none" : "contents" }}>
      <Tooltip text={label} side="bottom" className={wrapClassName}>
        <button type="button" className={className} aria-label={label} onClick={() => openReportBug(from)} data-testid="report-bug">
          <Bug size={22} />
        </button>
      </Tooltip>
    </span>
  );
}

/** Suggest a Feature as an icon, beside the bug button in the Friends header. */
export function SuggestIconButton({ from, className }: { from: BugFrom; className?: string | undefined }) {
  const label = t("suggestions.suggest");
  return (
    <Tooltip text={label} side="bottom">
      <button type="button" className={className} aria-label={label} onClick={() => openSuggest(from)} data-testid="suggest-feature">
        <Lightbulb size={22} />
      </button>
    </Tooltip>
  );
}

/** Suggest a Feature in a header, with its name: beside Report a Bug where that is labelled too. */
export function SuggestButton({ from }: { from: BugFrom }) {
  const label = t("suggestions.suggest");
  return (
    <button type="button" className={s.headerButton} onClick={() => openSuggest(from)} aria-label={label} data-testid="suggest-feature">
      <Lightbulb size={18} aria-hidden="true" />
      <span>{label}</span>
    </button>
  );
}

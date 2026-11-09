import { useEffect } from "react";
import { create } from "zustand";
import { billing as billingApi } from "@/api/billing";
import { ApiError } from "@/api/client";
import type { GiftLinkView, Plan } from "@/api/types";
import { t } from "@/i18n";
import { planName } from "@/lib/plans";
import { useAuth } from "@/stores/auth";
import { useBilling } from "@/stores/billing";

/** The plan column of a link that gives a one-character username. */
export const SHORT_NAME_GIFT = "short_name";

/** How long a gift lasts, as its title says it ("A Month of Pro"). */
export function giftTitle(plan: Plan | typeof SHORT_NAME_GIFT, days: number): string {
  if (plan === SHORT_NAME_GIFT) return t("support.giftLink.shortName");
  return t(days >= 365 ? "support.giftLink.yearOf" : "support.giftLink.monthOf", { plan: planName(plan) });
}

/** Gift links this device has asked about, by code, for the cards under messages. */
export type GiftEntry =
  | { status: "loading" }
  | { status: "ready"; gift: GiftLinkView; at: number }
  | { status: "invalid"; at: number }
  | { status: "failed"; at: number };

/** Someone else may claim it: a card drawn again after this long asks again. */
const FRESH_MS = 60_000;
const RETRY_MS = 30_000;

export const useGiftPreviews = create<{ byCode: Record<string, GiftEntry> }>()(() => ({ byCode: {} }));

let accountGeneration = 0;
const inflight = new Set<string>();
useAuth.subscribe((next, previous) => {
  if (next.me?.id === previous.me?.id) return;
  ++accountGeneration;
  inflight.clear();
  useGiftPreviews.setState({ byCode: {} });
});

function put(code: string, entry: GiftEntry): void {
  useGiftPreviews.setState((s) => ({ byCode: { ...s.byCode, [code]: entry } }));
}

export function loadGiftPreview(code: string, force = false): void {
  const now = Date.now();
  const had = useGiftPreviews.getState().byCode[code];
  if (inflight.has(code)) return;
  if (!force && had && had.status !== "loading" && now - had.at < (had.status === "failed" ? RETRY_MS : FRESH_MS)) return;
  const generation = accountGeneration;
  inflight.add(code);
  // a card already drawn keeps what it shows while it is asked again
  if (!had || had.status !== "ready") put(code, { status: "loading" });
  billingApi.giftLinkInfo(code).then(
    (gift) => {
      if (generation === accountGeneration) put(code, { status: "ready", gift, at: Date.now() });
    },
    (err) => {
      if (generation !== accountGeneration) return;
      put(code, { status: err instanceof ApiError && err.status === 404 ? "invalid" : "failed", at: Date.now() });
    },
  ).finally(() => {
    if (generation === accountGeneration) inflight.delete(code);
  });
}

/** Claims a gift from its card. Throws what the server said. */
export async function claimGiftPreview(code: string): Promise<void> {
  const gift = await billingApi.claimGift(code);
  put(code, { status: "ready", gift, at: Date.now() });
  void useBilling.getState().load();
  if (gift.plan === SHORT_NAME_GIFT) {
    const me = useAuth.getState().me;
    if (me) useAuth.getState().setMe({ ...me, short_name_credits: (me.short_name_credits ?? 0) + 1 });
  }
}

export function useGiftPreview(code: string): GiftEntry | undefined {
  useEffect(() => {
    loadGiftPreview(code);
  }, [code]);
  return useGiftPreviews((s) => s.byCode[code]);
}

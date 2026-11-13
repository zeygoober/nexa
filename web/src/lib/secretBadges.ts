import { users } from "@/api/users";
import { t } from "@/i18n";
import { isDemo } from "@/lib/demoMode";

/**
 * The Secret badges an app reports (store.AppFoundBadges); the server earns
 * Hopeful and Lucky itself. Nothing on screen says one was found: the Badge
 * Index shows it next time. Android: data/SecretBadges.kt.
 */
export type FoundBadge = "curious" | "nosy";

/** How long a padlock in the Badge Index is held for Curious. */
export const CURIOUS_HOLD_MS = 10_000;

const reported = new Set<FoundBadge>();

/** Reports a badge found, once per run; answers whether it is new to the account. */
export async function reportFound(badge: FoundBadge): Promise<boolean> {
  // the homepage demo has no account, and its API answers with a sign-up gate
  if (isDemo || reported.has(badge)) return false;
  reported.add(badge);
  try {
    return (await users.badgeFound(badge)).new;
  } catch {
    reported.delete(badge);
    return false;
  }
}

/** Whether a Settings search is the word Nosy listens for: English, or this language's. */
export function isSecretWord(query: string): boolean {
  const q = query.trim().toLocaleLowerCase();
  return q === "secret" || q === t("badges.secretWord").toLocaleLowerCase();
}

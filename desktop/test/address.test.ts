/**
 * Nexa moved to nexa.pm, and this app opens only the new address. A sign-in
 * at the old one is never carried over: whoever used it signs in again.
 */
import { describe, expect, it, vi } from "vitest";

vi.mock("electron", () => ({ app: { isPackaged: true } }));

const { ORIGIN, START_URL, isAppUrl } = await import("../src/main/config");

describe("Nexa's address", () => {
  it("is nexa.pm, and only pages there are the app", () => {
    expect(ORIGIN).toBe("https://nexa.pm");
    expect(START_URL).toBe("https://nexa.pm/channels/@me");
    expect(isAppUrl("https://nexa.pm/channels/@me")).toBe(true);
    expect(isAppUrl("https://chat.evilion.xyz/channels/@me")).toBe(false);
    expect(isAppUrl("https://nexa.pm.evil.example/")).toBe(false);
  });
});

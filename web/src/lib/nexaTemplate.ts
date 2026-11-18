const CODE_RE = /^[A-Za-z0-9]{8,32}$/;

export type ParsedNexaTemplate = { ok: true; code: string } | { ok: false; reason: "empty" | "invalid" };

/** Whether a host is this Nexa's: the one the app runs on, or nexa.pm (the desktop app and older links). */
function ownHost(host: string): boolean {
  const h = host.toLowerCase();
  return h === window.location.hostname.toLowerCase() || h === "nexa.pm" || h.endsWith(".nexa.pm");
}

/**
 * Takes a Nexa server template link (https://nexa.pm/template/<code>) or a
 * bare code. The server checks the address again, ALT_URLS included.
 */
export function parseNexaTemplate(input: string): ParsedNexaTemplate {
  const s = input.trim();
  if (!s) return { ok: false, reason: "empty" };
  if (!s.includes("/") && !s.includes(".")) return CODE_RE.test(s) ? { ok: true, code: s } : { ok: false, reason: "invalid" };
  let u: URL;
  try {
    u = new URL(s.includes("://") ? s : `https://${s}`);
  } catch {
    return { ok: false, reason: "invalid" };
  }
  const parts = u.pathname.split("/").filter(Boolean);
  if (!ownHost(u.hostname) || parts.length !== 2 || parts[0] !== "template" || !CODE_RE.test(parts[1] ?? "")) return { ok: false, reason: "invalid" };
  return { ok: true, code: parts[1]! };
}

/** Whether input is a Nexa template link (rather than a bare code). */
export function isNexaTemplateUrl(input: string): boolean {
  const s = input.trim();
  return (s.includes("/") || s.includes(".")) && parseNexaTemplate(s).ok;
}

/** The link a template code is shared as. */
export const nexaTemplateUrl = (code: string) => `${window.location.origin}/template/${code}`;

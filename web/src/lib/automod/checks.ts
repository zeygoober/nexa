import type { AutomodConfig, AutomodPresets } from "@/api/types";
import { parse, type Node } from "@/markdown";
import { Matcher, fold } from "./match";

/**
 * The checks of AutoMod's rules that are not lists of words: links, invites,
 * personal details, capitals, emoji, files and the signs of spam. Each says
 * what it found, or null.
 */

/** Hosts that are this app's own, where links and invites are Nexa's. */
export const OWN_HOSTS = ["nexa.pm"];

const TLDS = "com|net|org|io|gg|co|me|tv|app|dev|xyz|ru|cn|uk|de|fr|info|biz|link|ly|gift|fish|lol|site|online|shop|store|club|top|live|stream|fun|click|win|pro|cc|to|su|in|us|ca|eu|nl|br|jp|kr|au|es|it|pl|tk|ml|ga|cf|gq|ws";
const URL_RE = new RegExp(`\\bhttps?:\\/\\/[^\\s<>()]+|\\b(?:www\\.)?(?:[a-z0-9-]+\\.)+(?:${TLDS})\\b(?:\\/[^\\s<>()]*)?`, "giu");

export interface FoundLink {
  url: string;
  host: string;
}

/** The links in a message, with their hosts. */
export function linksIn(text: string): FoundLink[] {
  const out: FoundLink[] = [];
  for (const m of text.matchAll(URL_RE)) {
    const raw = m[0].replace(/[.,!?;:'"]+$/, "");
    try {
      const u = new URL(/^https?:\/\//i.test(raw) ? raw : `http://${raw}`);
      out.push({ url: raw, host: u.hostname.toLowerCase().replace(/\.$/, "") });
    } catch {
      // not an address after all
    }
  }
  return out;
}

/** Whether a host is a domain or one of its subdomains. */
export function hostIs(host: string, domain: string): boolean {
  const d = domain.replace(/^\*\./, "");
  return host === d || host.endsWith(`.${d}`);
}

function ownHost(host: string): boolean {
  const here = typeof window !== "undefined" ? window.location.hostname : "";
  return OWN_HOSTS.some((h) => hostIs(host, h)) || (!!here && host === here);
}

/** links: every link but the allowed sites, or only the listed ones. Nexa's own links are always fine. */
export function checkLinks(text: string, c: AutomodConfig): string | null {
  for (const l of linksIn(text)) {
    if (ownHost(l.host)) continue;
    if (c.mode === "list") {
      if ((c.domains ?? []).some((d) => hostIs(l.host, d))) return l.url;
    } else if (!(c.allow ?? []).some((d) => hostIs(l.host, d))) return l.url;
  }
  return null;
}

const INVITE_PATHS = /^\/(?:invite|i)\/[A-Za-z0-9_-]+\/?$/;

/** invites: invite links, to servers here or anywhere else people gather. */
export function checkInvites(text: string): string | null {
  for (const l of linksIn(text)) {
    let path = "";
    try {
      path = new URL(/^https?:\/\//i.test(l.url) ? l.url : `http://${l.url}`).pathname;
    } catch {
      continue;
    }
    if (ownHost(l.host) && INVITE_PATHS.test(path)) return l.url;
    if (hostIs(l.host, "discord.gg") || ((hostIs(l.host, "discord.com") || hostIs(l.host, "discordapp.com")) && /^\/invite\//.test(path))) return l.url;
    if (hostIs(l.host, "t.me") || hostIs(l.host, "chat.whatsapp.com") || hostIs(l.host, "revolt.chat") || hostIs(l.host, "guilded.gg")) return l.url;
  }
  return null;
}

const EMAIL_RE = /[\p{L}\p{N}._%+-]+@[\p{L}\p{N}-]+(?:\.[\p{L}\p{N}-]+)*\.[\p{L}]{2,}/u;
const PHONE_RE = /(?:^|[^\d])(\+?\d[\d\s().-]{8,18}\d)(?!\d)/gu;
const IPV4_RE = /\b(?:(?:25[0-5]|2[0-4]\d|1?\d?\d)\.){3}(?:25[0-5]|2[0-4]\d|1?\d?\d)\b/;
const IPV6_RE = /\b(?:[0-9a-f]{1,4}:){4,7}[0-9a-f]{1,4}\b/i;
const CARD_RE = /(?:\d[ -]?){12,18}\d/g;

function luhn(digits: string): boolean {
  let sum = 0;
  let double = false;
  for (let i = digits.length - 1; i >= 0; i--) {
    let d = digits.charCodeAt(i) - 48;
    if (double) {
      d *= 2;
      if (d > 9) d -= 9;
    }
    sum += d;
    double = !double;
  }
  return sum % 10 === 0;
}

/** personal: email addresses, phone numbers, IP addresses and card numbers. */
export function checkPersonal(text: string, c: AutomodConfig): string | null {
  if (c.emails) {
    const m = EMAIL_RE.exec(text);
    if (m) return m[0];
  }
  if (c.cards) {
    for (const m of text.matchAll(CARD_RE)) {
      const digits = m[0].replace(/\D/g, "");
      if (digits.length >= 13 && digits.length <= 19 && luhn(digits)) return m[0];
    }
  }
  if (c.phones) {
    for (const m of text.matchAll(PHONE_RE)) {
      const digits = m[1]!.replace(/\D/g, "");
      // a number grouped as phone numbers are, not a date, an amount or an id
      if (digits.length >= 10 && digits.length <= 15 && /[\s().+-]/.test(m[1]!) && !luhn(digits)) return m[1]!;
    }
  }
  if (c.ips) {
    const m = IPV4_RE.exec(text) ?? IPV6_RE.exec(text);
    if (m) return m[0];
  }
  return null;
}

/** caps: a message mostly in capitals, past a length. */
export function checkCaps(text: string, c: AutomodConfig): string | null {
  // mentions, emoji and links are not shouting
  const plain = text.replace(/<[@#&:a][^>]*>/g, "").replace(/https?:\/\/\S+/g, "");
  let letters = 0;
  let upper = 0;
  for (const ch of plain) {
    if (/\p{Lu}/u.test(ch)) {
      letters++;
      upper++;
    } else if (/\p{Ll}/u.test(ch)) letters++;
  }
  const min = c.min_length ?? 10;
  const percent = c.percent ?? 70;
  return letters >= min && (upper * 100) / letters >= percent ? `${Math.round((upper * 100) / letters)}%` : null;
}

function countEmoji(nodes: readonly Node[]): number {
  let n = 0;
  for (const node of nodes) {
    if (node.type === "unicodeEmoji" || node.type === "emoji") n++;
    else if (node.type === "details") n += countEmoji([...node.summary, ...node.children]);
    else if ("children" in node) n += countEmoji(node.children);
    else if (node.type === "list") for (const item of node.items) n += countEmoji(item);
  }
  return n;
}

/** emoji_spam: more emoji in one message than allowed. */
export function checkEmoji(text: string, c: AutomodConfig): string | null {
  const n = text ? countEmoji(parse(text)) : 0;
  return n > (c.max ?? 10) ? String(n) : null;
}

/** attachments: files ending in what the rule lists. */
export function checkAttachments(names: readonly string[], c: AutomodConfig): string | null {
  const banned = new Set((c.extensions ?? []).map((e) => e.toLowerCase()));
  for (const name of names) {
    const ext = name.toLowerCase().split(".").pop() ?? "";
    if (name.includes(".") && banned.has(ext)) return name;
  }
  return null;
}

/** How far apart two words are, in letters to change, add or take away. */
function distance(a: string, b: string): number {
  const row = Array.from({ length: b.length + 1 }, (_, i) => i);
  for (let i = 1; i <= a.length; i++) {
    let prev = row[0]!;
    row[0] = i;
    for (let j = 1; j <= b.length; j++) {
      const cur = row[j]!;
      row[j] = Math.min(row[j]! + 1, row[j - 1]! + 1, prev + (a[i - 1] === b[j - 1] ? 0 : 1));
      prev = cur;
    }
  }
  return row[b.length]!;
}

/** A link that hides where it goes, or passes for somewhere it is not. */
export function shadyLink(links: readonly FoundLink[], presets: AutomodPresets): string | null {
  for (const l of links) {
    if (ownHost(l.host)) continue;
    if (/^\d+\.\d+\.\d+\.\d+$/.test(l.host) || l.host.includes(":")) return l.url;
    if (l.host.split(".").some((label) => label.startsWith("xn--"))) return l.url;
    if (presets.shorteners.some((s) => hostIs(l.host, s))) return l.url;
    const folded = fold(l.host);
    for (const [name, real] of Object.entries(presets.impersonated)) {
      if (real.some((d) => hostIs(l.host, d))) continue;
      const labels = folded.split(/[.-]/);
      if (folded.includes(name) || (name.length >= 5 && labels.some((label) => label.length >= 5 && distance(label, name) === 1))) return l.url;
    }
  }
  return null;
}

/** Text drowned in marks stacked on its letters. */
export function zalgo(text: string): string | null {
  let marks = 0;
  let run = 0;
  for (const ch of text.normalize("NFD")) {
    if (/\p{M}/u.test(ch)) {
      marks++;
      run++;
      if (run >= 4) return ch;
    } else run = 0;
  }
  return marks >= 12 ? `${marks}` : null;
}

/** A wall of text: a great many lines, a run with no spaces, or one character over and over. */
export function wall(text: string): string | null {
  if (text.split("\n").length > 30) return "lines";
  if (/\S{300,}/u.test(text)) return "unbroken";
  if (/(.)\1{49,}/su.test(text)) return "repeated";
  return null;
}

let scamMatcher: { presets: AutomodPresets; m: Matcher } | null = null;

/** A scam's or an advert's usual words. */
export function scam(text: string, presets: AutomodPresets): string | null {
  if (scamMatcher?.presets !== presets) scamMatcher = { presets, m: new Matcher(presets.spam_phrases, [], [], true) };
  return scamMatcher.m.match(text);
}

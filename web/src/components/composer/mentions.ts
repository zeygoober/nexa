import type { PublicUser } from "@/api/types";

/**
 * What a message's `@name`, `#channel` and `:emoji:` can stand for where it is
 * being written. Typed by hand rather than picked from the suggestions, they
 * are still meant as the real thing, so they are turned into it on the way out.
 */
export interface MentionScope {
  /** The people in the conversation: their display names and server nicknames count as well as their usernames. */
  members?: { id: string; names: string[] }[];
  /** The roles this person may mention here. */
  roles?: { id: string; name: string }[];
  /** The server's channels, for `#name`. */
  channels?: { id: string; name: string }[];
  /**
   * Custom emoji for `:name:`, as the token that shows them. This server's own
   * win over the unicode emoji of the same name, as they do on Discord; the
   * rest (`fallback`) only stand in for a name no unicode emoji has.
   */
  emoji?: { name: string; token: string; fallback?: boolean }[];
  /** Whether a unicode emoji goes by this name. Not knowing yet counts as yes. */
  isShortcode?: (name: string) => boolean;
}

export interface Extracted {
  text: string;
  /** Everyone the message mentions. */
  mentions: string[];
  /** The roles it mentions. */
  roles: string[];
  everyone: boolean;
}

/**
 * The parts of a message that are left exactly as written: code, anything
 * already in angle brackets (a mention, an emoji, a timestamp, a link that is
 * not to be previewed) and bare links, whose paths can hold an @ or a # of
 * their own (youtube.com/@name, a page's #section).
 */
const KEEP = /(```[\s\S]*?```|`[^`\n]*`|<[^<>\s]+>|https?:\/\/[^\s<]+)/i;

const WORD = /[\p{L}\p{N}_]/u;
const isWord = (ch: string | undefined): boolean => !!ch && WORD.test(ch);

/** Something that can follow a name for the name to have ended. */
const ends = (ch: string | undefined): boolean => !isWord(ch);

/** Where a trigger may start: not inside a word, and not escaped. */
const opens = (prev: string | undefined, also = ""): boolean => prev === undefined || (!isWord(prev) && prev !== "\\" && prev !== "<" && !also.includes(prev));

interface Named {
  id: string;
  name: string;
  lower: string;
  /** Usernames come first, then roles, then the names people show, when two match the same text. */
  rank: number;
}

/** Names that belong to exactly one of their holders; a name two people share is nobody's. */
function unique(list: { id: string; name: string }[], rank: number): Named[] {
  const holders = new Map<string, Set<string>>();
  const first = new Map<string, string>();
  for (const { id, name } of list) {
    const clean = name.trim();
    if (!clean) continue;
    const lower = clean.toLowerCase();
    if (!holders.has(lower)) {
      holders.set(lower, new Set());
      first.set(lower, clean);
    }
    holders.get(lower)!.add(id);
  }
  const out: Named[] = [];
  for (const [lower, ids] of holders) if (ids.size === 1) out.push({ id: [...ids][0]!, name: first.get(lower)!, lower, rank });
  return out;
}

/** The candidate that the text at `at` spells out, the longest one first; one typed with the same capitals wins a tie. */
function longest(seg: string, at: number, names: Named[], boundary: (ch: string | undefined) => boolean): Named | null {
  let best: Named | null = null;
  let bestExact = false;
  for (const n of names) {
    if (best && n.name.length < best.name.length) continue;
    const typed = seg.slice(at, at + n.name.length);
    if (typed.length !== n.name.length || typed.toLowerCase() !== n.lower || !boundary(seg[at + n.name.length])) continue;
    const exact = typed === n.name;
    if (!best || n.name.length > best.name.length || (exact && !bestExact) || (exact === bestExact && n.rank < best.rank)) {
      best = n;
      bestExact = exact;
    }
  }
  return best;
}

/**
 * Turns what a message says by name into what it means: `@username` (any
 * capitals), the display name or server nickname of someone in the
 * conversation, and `@Role` into mentions, `#channel` into a link to the
 * channel, and `:name:` into this server's own emoji of that name. Names two
 * people share, parts of longer words, escaped names and anything in code or
 * in a link are left as they are.
 */
export function extractMentions(text: string, users: Record<string, PublicUser>, scope: MentionScope = {}): Extracted {
  const byUsername = new Map<string, PublicUser>();
  for (const u of Object.values(users)) if (u.username) byUsername.set(u.username.toLowerCase(), u);
  const people = unique((scope.members ?? []).flatMap((m) => m.names.map((name) => ({ id: m.id, name }))), 2);
  const roles = unique(scope.roles ?? [], 1);
  const channels = unique(scope.channels ?? [], 0);
  const emoji = scope.emoji ?? [];
  const mentions = new Set<string>();
  const roleIds = new Set<string>();
  let everyone = false;

  const userAt = (seg: string, at: number): Named | null => {
    // the longest run a username can be, then without the dots that end a sentence
    const run = /^[a-z0-9_.]{1,32}/i.exec(seg.slice(at))?.[0] ?? "";
    for (let typed = run; typed.length >= 1; typed = typed.slice(0, -1)) {
      const u = byUsername.get(typed.toLowerCase());
      if (u && ends(seg[at + typed.length])) return { id: u.id, name: u.username, lower: typed.toLowerCase(), rank: 0 };
      if (!typed.endsWith(".")) break;
    }
    return null;
  };

  const emojiFor = (name: string): string | null => {
    const lower = name.toLowerCase();
    const own = emoji.filter((e) => !e.fallback && e.name.toLowerCase() === lower);
    const hit = own.find((e) => e.name === name) ?? own[0];
    if (hit) return hit.token;
    if (scope.isShortcode?.(name) ?? true) return null;
    const other = emoji.filter((e) => e.fallback && e.name.toLowerCase() === lower);
    return (other.find((e) => e.name === name) ?? other[0])?.token ?? null;
  };

  const convert = (seg: string): string => {
    let out = "";
    let i = 0;
    while (i < seg.length) {
      const c = seg[i]!;
      const prev = i > 0 ? seg[i - 1] : undefined;
      if (c === "@" && opens(prev, "@")) {
        const rest = seg.slice(i + 1);
        const everyoneWord = /^(everyone|here)(?![\p{L}\p{N}_])/u.exec(rest);
        if (everyoneWord) {
          everyone = true;
          out += "@" + everyoneWord[1];
          i += 1 + everyoneWord[1]!.length;
          continue;
        }
        const candidates = [userAt(seg, i + 1), longest(seg, i + 1, roles, ends), longest(seg, i + 1, people, ends)].filter((n): n is Named => n !== null);
        let pick: Named | null = null;
        for (const n of candidates) {
          const exact = seg.slice(i + 1, i + 1 + n.name.length) === n.name;
          const pickExact = pick ? seg.slice(i + 1, i + 1 + pick.name.length) === pick.name : false;
          if (!pick || n.name.length > pick.name.length || (n.name.length === pick.name.length && ((exact && !pickExact) || (exact === pickExact && n.rank < pick.rank)))) pick = n;
        }
        if (pick) {
          if (pick.rank === 1) {
            roleIds.add(pick.id);
            out += `<@&${pick.id}>`;
          } else {
            mentions.add(pick.id);
            out += `<@${pick.id}>`;
          }
          i += 1 + pick.name.length;
          continue;
        }
      } else if (c === "#" && channels.length && opens(prev, "#&")) {
        // a channel's name goes on through its dashes, so #dev is not the start of #dev-chat
        const hit = longest(seg, i + 1, channels, (ch) => ends(ch) && ch !== "-");
        if (hit) {
          out += `<#${hit.id}>`;
          i += 1 + hit.name.length;
          continue;
        }
      } else if (c === ":" && emoji.length) {
        const m = /^:([A-Za-z0-9_]{1,32}):/.exec(seg.slice(i));
        const token = m ? emojiFor(m[1]!) : null;
        if (m && token) {
          out += token;
          i += m[0].length;
          continue;
        }
      }
      out += c;
      i += 1;
    }
    return out;
  };

  const out = text
    .split(KEEP)
    .map((seg, i) => {
      if (i % 2 === 0) return convert(seg);
      // a mention written out in full counts as one; code and links stay what they are
      const user = /^<@!?(\d+)>$/.exec(seg);
      if (user) mentions.add(user[1]!);
      const role = /^<@&(\d+)>$/.exec(seg);
      if (role) roleIds.add(role[1]!);
      return seg;
    })
    .join("");
  return { text: out, mentions: [...mentions], roles: [...roleIds], everyone };
}

/** Names to give mentions back when a message is read or edited as text. */
export interface HumanNames {
  roles?: Record<string, string>;
  channels?: Record<string, string>;
  /** Whether a custom emoji token may go back to its `:name:` (for an edit, only when that name brings the same emoji back). */
  emoji?: (token: string, name: string) => boolean;
}

/**
 * The inverse, for editing and copying: `<@id>` back to `@username`, and
 * roles and channels back to their names where the caller knows them (for an
 * edit, the ones that will turn back into the same mention when it is saved).
 * Code is left alone.
 */
export function humanizeMentions(text: string, users: Record<string, PublicUser>, names: HumanNames = {}): string {
  return text
    .split(KEEP)
    .map((seg, i) => {
      if (i % 2 === 0) return seg;
      return seg
        .replace(/^<@!?(\d+)>$/, (m, id: string) => {
          const u = users[id];
          return u ? `@${u.username}` : m;
        })
        .replace(/^<@&(\d+)>$/, (m, id: string) => {
          const name = names.roles?.[id];
          return name ? `@${name}` : m;
        })
        .replace(/^<#(\d+)>$/, (m, id: string) => {
          const name = names.channels?.[id];
          return name ? `#${name}` : m;
        })
        .replace(/^<a?:(\w+):\d+>$/, (m, name: string) => (names.emoji?.(m, name) ? `:${name}:` : m));
    })
    .join("");
}

// Writes android/app/src/test/resources/web-markdown-vectors.json.gz: what the
// web's own message-formatting code makes of a large corpus, for the Android
// port's tests to match exactly (fish.nexa.markdown.*).
//
// vite-node can only load files under web/, so this is copied there to run:
//
//   cp android/tools/markdown_vectors.ts web/md-vectors.gen.ts
//   (cd web && npx vite-node md-vectors.gen.ts)
//   rm web/md-vectors.gen.ts
//
// Timestamps are made in Asia/Kolkata. The zone is set below rather than by
// TZ in the shell, which Git Bash on Windows never hands to Node: run that
// way, every timestamp came out in the computer's own zone.
process.env.TZ = "Asia/Kolkata";
import { readFileSync, readdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { gzipSync } from "node:zlib";
import { createElement } from "react";
import { renderToString } from "react-dom/server";
import { parse, plainText, isEmojiOnly, trimUrl, extractUrls, misleadingLabel, fileLine, fileWidth, fileAlign } from "@/markdown/parse";
import { flattenInline } from "@/markdown/flatten";
import { colorToken, parseColorSpec, formatColorSpec, paintColor, swatchColor } from "@/markdown/colors";
import { ensureShortcodes, onShortcodesReady, shortcodesReady, shortcodeToEmoji, emojiShortcode, mightHaveShortcode } from "@/lib/shortcodes";
import { clampZalgo, hasZalgo } from "@/lib/zalgo";
import { twemojiFiles } from "@/lib/twemoji";
import { I18nProvider } from "@/i18n";
import * as time from "@/lib/time";
import { extractMentions, humanizeMentions } from "@/components/composer/mentions";
import { charsLeft, counterShown, tooLong, pasteOverflows } from "@/components/composer/limit";
import { inCode } from "@/components/composer/shortcodeTyping";
import { runCommand } from "@/components/composer/commands";
import { rankPeople, recentSpeakers } from "@/components/composer/rankPeople";
import { unicodeKey, customKey, reactionKey, scoreOf, withUse, rankedKeys, emojiKeysIn, usageFromHistory, sanitizeUsage } from "@/lib/emojiUsage";
import { popularity } from "@/lib/emojiPopular";
import { wordsOf, nameMatch, useBoost, suggestEmoji, STARTER_EMOJI, type EmojiCandidate } from "@/lib/emojiSuggest";
import { appLink, inviteLinksIn, noteLinksIn, profileLinksIn, messageTarget } from "@/lib/appLinks";
import { loadEmoji, allEmoji, emojiGroups, searchEmoji, withTone, emojiByShortcode, emojiByUnicode, toneOf } from "@/components/pickers/emojiIndex";
import { emojiToken, emojiReactionKey, searchCustomEmoji } from "@/stores/expressions";
import { reactionKeyOf } from "@/components/chat/Reactions";
import { shownInvite, parseInviteFragment } from "@/lib/inviteLink";
import { isPostLink } from "@/lib/postLinks";
import { isProxied, proxyDeadline, mediaKey, mediaSrc } from "@/lib/mediaLinks";

const OUT = join(process.cwd(), "../android/app/src/test/resources/web-markdown-vectors.json.gz");

// ---------------------------------------------------------------------------
// a seeded generator, so the corpus is the same every run
// ---------------------------------------------------------------------------
function mulberry32(seed: number): () => number {
  let a = seed >>> 0;
  return () => {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = a;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}
const rand = mulberry32(20261003);
const pick = <T,>(list: readonly T[]): T => list[Math.floor(rand() * list.length)]!;
const int = (n: number) => Math.floor(rand() * n);

const INLINE_TOKENS = [
  "*", "**", "***", "_", "__", "___", "~~", "||", "`", "``", "```", "\\", "\\*", "\\_", "\\`", "\\|", "\\\\",
  "[", "]", "(", ")", "{", "}", "<", ">", "@", "#", ":", "-", "=", "==", "^", "^{", "_{", "!", "|", "~",
  " ", " ", " ", "  ", "\t", "a", "b", "word", "Hello", "x", "1", "42", "é", "ß", "日本", "ـ", "İ", "ǅ",
  "http://a.b", "https://example.com/x_(y)", "https://en.wikipedia.org/wiki/Foo_(bar)", "https://x.y/a?b=c#d", "HTTPS://CAPS.example/",
  "https://example.com.", "https://example.com)", "http://", "https://", "www.example.com", "example.com", "bank.example",
  "<https://a.example/x>", "<http://b.example>", "<@123>", "<@!456>", "<#789>", "<@&10>", "<:blob:11>", "<a:party_parrot:12>",
  "<t:1700000000>", "<t:1700000000:R>", "<t:-1:d>", "<t:9999999999999999999:T>", "<t:12:x>", "<:a:1>",
  ":smile:", ":+1:", ":heart:", ":thumbsup:", ":nope_not_a_name:", ":SMILE:", "::", ":flag-us:",
  "😀", "👍🏽", "❤️", "❤", "🇺🇸", "🇺", "#️⃣", "1️⃣", "*⃣", "™", "©️", "©", "↔", "▶️", "👨‍👩‍👧", "🏳‍🌈", "🏳️‍🌈", "🫠", "\u{1F3FB}", "️", "‍",
  "@everyone", "@here", "a@everyone", "@everyones", "[^1]", "[^note]", "[^ x]", "{red}", "{on yellow}", "{red on #fff}", "{#abc}", "{blue on}", "{nope}",
  "[x](https://a.example)", "[bank.example](https://evil.example)", "[www.a.example](https://a.example/x)", "[a](<https://a.example>)",
  "[a [b] c](https://x.example)", "[words]{green}", "[[nested]](https://n.example)", " ", " ", "﻿", "̀́̂̃̄",
  "\ud83d", "\ude00", "&amp;", "'", "\"",
];

const LINE_STARTS = [
  "", "", "", "> ", ">", ">>> ", ">>>", "# ", "## ", "###### ", "####### ", "#", "-# ", "-#", "- ", "* ", "+ ", "1. ", "2) ", "10. ", "1234567890. ",
  "  - ", "    - ", "\t- ", "   1. ", "- [ ] ", "- [x] ", "- [X] ", "- [ ]", "---", "***", "___", "- - -", "    ---", "```", "```js", "````", "``` ",
  "::: details ", ":::details", "::: DETAILS Summary", ":::", "[^1]: ", "[^a]:", "> [!NOTE]", "> [!tip] ", "> [!WARNING] x", ">>> [!CAUTION] ",
  "| ", "|", "![name](nexa-file:abc)", "![p](nexa-file:id_1?w=60)", "![p](nexa-file:x?w=999) ", "\\# ", "\\- ",
];

function inline(len: number): string {
  let s = "";
  for (let k = 0; k < len; k++) s += pick(INLINE_TOKENS);
  return s;
}

function line(): string {
  const start = pick(LINE_STARTS);
  return start + inline(int(8));
}

function tableBlock(): string {
  const cols = 1 + int(4);
  const row = (fill: () => string) => (rand() < 0.5 ? "| " : "") + Array.from({ length: cols }, fill).join(" | ") + (rand() < 0.5 ? " |" : "");
  const delim = () => pick([":--", "---", "-:", ":-:", "--", "-", "x", ":"]);
  const rows = [row(() => inline(int(3))), row(delim)];
  for (let k = int(4); k > 0; k--) rows.push(row(() => inline(int(4))));
  return rows.join("\n");
}

function block(): string {
  const lines: string[] = [];
  for (let k = 1 + int(6); k > 0; k--) {
    const r = rand();
    if (r < 0.1) lines.push(tableBlock());
    else if (r < 0.15) lines.push("```" + pick(["", "js", "py", "a b"]) + "\n" + inline(int(6)) + "\n" + pick(["```", "", "````", "``` tail"]));
    else if (r < 0.2) lines.push("::: details " + inline(int(3)) + "\n" + line() + "\n" + pick([":::", "", "::: details inner\nx\n:::\n:::"]));
    else lines.push(line());
  }
  return lines.join(pick(["\n", "\n", "\n\n", "\r\n", "\r"]));
}

// ---------------------------------------------------------------------------
// inputs the web's own tests use
// ---------------------------------------------------------------------------
function literalsFrom(file: string): string[] {
  const text = readFileSync(file, "utf8");
  const out: string[] = [];
  // a string literal after `src:` or as the first argument of parse(), Markdown source= or extractUrls(
  const re = /(?:\bsrc:\s*|\bparse\(\s*|\bsource=\{\s*|\bextractUrls\(\s*|\bsource:\s*)("(?:[^"\\\n]|\\.)*"|'(?:[^'\\\n]|\\.)*'|`(?:[^`\\$]|\\.)*`)/g;
  for (const m of text.matchAll(re)) {
    try {
      const v = new Function("return " + m[1])() as unknown;
      if (typeof v === "string") out.push(v);
    } catch {
      /* not a plain literal */
    }
  }
  return out;
}

function testInputs(): string[] {
  const files = [
    "src/markdown/parse.test.ts",
    "src/markdown/render.test.tsx",
    "src/markdown/colors.test.ts",
    "src/lib/emojiUsage.test.ts",
    "src/lib/appLinks.test.ts",
  ];
  for (const f of readdirSync("src/components/composer")) if (f.endsWith(".test.ts") || f.endsWith(".test.tsx")) files.push("src/components/composer/" + f);
  const out: string[] = [];
  for (const f of files) {
    try {
      out.push(...literalsFrom(f));
    } catch {
      /* missing */
    }
  }
  return out;
}

const HAND = [
  "", "\n\n  \n", "plain", "**bold**", "a*b*c", "a_b_c", "_a_b_", "***x***", "___x___", "__**both**__", "**a\nb**", "`a` and `b`", "``a`b``",
  "# Heading", "## Title ##", "# C#", "# ##", "#\ttab", "####### seven", "-# small", "> quote\n> more", "> [!NOTE]\n> body", "> [!NOTE] inline\nafter",
  ">>> rest\nof\nmessage", ">>> [!WARNING] all\nof it", "- a\n- b\n  - c\n    - d\n- e", "1. a\n2. b\n- c", "3) x\n4) y", "- [ ] todo\n- [x] done",
  "---", "- - -", "***", "    ---", "```\ncode\n```", "```js\nconst x = 1;\n```", "```code```", "````\n```\n````", "```\nunclosed",
  "::: details Summary\nbody\n:::", "::: details\n:::", "::: details a\n::: details b\nx\n:::\n:::", "| a | b |\n|---|:-:|\n| 1 | 2 |",
  "| a |\n| --- |\n| `x|y` |\n| ||s|p|| |", "a[^1]\n\n[^1]: note", "[^1]: unused", "x[^a] y[^b]\n[^b]: B[^a]\n[^a]: A", "x[^nope]",
  "[red words]{red}", "[x]{on yellow}", "[x]{red on #ff0}", "[x]{grey}", "[x]{nope}", "==mark==", "a==b==", "x == y", "^{up} and _{down}",
  "||spoiler||", "~~strike~~", "@everyone @here", "<@1><#2><@&3>", "<t:1700000000:F>", "<t:1700000000:R>", "<:name:1>", "<a:dance:2>",
  "https://example.com/path_(x).", "see https://a.example, then", "[label](https://a.example)", "[a.example](https://b.example)",
  "[www.example.com](https://example.com)", "[go](https://sub.example.com)", "[example.com](https://sub.example.com)",
  ":smile: :+1: :nope:", "👍🏽❤️🇺🇸", "Nexa™ and ©", "1️⃣2️⃣", "🏳‍🌈", "😀".repeat(31), "😀".repeat(30), " # not heading",
  "# heading tail", "- item ", "a\r\nb\rc", "x".repeat(9000), "*".repeat(200), "[".repeat(300) + "x", "_".repeat(60) + "a",
  "![name](nexa-file:abc)", "![p](nexa-file:id_1?w=60)\ntext", "| a | b |\n|---|---|\n![x](nexa-file:y)",
  "![p](nexa-file:id_1?w=60&a=left)\ntext", "![p](nexa-file:a?a=right)", "![p](nexa-file:a?w=40&a=up)", "![p](nexa-file:a?a=left&w=60)", "![p](nexa-file:a?w=50&a=right)  ",
];

const corpus: string[] = [];
const seen = new Set<string>();
const add = (s: string) => {
  if (!seen.has(s)) {
    seen.add(s);
    corpus.push(s);
  }
};
for (const s of HAND) add(s);
for (const s of testInputs()) add(s);
for (let k = 0; k < 2500; k++) add(inline(1 + int(14)));
for (let k = 0; k < 2000; k++) add(block());

// ---------------------------------------------------------------------------
// run
// ---------------------------------------------------------------------------
async function main(): Promise<void> {
  if (!shortcodesReady()) {
    await new Promise<void>((resolve) => {
      onShortcodesReady(() => resolve());
      ensureShortcodes();
    });
  }

  const parsed = corpus.map((src) => {
    const entry: Record<string, unknown> = { src, ast: parse(src) };
    return entry;
  });
  // the other modes, on a share of the corpus
  const modes = corpus.filter((_, i) => i % 3 === 0).flatMap((src) => [
    { src, opts: { inline: true }, ast: parse(src, { inline: true }) },
    { src, opts: { files: true }, ast: parse(src, { files: true }) },
    { src, opts: { exact: true }, ast: parse(src, { exact: true }) },
    { src, opts: { files: true, exact: true, max: 20000 }, ast: parse(src, { files: true, exact: true, max: 20000 }) },
  ]);

  const names = {
    users: { "123": { display_name: "Ann", username: "ann" }, "456": { display_name: "", username: "bob" } },
    channels: { "789": { name: "general" } },
    roles: { "10": { name: "Mods", color: 0 } },
  };
  const plain = corpus.filter((_, i) => i % 2 === 0).map((src) => {
    const nodes = parse(src);
    return { src, bare: plainText(nodes), named: plainText(nodes, names), emojiOnly: isEmojiOnly(nodes), urls: extractUrls(src), flat: flattenInline(nodes) };
  });

  const urls = new Set<string>();
  for (const src of corpus) {
    for (const m of src.matchAll(/https?:\/\/[^\s<]+/gi)) urls.add(m[0]);
  }
  for (const u of ["https://a.b.c.)", "https://x.y/(a)", "https://x.y/a)", "https://x.y/[a]]", "https://x.y/?!:;,.", "https://", ")", ""]) urls.add(u);
  const trimmed = [...urls].map((url) => ({ url, out: trimUrl(url) }));

  const labels = ["bank.example", "www.bank.example", "https://bank.example/x", "a b.example c", "(evil.example)", "<x.example>", "[y.example]", "no host", "1.2.3", "exa_mple.com", "bücher.example", "a.b", "x.co.uk", "sub.example.com", "example.com", "EXAMPLE.COM", "go to https://foo.example now", "foo.example?", "foo.example#", "foo.examplé", "foo.ex1"];
  const targets = ["https://bank.example", "https://www.bank.example/", "https://evil.example", "https://sub.example.com", "https://example.com", "https://xn--bcher-kva.example", "https://bücher.example", "https://[::1]/", "https://1.2.3.4", "https://0x7f.1/", "https://user:pw@host.example:8080/p", "https://a..b/", "https://%41.example", "http://exa mple.com", "https://foo.example?q", "https:foo.example", "https://", "https://x.co.uk"];
  const misleading = labels.flatMap((label) => targets.map((url) => ({ label, url, value: misleadingLabel(label, url) })));

  const urlParts = targets.concat([...urls].slice(0, 400)).map((url) => {
    try {
      const u = new URL(url);
      return { url, ok: true, hostname: u.hostname, host: u.host, origin: u.origin, pathname: u.pathname, search: u.search, hash: u.hash, href: u.href };
    } catch {
      return { url, ok: false };
    }
  });

  const specs = ["red", "Red", "grey", "GRAY", "#abc", "#AABBCC", "#abcd", "on yellow", "red on yellow", "red  on\tyellow", " red ", "on", "red on", "a on b", "#fff on #000", "purple on #123456", "", "on on on"].map((raw) => {
    const spec = parseColorSpec(raw);
    return { raw, spec, token: colorToken(raw), formatted: spec ? formatColorSpec(spec) : null };
  });
  const surfaces = [
    { bg: [0x2c, 0x2d, 0x32], text: [0xf3, 0xf3, 0xf4] },
    { bg: [255, 255, 255], text: [0x11, 0x12, 0x14] },
    { bg: [0x13, 0x10, 0x1a], text: [0xf0, 0xed, 0xf7] },
    { bg: [0x80, 0x00, 0xff], text: [255, 255, 255] },
    { bg: [0xfd, 0xb8, 0x33], text: [0, 0, 0] },
  ] as { bg: [number, number, number]; text: [number, number, number] }[];
  const tokens = ["red", "orange", "yellow", "lime", "green", "teal", "cyan", "blue", "purple", "pink", "brown", "gray", "#000000", "#ffffff", "#ff0000", "#123456", "#fedcba", "#808080"];
  const paints: unknown[] = [];
  for (const surface of surfaces) {
    for (const fg of [null, ...tokens]) {
      for (const bg of [null, "yellow", "#000000", "#ffffff", "blue", "#7f7f7f"]) {
        if (!fg && !bg) continue;
        paints.push({ fg, bg, surface, out: paintColor({ fg, bg }, surface) });
      }
    }
    for (const token of tokens) paints.push({ swatch: token, surface, out: swatchColor(token, surface) });
  }

  const shortcodes = ["smile", "SMILE", "+1", "-1", "thumbsup", "heart", "flag-us", "us", "nope", "grinning_face", "e-mail", "100", "1234"].map((name) => ({ name, emoji: shortcodeToEmoji(name) ?? null }));
  const reverse = ["😀", "👍", "👍🏽", "❤️", "❤", "🇺🇸", "#️⃣", "🏳️‍🌈", "x"].map((emoji) => ({ emoji, name: emojiShortcode(emoji) ?? null }));
  const might = ["a:b:c", ":smile:", "12:30", "re: x", ":a:", ":" + "a".repeat(60) + ":", ":" + "a".repeat(61) + ":", "::"].map((text) => ({ text, value: mightHaveShortcode(text) }));

  const zalgo = ["à́̂", "à́̂̃", "à́̂̃̄b⃒⃓⃐⃑", "plain", "҃҄҅҆҇"].map((text) => ({ text, has: hasZalgo(text), clamped: clampZalgo(text) }));
  const twemoji = ["😀", "❤️", "❤", "🏳‍🌈", "🏳️‍🌈", "👁‍🗨", "👨‍👩‍👧", "👍🏽", "#️⃣", "🇺🇸", "", "a"].map((text) => ({ text, files: twemojiFiles(text) }));

  const files = [["abc", "name", undefined, null], ["id", "with [brackets] and\nnewline", 60, "left"], ["x", "  spaced  ", 100, "right"], ["y", "a".repeat(300), 3, null], ["z", "p", 15.4, "right"], ["q", "p", undefined, "left"], ["r", "p", 50, "up"]].map(([id, name, width, align]) => ({ id, name, width: width ?? null, align, line: fileLine(id as string, name as string, width as number | undefined, align as "left" | "right" | null), w: fileWidth(width as number | undefined) ?? null, a: fileAlign(align as string | null) ?? null }));

  // timestamps and the app's other dates, in each language, at a fixed now
  const NOW = Date.UTC(2026, 9, 3, 14, 30, 15);
  const realNow = Date.now;
  Date.now = () => NOW;
  const stamps: unknown[] = [];
  const dates: unknown[] = [];
  const moments = [NOW, NOW - 30_000, NOW + 90_000, NOW - 3_600_000 * 5, NOW - 86_400_000, NOW - 86_400_000 * 2, NOW + 86_400_000 * 9, NOW - 86_400_000 * 40, NOW - 86_400_000 * 400, Date.UTC(2026, 0, 5, 9, 5, 7), 0, -86_400_000 * 365 * 60];
  for (const locale of ["en", "es", "fr", "de", "sv", "pt-BR", "ru", "ja", "ko", "zh-CN", "hi", "ar"]) {
    renderToString(createElement(I18nProvider, { locale, children: createElement("span") }));
    for (const ms of moments) {
      for (const style of ["t", "T", "d", "D", "f", "F", "R"] as const) stamps.push({ locale, unix: ms / 1000, style, out: time.formatStamp(new Date(ms), style) });
      dates.push({
        locale,
        ms,
        time: time.formatTime(ms),
        divider: time.formatDateDivider(ms),
        full: time.formatFull(ms),
        exact: time.formatExact(ms),
        listWhen: time.formatListWhen(ms, NOW),
        shortDate: time.formatShortDate(ms),
        sameDay: time.sameDay(ms, NOW),
      });
    }
    for (const seconds of [0, 5, 59, 60, 61, 90, 120, 3600, 7200, 3601, 86400]) dates.push({ locale, duration: seconds, short: time.formatShortDuration(seconds), long: time.formatDuration(seconds) });
    for (const n of [0, 999, 1000, 1234, 15_300, 1_200_000, 3_400_000_000]) dates.push({ locale, compact: n, out: time.formatCompact(n) });
    for (const p of [0, 12.5, 75, 100]) dates.push({ locale, percent: p, out: time.formatPercent(p) });
  }
  renderToString(createElement(I18nProvider, { locale: "en", children: createElement("span") }));
  for (const ms of moments) dates.push({ locale: "en", ms, timestamp: time.formatTimestamp(ms, NOW), relative: time.formatRelative(ms, NOW) });
  for (const n of [0, 1023, 1024, 1536, 1048576, 5_000_000, 1073741824, 5e12]) dates.push({ locale: "en", bytes: n, out: time.formatBytes(n) });
  Date.now = realNow;

  // the composer's helpers
  const users = {
    "1": { id: "1", username: "alice", display_name: "Alice A" },
    "2": { id: "2", username: "bob.smith", display_name: "Bob" },
    "3": { id: "3", username: "carol_", display_name: "Carol" },
    "4": { id: "4", username: "dave", display_name: "Ann" },
  } as unknown as Parameters<typeof extractMentions>[1];
  const scope = {
    members: [
      { id: "1", names: ["alice", "Alice A", "Al"] },
      { id: "2", names: ["bob.smith", "Bob", "Robert"] },
      { id: "4", names: ["dave", "Ann"] },
      { id: "5", names: ["Ann"] },
      { id: "6", names: ["Zoë Q"] },
    ],
    roles: [{ id: "10", name: "Mods" }, { id: "11", name: "Mod Team" }, { id: "12", name: "alice" }],
    channels: [{ id: "20", name: "general" }, { id: "21", name: "dev" }, { id: "22", name: "dev-chat" }],
    emoji: [{ name: "blob", token: "<:blob:30>" }, { name: "Blob", token: "<:Blob:31>" }, { name: "smile", token: "<:smile:32>" }, { name: "party", token: "<a:party:33>", fallback: true }, { name: "fire", token: "<:fire:34>", fallback: true }],
    isShortcode: (name: string) => ["smile", "fire", "heart", "+1"].includes(name.toLowerCase()),
  };
  const mentionInputs = [
    "hi @alice", "@Alice A and @al", "@bob.smith.", "@bob.smith...", "@BOB.SMITH hey", "@Ann", "@dave", "@Mods @Mod Team", "@alice's", "email@alice.com",
    "\\@alice", "<@1>", "<@!2> <@&10>", "`@alice` ```\n@alice\n```", "https://x.example/@alice#general", "#general #dev #dev-chat #devs", "a#general",
    "&#general", ":blob: :BLOB: :Blob: :smile: :party: :fire: :nope:", "@everyone @here @everyones", "@@alice", "<@alice>", "@Zoë Q", "@zoë q!", "@Robert",
    "#general\n@alice", "@al", "x".repeat(10) + "@alice",
  ];
  const mentions = mentionInputs.map((text) => ({ text, out: extractMentions(text, users, scope), bare: extractMentions(text, users) }));
  const human = [
    "<@1> <@!2> <@99>", "<@&10> <@&99>", "<#20> <#99>", "<:blob:30> <a:party:33>", "`<@1>`", "```\n<@1>\n```", "https://x.example/<@1>", "plain",
  ].map((text) => ({
    text,
    out: humanizeMentions(text, users, { roles: { "10": "Mods" }, channels: { "20": "general" }, emoji: (token) => token.includes("blob") }),
    bare: humanizeMentions(text, users),
  }));
  const limits = ["", "a".repeat(3799), "a".repeat(3800), "a".repeat(4000), "a".repeat(4001), "  " + "a".repeat(4000) + "  ", "😀".repeat(2001)].map((text) => ({
    len: text.length,
    text: text.length < 50 ? text : null,
    gen: text.length >= 50 ? { ch: text.trim()[0] ?? "", n: text.length, pad: text.startsWith(" ") } : null,
    left: charsLeft(text),
    shown: counterShown(text),
    tooLong: tooLong(text),
    paste: pasteOverflows(text, 0, 0, "abc"),
  }));
  const codes = ["", "`a", "`a`", "```\nx", "```\nx\n```\n`", "a\n`b", "``", "```js\n:smile"].map((before) => ({ before, value: inCode(before) }));
  const commands = ["/shrug", "/shrug hi", "/tableflip", "/me waves", "/me", "/spoiler secret", "/spoiler", "/ban alice", "/usr/bin", "no slash", "  /SHRUG  x ", "/lenny\nmore", "/unflip", "/", "/ me"].map((text) => {
    const r = runCommand(text);
    return { text, kind: r.kind, out: r.kind === "command" ? r.text : r.kind === "unknown" || r.kind === "empty" ? r.name : null };
  });
  const people = [
    { id: "1", username: "alice", display_name: "Alice" },
    { id: "2", username: "michal", display_name: "Michał" },
    { id: "3", username: "al_bundy", display_name: "" },
    { id: "4", username: "xal", display_name: "Sal" },
    { id: "5", username: "bob", display_name: "Bob Al" },
    { id: "6", username: "Ålex", display_name: "" },
  ];
  const nick = (id: string) => (id === "4" ? "Big Al" : "");
  const recent = recentSpeakers(["5", "5", "1", "9", "3"], "9");
  const ranks = ["", "al", "AL", "a", "alice", "x", "big", "ål", "michał", "zz"].map((q) => ({ q, ids: rankPeople(q, people, nick, recent).map((p) => p.id) }));

  // emoji usage and suggestions
  const H = 7 * 24 * 60 * 60 * 1000;
  const usage = { "😀": { s: 3, at: NOW - H }, "c:30": { s: 1, at: NOW }, "👍": { s: 10, at: NOW - 4 * H } };
  const usageOut = {
    keys: ["😀", "👍🏽", "❤️", "👩🏻‍💻", "🏳️‍🌈", "<:blob:30>", "a:party:33", ":blob:30", "x"].map((e) => ({ e, unicode: unicodeKey(e), reaction: reactionKey(e), custom: customKey(e) })),
    score: [scoreOf(usage["😀"], NOW), scoreOf(undefined, NOW), scoreOf(usage["👍"], NOW + H)],
    withUse: withUse(usage, ["😀", "😀", "🔥"], NOW),
    ranked: rankedKeys(usage, NOW),
    keysIn: ["hi 😀😀 :smile: <:blob:30> ||👍|| `🔥`", "::: details 🎉\n❤️\n:::", "| a | 🎈 |\n|---|---|\n| <a:x:1> | b |", "x[^1]\n[^1]: 🌟"].map((text) => ({ text, keys: emojiKeysIn(text) })),
    history: usageFromHistory(usage, [{ text: "😀 🔥", at: NOW - H }, { text: "🔥", at: NOW }, { text: "<:blob:30>", at: NOW - 2 * H }], NOW),
    sanitized: sanitizeUsage({ ok: { s: 1, at: 5 }, bad: { s: -1, at: 5 }, nan: { s: "x", at: 5 }, ["k".repeat(81)]: { s: 1, at: 1 }, arr: [1] }),
    trimmed: withUse(Object.fromEntries(Array.from({ length: 70 }, (_, i) => [`k${i}`, { s: i + 1, at: NOW }])), ["new"], NOW),
    popularity: ["😂", "❤️", "❤", "💐", "🫠", "x", "👍🏽"].map((e) => ({ e, p: popularity(e) })),
    words: ["broken_heart", "heartBeat", "flag-us", "a.b c", "ABC", "x2Y"].map((n) => ({ n, words: wordsOf(n) })),
    match: ["heart", "broken_heart", "heartbeat", "smiling_face_with_smiling_eyes", "+1", "flag-us"].flatMap((name) => ["", "h", "hea", "heart", "brok_hea", "bh", "sfwse", "eat", "art", "1", "us"].map((q) => ({ name, q, score: nameMatch(name, q) }))),
    boost: [0, 0.5, 1, 3, 100].map((s) => ({ s, b: useBoost(s) })),
  };

  // links to the app
  const here = "https://nexa.pm/channels/@me";
  const hrefs = [
    "https://nexa.pm/invite/abc123#k=1.AAAA", "https://nexa.pm/invite/abc123/", "http://nexa.pm/note/abcdefghijklmnop#key", "https://nexa.pm/@maya.dev",
    "https://nexa.pm/channels/1/2/3", "https://nexa.pm/api/v1/x", "https://nexa.pm/file.png", "https://nexa.pm/call/xyz", "https://nexa.pm/channels/@me#top",
    "https://other.example/invite/abc", "https://NEXA.FISH/Invite/x", "https://nexa.pm:8443/invite/x", "nexa.pm/invite/x", "https://nexa.pm/download?x=1#y",
  ];
  const links = hrefs.map((href) => ({ href, link: appLink(href, { href: here, host: "nexa.pm" }) }));
  const targetsOut = ["/channels/@me/1/2", "/channels/5/6/7/", "/channels/5/6", "/channels/5/6/7?x#y"].map((path) => ({ path, target: messageTarget(path) }));
  const cards = ["https://nexa.pm/invite/a#k=1 https://nexa.pm/invite/a https://nexa.pm/invite/b <https://nexa.pm/invite/c> `https://nexa.pm/invite/d`", "https://nexa.pm/@ann https://nexa.pm/@bob https://nexa.pm/@ann https://nexa.pm/@c https://nexa.pm/@d", "https://nexa.pm/note/abcdefghijklmnop#k ||https://nexa.pm/note/bbbbbbbbbbbbbbbb||"].map((src) => {
    const nodes = parse(src);
    const at = { href: here, host: "nexa.pm" };
    return { src, invites: inviteLinksIn(nodes, at), notes: noteLinksIn(nodes, at), profiles: profileLinksIn(nodes, at) };
  });
  const g = globalThis as unknown as { window?: unknown };
  const hadWindow = "window" in g;
  g.window = { location: { origin: "https://nexa.pm" } };
  const invites = [
    "https://nexa.pm/invite/abc#k=1.x", "https://nexa.pm/invite/abc/#k", "https://nexa.pm/invite/abc", "https://nexa.pm/invite/a/b#k", "https://evil.example/invite/abc#k",
    "http://nexa.pm/invite/abc#k", "https://nexa.pm:443/invite/abc#k", "not a url",
  ].map((url) => ({ url, shown: shownInvite(url) }));
  // media links, as a page at https://nexa.pm sees them at NOW
  const nowWas = Date.now;
  Date.now = () => NOW;
  (g as { location?: unknown }).location = { href: "https://nexa.pm/channels/@me", origin: "https://nexa.pm" };
  const fresh = Math.floor((NOW + 3_600_000) / 1000);
  const soon = Math.floor((NOW + 60_000) / 1000);
  const mediaUrls = [
    `https://nexa.pm/media/proxy/${fresh}-abc/key1/pic.png`, `/media/proxy/${fresh}-abc/key2`, `https://nexa.pm/media/proxy/${soon}-x/key3`,
    `https://nexa.pm/media/proxy/old-signature/key4`, "https://nexa.pm/media/avatar/1", "/media/avatar/2", "//evil.example/x", "/\\evil.example/x",
    "/\tx", "https://evil.example/pic.png", "data:image/png;base64,AA", "blob:https://nexa.pm/1", "relative/path.png", "https://nexa.pm:8443/media/x",
    `https://other.example/media/proxy/${fresh}-a/k`, "",
  ];
  const media = mediaUrls.map((url) => {
    const src = mediaSrc(url);
    return { url, proxied: isProxied(url), deadline: proxyDeadline(url), key: mediaKey(url), src: src === undefined ? "pending" : src === null ? "blocked" : src === url ? "usable" : "renewed" };
  });
  Date.now = nowWas;
  delete (g as { location?: unknown }).location;
  if (!hadWindow) delete g.window;
  const posts = [
    "https://x.com/a/status/1", "https://mobile.twitter.com/a", "https://a.b.c.x.com/x", "https://a.b.c.d.x.com/x", "https://i.redd.it/x.png", "https://redd.it/abc",
    "https://www.youtube.com/watch?v=1", "https://youtu.be/1", "https://m.youtu.be/1", "https://mastodon.social/@ann/1234567890", "https://m.example/@ann@other.example/12345678901",
    "https://m.example/users/ann/statuses/1234567890/", "https://example.com/@ann/123", "ftp://x.com/a", "not a url", "https://X.COM./a", "https://bsky.app/profile/x/post/y",
  ].map((url) => ({ url, value: isPostLink(url) }));
  const fragments =["#k=12." + "A".repeat(43), "#x=1&k=3." + "_".repeat(43) + "&y", "#k=1." + "A".repeat(39), "#k=1." + "A".repeat(44), "", "#k=a.B"].map((hash) => {
    const key = parseInviteFragment(hash);
    return { hash, key: key ? { epoch: key.epoch, root: Buffer.from(key.root).toString("hex") } : null };
  });

  // the emoji picker's list, as the web builds it (every picture counts as drawn here)
  await loadEmoji();
  const groupsOut = emojiGroups().map((gr) => ({ id: gr.id, size: gr.items.length, head: gr.items.slice(0, 8).map((e) => e.unicode), tail: gr.items.slice(-4).map((e) => e.unicode) }));
  const sampleUnicodes = ["😀", "👍", "❤️", "🧑‍🤝‍🧑", "👨‍👩‍👧", "🏳️‍🌈", "🇺🇸", "#️⃣", "🫠", "🤌", "👋", "🧔‍♀️"];
  const entries = sampleUnicodes.map((u) => {
    const e = emojiByUnicode(u);
    return e ? { unicode: u, hexcode: e.hexcode, label: e.label, group: e.group, tags: e.tags, shortcodes: e.shortcodes, skins: e.skins ?? null } : { unicode: u, missing: true };
  });
  const searches = ["smile", ":heart:", "HEART", "thumbs", "flag", "us", "zzz", "", "  cat ", "face with", "+1", "-1"].map((q) => ({ q, out: searchEmoji(q).map((e) => e.unicode) }));
  const tones = ["👍", "🧑‍🤝‍🧑", "👋", "😀", "🤝"].flatMap((u) => [0, 1, 3, 5].map((tone) => {
    const e = emojiByUnicode(u);
    return { unicode: u, tone, out: e ? withTone(e, tone) : null };
  }));
  const byCode = ["smile", "+1", "thumbsup", "heart", "flag-us", "us", "nope", "SMILE"].map((name) => ({ name, unicode: emojiByShortcode(name)?.unicode ?? null }));
  const toneOfOut = ["1F44D-1F3FB", "1F9D1-1F3FC-200D-1F91D-200D-1F9D1-1F3FF", "1F600", "1f3fd"].map((hex) => ({ hex, tone: toneOf(hex) ?? null }));
  const popular: Record<string, number> = {};
  for (const e of allEmoji()) {
    for (const u of [e.unicode, ...(e.skins ?? []).map((s) => s.unicode)]) if (popularity(u)) popular[u] = popularity(u);
  }
  const picker = { count: allEmoji().length, groups: groupsOut, entries, searches, tones, byCode, toneOf: toneOfOut, popular };

  // smart suggestions over the whole list and a few custom emoji
  const custom = [
    { id: "e1", server_id: "s1", name: "blob_heart", blob_id: "900", animated: false, creator_id: "u", created_at: 0 },
    { id: "e2", server_id: "s1", name: "partyBlob", blob_id: "901", animated: true, creator_id: "u", created_at: 0 },
    { id: "e3", server_id: "s2", name: "heart", blob_id: "902", animated: false, creator_id: "u", created_at: 0 },
  ];
  const candidates: EmojiCandidate[] = [
    ...custom.map((emoji, i) => ({ kind: "customEmoji" as const, emoji, own: i < 2 })),
    ...allEmoji().map((entry) => ({ kind: "emoji" as const, entry })),
  ];
  const sugUsage = { [unicodeKey("💔")]: { s: 4, at: NOW - H }, "c:901": { s: 2, at: NOW }, [unicodeKey("🔥")]: { s: 1, at: NOW - 30 * H } };
  const suggestions = ["", "h", "hea", "heart", "brok", "fire", "blob", "pb", "us", "smiling face", "xyzzy", "thu", "+1", "flag"].flatMap((query) =>
    [{ usage: {}, recent: [] as string[] }, { usage: sugUsage, recent: ["😀", "❤️", "🔥"] }].map(({ usage: u, recent: r }, variant) => ({
      query,
      variant,
      out: suggestEmoji({ query, candidates, usage: u, recent: r, now: NOW }).map((s) => ({ kind: s.kind, key: s.kind === "emoji" ? s.entry.unicode : s.emoji.blob_id, name: s.name })),
    })),
  );

  // reactions and custom emoji tokens
  const reactions = {
    tokens: custom.map((e) => ({ blob: e.blob_id, token: emojiToken(e), key: emojiReactionKey(e) })),
    picks: [
      { unicode: "👍" }, { unicode: "<:blob:12>" }, { unicode: "<a:party:13>" }, { unicode: "<:bad name:14>" }, { unicode: "x", custom: custom[1] },
    ].map((p) => ({ unicode: p.unicode, custom: p.custom ? p.custom.blob_id : null, key: reactionKeyOf(p as Parameters<typeof reactionKeyOf>[0]) })),
    search: ["blob", "BLOB", ":heart:", "art", "", "party", "zz"].map((q) => ({ q, out: searchCustomEmoji({ s1: custom.slice(0, 2), s2: custom.slice(2) } as never, q).map((e) => e.blob_id) })),
    starter: STARTER_EMOJI,
  };

  const out = {
    generated: { node: process.versions.node, icu: process.versions.icu, unicode: process.versions.unicode, zone: Intl.DateTimeFormat().resolvedOptions().timeZone, now: NOW },
    parse: parsed,
    modes,
    plain,
    trimUrl: trimmed,
    misleading,
    url: urlParts,
    colorSpec: specs,
    paint: paints,
    shortcodes,
    reverse,
    might,
    zalgo,
    twemoji,
    files,
    stamps,
    dates,
    mentions,
    human,
    limits,
    codes,
    commands,
    ranks,
    recent: [...recent.entries()],
    usage: usageOut,
    links,
    targets: targetsOut,
    cards,
    invites,
    media,
    posts,
    fragments,
    picker,
    suggestions,
    reactions,
  };
  writeFileSync(OUT, gzipSync(JSON.stringify(out), { level: 9 }));
  console.log("wrote", OUT, corpus.length, "sources");
}

void main();

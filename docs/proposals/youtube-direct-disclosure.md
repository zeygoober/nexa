# Proposed Privacy Policy wording for YouTube playing straight from YouTube

This is only a proposal. Nothing in `docs/legal/`, `web/src/lib/legal.ts` or the `legal.*` strings has been
changed. If you want it in the documents, replace the passages below in the Privacy Policy, bump
`LEGAL_EFFECTIVE` and add a `legal.new*` line (CLAUDE.md, Public source). Every statement was checked against
`web/src/components/chat/YouTubePlayer.tsx`, `internal/api/static.go` (the page's `frame-src`) and
`desktop/src/main/permissions.ts`.

Why it matters: since 2026-11-18 a YouTube video no longer plays through the server. When someone presses
play, the web and desktop apps load YouTube's privacy-enhanced player (`www.youtube-nocookie.com`) directly,
because YouTube's bot check on the server's Cloudflare address stopped every video. Until you change the
documents, these passages say something that is no longer true:

- the summary near the top (line 34): "YouTube videos ... play through the server, so those services never
  learn who is watching";
- section 4, the link previews item (lines 386 to 395): "A YouTube video plays in YouTube's own player, loaded
  through the server ...";
- section 6, Cloudflare WARP (line 476): "YouTube videos ... go out through it";
- section 6, IGDB (line 491): "Trailers on a game page play through the server like other YouTube videos."

The old wording also said YouTube "gets no cookies". The player now needs its own origin to run, so it can
keep cookies and storage for youtube-nocookie.com once a video plays; the wording below says so.

What did not change: the video's picture, title and length still come through the server and WARP, and
nothing of YouTube's loads until play is pressed. The Android app already opened YouTube videos in YouTube's
own app or the browser and is unchanged.

## Summary near the top, replacing the bullet about links

> - Links you share are previewed by the server through Cloudflare, so the sites never see your address, and
>   music from Spotify, Apple Music, Deezer and SoundCloud plays through the server, so those services never
>   learn who is listening. A YouTube video's picture comes through the server too, but once you press play it
>   loads from YouTube itself (section 4).

## Section 4, the link previews item, replacing its YouTube sentences

> - Link previews, pictures and videos linked from elsewhere, GIFs, the pictures of YouTube videos, and music
>   from Spotify, Apple Music, Deezer and SoundCloud are fetched by the Nexa server, not by your device, and
>   the server fetches them through Cloudflare's WARP network. The sites see a Cloudflare address, never yours
>   and never the server's, and your device never connects to them. A YouTube video is different once you
>   press play: it plays in YouTube's own privacy-enhanced player (youtube-nocookie.com), which your device
>   loads from YouTube directly. YouTube then sees your IP address and your browser's description, and is
>   told the video is shown on Nexa. YouTube's player may also keep its own cookies and storage for
>   youtube-nocookie.com in your browser once a video plays. Nexa does not tell YouTube who you are, and
>   nothing of YouTube's loads until you press play. Spotify's and SoundCloud's own players ... (the rest unchanged)

## Section 6, Cloudflare WARP

> Link previews, pictures and videos linked from elsewhere, GIFs, the pictures of YouTube videos, and game,
> music, book, show and film searches go out through it, ...

## Section 6, a new item for YouTube

> - YouTube (Google), for playing YouTube videos. When you press play on a YouTube video in a chat or a game
>   page's trailer, your device loads YouTube's player from youtube-nocookie.com, so YouTube sees your IP
>   address and your browser's description, may keep its own cookies and storage for youtube-nocookie.com, and
>   knows the video was played on Nexa. YouTube's own privacy
>   policy applies to what it does with that. Why: YouTube refuses to play videos for the server's shared
>   address.

## Section 6, IGDB, its last sentence

> Trailers on a game page play from YouTube like other YouTube videos (see YouTube, below).

## Also worth knowing

The What's New entry for 2026-10-01 (`whatsNew.r20261001.youtubeBody`) still says YouTube never learns who is
watching. It was true then, and people who open What's New from Settings, About will read it. The new
2026-11-18 entry says what changed. Leave the old one as history, or reword it: your choice.

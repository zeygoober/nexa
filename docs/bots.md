---
title: Bots and Apps
description: How to make an app with a bot for Nexa, point a bot written with a library made for Discord at it, and what such a bot can read and do.
outline: [2, 3]
---

# Bots and Apps

An app is a program you make in Settings, Developer Portal. Most apps have a
bot: an account your program signs in as, which people add to their servers.
An app can also be added to a person's own account, so its commands work for
them anywhere apps can be used.

Nexa speaks the bot protocol of the libraries made for Discord: version 10 of
its gateway and REST API, with versions 6 to 9 answered the same way. A bot
written with discord.py, discord.js, discordgo, JDA, Nostrum or another such
library runs on Nexa once it is pointed at Nexa's addresses and given its
token. The rest of its code stays as it is, its intents and commands too.

[[toc]]

## Make an App

1. Open Settings, Developer Portal and press New App. Give it a name; its bot
   goes by the same name.
2. The bot's token is shown once, as the app is made. Put it in your bot's
   settings. If it leaks, reset it on the Bot tab: the old one stops working
   at once.
3. On the Install tab, choose where the app can be added (servers, people's
   accounts, or both) and what its bot asks for when it is added to a server.
4. Copy the install link from the Install tab and open it to add the bot to a
   server you manage, or share it.

People see what an app asks for, and what it can read, before they add it.
They can take it back at any time: from a server in Server Settings,
Integrations, and from their account in Settings, Authorized Apps.

## Point Your Library at Nexa

| What | Address |
| --- | --- |
| REST API | `https://nexa.pm/api/v10` |
| Gateway | `wss://nexa.pm/bot-gateway` |
| Pictures and files | `https://nexa.pm/cdn` |
| OAuth2 | `https://nexa.pm/oauth2/authorize`, tokens at `https://nexa.pm/api/v10/oauth2/token` |

Another Nexa server answers at its own address in the same places. The
Developer Portal shows the lines below with its own address filled in.

### discord.py

```py
import discord
import yarl

BASE = "https://nexa.pm"
discord.http.Route.BASE = BASE + "/api/v10"
discord.gateway.DiscordWebSocket.DEFAULT_GATEWAY = yarl.URL("wss://nexa.pm/bot-gateway/")
discord.Asset.BASE = BASE + "/cdn"
```

### discord.js

```js
const { Client, GatewayIntentBits } = require("discord.js");

const client = new Client({
  intents: [GatewayIntentBits.Guilds, GatewayIntentBits.GuildMessages],
  rest: { api: "https://nexa.pm/api", cdn: "https://nexa.pm/cdn" },
});
```

The gateway's address comes from `GET /gateway/bot`, so the REST address is
all discord.js needs.

### discordgo

```go
const base = "https://nexa.pm"

func pointAtNexa() {
	api := base + "/api/v" + discordgo.APIVersion + "/"
	discordgo.EndpointDiscord = base + "/"
	discordgo.EndpointAPI = api
	discordgo.EndpointGuilds = api + "guilds/"
	discordgo.EndpointChannels = api + "channels/"
	discordgo.EndpointUsers = api + "users/"
	discordgo.EndpointGateway = api + "gateway"
	discordgo.EndpointGatewayBot = discordgo.EndpointGateway + "/bot"
	discordgo.EndpointWebhooks = api + "webhooks/"
	discordgo.EndpointStickers = api + "stickers/"
	discordgo.EndpointApplications = api + "applications"
	discordgo.EndpointOAuth2 = api + "oauth2/"
	discordgo.EndpointOAuth2Applications = discordgo.EndpointOAuth2 + "applications"
	cdn := base + "/cdn/"
	discordgo.EndpointCDN = cdn
	discordgo.EndpointCDNAttachments = cdn + "attachments/"
	discordgo.EndpointCDNAvatars = cdn + "avatars/"
	discordgo.EndpointCDNIcons = cdn + "icons/"
	discordgo.EndpointCDNBanners = cdn + "banners/"
	discordgo.EndpointCDNGuilds = cdn + "guilds/"
	discordgo.EndpointCDNRoleIcons = cdn + "role-icons/"
}
```

Call `pointAtNexa()` before `discordgo.New`.

### Other Libraries

Set the library's REST base to the API address above and its CDN base to the
pictures address. Most libraries ask `GET /gateway/bot` for the gateway's
address; one that has the address written in needs the gateway address too.

## What Your Bot Can Read

Messages on Nexa are end-to-end encrypted: the server stores them as
ciphertext. A bot reads two kinds of message.

- **Messages meant for it.** A direct message to the bot, a message that
  mentions it, a reply to one of its messages, and a message someone uses one
  of its message commands on. The sender's app seals a copy of such a message
  for the bot, and the server opens it to hand the bot the message, files
  included, as the library expects.
- **Every message, if you ask.** On the Bot tab, the card that says what your
  app can read has Every Message It Can See: the Message Content intent. The bot then holds
  the keys of the servers it is in, as a member does, and gets every message
  in the channels it can see, earlier ones included. People are told before
  they add such a bot, and the start of every channel it can see names it.

Without Message Content, other messages reach the bot as the libraries know
them without that intent: everything but their content, embeds, files and
components. Commands, buttons, select menus and forms work either way, since
what someone sends through them is sent to the bot.

What a bot sends is not end-to-end encrypted, and the apps mark it.

## Intents

The gateway sends what the intents a bot asks for cover, as the protocol does.
Three of them are privileged, switched on for the app on the Bot tab:

| Intent | On the Bot tab |
| --- | --- |
| Presence | Receive presence |
| Server members | Receive the member list |
| Message content | Every Message It Can See, in the card that says what it can read |

A bot that asks for a privileged intent its app does not have is closed with
code 4014, and an unknown intent with 4013.

## Commands and Interactions

- Global commands and commands for one server, of every kind: slash commands
  with options, subcommands and groups, choices and autocomplete, and entries
  in the menus of a person and of a message. Names and descriptions can be
  given in other languages; people see the ones in their own.
- A server's managers can choose who may use each command, in Server
  Settings, Integrations; bots read and change the same permissions through
  the API.
- Answers of every kind: a message, a message only the person sees, thinking
  first, updating the message a button is on, follow-ups, editing and deleting
  the original answer, autocomplete choices, and forms.
- Messages with embeds, components (buttons, every kind of select menu, and the
  layout components), polls, stickers, replies and files.
- Forms with text inputs, select menus, file uploads, radio groups and
  checkboxes, in labels, with text between them.
- An interaction's token lasts 15 minutes.

Interactions come over the gateway, or, when the app has an Interactions
Endpoint URL on its General tab, as HTTP requests to that address, signed with
the app's key (`X-Signature-Ed25519` and `X-Signature-Timestamp`, checked with
the Public Key the portal shows). Nexa checks the address with a ping, and
with a badly signed request it must refuse, before it saves it.

## OAuth2

The OAuth2 tab makes the app's secret (shown once), lists the addresses people
may be sent back to after they agree, and builds links. Supported:

- The authorization code grant, with PKCE, the implicit grant, client
  credentials, and refreshing.
- The scopes `bot`, `applications.commands`, `identify`, `email` (Nexa holds
  no email addresses, so it is always empty), `guilds`, `guilds.join`,
  `guilds.members.read`, `connections`, `applications.commands.update`,
  `applications.commands.permissions.update` and `webhook.incoming`.
- Access tokens last 7 days. A refresh token lasts 90 days unused, and each
  refresh gives a new pair.

## Ids

The ids a bot sees are Nexa's own, moved by a fixed amount so the time inside
them reads right in the libraries. The portal shows them this way (Application
ID, Client ID). Use them only with the bot API; Nexa's own app API uses the
ids without the offset.

## Limits

- REST requests have the protocol's per-route limits, with its
  `X-RateLimit-*` headers, and a global limit of 50 requests a second. Over a
  limit, the answer is a 429 with `retry_after`.
- On the gateway, a bot may send 120 frames a minute, must identify within 20
  seconds, and heartbeats every 41.25 seconds. A session can be resumed for 2
  minutes after its connection drops. A bot may start 1,000 sessions a day and
  is told to use one shard for every 1,000 servers.
- The gateway speaks JSON or ETF, with `zlib-stream` or `zstd-stream`
  compression, or each frame compressed on its own.

## Voice Channel Text Chat

Bots can send and receive text in a server's voice channel without joining
its call. The channel keeps type `2` (`discord.VoiceChannel` in discord.py)
and uses the usual message endpoints, intents and Message Content rules.
Sending requires View Channel and Send Messages; fetching messages also
requires Read Message History and Connect.

```py
channel = await client.fetch_channel(VOICE_CHANNEL_ID)
message = await channel.send("Hello from the voice channel chat")
await message.edit(content="Hello again")
async for message in channel.history(limit=10):
    print(message.content)
```

## What Is Not Here Yet

- **Voice calls.** Bots cannot join calls yet. A bot that tries is told it is in no
  channel, and its library gives up as it would on a failed connection.
- **What Nexa does not have.** Stage channels, scheduled events, soundboard
  sounds, server templates, widgets, welcome screens, onboarding prompts,
  membership screening, linked roles, voice channel statuses, and anything
  sold (SKUs, entitlements and subscriptions come back as empty lists). Bots
  cannot make servers or group direct messages. These answer with the
  protocol's own errors, "… are not available on this server." (code 50024),
  so libraries handle them as any refused request.

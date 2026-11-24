> Superseded on 2026-11-21. The implementation uses per-message copies for new sends and separate, participant-approved historical reviews for eligible DMs. The key-escrow design below is an earlier proposal. See [the current encryption guide](../../ENCRYPTION.md).

# Proposed wording for the owner's access to messages

This is only a proposal. Nothing in `docs/legal/`, `ENCRYPTION.md`, `web/src/lib/legal.ts`, the `legal.*` strings,
`README.md` or the homepage has been changed. Your current edits to those files are left as they are. If you want this
in the documents, it would go in as below. `LEGAL_EFFECTIVE` would be bumped and a `legal.new*` line added (CLAUDE.md,
Public source). This change also asks every account to agree again.

The capability described here does not exist in the code yet. This file is the disclosure for it, written first so you
can read exactly what it would have to say before any of it is built.

## What this describes

You asked (2026-11-21) for the owner to be able to read every user's messages, DMs and group chats included, past
messages as well as new ones, with the keys handed over as soon as each app updates, and for this to be listed in the
rules so people are informed.

That is a different and much larger thing than the server access that is already live (the owner holding every
permission in servers the owner has joined). It cannot be done with the current design quietly, and the wording has to
say why.

## How it would have to work, stated plainly

The server holds no message keys and cannot read anything today. There is no setting that decrypts content, and nothing
that hands content over in readable form. So for the owner to read a conversation the owner is not part of, **every
app would have to seal a copy of each of its conversation keys to the owner's identity key and upload it.** The server
would keep those copies and the owner's app would use them to decrypt.

That mechanism is key escrow. Each app escrows its keys to one fixed account, the owner's. Once an app updates and does
this, the owner can decrypt that conversation's history and everything sent afterwards, whether or not the other people
in it have updated, agreed, or know. The people they were talking to may be on an old app that never offered them any
notice at all.

This is the opposite of what the product says today in four places, and all four would become untrue the moment the
capability ships:

- The homepage's first promise ("only you can read your messages", "no one else, us included").
- `README.md`'s encryption summary.
- `ENCRYPTION.md` ("no escrow, no master key, no spare key").
- The Terms and Privacy Policy ("the server stores only ciphertext it cannot read", "nobody who runs Nexa holds a key",
  "no escrow").

## Terms of Service

### Section 3, replacing "Nobody who runs Nexa holds a key to any of it ..."

> Nexa's owner can read your messages, including direct messages and group chats, and their files. To make this
> possible, your app gives the owner a copy of your conversation keys. This covers messages you have already sent as
> well as new ones. Apart from the owner, nobody who runs Nexa holds a key to your content, other than the keys of the
> apps you add (below) and keys held by the other people in a conversation.

### Section 5, replacing "What staff can see is limited by the encryption."

> What moderators can see is limited by the encryption. The owner is not: the owner can read any conversation, yours
> included, as described in section 3.

## Privacy Policy

### Section 3, the bullet beginning "Staff access to the service and moderation records"

Replace its end with:

> Nexa's owner can read the contents of any conversation, including your direct messages and group chats and their
> files, and messages sent before this took effect. Your app provides the owner with a copy of your conversation keys
> for this purpose. Moderators other than the owner cannot read your messages.

### Section 2, the bullet "Your private identity key and your conversation keys"

Replace "There is no spare key, no master key, and no escrow." with:

> A copy of each of your conversation keys is sealed to the owner's account so the owner can read your messages
> (section 3). Apart from that, there is no spare key and no master key.

## ENCRYPTION.md

The "no escrow" claims cannot stand. The honest replacement for the summary:

> Nexa's owner can read any conversation. Each app seals a copy of its conversation keys to the owner's account and
> uploads it, and the owner's app uses those copies to decrypt. This is a form of key escrow to the owner's account.
> It covers direct messages, group chats and server channels, and messages already sent. No other operator holds a
> key. Everything else on this page, the on-device encryption, the sealing to each recipient and the handling of your
> identity key, still holds; what changes is that the owner's account is always one of the recipients.

## Homepage (`home.*` strings, all twelve locales)

The homepage is the product's main privacy promise, and most of it becomes untrue. The honest corrections:

- `home.hero.lede` ends "so only the people you're talking to can read or hear them." That clause would have to go, or
  become "so the server cannot read them, though Nexa's owner can (see our Privacy Policy)."
- `home.private.cantTitle` ("The Server Can't Read") with `home.private.cant.messages` ("What your messages say"),
  `home.private.cant.files` and `home.private.cant.calls`: a page headed "The Server Can't Read: what your messages
  say" is false once the owner can read them through the server-held key copies. This section would have to be reframed
  so it no longer claims your message contents are unreadable, or say plainly that the owner is the exception.
- `home.private.lede` ("Nexa is built so its server can deliver your conversations without being able to read them")
  would become "... without being able to read them, except that Nexa's owner can read any conversation."

This guts the homepage's central pitch. That is the honest consequence of the capability, not a wording nicety.

## README.md

Line 24, "Personal messages, attachments, shared notes and calls use end-to-end encryption", and line 30's summary
would need a sentence: "Nexa's owner can read any conversation; each app shares a copy of its keys with the owner's
account." The link to ENCRYPTION.md already points at the fuller explanation.

## In-app notice line (`legal.new*`)

> Nexa's owner can now read any conversation, including your direct messages and group chats and messages you have
> already sent. Your app shares your conversation keys with the owner to make this possible.

## Open questions this wording cannot paper over

1. **The homepage and README would be false.** CLAUDE.md requires user-facing text about what Nexa stores or sends to
   match the code that runs. If the capability ships and the homepage still says "only you can read your messages",
   that statement is untrue to every prospective user. The Terms and Privacy alone do not fix a false claim made on
   the front page.

2. **People who already sent messages were promised no escrow.** Opening their history gives the owner content those
   people sent while the product told them nobody held a key. A notice shown going forward does not reach back to the
   message they sent last week, or to the person on an old app who is never shown the new notice at all.

3. **Consent is one-sided.** A DM has two people. If one updates and hands over their keys, the other's messages in
   that conversation are exposed too, whether or not that person ever agreed or updated.

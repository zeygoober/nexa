# Proposed wording for the owner's access to servers

This is only a proposal. Nothing in `docs/legal/`, `ENCRYPTION.md`, `web/src/lib/legal.ts` or the `legal.*` strings
has been changed. Your current edits to those files are left as they are. If you want this in the documents, it would go
in as below. `LEGAL_EFFECTIVE` would be bumped and a `legal.new*` line added (CLAUDE.md, Public source). This change also
asks every account to agree again.

Why it matters: until now the documents say staff see only what is public or reported. With this change the owner holds
every permission in any server the owner is a member of, private channels included. The owner's app then receives those
channels' keys, earlier history included, as a server's own owner would. The owner can also join any server from the
staff panel. Together that means the owner can read every channel of any server. Leaving that out would make the
current wording untrue for servers.

Every statement below was checked against the code: `internal/store/permcache.go` (`privileged`, `nexaOwner`),
`internal/api/server_handlers.go` (`actorHighest`), `internal/api/moderation_owner.go` (Join Server),
`internal/api/moderation_manage.go` (Manage Server and its records), `internal/store/keys.go` (`Needs`, which asks for keys for every viewer `permcache.go` names) and
`web/src/lib/convMembers.ts` (members' apps check membership only).

## Terms of Service

### Section 1, after "The owner and a team of moderators ... keep Nexa running."

> The owner of Nexa has every permission in any server the owner is a member of, the same as that server's own owner,
> including private channels and their earlier messages. The owner can join any server, appears in its member list like
> anyone else, and cannot remove or ban that server's owner. This never applies to DMs or group chats.

### Section 5, after "What staff can see is limited by the encryption."

> This holds for every member of staff except the owner of Nexa, who can read a server's channels after joining it
> (section 1).

## Privacy Policy

### The bullet that begins "Staff access to the service and moderation records in section 3"

Add at its end:

> The owner of Nexa is an exception for servers. Once the owner is a member of a server, the owner has every permission
> there, private channels included. The members' apps then give the owner the keys to read its channels, earlier
> messages included, as they would for the server's own owner. The owner can join any server from the moderation panel;
> that join is recorded in the moderation log, and the owner shows in the member list. Changes the owner makes to a
> server through the moderation panel are recorded in that server's audit log, marked as made by Nexa's owner, and in
> the moderation log. Reading messages is not recorded anywhere. None of this applies to DMs or group chats.

### Section 3d, Membership and structure

No new data is kept, so 3d needs no change. The audit log entries 3d already lists now include changes made by Nexa's
owner.

## ENCRYPTION.md

### "What is encrypted, and what is visible", a sentence after the table

> In a server, the people who can read a channel are its members with access to it, the server's owner, and the owner
> of Nexa once a member of that server. The owner of Nexa has every permission there, private channels included, and so
> receives those channels' keys, earlier epochs included, through the usual key requests. The owner appears in the
> member list. DMs and group chats are not affected.

### "Checking identities and understanding server trust", after "membership and permissions ultimately come from the server."

> Nexa's server gives its owner every permission in the servers the owner is a member of. Members' apps check only that
> a key recipient is in the member list, so they hand those keys over.

### "History, new members and key rotation", at the end

> Keys handed to the owner of Nexa stay with the owner until that channel's key rotates, as with any member.

## In-app notice line (`legal.new*`, if you add one)

> Nexa's owner can now join any server and has every permission there, private channels included. This never applies to
> DMs or group chats.

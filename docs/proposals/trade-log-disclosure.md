# Proposed Privacy Policy wording for badge trades and the owner's Trade Log

This is only a proposal. Nothing in `docs/legal/`, `web/src/lib/legal.ts` or the `legal.*` strings has been
changed. If you want it in the documents, it would go in the Privacy Policy as below, with `LEGAL_EFFECTIVE`
bumped and a `legal.new*` line added (CLAUDE.md, Public source). Every statement was checked against
`internal/store/badge_trades.go`, `internal/store/badge_owners.go`, `internal/api/moderation_owner_trades.go`
and `internal/api/moderation_owner.go`.

Why it matters: trades were already kept on the server (CLAUDE.md, Badge trades, says the documents need a
line about them). Now the owner can also read every trade and see who holds copies of any badge, so the
documents should say so.

One choice for you: on 2026-11-08 you decided the documents would not describe owner-only tools, only the records
kept and that moderation actions are logged. If that still holds, leave out the sentence starting "The owner can
read every trade" and keep the rest.

## Section 3, a new item beside Gems (or after Plans)

> o) Badge trades (not end-to-end encrypted, kept by the server because it moves badges): each trade you open
> or are offered, who it was between, what each of you offered, whether each of you was ready, how it ended
> (completed, declined, cancelled or still open) and when it was opened and ended, and how many copies of
> each badge trades have given you. The owner can read every trade, past ones included, and see who holds
> copies of a badge and how many. Each time the owner opens the trade list or a badge's owners, that is
> written to the moderation log.

## Section 7, How long things are kept, a new line

> - Badge trades (3o): while both accounts in the trade exist. Deleting your account deletes your trades and
>   the copies trades gave you.

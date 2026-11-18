# Proposed Privacy Policy wording for Gems

This is only a proposal. Nothing in `docs/legal/`, `web/src/lib/legal.ts` or the `legal.*` strings has been
changed. If you want it in the documents, it would go in the Privacy Policy as below, with `LEGAL_EFFECTIVE`
bumped and a `legal.new*` line added (CLAUDE.md, Public source). Every statement was checked against
`internal/api/gems.go`, `internal/store/gems.go`, `internal/api/moderation_owner.go` (the owner's Give or Take Gems) and migration 0064.

## Section 3, a new item after l) (or after k), Plans)

> n) Gems (not end-to-end encrypted, kept by the server because it moves them): how many Gems you have, how
> many are on hold, how many you have earned in all, and whether earning is paused for a check or by staff.
> A record of what you spent on capsules, Gems you traded and held Gems released or taken. Earning a Gem
> writes no record of its own. To spot automated farming, the server also keeps, for each day you earn: how
> many Gems you earned, how many came right after the one minute wait, how many came where nobody else had
> spoken for half an hour, which hours of the day they came in, and roughly how long the messages that
> earned them were. Nothing a message says, where it was sent or who it was sent to is kept for Gems. If
> those counts look automated, you are asked to complete a captcha; if they still do, a Gem Farming report
> with those daily counts and the number of accounts that share an address with yours goes to staff, as
> section 3g describes. Gems cannot be traded between accounts that share an address. The owner can give
> or take Gems from an account; each change is written to the moderation log with its amount and any reason,
> and appears in your record. The owner can also give or take copies of a collectable badge; each change
> is written to the moderation log with the badge, the amount and any reason.

## Section 7, How long things are kept, new lines

> - Gems (3n): your balance and the record of spending, trades and held Gems while your account exists. The
>   daily earning counts: 14 days.
> - A Gem Farming report keeps the daily counts it was filed with as part of the moderation record, like
>   other safety reports.

## Section 8, Deletion

Deleting an account deletes its wallet, record and daily counts. A Gem Farming report already filed stays
with the moderation record, which section 7 already says outlives the account.

## Section 11, Your rights (data export)

> Your data export includes your Gems: the balance, the record and the daily counts (`gems.json`).

## Section 10, Legal bases (if you list them per record)

> Gems and their records: to provide the feature you use. The daily earning counts: legitimate interest in
> keeping Gems fair and stopping automated farming.

## Terms of Service, if wanted

The Terms do not mention Gems. You may want a line that Gems have no money value, cannot be bought, sold or
exchanged for money, can be held or taken for farming, can be given or taken by the owner (collectable badges too), and can be changed
or removed. That is a decision for
you; no wording is proposed here beyond that outline.

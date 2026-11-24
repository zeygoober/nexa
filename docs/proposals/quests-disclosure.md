# Proposed Privacy Policy wording for Quests

This is only a proposal. Nothing in `docs/legal/`, `web/src/lib/legal.ts` or the `legal.*` strings has been
changed. If you want it in the documents, it would go in the Privacy Policy as below, with `LEGAL_EFFECTIVE`
bumped and a `legal.new*` line added (CLAUDE.md, Public source). Every statement was checked against
`internal/api/quests.go`, `internal/store/quests.go`, `internal/store/users.go` (the account purge),
`internal/api/legal_handlers.go` and migration 0068. It assumes the Gems wording in
`docs/proposals/gems-disclosure.md` goes in as item n); if that is placed differently, renumber.

## Section 3, a new item after the Gems item

> o) Quests (not end-to-end encrypted, kept by the server because it counts them and pays Gems for them):
> the daily quests and Quest Tower step you have, how far each has got, when it was started and finished
> and what it paid; the Quest Tower step you have reached; and the days you last swapped a daily quest or skipped
> a Quest Tower quest. To count a quest about different servers, people or reports only once each, the server
> notes which ones that quest has already counted, and forgets them when the quest is finished. To count
> only new friends and new servers, it keeps which people you have befriended and which servers you have
> joined since quests began (and, from when they began, the friends and servers you already had). To count
> a message towards a quest, the server checks whether someone else spoke in that conversation in the last
> half hour; nothing the message says is read or kept. Quests about other people do not count accounts that
> share an address with yours, using the same check as section 3e. Gems a quest pays go to your Gems
> wallet as section 3n describes, held or not paid while that section says earning is held or paused.

## Section 7, How long things are kept, new lines

> - Quests (3o): a day's quests are kept 14 days after the day ends, and a finished Quest Tower step 14 days after
>   it was finished. Your Quest Tower step, the days you last swapped or skipped, and the list of people
>   befriended and servers joined are kept while your account exists.

## Section 8, Deletion

Deleting an account deletes its quests, its Quest Tower step and both lists. Deleting an account also removes it
from other people's lists of people they have befriended.

## Section 11, Your rights (data export)

> Your data export includes your quests, in `gems.json` with your Gems.

## Section 10, Legal bases (if you list them per record)

> Quests and their records: to provide the feature you use. The check that the people and servers counted
> are new and not your own other accounts: legitimate interest in keeping quests fair.

## Terms of Service, if wanted

The Terms do not mention quests. If a Gems line goes in (no money value, can be held or taken for farming,
can be changed or removed), it could say the same of quests and what they pay. That is a decision for you;
no wording is proposed here beyond that outline.

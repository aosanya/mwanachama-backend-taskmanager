# Project parties

A project holds a `contract_reference` and any number of parties, each in a
role such as client or donor. A party is the id of a record row another module
holds, plus the name as written when it was added.

## Rules the module enforces

| Rule | Where |
| --- | --- |
| A record row id is a 36-character hyphenated uuid and is not the nil uuid. Anything else is refused: 400 on add and remove (`ErrInvalidProjectParty`), 400 on a list (`ErrInvalidFilter`). | `canonicalRecordRowID` |
| An id is stored, compared and filtered in lower case, so the same row in upper case is the same party: a second add is 409 (`ErrProjectPartyExists`), a remove or a list by the upper-case id finds it. | `AddProjectParty`, `RemoveProjectParty`, `FindProjects` |
| A party holds a role in a project once. | `AddProjectParty` |
| `FindProjects` never returns a deleted project, with or without a party filter. | `TestFindProjects_ADeletedProjectIsNotListedByReferenceOrByNoFilter` |

## What the mounting host wraps

The module never reads another module's rows. A host that can reads them hands
in one value with two methods through `UsePartyRows`:

| Method | The host answers | The module uses it |
| --- | --- | --- |
| `RowExists(ctx, recordRowID)` | whether the row is there | on `AddProjectParty`: an unknown row is refused with 400 (`ErrProjectPartyRowUnknown`) naming the id, and a lookup error is returned as it came, never taken as yes |
| `RowChain(ctx, recordRowID)` | the ids of the row and of every row it replaced, at any depth | on `FindProjects` with `party_row_id`: the list matches a party stored under any id of the chain; each id of the answer must be a record row id, or the list is refused |

Both are asked with the canonical lower-case id.

## When nothing is wired

`UsePartyRows(nil)`, or never calling it, leaves the module as it was: the
shape and the nil uuid are the only checks, an add for a row nobody holds is
accepted, and a list by a corrected row's new id finds only parties added under
that exact id. A hub that mounts the module for a record module must wire both
methods; the module does not fall closed, because a board with no record module
has no row to check.

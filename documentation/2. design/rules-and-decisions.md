# Rules, decisions and questions

One object, `rule`, holds the three kinds a team writes down: a rule says what to do today, a decision says what was chosen once and why, a question is a standing question about how the team works that a decision will answer. The owner asked for this on 2026-11-25 because rules sat in markdown files that contradict each other. Questions were added by ISS-253. Hub issue ISS-234; the choices below are the owner's decisions 171 to 175 in `hub-steward-swe`.

## Two kinds, one object

`kind` is `rule` or `decision`. They share every field below and differ in two things: their code series and their statuses.

| | Rule | Decision |
|---|---|---|
| Code | `R-<n>` | `D-<n>` |
| Statuses | `proposed`, `in_force`, `superseded`, `retired` | `proposed`, `decided`, `superseded` |

A status outside its kind's set is refused with a sentence that names the allowed ones. A decision is never retired, only replaced. A rule is retired when it no longer applies and nothing replaces it.

## Questions

A third kind, `question`, for what was a loose line in a markdown file and got asked again in a new chat. It has its own series `Q-<n>`, never reused, and the statuses `open`, `answered` and `dropped`. A question starts `open`.

| Piece | Behaviour |
|---|---|
| `priority` | `P1`, `P2` or `P3`, on a question only. A question created without one is `P2`. A priority on a rule or a decision is refused with `ErrInvalidRule`, on create and on update, and so is any other value |
| Answering | `AnswerQuestion(code, decisionCode)` sets `answered_by` to the decision's code and the status to `answered`. The second code must name an existing decision: unknown is `ErrRuleNotFound`, a rule or another question is `ErrInvalidRule`. A dropped question is reopened before it is answered |
| Dropping and reopening | `SetRuleStatus` to `dropped` or back to `open`; both clear `answered_by`, so only an `answered` question names a decision (chosen in the ISS-234 review fixes; the page was silent). Setting `answered` by hand is refused, because an answer names its decision |
| Superseding | A question is answered or dropped, never superseded, so `SupersedeRule` refuses it |
| Listing | `ListRules` with `kind=question` returns `open` ones by default, sorted P1 first and by code inside one priority; with no kind the default list is rules, decisions and open questions, kinds in the order decision, question, rule |
| Tags, issue links, files | Work as for rules and decisions: the same tag cleaning, the same `applies_to`, `decided_in` and `broken_by` links, the same file links |
| Codes in text | `Q-12` typed in an issue is recognised like `R-12` and `D-3` |

A question is not an issue: work that follows an answer becomes an issue linked to the `Q-n`.

On the issue brief, `rules.questions` carries the open questions linked to the issue or sharing a tag with it, P1 first. A question never appears under `linked` or `suggested`; an answered or dropped one leaves the brief but still lists its issues from the question's side.

## Fields

| Field | Meaning |
|---|---|
| `id` | A uuid minted by the server, like every other object |
| `kind` | `rule`, `decision` or `question` |
| `code` | `R-41` or `D-41`. Set when the record is created, never changed, never reused, even after the record is superseded or retired |
| `title` | One sentence stating the rule or the choice |
| `body` | The reason and the detail, in plain words |
| `status` | One of the kind's statuses |
| `tags` | The same tags issues carry, so a rule meets the issues it governs |
| `applies_when` | Optional conditions that say which issues the rule governs (see Contradiction check below); empty when the rule is not tied to conditions |
| `supersedes` | The `code` of the record this one replaces, empty when none |
| `superseded_by` | Set on the old record when a replacement is created, so a reader of the old code finds the new one |
| `decided_by`, `decided_at` | Who decided and when |
| `priority` | `P1` to `P3`, questions only; empty on a rule or decision |
| `answered_by` | The `D-<n>` that settles a question; empty until answered |
| `created_at`, `updated_at` | As elsewhere |

## Tags

A record's tags are the issues' tags. Creating, updating or superseding a record cleans the tags it is given: each is trimmed, empty ones are dropped, and a tag that repeats another one in any letter case is dropped, keeping the first spelling. Each cleaned tag is then registered in the shared tag set through the same lookup issues use, so a tag first written on a rule exists for issues to carry and filter by, and a tag already in the set keeps its code. The list takes any number of tags and returns the records that carry every one of them, compared without regard to letter case. A tag in the filter is cleaned the same way, so a blank one is ignored. The tag test runs on every row of the chosen kind and statuses, read a page at a time in `id` order, never on a first page cut at 500 rows.

## Codes

A code comes from the existing `codesequence` counter (`nextCode(ctx, tx, entityType, prefix)`), one entity type per series, so two records created at once never share a number and a number is never given twice. A decision's code `D-41` and a rule's code `R-41` are different records.

## Supersede

Superseding takes the old record's code and the replacement's fields. It creates the replacement with a new code, sets the old record to `superseded` and its `superseded_by`, and sets the replacement's `supersedes`, in one transaction. Only a record that is `in_force` (rule) or `decided` (decision) can be superseded, and a record that is already superseded is refused. The old record stays readable with its code and history. A replacement sent without `applies_when` (omitted or `null`) carries over the old rule's set, as an omitted field keeps its value on update; `{}` clears it, and a set of its own replaces it. So a superseding rule does not silently leave the overlap check. The replacement is checked as a create is: a priority on it is refused with `ErrInvalidRule` and any `answered_by` is cleared.

## Editing and concurrent writes

- `UpdateRule` refuses a `superseded` or `retired` record with `ErrInvalidRule`: the words of a record that has left force are the history and stay as they are.
- Every write that changes a record (`UpdateRule`, `SetRuleStatus`, `SupersedeRule`, `AnswerQuestion`) puts the status it read in the `WHERE` of its own `UPDATE`. A write that matches no row returns `ErrRuleChanged`: the record changed meanwhile, read it and try again. So a stale write cannot bring a superseded record back, and two supersedes of one record make one replacement (the second one's transaction rolls back, replacement and code number included). `ErrRuleChanged` wraps `ErrInvalidRule` but answers 409, so a client can tell read-again-and-retry from bad input. The status table is a map with no order, so the route layer marks the four rule writes' error with its own type (`routes/rule_changed.go`) and matches that type first; mapping the sentinel alone answers 400 on some runs.
- `UpdateRule` takes an optional `expected_updated_at` (a field of the body, so the route and the tool both carry it). Left out, nothing changes and the last write wins. Sent, it must equal the stored `updated_at` or the write is refused with `ErrRuleStale` (409, and it does not wrap `ErrInvalidRule`, so the status table needs no wrapper); a blank value is 400. The stamp is also put in the `WHERE` of the write, so a change landing between the check and the write answers 409 as `ErrRuleChanged`. Every rule write moves `updated_at` strictly forward, one second past the previous stamp when the clock has not moved, so a read from before any write is always stale.
- The memory backend does not reproduce a true race, so the tests replay it as an interleaved sequence: read the record, change it through the API, then write from the stale read.

## Links

- **Tags** are the issues' tags (ISS-235).
- **Issues** (ISS-236): a rule or decision links to an issue through the existing relationship machinery, not a second mechanism. Three labels, each an edge from the rule's `id` to the issue's `id`: `applies_to` (the rule governs the issue), `decided_in` (the issue settled it), `broken_by` (the issue breaks it). They are stored in one join object, `rule_link` (`rule_id`, `task_id`, `relation`, `linked_at`), and `CreateRelationship`, `DeleteRelationship` and `TraverseRelationships` handle them like `has_tag`, with the join spec carrying a relation column. Linking the same rule, issue and relation twice is one link.
- **Reading from both sides**: `LinkRule` and `UnlinkRule` take a rule code, an issue id and a relation. `ListIssuesOfRule(code)` returns each linked issue with its id, code, title and relation. `ListRulesOfIssue(issueID)` returns `linked` (each rule with its relation, and the in-force replacement when the rule is superseded), `suggested` and `mentioned`. The issue brief (`TaskBriefFor`) carries the same three lists under `rules`.
- **Suggested**: rules that are `in_force` and decisions that are `decided` sharing at least one tag with the issue (compared without regard to letter case), not already linked. A proposed, superseded or retired record is never offered.
- **Mentioned**: a code such as `R-12` or `D-3` typed in the issue's title, description, or a todo's title, description or instructions is recognised when it names an existing record and is not already linked. A mentioned rule that has been superseded is returned as its in-force replacement (the end of the `superseded_by` chain, as the linked list shows it), never as the superseded record, and listed once when the replacement is also typed or linked. Retired, proposed and dropped records are returned as they are, with their status, because the author named them. A record already under `questions` is not repeated under `mentioned`, so an open question shows in one place: under `questions` when it is linked to the issue or shares a tag with it, otherwise under `mentioned` when its code is typed.
- **Rules and decisions**: a rule links to the decision it comes from (ISS-236).
- **Files** (decision 173, ISS-237): a rule carries file links to the best example of it done right. A link is a link only: `repo`, `path` on the main branch, an optional `line` (zero means the whole file) and a `note`; no copy of the file is stored. They live in one object, `rule_file`, whose id is a digest of the rule, repo, path and line, so the same place is linked once and a second attempt is refused with `ErrDuplicateRuleFile`; another line of the same file is a different link. The path is cleaned the way an issue reference's is (a leading slash and `.` or empty parts dropped, `..` refused), and a missing repo, an empty path or a negative line is `ErrInvalidRuleFile`.
  - `AddRuleFile(code, repo, path, line, note)` returns the link and a `warning`; `ListRuleFiles(code)` returns the links in the order they were added; `RemoveRuleFile(code, fileID)` removes one, or says `ErrRuleFileNotFound`.
  - **Missing path**: when a `RepoPathChecker` is attached with `WithRepoPathChecker(mgr, checker)`, `AddRuleFile` asks it whether the path exists on main. A path reported missing, or a check that fails, puts a sentence in `warning`; the link is saved either way. With no checker attached there is no warning. The module does not import the git custodian: the host that mounts it supplies a checker built on the custodian's file read.
  - **Missing flag** (ISS-255): `ListRuleFiles` returns each link with `missing: true` when the attached `RepoPathChecker` reports the path gone from main, from the same check `AddRuleFile` warns with. It is a flag only, never an error. A link whose path exists, a check that fails and a module with no checker attached carry no flag.
  - The issue did not ask for a good or bad marker on a link, so none exists; every file link is a good example.

## Routes and MCP tools

Every rule method is a declared operation in `taskmanager.operations.json`, so each is a route and a tool at once (ISS-239). The hub does not copy them; it imports a tagged version of this module and picks them up by moving to the new tag. A tool's name is its action with dots as underscores.

| Tool | Route | Call |
|---|---|---|
| `taskmanager_rule_create` | `POST /rules` | `CreateRule` |
| `taskmanager_rule_read` | `GET /rules/{ruleCode}` | `ReadRule` |
| `taskmanager_rule_update` | `PATCH /rules/{ruleCode}` | `UpdateRule` |
| `taskmanager_rule_list` | `GET /rules?kind=&status=&tag=` | `ListRules` |
| `taskmanager_rule_set_status` | `PUT /rules/{ruleCode}/status` | `SetRuleStatus` |
| `taskmanager_rule_answer` | `PUT /rules/{ruleCode}/answer` | `AnswerQuestion` |
| `taskmanager_rule_supersede` | `POST /rules/{ruleCode}/supersede` | `SupersedeRule` |
| `taskmanager_rule_link` | `POST /rules/{ruleCode}/links` | `LinkRule` |
| `taskmanager_rule_unlink` | `DELETE /rules/{ruleCode}/links/{issueID}/{relation}` | `UnlinkRule` |
| `taskmanager_rule_issue_list` | `GET /rules/{ruleCode}/issues` | `ListIssuesOfRule` |
| `taskmanager_rule_list_for_issue` | `GET /tasks/{taskID}/rules` | `ListRulesOfIssue` |
| `taskmanager_rule_file_add` | `POST /rules/{ruleCode}/files` | `AddRuleFile` |
| `taskmanager_rule_file_list` | `GET /rules/{ruleCode}/files` | `ListRuleFiles` |
| `taskmanager_rule_file_remove` | `DELETE /rules/{ruleCode}/files/{fileID}` | `RemoveRuleFile` |
| `taskmanager_rule_produced_list` | `GET /rules/{ruleCode}/rules` | `ListRulesOfDecision` |
| `taskmanager_rule_overlap_list` | `GET /rules/overlaps` | `ListRuleOverlaps` |
| `taskmanager_rule_compatible_add` | `POST /rules/{ruleCode}/compatible/{otherCode}` | `AddRuleCompatible` |
| `taskmanager_rule_compatible_remove` | `DELETE /rules/{ruleCode}/compatible/{otherCode}` | `RemoveRuleCompatible` |
| `taskmanager_rule_note_add` | `POST /rules/{ruleCode}/notes` | `AddRuleNote` |
| `taskmanager_rule_note_list` | `GET /rules/notes?rule=&kind=&state=&related=&since=&limit=` | `ListRuleNotes` |
| (no tool) | `GET /rules/{ruleCode}/notes` | `ListRuleNotes` |
| `taskmanager_rule_note_resolve` | `PUT /rules/notes/{noteID}/resolve` | `ResolveRuleNote` |

- The path parameter is `ruleCode`, not `code`, because the create, update and supersede bodies are whole rule records that already carry a `code`; two arguments arriving under one name are refused when the table is built.
- `list` repeats `status` and `tag`: `?tag=style&tag=data` returns the records carrying both. `GET /tasks/{taskID}/rules` is how an agent reads the linked, suggested and mentioned rules before starting an issue.
- **Questions over the routes** (ISS-253 on the wire): `kind` accepts `question` on create and on `taskmanager_rule_list`, whose `status` takes `open`, `answered` and `dropped`. `priority` (questions only) and `answered_by` ride on the create, update and read bodies as optional fields; `answered_by` is set by answering, never sent. The answer body is `{decision_code}`. A decision code that does not exist, or a question that does not, is 404; a missing `decision_code`, a code that is not a question, a decision code that names a rule or question, a dropped question, a bad priority, or a priority on a rule or decision is 400. `GET /tasks/{taskID}/rules` carries a fourth list, `questions`: the open questions linked to the issue or sharing a tag with it, P1 first, never repeated under `linked`.
- Errors: `ErrInvalidRule` and `ErrInvalidRuleFile` are 400, `ErrRuleNotFound` and `ErrRuleFileNotFound` 404, `ErrDuplicateRuleFile`, `ErrDuplicateRuleCompatible`, `ErrRuleChanged` and `ErrRuleStale` 409, `ErrRuleCompatibleNotFound` 404; a bad link relation is `ErrInvalidRelationship`, 400. The note errors are in the Notes section.

### Who may write

Rules belong to the agency, not to one backlog, so access reuses the backlog levels over the whole board. A person with no grants anywhere, the instance owner and a caller with no id see everything. Otherwise reading a rule, a decision, its issues or its files needs Can see on at least one backlog, and every write (create, update, set status, supersede, link, unlink, add or remove a file, resolve a note) needs Can move on at least one. Adding a note is the exception: Can see is enough (see Notes). A refusal is `ErrBacklogForbidden`, 403. Reading the rules of an issue follows the issue: a person who cannot see the issue gets 404.

## Reading by backlog

A backlog's Rules and Decisions views list the records whose tags match the tags on the issues inside the backlog (decision 172; ISS-246).

| Tool | Route | Call |
|---|---|---|
| `taskmanager_backlog_rules_list` | `GET /backlogs/{backlogID}/rules?status=` | `ListBacklogRules` |
| `taskmanager_backlog_decisions_list` | `GET /backlogs/{backlogID}/decisions?status=` | `ListBacklogDecisions` |
| `taskmanager_backlog_questions_list` | `GET /backlogs/{backlogID}/questions?status=` | `ListBacklogQuestions` |

- **Matching**: the backlog's tags are the distinct tags of the issues in it, compared without regard to letter case; a record is listed when it carries at least one of them (the same test the suggested rules of an issue use, not the every-tag test of `taskmanager_rule_list`). A backlog whose issues carry no tag lists nothing. Kind in the query is ignored: the first tool returns rules, the second decisions and the third questions. Tag narrows the list further: a record must be in the backlog's scope and carry every tag given, without regard to case (ISS-272).
- **Statuses** (decision 175): `status` repeats. With none given the rules list returns `in_force` and `proposed`, the decisions list `decided` and `proposed`, so superseded and retired records are hidden until asked for. A status of the other kind is refused with 400, as is an unknown backlog.
- **Access**: the caller needs Can see on that backlog, otherwise 403 (`ErrBacklogForbidden`); the rule access of the agency-wide list applies as well.
- A superseded record carries `superseded_by`, so the app can show the code of its replacement from the same answer.

- **Questions have their own list** (ISS-255). The rules and decisions lists fix their kind, so an open question never appears in them and `status=open` on either is refused with 400. `taskmanager_backlog_questions_list` matches the same way and returns questions: `open` by default, `answered` or `dropped` when asked for, P1 first and by code inside one priority; a status that is not a question's is refused with 400. Access is the same: Can see on that backlog, otherwise 403.
- **Rules a decision produced** (ISS-255): `taskmanager_rule_produced_list` (`GET /rules/{ruleCode}/rules`, `ListRulesOfDecision`) returns the rules linked `decided_in` to any issue the decision is also linked `decided_in` to, in code order and in every status, so a decision's page can list what came out of it. The issue that settled the decision is the join, because a rule links to issues only. A code that is not a decision is 400, an unknown one 404, and reading needs Can see on at least one backlog.

## Contradiction check (ISS-238, the part the engine can do)

The research in `automatic-checks.md` found that matching rules by tag gives about 91 false alarms in 113 pairs. So overlap is measured on conditions the taskmanager's own rules engine can evaluate, never on tags alone, and there is no second engine: the conditions are a `RuleSet` from `backlog_rules.go` and use its `Normalise`, `Key` and `Matches`.

- **`applies_when`** is an optional `RuleSet` on a rule: `{match: all|any, rules: [{field, operator, value}]}` with the fields and operators of a smart backlog (for example tag has css, priority is high). It is a JSON column on the `rule` object in the blueprint, so it travels as an object on the create, update, supersede and read bodies. It is stored normalised (fields, operators and tags lower-cased, duplicates dropped, rules in a fixed order), so two rules that say the same thing in different words store the same text. A set the engine refuses, a body that is not `{match, rules}`, or an unknown member is `ErrInvalidRule`, 400, and nothing is saved. On update a new set replaces the old one and `{}` clears it; a body with no `match` and no rules, such as `{"rules":[]}`, is the same as `{}`, treated as empty and not refused. Only a rule has a set: sending a non-empty one on a decision or a question is `ErrInvalidRule`, 400, on create, update and supersede, the way a priority is refused on a rule (`{}` on a decision is just empty). A rule without one is never part of an overlap.
- **Overlap read**: `ListRuleOverlaps` (tool `taskmanager_rule_overlap_list`, `GET /rules/overlaps`, Can see on a backlog like the other rule reads) takes the rules that are `in_force`, are of kind `rule` and carry a set, and returns each pair, lower code first, that meets in one of two ways: the two sets have the same `Key` (reason `shared_key`), or both sets match at least one issue the caller can see (reason `shared_issue`, the issue codes in `shared_issues`). A pair that is both is one item with both reasons. Matching runs the engine over the open issues only: an issue that is deleted, or whose status is closed (completed, failed or cancelled, which is what the done, archive and cancelled steps set), is left out, so a pile of old finished issues does not make two broad sets (tag has css, priority is high) overlap. It uses the same subject builder a smart backlog uses (`visibleRuleSubjects`), so a rule on `waits_on` or on the step name behaves as it does there. A set that cannot be evaluated is skipped, never an error.
- **Compatible pairs** (ISS-238, owner decision): a person who has read an overlapping pair and judged the two rules compatible records it with `AddRuleCompatible(code, otherCode)` (tool `taskmanager_rule_compatible_add`, `POST /rules/{ruleCode}/compatible/{otherCode}`, 204) and takes it back with `RemoveRuleCompatible` (`taskmanager_rule_compatible_remove`, `DELETE` on the same path, 204). Both writes need Can move on a backlog.
  - **Storage**: one object, `rule_compatible` (`id`, `first_rule_id`, `second_rule_id`, `added_by`, `created_at`), not a `rule_link` edge, because a `rule_link` runs from a rule to an issue and this joins two rules. The id is a digest of the two rule ids in sorted order, so the pair is symmetric, stored once and found from either side; the order of the codes in a call does not matter.
  - **Rules only**: both codes must be of kind `rule`. A decision or a question, or the same code twice, is `ErrInvalidRule`, 400; an unknown code is `ErrRuleNotFound`, 404; a pair already recorded is `ErrDuplicateRuleCompatible`, 409; removing a pair that was not recorded is `ErrRuleCompatibleNotFound`, 404.
  - **Superseded or retired**: the rows stay when a rule is superseded or retired, so putting a retired rule back in force brings its pairs back. A pair with such a rule is not shown on the read view and is not counted by the overlap read, which lists in-force rules only.
  - **Overlap read**: `ListRuleOverlaps` now answers `{items, hidden_compatible}` instead of a bare list. A pair that would be listed and is recorded compatible is left out of `items` and counted in `hidden_compatible`, so a reader can see that something was hidden. The count is of pairs that would otherwise have been listed, whichever reason made them meet.
  - **Rule read**: `ReadRule` (`taskmanager_rule_read`, `GET /rules/{ruleCode}`) returns the record with `compatible`, the codes of the rules judged compatible with it, in code order and an empty list when there are none, and with the Log's `notes` and `note_count` (see Notes). `GetRuleByCode` still returns the bare record for the module's own use.
- **No supersede check**: there is none, because the old rule of a supersede is `superseded`, never in force, so it is never in the pair list. Carrying the set over (below) is what keeps the replacement in the check. Rules relate to each other by supersede and by the compatible judgement below; the only other links are rule to issue (`applies_to`, `decided_in`, `broken_by`).
- **Reading, not judging**: the list says two rules meet, not that they disagree. The issues come from `visibleRuleSubjects`, which narrows to the backlogs the caller can see, so an issue in a hidden backlog never appears in `shared_issues`, and it loads the tag names of every untagged row in one batched read (500 ids at a time), so the number of queries does not grow with the number of issues; smart backlogs use the same function and now also match on the tagging rows, so an issue whose own tag list is empty but that has tagging rows matches a tag rule. A person or the AI judge reads each pair next. No AI judge is built here.
- **Lint finding**: `Lint` (`taskmanager_task_lint`) returns a `rules` list beside its issue items on the first unfiltered page. The list is added only to the first page of an unfiltered lint (no `taskID`, `backlog` or `step`, `offset` 0), so it comes once and not on every page, and a lint of one backlog or step does not pay for the rule file checks. Each item is an in-force rule with one `rule_file_missing` finding for every file link that `ListRuleFiles` flags `missing`, which is the attached `RepoPathChecker`'s answer. With no checker attached, or a retired, proposed or superseded rule, there is nothing. A check that fails is not a finding, as in `ListRuleFiles`.

## Notes (ISS-271)

Every rule, decision and question has a Log of dated notes, like an issue's Log. A note carries a person's, a worker's or the judge's reason, so a contradiction or a judgement is found where the record is read as well as computed from conditions. The owner decided on 2026-11-25 that notes sit beside `applies_when`, the overlap read and the compatible link, and replace none of them (see Notes beside the older checks).

### The object

One object, `rule_note`, declared in the blueprint and in both spec examples (`rule_notes`), with a store role of the same name.

| Field | Meaning |
|---|---|
| `id` | A uuid minted by the server |
| `rule_id`, `rule_code` | The record whose Log it belongs to. The code is kept beside the id so a list of notes from many records reads without a lookup; a code never changes |
| `kind` | `note` (the default), `contradiction`, `compatible` or `question`. Any other value is `ErrInvalidRuleNote`, 400 |
| `source` | `person` (the default), `worker` or `judge`, as an issue note's kind is `person` or `worker`. The loader's judge sends `judge`, so its verdicts go straight in marked as the judge. Any other value is 400 |
| `state` | `open` or `resolved` on a `contradiction`; empty on every other kind. A contradiction starts `open` |
| `text` | Required, trimmed, at most 4000 characters. Never changed |
| `related_codes` | A list of codes (`R-9`, `D-3`, `Q-5`) of other records the note is about. Each is trimmed, blanks and repeats are dropped, at most 20 |
| `links` | A list of links to the work: a commit, a file, a drawing, a CI run |
| `author_id` | Set by the hub from the caller, whatever the body says; `source` says what kind of writer it was, not who |
| `created_at` | Set by the hub, to the nanosecond, so the Log has one order |
| `resolved_at`, `resolved_by`, `resolution`, `resolved_by_code` | Empty until a contradiction is resolved. `resolved_by` is the caller. `resolution` is optional words, at most 4000 characters. `resolved_by_code` is the optional code of the rule or decision that settled it |

### Adding

`AddRuleNote(draft)` adds to the Log of any record, a rule, a decision or a question, in any status: a superseded or retired record still takes notes, because the reason it left force is the kind of thing a note holds. Nothing already in a Log is changed or deleted; there is no route that edits or removes a note, and the only change a note ever takes is the resolution below.

The request is one whole body, bound as a struct the way `POST /rules` is, so the body takes exactly `kind`, `source`, `text`, `related_codes` and `links` and any other key is refused with 400 before anything runs: a typo such as `relatedCodes` or `related_code` can no longer save a contradiction with no related code, and `author_id` in the body is refused, not ignored. The author and the record code are never read from the body: the author is the caller and the code is the path's `ruleCode`.

The checks run in this order, and the first to fail answers, so a request that fails two checks always gets the same status:

| Check | Answer |
|---|---|
| The caller has Can see on at least one backlog | `ErrBacklogForbidden`, 403 |
| The record named in the path exists | `ErrRuleNotFound`, 404 |
| `kind` and `source` are in their lists; `text` is not empty and not over the maximum | `ErrInvalidRuleNote`, 400 |
| `related_codes` has no more than 20 codes | `ErrInvalidRuleNote`, 400 |
| No related code is the record's own code | `ErrInvalidRuleNote`, 400 |
| Every related code names an existing rule, decision or question | `ErrRuleNoteCodeNotFound`, 404 |

Nothing is saved when a check fails.

### Resolving

`ResolveRuleNote(request)` takes a whole body of exactly `resolution` and `resolved_by_code` (any other key is 400, and the resolver and the note come from the caller and the path, never the body) and sets `state` to `resolved` and `resolved_at`, `resolved_by` (the caller), `resolution` and `resolved_by_code`, and changes nothing else: the text, kind, source, author, codes and links stay as written. It needs Can move on a backlog, as the other rule writes do. The checks, in order:

| Check | Answer |
|---|---|
| The caller has Can move on at least one backlog | `ErrBacklogForbidden`, 403 |
| The note exists | `ErrRuleNoteNotFound`, 404 |
| The note is a `contradiction` | otherwise `ErrInvalidRuleNote`, 400 |
| The note is still `open` | otherwise `ErrRuleNoteResolved`, 409 |
| `resolution` is not over the maximum | `ErrInvalidRuleNote`, 400 |
| `resolved_by_code`, when sent, names an existing record | `ErrRuleNoteCodeNotFound`, 404 |
| That record is a rule or a decision, not a question | `ErrInvalidRuleNote`, 400 |

The write puts `state = open` in the `WHERE` of its own `UPDATE`, so two people resolving one note at once make one resolution and the other gets 409 with the first one's words standing. A resolved note cannot be reopened.

### Reading

- **One record's Log**: `GET /rules/{ruleCode}/notes`, oldest first, with the filters of the list below apart from `rule`. This route has no tool of its own: its action is `taskmanager.rule.note_list_of` and `taskmanager_rule_note_list` with `rule=<code>` is the same call. An unknown code is 404.
- **Every record's notes**: `GET /rules/notes`, tool `taskmanager_rule_note_list`, oldest first. Filters, all optional: `rule` (a code; 404 if it does not exist), `kind` (repeat it for several), `state` (`open` or `resolved`; only a contradiction has one, so the filter leaves every other kind out), `related` (a code), `since` (an RFC 3339 time, strictly newer notes only) and `limit` (1 to 200, default 200, keeps the newest). An unknown kind or state, a `since` that is not a time or a `limit` outside 1 to 200 is `ErrInvalidRuleNoteFilter`, 400.
  - Every open contradiction is one call: `kind=contradiction&state=open`.
  - `related=D-3` finds a note where `D-3` is on either record: the notes written on `D-3` and the notes on other records that name `D-3` in `related_codes`. A code that does not exist finds nothing; it is not an error in a filter.
  - The order is `created_at` then `id`, so a note appended later is always after an earlier one.
- **On the record**: `ReadRule` (tool `taskmanager_rule_read`, `GET /rules/{ruleCode}`) returns the record with three more members: `compatible`, the codes of the rules judged compatible with it (see Contradiction check), `notes`, its latest 50 notes oldest first, each with its `kind`, `source` and `state`, and `note_count`, how many its Log holds in all. A record with no notes has `notes: []` and `note_count: 0`. `GetRuleByCode` stays the plain record for the module's own use.

### Access

Follows `rule_access.go`. Adding and reading need Can see on at least one backlog, the same as the other rule reads, the way an issue's Log takes a note from anyone who can see the issue. Resolving needs Can move. A person with no grants anywhere, the instance owner and a caller with no id are not held back. A caller who can see no backlog gets `ErrBacklogForbidden`, 403, on every note route.

### Status mapping

Five sentinel errors, each standing alone: none wraps `ErrInvalidRule` or another, so the route layer's map, which has no order, can never answer 400 where the answer is 404. `ErrInvalidRuleNote` and `ErrInvalidRuleNoteFilter` are 400; `ErrRuleNoteCodeNotFound` and `ErrRuleNoteNotFound` 404; `ErrRuleNoteResolved` 409. An unknown record in a path is the existing `ErrRuleNotFound`, 404. A test holds that no note sentinel satisfies `errors.Is` against another.

### Notes beside the older checks

| Kept | How it sits with notes |
|---|---|
| `applies_when` and the overlap read | They still say which rules meet. A person or the judge who reads a pair records the verdict as a `contradiction` note with the two codes in `related_codes`, or as a `compatible` note |
| The compatible link (ISS-238) | A stored pair that leaves the overlap read and is counted in `hidden_compatible`. A note never writes it and it never writes a note |
| The `compatible` note kind | Free words with a reason in a record's Log. It does not hide a pair; to hide one, record the link |
| `ReadRule` | One answer: the record, `compatible`, `notes` and `note_count`. `GetRuleByCode` stays the bare record for the module's own use |

## Open

- The AI judge of each pair of rules posts its verdicts as notes with source `judge` (ISS-243, ISS-238); it is built in the loader, not here.
- The issue scale is low to critical in this module, while a question's is `P1` to `P3`; the two are not mapped to each other.

- Whether `body` is plain text or Markdown.
- The first two questions to enter are whether a bad example sits beside the good one, and how a printed PDF shows file links.
- Whether `decided_by` is an actor id or a name; the app shows it as "Owner · 25 Nov 2026".
- The record formats research (ISS-242) may add or drop a field; the fields above are the working set.

## Writing a rule link through the relationship routes

`POST` and `DELETE /relationships` with a rule id as `from_id` and the label `applies_to`, `decided_in` or `broken_by` need the same access as `LinkRule` and `UnlinkRule`: Can move on at least one backlog, otherwise 403. A caller with Can see only cannot link or unlink a rule by this route either. An edge from an issue keeps its rule: Can move on the issue's backlog.

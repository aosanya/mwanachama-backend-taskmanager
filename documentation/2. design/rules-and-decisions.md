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

Superseding takes the old record's code and the replacement's fields. It creates the replacement with a new code, sets the old record to `superseded` and its `superseded_by`, and sets the replacement's `supersedes`, in one transaction. Only a record that is `in_force` (rule) or `decided` (decision) can be superseded, and a record that is already superseded is refused. The old record stays readable with its code and history. The replacement is checked as a create is: a priority on it is refused with `ErrInvalidRule` and any `answered_by` is cleared.

## Editing and concurrent writes

- `UpdateRule` refuses a `superseded` or `retired` record with `ErrInvalidRule`: the words of a record that has left force are the history and stay as they are.
- Every write that changes a record (`UpdateRule`, `SetRuleStatus`, `SupersedeRule`, `AnswerQuestion`) puts the status it read in the `WHERE` of its own `UPDATE`. A write that matches no row returns `ErrRuleChanged`: the record changed meanwhile, read it and try again. So a stale write cannot bring a superseded record back, and two supersedes of one record make one replacement (the second one's transaction rolls back, replacement and code number included). `ErrRuleChanged` wraps `ErrInvalidRule`, so it answers 400 until a 409 mapping is added to the route table.
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
| `taskmanager_rule_read` | `GET /rules/{ruleCode}` | `GetRuleByCode` |
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

- The path parameter is `ruleCode`, not `code`, because the create, update and supersede bodies are whole rule records that already carry a `code`; two arguments arriving under one name are refused when the table is built.
- `list` repeats `status` and `tag`: `?tag=style&tag=data` returns the records carrying both. `GET /tasks/{taskID}/rules` is how an agent reads the linked, suggested and mentioned rules before starting an issue.
- **Questions over the routes** (ISS-253 on the wire): `kind` accepts `question` on create and on `taskmanager_rule_list`, whose `status` takes `open`, `answered` and `dropped`. `priority` (questions only) and `answered_by` ride on the create, update and read bodies as optional fields; `answered_by` is set by answering, never sent. The answer body is `{decision_code}`. A decision code that does not exist, or a question that does not, is 404; a missing `decision_code`, a code that is not a question, a decision code that names a rule or question, a dropped question, a bad priority, or a priority on a rule or decision is 400. `GET /tasks/{taskID}/rules` carries a fourth list, `questions`: the open questions linked to the issue or sharing a tag with it, P1 first, never repeated under `linked`.
- Errors: `ErrInvalidRule` and `ErrInvalidRuleFile` are 400, `ErrRuleNotFound` and `ErrRuleFileNotFound` 404, `ErrDuplicateRuleFile` 409; a bad link relation is `ErrInvalidRelationship`, 400.

### Who may write

Rules belong to the agency, not to one backlog, so access reuses the backlog levels over the whole board. A person with no grants anywhere, the instance owner and a caller with no id see everything. Otherwise reading a rule, a decision, its issues or its files needs Can see on at least one backlog, and every write (create, update, set status, supersede, link, unlink, add or remove a file) needs Can move on at least one. A refusal is `ErrBacklogForbidden`, 403. Reading the rules of an issue follows the issue: a person who cannot see the issue gets 404.

## Reading by backlog

A backlog's Rules and Decisions views list the records whose tags match the tags on the issues inside the backlog (decision 172; ISS-246).

| Tool | Route | Call |
|---|---|---|
| `taskmanager_backlog_rules_list` | `GET /backlogs/{backlogID}/rules?status=` | `ListBacklogRules` |
| `taskmanager_backlog_decisions_list` | `GET /backlogs/{backlogID}/decisions?status=` | `ListBacklogDecisions` |
| `taskmanager_backlog_questions_list` | `GET /backlogs/{backlogID}/questions?status=` | `ListBacklogQuestions` |

- **Matching**: the backlog's tags are the distinct tags of the issues in it, compared without regard to letter case; a record is listed when it carries at least one of them (the same test the suggested rules of an issue use, not the every-tag test of `taskmanager_rule_list`). A backlog whose issues carry no tag lists nothing. Kind and tag in the query are ignored: the first tool returns rules, the second decisions and the third questions.
- **Statuses** (decision 175): `status` repeats. With none given the rules list returns `in_force` and `proposed`, the decisions list `decided` and `proposed`, so superseded and retired records are hidden until asked for. A status of the other kind is refused with 400, as is an unknown backlog.
- **Access**: the caller needs Can see on that backlog, otherwise 403 (`ErrBacklogForbidden`); the rule access of the agency-wide list applies as well.
- A superseded record carries `superseded_by`, so the app can show the code of its replacement from the same answer.

- **Questions have their own list** (ISS-255). The rules and decisions lists fix their kind, so an open question never appears in them and `status=open` on either is refused with 400. `taskmanager_backlog_questions_list` matches the same way and returns questions: `open` by default, `answered` or `dropped` when asked for, P1 first and by code inside one priority; a status that is not a question's is refused with 400. Access is the same: Can see on that backlog, otherwise 403.
- **Rules a decision produced** (ISS-255): `taskmanager_rule_produced_list` (`GET /rules/{ruleCode}/rules`, `ListRulesOfDecision`) returns the rules linked `decided_in` to any issue the decision is also linked `decided_in` to, in code order and in every status, so a decision's page can list what came out of it. The issue that settled the decision is the join, because a rule links to issues only. A code that is not a decision is 400, an unknown one 404, and reading needs Can see on at least one backlog.

## Open

- The issue scale is low to critical in this module, while a question's is `P1` to `P3`; the two are not mapped to each other.

- Whether `body` is plain text or Markdown.
- The first two questions to enter are whether a bad example sits beside the good one, and how a printed PDF shows file links.
- Whether `decided_by` is an actor id or a name; the app shows it as "Owner · 25 Nov 2026".
- The record formats research (ISS-242) may add or drop a field; the fields above are the working set.

## Writing a rule link through the relationship routes

`POST` and `DELETE /relationships` with a rule id as `from_id` and the label `applies_to`, `decided_in` or `broken_by` need the same access as `LinkRule` and `UnlinkRule`: Can move on at least one backlog, otherwise 403. A caller with Can see only cannot link or unlink a rule by this route either. An edge from an issue keeps its rule: Can move on the issue's backlog.

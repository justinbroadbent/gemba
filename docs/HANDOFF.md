# Gemba: handoff for continuing development

This is the working brief for whoever picks the project up next, human or
agent. It describes what Gemba is, how it is built, what it needs from GitHub,
what is proven, and what is left. It is current as of the commit that added it.

`docs/PLAN.md` is the original design record, with personas and requirements.
It explains why things are the way they are, but where the two disagree, this
document and the code are right.

## 1. What Gemba is

Gemba is a thin layer over GitHub Issues and a GitHub Project for a team that
takes in both planned work and a steady stream of unplanned tickets. It answers
three questions that a project board leaves each person to work out alone:

- **For an engineer:** what should I pull next, and why that?
- **For the team:** what is everyone holding, what is stuck, and what is coming?
- **For a manager:** if this new thing is the priority, what does it displace?

It borrows from lean manufacturing: work cells, pull, limits on work in
progress, classes of service, and showing exceptions, not inventory.

### Principles that must survive further development

1. **GitHub is the system of record.** Gemba has no database. Every page is
   computed from the issues and the project. Every change Gemba makes is
   written to GitHub. If it cannot be derived from GitHub, think hard before
   adding it.
2. **People work where they already work.** Engineers work planned items in
   GitHub and tickets in the ticketing system. Gemba never asks them to keep a
   third system up to date. It links to the item and gets out of the way.
3. **Field ownership is narrow and explicit** (section 5). Gemba writes a small
   set of fields and nothing else.
4. **Show exceptions.** The Floor leads with sentences about what needs help.
   Anything not flagged can be left alone. Do not add dashboards that show
   everything.
5. **It measures the system, not the person.** There are no scores, rankings or
   comparisons between people. Private manager notes and ratings were
   considered and deliberately left out; do not add them without that decision
   being revisited by the people who own the tool.
6. **Plain language in the interface.** The only Japanese is the logo. Labels
   say "Triage lead", not a borrowed term.
7. **Generic and rebrandable.** Nothing in the repository names an employer.
   Names, colours, cells, kinds of work and thresholds all come from config.

## 2. Vocabulary

| Term | Meaning |
|---|---|
| Workspace | One team's floor. Several can run in one deployment, isolated from each other. |
| Cell | A discipline with its own queue (for example network, platform, data). |
| Item | One GitHub issue on the project. |
| Mirrored item | An issue copied in from the ticketing system by an external sync. It is worked there, not in GitHub. |
| Unplanned | Any mirrored item, plus anything expedited. Derived, never set. |
| Kind | A kind of planned work, from config. Defaults: enterprise project, department project, maintenance. |
| Type | What the interface shows: "unplanned", a kind, or "other" (planned, kind not set). |
| Class of service | How an item is handled: quick, standard, fixed-date, background, expedite. |
| Rank | A number giving position in a cell's queue, lowest first. |
| Hand | One slot of a person's work-in-progress limit. |
| Stage | Derived: intake, queued, doing, waiting, done. |
| Fast lane | Fresh mirrored tickets, left out of triage for their first hours so quick ones resolve unaided. |
| Quick list | A cell's small jobs, kept off the ranked queue, oldest first. |
| The line | A cell's ranked queue drawn through time. |
| Capacity line | How many queued items fit in the planning horizon at the measured pace. |
| Waiting | Started, then parked until someone else acts. Frees the hand; limited per person. |
| Blocked | In progress and cannot move; needs help. Holds the hand; flagged at once. |
| Triage lead | The one person answerable for triage this week. Holds delegated authority to expedite. |

## 3. Pages

| Page | Route | For | What it does |
|---|---|---|---|
| Now | `/w/{ws}/now` | Each engineer | Hands, waiting items, the one thing to pull next with its reason, what comes after, the cell's quick list, the team's priorities. Park and resume your own work here. |
| Floor | `/w/{ws}/floor` | Standup | Attention sentences, then a room per cell: a mat per person, the quick list, the line (drag to reorder), where finished work went. |
| Queues | `/w/{ws}/queues` | Planning | The full ranked queues, searchable, folded past the capacity line. |
| Projects | `/w/{ws}/projects` | Planning, reporting | Roll-up per project with progress and a pace-based forecast. |
| Triage | `/w/{ws}/triage` | Everyone; the lead answers for it | Tickets and planned work awaiting a decision, filter by type, batch-queue a project's new issues, name the triage lead. |
| Trade-off | `/w/{ws}/tradeoff` | Manager with their own manager | Shows what a new priority displaces, and records the decision as comments on the affected issues. |
| Review | `/w/{ws}/review` | Planning, reporting upward, one-to-ones | What was finished over a period, for the team or one person. Facts and links only. |

Any item anywhere opens a side panel with a button to the ticket or issue.

**Toasts.** Every 30 seconds, and on "Check now", the browser fetches a small
pulse and compares it with the last one. New triage items, and changes to the
viewer's own hands, waiting items or next item, are announced in a toast. A
change made from the same browser resets the comparison silently, so nobody is
told about what they just did. Nothing is stored on the server for this, so a
change the viewer makes directly in GitHub is announced to them like any other,
and nothing is announced while the app is closed (`web/src/Toasts.tsx`).

## 4. How it is built

- **Backend:** Go 1.24, standard library HTTP server, one dependency
  (`gopkg.in/yaml.v3`). No database, no queue, no background workers.
- **Frontend:** React 19, TypeScript, Vite. Plain CSS with theme tokens as
  custom properties. No router, state or component libraries. Native HTML drag
  and drop.
- **Packaging:** the built web app is embedded in the Go binary. One container,
  one process.
- **Intended hosting:** a container platform with built-in sign-in in front of
  it (written with Azure Container Apps and Entra in mind).

### Code map

```
cmd/gemba/          the server: loads config, builds a store per workspace, serves
cmd/seedgithub/     fills a GitHub test project with fictional issues
internal/config/    config types, defaults, validation; default.yml is the built-in demo config
internal/store/     the contract with wherever items live
  store.go            Item, Change, Store, and the optional Rota and Prioritizer interfaces
  github.go           GitHub Projects v2 over GraphQL, with a short read cache
  demo.go             in-memory fictional data; resets on restart
internal/flow/      every rule, as pure functions over []Item (no I/O)
  flow.go             stage, queue, quick list, lane, inbox and fast lane
  types.go            planned/unplanned typing, mix, norms for aging, projects, one person's view
  guide.go            likely start, dates at risk, reasons, pull suggestions, the attention list
internal/actions/   the writes: triage, move, trade-off, wait, resume
internal/api/       HTTP routes, access control, view models
  views.go            view models: the web app receives ready-to-render data
  review.go           the look-back summary
web/src/            the React app; pages/ has one file per page
web/e2e/            browser tests
e2e/                tests against a real GitHub project
examples/github.yml annotated example of a GitHub workspace config
```

### Design rules in the code

- **Rules live in `internal/flow` and nowhere else.** The API builds view
  models from them. The web app applies no flow rules; if a page needs a
  derived value or a sentence, add it to the view model.
- **A store is small on purpose.** `Items`, `Update` with a `Change` of
  nullable fields, and `Comment`. Optional abilities are separate interfaces
  (`Rota` for the triage lead, `Prioritizer` for the priorities text) found by
  type assertion. A new backing system means a new store, not changes to flow.
- **Writes go through `internal/actions`**, which validates against current
  state and returns an `actions.Error` the user can act on. `Conflict: true`
  means the data moved underneath them; the API returns 409 and the page
  reloads.
- **Prose in the interface is generated on the server** so every client says
  the same thing.

## 5. Field ownership

This is the contract that keeps Gemba, the engineers and the ticket sync from
overwriting each other.

| Field | Owner | Gemba |
|---|---|---|
| Title, body, labels, open/closed | Engineers or the sync | reads |
| Assignee | Engineers or the sync | reads |
| Status | Engineers or the sync | reads; writes only in the cases below |
| Blocked | Engineers | reads |
| Source priority, Source URL | The sync | reads |
| Project, Project URL, parent issue | Engineers or automation | reads |
| Cell | Gemba (triage), or pre-set by automation | reads and writes |
| Work type | Gemba (triage), or pre-set by automation or labels | reads and writes |
| Class of service | Gemba | writes |
| Rank | Gemba | writes |
| Due | Engineers; Gemba at triage of fixed-date work that has none | reads; writes once |
| Waiting on | Gemba, for non-mirrored items | writes |
| Comments | Gemba adds one per trade-off decision | appends |
| Triage issue assignee | Gemba | writes |

**Status writes, never on a mirrored item:**
- back to the open status when a trade-off pauses work in progress;
- to the waiting status when the holder sets work aside, and back to the
  in-progress status when they take it back.

For mirrored items the same changes are made in the ticketing system and
arrive through the sync.

## 6. What GitHub must provide

One repository holds the issues. One Projects (v2) project, owned by the same
user or organisation, holds them as items. Gemba finds fields **by name,
case-insensitively**; every name below is a default that config can change.

### Project fields

| Field (default name) | Type | Options or content | Required |
|---|---|---|---|
| Status | Single select | `Todo`, `In Progress`, `Waiting`, `Done` | Yes. `Waiting` only if the waiting feature is used. |
| Cell | Single select | One option per cell, named as the cell's `name` or `id` in config | Yes |
| Class of service | Single select | `Quick`, `Standard`, `Fixed date`, `Background`, `Expedite` | Yes |
| Rank | Number | Written by Gemba | Yes |
| Work type | Single select | One option per kind, named as the kind's `name` or `id` | For typed planned work |
| Blocked | Text | The reason; empty means not blocked | For blocked items |
| Waiting on | Text | Who or what; written by Gemba | Optional; without it the status still changes |
| Due | Date | | For fixed-date work |
| Project | Text or single select | The project's name | For project roll-ups |
| Project URL | Text | Link to where the project is described | Optional |
| Source priority | Single select or text | The ticket's priority at the source | For mirrored tickets |
| Source URL | Text | Link to the ticket | For mirrored tickets |

Notes:
- Class of service options are matched by lower-casing and replacing spaces
  with hyphens, so `Fixed date` is `fixed-date`.
- Several status names can map to one state (`in_progress_statuses`,
  `waiting_statuses`, `done_statuses`). Anything unrecognised is open.
- A closed issue is done whatever the Status field says.
- When Gemba writes a status it uses the first name in each list.
- A missing field or option makes that one write fail with a clear error; reads
  carry on.

### Labels

| Label (default) | Meaning |
|---|---|
| `mirrored` | The issue is a copy of a ticket. This alone makes it unplanned. |
| `enterprise-project`, `department-project`, `maintenance` | Fallback for work type when the Work type field is empty. Set per kind in config. |

Gemba reads labels and never changes them.

### Two ordinary issues (optional)

| Config key | What it is |
|---|---|
| `priorities_issue` | An issue whose body is the team's current priorities, one per line. Edited in GitHub. |
| `triage_issue` | An issue whose assignee is this week's triage lead. Gemba reassigns it on hand-over. |

Neither needs to be on the project.

### Projects and sub-issues

An item belongs to a project if its Project field is set, or, failing that, if
it is a sub-issue: the parent issue's title and link are used. Gemba does not
connect to any other planning tool. If projects live elsewhere, automation must
put the name and link on the GitHub issue.

### What the ticket sync must do

Gemba does not talk to the ticketing system. An external automation mirrors
tickets into GitHub, and it must:

1. Create an issue per ticket assigned to the team, with the `mirrored` label,
   and add it to the project.
2. Set Source URL to the ticket and Source priority to its priority.
3. Keep Status in step with the ticket: open, in progress, pending or waiting
   (to `Waiting`), and resolved (to `Done`, or close the issue).
4. Keep the assignee in step, mapped to a GitHub login.
5. Optionally set Cell, if the ticket's group makes it obvious.
6. Never write Class of service, Rank, Work type or Waiting on.

### Access

The prototype uses one token (`GITHUB_TOKEN`) with read and write access to
the repository's issues and to the project. Comments and changes are therefore
attributed to the token's owner. See section 11 for the planned replacement.

## 7. The rules, in brief

All in `internal/flow`. Thresholds are policies in config (defaults shown).

- **Stage:** done if closed or in a done status; doing if in progress; waiting
  if in a waiting status; queued if it has a cell and a class of service;
  otherwise intake.
- **Triage inbox:** intake items, except mirrored tickets younger than
  `triage_after_hours` (8) unless their source priority is in
  `urgent_priorities` (high, urgent). Those young tickets are the fast lane.
- **Queue order:** expedite first, then rank, then age. Quick work is not in
  the queue.
- **Pace:** triaged items finished per week over `throughput_weeks` (4), per
  cell. Fast-lane tickets and quick work are counted separately and do not set
  the pace.
- **Likely start:** free hands take the top of the queue now; each later item
  waits `(position - free hands + 1) / pace` weeks. Unknown when there is no
  measured pace.
- **Date at risk:** due date minus how long work of its type usually takes
  gives a start-by date; an item whose likely start is later is flagged.
- **Usual duration:** the 85th percentile of start-to-finish for that type over
  13 weeks, when at least 8 items were finished; otherwise `aging_days` (5).
- **Aging:** in progress longer than usual for its type.
- **Quiet:** in progress, not blocked, nothing changed for `quiet_days` (3).
- **Capacity line:** `pace x horizon_weeks (2)`, less work already in progress.
- **Work-in-progress limit:** per person (`wip_limit`, default 2); a cell's
  limit is the sum.
- **Waiting:** does not hold a hand. More than `waiting_limit` (2) per person,
  or longer than `waiting_days` (3), raises a warning.
- **Quick:** waiting longer than `quick_days` (2) raises a warning.
- **Attention list, in order:** expedited and not started; dates at risk;
  blocked (longest first); cell over its limit; waiting too long or too many;
  quick and stale; aging; quiet; unusual unplanned arrivals.

## 8. Configuration

One YAML file, path in `GEMBA_CONFIG`. With none, the built-in
`internal/config/default.yml` gives two demo workspaces. `examples/github.yml`
is an annotated GitHub workspace.

```yaml
brand: { name: Gemba, logo: path/to/logo.svg }   # logo optional (.svg or .png)
theme:                                           # optional; hex colours
  light: { paper, surface, ink, muted, rule, wood, wood-ink, accent, accent-ink, ok, warn, stop }
  dark:  { ...same keys... }
workspaces:
  - slug: infra                # in URLs; lower case, digits, hyphens
    name: Infrastructure
    store: github              # or demo
    github: { owner, repo, project, priorities_issue, triage_issue, ...field names }
    cells:  [{ id, name }]
    kinds:  [{ id, name, label, project: true|false, icon }]
    people: [{ login, name, cell, wip_limit, signin }]
    managers: [sign-in names]  # may do everything
    members:  [sign-in names]  # may read; may triage if also listed in people with a signin
    priorities: |              # used when there is no priorities_issue
      One line per priority.
    policies: { default_wip_limit, triage_after_hours, urgent_priorities,
                aging_days, quiet_days, quick_days, waiting_days, waiting_limit,
                throughput_weeks, horizon_weeks }
```

- `people[].login` is the GitHub login, used to match assignees.
- `people[].signin` is the name the sign-in platform reports, used to open the
  Now page on the right person and to decide who may triage.
- Icons available for kinds: `tower`, `house`, `wrench`, `flag`, `shield`,
  `dot`. Unplanned always uses `bolt`. Add more in `web/src/TypeIcon.tsx`.
- `*.local.yml` is ignored by git, for local configs that name real projects.

### Environment

| Variable | Purpose |
|---|---|
| `GEMBA_CONFIG` | Path to the config file. Empty means the built-in demo. |
| `GITHUB_TOKEN` | Token for GitHub workspaces. |
| `GEMBA_ADDR` | Listen address. Default `127.0.0.1:8080`; the container sets `0.0.0.0:8080`. |
| `GEMBA_REQUIRE_AUTH` | `1` refuses any request without a signed-in identity. The container sets it. |

## 9. Sign-in and permissions

Gemba does not sign anyone in. It trusts the identity header set by the
platform in front of it: `X-MS-CLIENT-PRINCIPAL-NAME`. The platform must strip
any copy of that header sent by a client. With `GEMBA_REQUIRE_AUTH=1` a request
without it gets 401. Without that setting (local use only) the caller is
"local" and may do everything.

| Role | Who | May |
|---|---|---|
| Reader | In `members` | See everything in the workspace |
| Team | A reader who is also in `people` with a matching `signin` | Triage to the back of a queue or the quick list; park and resume their own work; "Check now" |
| Triage lead | The team member who holds the triage issue | Also expedite and place at the front |
| Manager | In `managers` | Everything, including reorder, trade-off and naming the lead |

- A workspace the caller cannot read answers 404, so its existence is not
  revealed.
- Every write needs an `X-Gemba` header, which a cross-site form cannot send.
- Every write is logged as an `audit` line with the actor.

## 10. API

All JSON. View models are shaped for the pages; see `web/src/api.ts` for types.

| Method and path | Needs | Purpose |
|---|---|---|
| `GET /healthz` | nothing | Liveness |
| `GET /api/session` | sign-in | Who you are, theme, workspaces and what you may do in each |
| `GET /api/w/{ws}/floor` | reader | Attention list and a lane per cell |
| `GET /api/w/{ws}/mine?as={login}` | reader | One person's page |
| `GET /api/w/{ws}/inbox` | reader | Triage rows, fresh tickets, cell outlooks, the lead |
| `POST /api/w/{ws}/inbox/{id}` | team | Triage: `{cell, cos, kind, top, due}` |
| `POST /api/w/{ws}/triage-lead` | manager | `{login}` |
| `POST /api/w/{ws}/queues/{cell}/move` | manager | `{id, before}`; `before: null` means the end |
| `GET /api/w/{ws}/projects` | reader | Project roll-ups |
| `GET /api/w/{ws}/tradeoff?item=&cell=` | reader | Candidates and a preview of the cost |
| `POST /api/w/{ws}/tradeoff` | manager | `{item, cell, pause, authorizedBy}` |
| `POST /api/w/{ws}/items/{id}/wait` | holder or manager | `{on}` |
| `POST /api/w/{ws}/items/{id}/resume` | holder or manager | `{}` |
| `GET /api/w/{ws}/pulse?as={login}` | reader | The little a browser needs to notice change: triage ids, and one person's hands, waiting items and next item |
| `POST /api/w/{ws}/refresh` | team | Forget cached items so the next read goes to GitHub |
| `GET /api/w/{ws}/review?days=&as=` | reader | Look-back for the team or one person; 7 to 731 days |

## 11. Build, run, test

```sh
npm --prefix web ci && npm --prefix web run build   # builds into web/dist, which Go embeds
go build -o bin/gemba ./cmd/gemba
./bin/gemba                                          # built-in demo at http://127.0.0.1:8080
GEMBA_CONFIG=my.local.yml GITHUB_TOKEN=... ./bin/gemba

npm --prefix web run dev                             # web app with hot reload, proxying /api to :8080
docker build -t gemba . && docker run -p 8080:8080 -e GEMBA_REQUIRE_AUTH=0 gemba
```

The web app must be built before the Go binary, or the binary serves a "not
built" message.

### Tests

| Layer | Command | Covers |
|---|---|---|
| Unit | `go test ./...` | Flow rules, actions, access control, workspace isolation |
| Real GitHub | `GEMBA_E2E=1 GEMBA_CONFIG=/abs/path.yml GITHUB_TOKEN=... go test ./e2e -v` | Every write, read back from GitHub by an independent query. Creates its own issues, removes them from the project and closes them. Takes about two minutes. |
| Browser | `npm --prefix web run e2e` | Clicks, drags and typing through the built app on the demo floor, in Edge or Chrome already installed (`BROWSER=chrome`) |

The GitHub test needs an absolute config path, a workspace named `testfloor`
(or `GEMBA_E2E_WS`), one person whose login is a real GitHub user, a
`triage_issue`, and every field in section 6. Point it only at a project you
can afford to change.

Keep all three passing. Add a case to the GitHub test for any new write, and to
the browser test for any new interaction.

## 12. What is proven and what is not

**Proven:** the rules (unit tests); every write reaching GitHub (triage with
type and date, quick, reorder, waiting and resume, trade-off with pause and
comment, triage lead); the main interactions in a browser, including dragging.

**Not proven:**
- Behaviour with hundreds or thousands of real items. Every read loads the
  whole project.
- The platform sign-in in a real deployment, and the header being stripped.
- Any real ticket sync.
- Dark mode and narrow screens beyond a glance.
- The "changed since you last looked" markers on the Now page.

## 13. Known gaps, in suggested order

1. **GitHub App and webhooks** in place of one personal token and polling.
   Gives per-installation access, proper attribution, and updates in seconds
   without re-reading the project. The store interface should not need to
   change.
2. **Scale.** `github.go` pages through the whole project on each cache miss
   (20-second cache). Past a few hundred open items this wants incremental
   updates from webhooks, and done items trimmed to a window.
3. **History.** GitHub holds only the current state, so Gemba cannot report
   time spent blocked or waiting, past triage leads, or trends beyond what is
   still on the project. Archiving project items removes them from Review. A
   small store of daily snapshots would fix this; it is the first thing that
   would justify a database, and it must stay derived data that can be rebuilt.
4. **Sign-in groups.** Managers and members are lists of names in config.
   Reading group membership from the identity platform would remove that
   upkeep, as would mapping sign-in names to GitHub logins automatically.
5. **Infrastructure as code** for the intended hosting, and a pipeline that
   runs the three test layers.
6. **Accessibility pass.** Drag and drop has a keyboard alternative only on
   the Queues page.
7. **Things designed but not built:** a requester's "where is my request"
   view; trade-offs between whole projects; an intended mix per cell to compare
   with the actual; a weekly planning snapshot; skipping triage for planned
   issues that arrive with cell and work type already set.

## 14. Known issues

- Floor and Now can disagree about which of a person's items is "beyond the
  limit" when they hold too many; the two pages order the items differently.
- Dates are formatted in UTC, so "since" and "finished" can read a day ahead
  late in the day.
- For GitHub items, "started" is when the Status field last changed. An item
  that was set to waiting and taken back looks freshly started.
- The triage lead is "this week's" by convention only. Nothing rotates it; a
  warning shows after seven days.
- Nothing caps the quick list or checks that a quick item was in fact quick.
- Batch triage sends one request per item and stops at the first failure.
- `README.md` and `docs/PLAN.md` predate the triage lead, quick list, waiting
  state and Review page. Bring them up to date or point them here.

## 15. Adapting it to a real team

1. Create the project fields, options and labels in section 6. Adjust names in
   config instead of renaming an existing project's fields.
2. Write the workspace config: cells, people with GitHub logins and sign-in
   names, managers, members.
3. Extend the ticket sync to meet the contract in section 6.
4. Create the priorities issue and the triage issue.
5. Run the GitHub end-to-end test against a copy of the project first.
6. Rebrand through `brand` and `theme`; replace the logo if wanted (`NOTICE`
   credits the font the built-in mark is drawn from).
7. Deploy the container behind platform sign-in with `GEMBA_REQUIRE_AUTH=1`.

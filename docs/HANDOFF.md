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
| Triage lead | The one person answerable for triage this week. Holds delegated authority to expedite. Decided by the rota. |
| Rota | The triage schedule: a rotation taking a week each, weeks given to a named person, and a stand-in for the rest of the current week. |

## 3. Pages

| Page | Route | For | What it does |
|---|---|---|---|
| Now | `/w/{ws}/now` | Each engineer | Hands, waiting items, the one thing to pull next with its reason, what comes after, a prominent Quick work panel for the cell, the team's priorities. Park and resume your own work here. |
| Floor | `/w/{ws}/floor` | Standup | "Start standup" steps through the team one person at a time (`?standup=1&at=login`): in hand, in the way, finished since the last standup, waiting, next. Read-only, open to everyone. Otherwise:  Attention sentences, then a room per cell: a mat per person, the quick list, the line (drag to reorder), where finished work went. |
| Queues | `/w/{ws}/queues` | Planning | The full ranked queues, searchable, folded past the capacity line. |
| Projects | `/w/{ws}/projects` | Planning, reporting | Roll-up per project with progress and a pace-based forecast. |
| Triage | `/w/{ws}/triage` | Everyone; the lead answers for it | Tickets and planned work awaiting a decision, filter by type, batch-queue a project's new issues, name the triage lead. |
| Trade-off | `/w/{ws}/tradeoff` | Manager with their own manager | Search for the new priority; see how many items move back and by how much, which dates it puts at risk, and what stopping each piece of work in progress would cost; record the decision as comments on the affected issues. |
| Review | `/w/{ws}/review` | Planning, reporting upward, one-to-ones | What was finished over a period, for the team or one person. Facts and links only. |

Any item anywhere opens a side panel with a button to the ticket or issue.

**Toasts and notifications.** Every 30 seconds, and on "Check now", the
browser fetches a small pulse and compares it with the last one
(`web/src/Toasts.tsx`). It announces, in a toast:

- new triage items;
- for the viewer's own work: something new in hand, something gone, something
  set to waiting, an item no longer blocked, a date newly at risk, a waiting
  item due a chase, a different next item;
- for managers: a new serious item on the Floor's attention list.

Becoming blocked is not announced to the holder, who usually set it. Keep this
list short; the bar for adding to it is "worth an interruption".

A change made from the same browser resets the comparison silently, so nobody
is told about what they just did. Nothing is stored on the server for this, so
a change the viewer makes directly in GitHub is announced to them like any
other.

"Notify me" in the top bar also raises the same messages through the operating
system (the browser's Notification API) while the viewer is looking elsewhere.
The tab must stay open, and the site must be served over HTTPS (or localhost).
Notifying people when the app is closed needs a server-side watcher and a push
channel; that is not built.

**The triage rota.** Who leads triage is worked out each time it is asked, in
`internal/flow/rota.go`; nothing rolls over on a timer. In order:

1. a stand-in: whoever was assigned the triage issue during the current week
   (the "Stand-in" button, or assigning the issue in GitHub);
2. a named week: a week the manager gave to one person, for example to cover
   leave;
3. the rotation: an ordered list of people taking a week each, Monday to
   Sunday (UTC).

The rotation and named weeks are kept as a small YAML block in the body of the
triage issue, written by Gemba and editable by hand. The time of a stand-in is
the issue's last "assigned" event. With no schedule, the lead is simply the
issue's assignee, as before. Managers edit the rota on the Triage page;
everyone can see the next eight weeks.

**Chat.** `internal/notify` is a stub for chat integration. With
`notify.teams_webhook` set on a workspace (use `env:NAME` to keep the address
out of the file), the server posts a card to a Microsoft Teams incoming webhook
when a trade-off is recorded, when work is expedited at triage, and when the
triage lead changes. The sender is tested against a local server only, never
against Teams. Per-person chat messages would need the same server-side watcher.

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
  settings.go         the team's settings file, and Resolve, which puts the three sources together
internal/notify/    chat announcements (a stub; see section 3)
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
examples/github.yml annotated example of a GitHub workspace in the deployment config
examples/gemba.yml  annotated example of the team's settings file
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
| Triage issue assignee and body | Gemba (the stand-in and the rota) | writes |

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
- The options of Cell and Work type **are** the team's cells and kinds of
  work; adding an option adds a cell or a kind (section 8).
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

### The settings file (optional)

`.gemba.yml` at the root of the repository's default branch. See section 8 and
`examples/gemba.yml`.

### Two ordinary issues (optional)

| Config key | What it is |
|---|---|
| `priorities_issue` | An issue whose body is the team's current priorities, one per line. Edited in GitHub. |
| `triage_issue` | Holds the triage rota. Its body carries the schedule (rotation and named weeks); its assignee is the stand-in for the current week. Gemba writes both. |

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

## 8. Where a workspace is described

There is no settings page and no database. A workspace is described in three
places, each owned by someone different. `config.Workspace.Resolve` in
`internal/config/settings.go` puts them together, and the GitHub store does so
on every read, so a change in GitHub shows within the cache time (20 seconds).

| Source | Owner | Says |
|---|---|---|
| **GitHub itself** | The team, by using GitHub | Cells: the options of the project's Cell field. Kinds of work: the options of the Work type field. People: the members of the GitHub teams the settings file names. |
| **Settings file** `.gemba.yml` in the work-items repository | The team's manager, editing in GitHub | Which team works in which cell; per-person limits and sign-in names; people not in a team; icons for kinds; limits and thresholds. |
| **Deployment config** (`GEMBA_CONFIG`) | Whoever deploys Gemba | Which repository and project; who may get in and who is a manager; brand and colours; field names if they differ from the defaults. |

Rules:

- **Everything in the settings file is optional.** With no file, cells and
  kinds still come from the project and the floor loads; only people are
  missing.
- **The settings file cannot grant access.** `managers` and `members` exist
  only in the deployment config, and an unknown key in the settings file is an
  error. This is deliberate: anyone who can commit to the repository can edit
  the file. Do not move access into it. If the file should be change-controlled,
  protect it with a CODEOWNERS rule in that repository.
- **A mistake never takes the floor down.** A file that does not parse is
  ignored whole. A reference that cannot be honoured (a team that does not
  exist, a person in a cell that does not exist) is skipped. Either way the
  problem is listed in `Workspace.Problems` and shown to managers at the top of
  the Floor as "Setup: ...".
- **Cells and kinds listed in the deployment config are pinned** and override
  the project. That is how the demo works, and it is an escape hatch; a real
  workspace should leave them out.
- **Ids are made from names.** A cell option "Data Services" has id
  `data-services`; a kind "Enterprise project" has id `enterprise-project`.
  Renaming an option in GitHub therefore changes the id, which only appears in
  URLs and API calls.
- **Icons for kinds are guessed from the name** (enterprise, department,
  maintenance) and can be set in the settings file.

### People and GitHub Teams

The intended setup is one GitHub team per cell, named in the settings file:

```yaml
teams:
  Network: infra-network
  Platform: infra-platform
```

Members come from `organization.team.members` (first 100, including child
teams' members as GitHub reports them). Names come from their profiles, falling
back to the login. The token needs `read:org`, and must be able to see the
teams (secret teams are invisible to non-members).

**This path has not been run against a real organisation.** The development
account is a personal one, which has no teams. It is covered by
`internal/store/github_test.go` against a stand-in for GitHub that answers the
same queries. The first job when adopting this is to point it at the real
organisation and confirm the people appear; if a team cannot be read, the Floor
will say so. Until teams are set up, list people under `people:` with a `cell`,
which is proven end to end.

### The deployment config

One YAML file, path in `GEMBA_CONFIG`. With none, the built-in
`internal/config/default.yml` gives two demo workspaces. A real workspace needs
only this:

```yaml
brand: { name: Gemba, logo: path/to/logo.svg }   # logo optional (.svg or .png)
theme:                                           # optional; hex colours
  light: { paper, surface, ink, muted, rule, wood, wood-ink, accent, accent-ink, ok, warn, stop }
  dark:  { ...same keys... }
workspaces:
  - slug: infra                # in URLs; lower case, digits, hyphens
    name: Infrastructure
    store: github
    github:
      owner: your-org
      repo: work-items
      project: 1
      priorities_issue: 12     # optional
      triage_issue: 13         # optional
      settings_path: .gemba.yml  # the default
      # ...field names, only where they differ from the defaults in section 6
    managers: [sign-in names]  # may do everything
    members:  [sign-in names]  # may read; may triage if the settings file gives them a signin
    notify: { teams_webhook: "env:TEAMS_WEBHOOK" }   # optional; announces decisions to chat
    policies: { ... }          # optional deployment-wide defaults; the settings file overrides them
```

`cells`, `kinds`, `people` and `priorities` may also be given here. The demo
store needs them, since it has no GitHub to ask.

- A person's `login` is their GitHub login, used to match assignees.
- A person's `signin` is the name the sign-in platform reports. It opens the
  Now page on the right person and decides who may triage. It does not make
  anyone a manager.
- Icons available for kinds: `tower`, `house`, `wrench`, `flag`, `shield`,
  `dot`. Unplanned always uses `bolt`. Add more in `web/src/TypeIcon.tsx`.
- `*.local.yml` is ignored by git, for local configs that name real projects.

### Environment

| Variable | Purpose |
|---|---|
| `GEMBA_CONFIG` | Path to the deployment config. Empty means the built-in demo. |
| `GITHUB_TOKEN` | Token for GitHub workspaces. Needs the repository's issues and contents (to read the settings file), the project, and `read:org` for teams. |
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
| Team | A reader whom the workspace's people list recognises by `signin` | Triage to the back of a queue or the quick list; park and resume their own work; "Check now" |
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
| `POST /api/w/{ws}/triage-lead` | manager | `{login}`: a stand-in for the rest of this week |
| `POST /api/w/{ws}/triage-rota` | manager | `{rotation: [logins], weeks: {"2026-12-07": login}}`; either part optional, an empty login returns a week to the rotation |
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

**Proven:** the rules (unit tests); cells and kinds read from the project's
fields and people from the settings file, on a real project; every write reaching GitHub (triage with
type and date, quick, reorder, waiting and resume, trade-off with pause and
comment, triage lead); the main interactions in a browser, including dragging.

**Not proven:**
- People read from GitHub Teams, against a real organisation (section 8).
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
4. **Access from the identity platform.** Managers and members are still
   lists of sign-in names in the deployment config, and each person's sign-in
   name is mapped to their GitHub login by hand in the settings file. Reading
   group membership from the identity platform would remove the first; an
   organisation whose GitHub accounts are linked to that platform could supply
   the second.
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
- Rota weeks run Monday to Sunday in UTC, not the team's own time zone.
- Nothing caps the quick list or checks that a quick item was in fact quick.
- Batch triage sends one request per item and stops at the first failure.
- Reading the settings file and each team costs one extra GitHub query per
  cache refresh. Fold this into the webhook work (gap 1).
- `README.md` and `docs/PLAN.md` predate the triage lead, quick list, waiting
  state and Review page. Bring them up to date or point them here.

## 15. Adapting it to a real team

1. Create the project fields, options and labels in section 6. The options of
   Cell and Work type become the cells and kinds. Adjust field names in the
   deployment config instead of renaming an existing project's fields.
2. Write a deployment config with the repository, project, managers and
   members. Add `.gemba.yml` to the repository from `examples/gemba.yml`: a
   team per cell, and each person's sign-in name. Open the Floor as a manager
   and clear anything listed under "Setup".
3. Extend the ticket sync to meet the contract in section 6.
4. Create the priorities issue and the triage issue.
5. Run the GitHub end-to-end test against a copy of the project first.
6. Rebrand through `brand` and `theme`; replace the logo if wanted (`NOTICE`
   credits the font the built-in mark is drawn from).
7. Deploy the container behind platform sign-in with `GEMBA_REQUIRE_AUTH=1`.

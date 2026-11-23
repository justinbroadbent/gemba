# Gemba: product and build plan

Status: draft for review. Nothing below is built yet.

## 1. What this is

Gemba is a control panel for a manager running an operations team. GitHub Issues and
Projects remain the system of record for all work. Gemba reads that work, shows it the
way a lean factory floor shows work, and writes back a small number of fields.

It exists because GitHub has no concept of:

- a ranked queue per discipline, with a line showing what realistically gets done
- work-in-progress limits per person
- a triage step that keeps trivial tickets away from the manager
- a recorded answer to "this is now the priority, so what gets displaced?"

It is not a ticketing system, a time tracker, an estimation tool, or a second place for
engineers to update their work.

### Lean ideas the product is built on

| Idea | What it means in Gemba |
|---|---|
| Pull, not push | The manager orders the queue. Engineers pull the top item when they have room. Nobody is assigned tasks by the app. |
| Limit work in progress | Each person has a small limit. Starting something new while over the limit is visible to everyone. |
| Manage the constraint | The cell with the least capacity sets the pace. Its queue is the one to protect. |
| Make problems visible | Blocked and aging work is loud. Everything else is quiet. |
| Measure flow, not people | Throughput and wait time per cell. Never output per person. |
| Go and see (gemba) | One board that shows the real state of the floor, used at standup. |

## 2. Personas

Each persona lists what they need, what would make them reject the tool, and the
requirements that follow. Requirement IDs are referenced in later sections.

### 2.1 People who use it

**Morgan, team manager (primary user).** Runs a ten-person infrastructure team across
three disciplines. Takes work in through a ticketing system and plans in GitHub.

- Needs: to see capacity at a glance, set priorities without assigning tasks, clear
  blockers, and walk into the director's office with evidence.
- Rejects it if: it needs daily upkeep, shows numbers that do not lead to a decision, or
  disagrees with GitHub.
- Requirements:
  - R1. A board that answers "what is in flight, what is stuck, who has room" in one screen.
  - R2. A triage inbox that only contains work needing a decision.
  - R3. A ranked queue per cell, reordered by drag.
  - R4. A capacity line per cell, from measured throughput.
  - R5. A trade-off action that shows what a new priority displaces and records the decision.

**Dana, director (Morgan's manager).** Brings new priorities. Makes the call when
priorities collide. Opens the tool a few times a month.

- Needs: to see the cost of a new priority before asking for it, and a short trend view.
- Rejects it if: it needs training, or shows operational detail instead of outcomes.
- Requirements:
  - R6. The trade-off screen is readable by someone who has never seen the app.
  - R7. Decisions record who authorized them, on the issues themselves.
  - R8. (Later) A one-page flow summary: planned against unplanned, wait times, displaced work.

**Sam, triage delegate.** A senior engineer who covers triage and queue ordering when
Morgan is out.

- Needs: the same triage and reorder powers, limited to their team.
- Requirements:
  - R9. A manager role that can be granted to several people per workspace.
  - R10. Every write is attributed to the person who made it.

**Priya, database engineer.** One of four. Work is a mix of interrupts (access requests,
slow queries) and long jobs (migrations, upgrades) that wait on change windows.

- Needs: to know what to pull next without asking, and protection from being loaded with
  five things at once.
- Rejects it if: it is one more place to update status, or it looks like surveillance.
- Requirements:
  - R11. Engineers never enter data in Gemba. Their updates happen where they already work.
  - R12. No per-person performance numbers anywhere, including history. Per-person views
    show current load against the limit and nothing else.
  - R13. "Waiting for a change window" is a first-class blocked reason, distinct from being stuck.

**Rin, network engineer.** The only one. Every network request lands on Rin, often by
direct message instead of through the queue.

- Needs: a visible queue so sideways requests can be redirected to it, and a manager who
  can see the pile building.
- Rejects it if: the tool frames a one-person cell as an underperforming person.
- Requirements:
  - R14. Constraint reporting is at cell level and framed as wait time for the requester,
    not speed of the engineer.
  - R15. A one-person cell works without special handling.

**Alex, cloud and platform engineer.** Mostly project and automation work, lives in
GitHub, rarely opens the ticketing system. Improvement work is always the first thing dropped.

- Needs: improvement work to have a protected place in the queue, and zero extra steps.
- Requirements:
  - R16. A "background" class of service so improvement work is visibly queued and visibly displaced.
  - R17. Items that originate in GitHub and items mirrored from a ticketing system appear
    side by side in the same queue.

**Jo, systems administrator.** Works almost entirely from tickets. May never open GitHub.

- Needs: to keep working tickets in the ticketing system and nowhere else.
- Requirements:
  - R18. Mirrored items are fully usable in Gemba without the engineer touching GitHub.
    Their status, assignee and blocked state come from the sync, and Gemba never overwrites them.

**Lee, peer manager (second tenant).** Runs a different team and wants the same board.

- Needs: their own cells, people and limits, with no view into Morgan's team.
- Rejects it if: onboarding needs a developer.
- Requirements:
  - R19. Workspaces are isolated. A user sees only workspaces they belong to.
  - R20. A new workspace is set up through configuration, not code.

### 2.2 People who build and run it

**Software engineer.** Will adapt the code to an environment the original author cannot see.

- Needs: clear seams. The flow rules must be testable with no network, and everything
  that touches GitHub, Entra or Postgres must sit behind an interface.
- Requirements:
  - E1. A pure flow engine with no I/O.
  - E2. A store interface with two implementations: in-memory demo and GitHub.
  - E3. A demo mode that runs the whole product with fictional data and no credentials.
  - E4. One binary, one container. The React build is embedded in the Go binary.

**QA engineer.** Has to prove it is correct without access to the real organisation.

- Needs: deterministic tests, and a list of the ways this can be quietly wrong.
- Requirements:
  - Q1. The clock is injected everywhere. No test depends on today's date.
  - Q2. One contract test suite that every store implementation must pass.
  - Q3. Automated tests that a user of workspace A cannot read or write workspace B.
  - Q4. End-to-end browser tests run against demo mode.

**Operator.** Deploys it to Azure and keeps it running.

- Needs: no secrets in the image, health checks, and a small list of settings.
- Requirements:
  - O1. Configuration through environment variables; secrets from Key Vault.
  - O2. Health and readiness endpoints; structured logs including an audit stream.
  - O3. Infrastructure defined as code in the repo.

### 2.3 Where personas pull against each other

| Tension | Resolution |
|---|---|
| Morgan wants per-person load. Priya fears surveillance. | Per-person views show current work against the limit only. All rates and trends are per cell (R12). |
| Dana wants reports. Morgan wants no noise. | One summary page, added after the core is in use, limited to numbers that change a decision (R8). |
| Rin is the constraint. A dashboard can read as blame. | Language and metrics describe the queue and the wait, not the person (R14). |
| Lee wants self-service. The MVP should stay small. | Tenancy is in the data model from the first commit. Onboarding is a config file plus one admin step; a settings screen comes later (R20). |
| Engineers must not do double entry. Gemba needs status. | Gemba never asks for status. It reads it from GitHub, where the engineer or the sync already put it (R11, R18). |

## 3. Domain model

**Workspace.** One team's view: a GitHub repository and project, a set of cells and
people, and policies. The unit of tenancy.

**Cell.** A discipline with its own queue, for example network, platform, data services.

**Person.** A GitHub login, a display name, a home cell and a WIP limit.

**Item.** A GitHub issue. Fields, and who owns each:

| Field | Owner | Notes |
|---|---|---|
| Title, body, comments | GitHub | |
| Status: open, in progress, done | Engineer or sync | Gemba reads only |
| Assignee | Engineer or sync | Gemba reads only |
| Blocked flag, reason, since | Engineer or sync | Gemba reads only |
| Mirrored marker, source priority, due date | Sync | Set when a ticket is copied in |
| Cell | Gemba (sync may pre-fill) | |
| Class of service | Gemba | Setting it is what "triaged" means |
| Rank | Gemba | Position in the cell queue |

The single exception: when a trade-off pauses a non-mirrored item, Gemba sets it back to open.

**Class of service.**

- Expedite: goes to the front and may displace work in progress.
- Fixed date: has a due date that the queue order must respect.
- Standard: worked in queue order.
- Background: improvement work, deliberately queued so displacement is visible.

**Stage.** Derived, never stored:

- Intake: open, no class of service yet.
- Queued: open, has a cell and a class of service.
- Doing: in progress.
- Done: closed.

## 4. Flow rules

These are the rules the flow engine implements. Each threshold is a workspace policy.

1. **Fast lane.** A mirrored ticket in intake is left alone until it has been open
   longer than the triage threshold (default 8 hours). If it is resolved before then, it
   never needed triage and is counted as a quick ticket.
2. **Triage inbox.** Contains intake items that are: past the threshold; marked high or
   urgent at the source; or planned work that has not been queued.
3. **Queue order.** Expedite first, then by rank.
4. **Throughput.** Triaged items a cell finished per week, averaged over a window
   (default 4 weeks). Quick tickets are counted separately as a per-week rate.
5. **Capacity line.** Throughput times the horizon (default 2 weeks), minus items already
   in progress. That many queued items sit above the line.
6. **WIP.** A person is over their limit when their in-progress items exceed it. Blocked
   items still count.
7. **Aging.** In-progress work older than the aging threshold (default 5 days) is flagged.
8. **Since last standup.** The same time on the previous working day.

Known simplifications to revisit: thresholds use elapsed hours, not business hours; all
items count equally regardless of size.

## 5. Screens

1. **Board (看板).** The standup screen. One swim lane per cell, three columns: next up,
   in progress, done since last standup. Above the lanes, four counts: to triage,
   blocked, aging, over limit. Each lane header shows people with their load as dots
   against their limit.
2. **Triage (受付).** The inbox. Each row: why it surfaced, then cell, class of service
   and front or back of queue, set in one action.
3. **Queues (行列).** One ranked column per cell with the capacity line drawn in. Drag to reorder.
4. **Trade-off (決断).** Choose an item and a cell. See the queue, what would fall past the
   line, and what is in progress. Choose to pause something or not, name who authorized
   it, and confirm. The decision is written as comments on the affected issues.

### Design language

- Japanese minimalism: paper background, ink text, one strong accent, generous space.
- Flat panels with a firm ink outline, in the manner of manga panels. No gradients or decoration.
- Small kanji labels as accents beside English labels, never instead of them.
- Colour carries meaning only: the accent for expedite and brand, red for stopped, amber
  for aging. A calm board has almost no colour on it.
- Every number on screen must lead to a decision. If it does not, it is removed.
- All colours are design tokens set per deployment, so the app can be rebranded without
  touching code. Light and dark themes are both defined.
- System fonts only, so it works on locked-down corporate networks.

## 6. Architecture

```
Browser (React)
    |  same origin
Azure Container Apps: built-in Entra sign-in
    |
Go service  --  serves the React build and /api
    |-- flow engine (pure)
    |-- store interface --> GitHub adapter (GitHub App)  |  demo adapter
    |-- Postgres: workspaces, item cache, history, audit
```

### Backend (Go)

- One binary. Standard library HTTP router; `pgx` for Postgres; no web framework.
- The React production build is embedded with `embed` and served by the same binary.
- Packages:
  - `flow`: the rules in section 4. No I/O.
  - `store`: the interface, the demo implementation, the GitHub implementation.
  - `actions`: triage, reorder, trade-off. Validates, calls the store, writes the audit record.
  - `tenant`: workspace lookup and membership checks.
  - `api`: HTTP handlers and JSON types.
  - `db`: migrations and queries.

### Frontend (React)

- React, TypeScript, Vite.
- TanStack Query for server state; `dnd-kit` for drag and drop.
- Plain CSS with custom properties for the tokens. No component framework.
- The API returns view models that are ready to render. Flow rules live only in Go.

### GitHub integration

- A GitHub App installed on each workspace's repository, so access is per repository and
  not tied to a person.
- Reads through the GraphQL API for project items and fields.
- Webhooks keep the cache current; a periodic full reconcile catches anything missed.
- Writes are limited to the fields Gemba owns, plus comments for trade-off decisions.
- Writes appear in GitHub as the app. The comment text names the signed-in person.
- Rank is stored as a number field on the project, using fractional ranks so a reorder
  usually updates one item instead of the whole queue.
- Field and label names are mapped in the workspace config, so no naming convention is forced.

### Data (Postgres)

GitHub holds the work. Postgres holds:

- Workspaces and their GitHub installation details.
- A cache of items, rebuildable from GitHub at any time.
- Stage-change history, which GitHub does not expose in a convenient form and which
  throughput and wait-time figures depend on.
- An audit log of every write Gemba made, with the signed-in user.

### Multi-tenancy

- Every row carries a workspace ID. All queries go through one layer that requires it.
- URLs are workspace-scoped: `/w/{workspace}/board`.
- Membership comes from Entra groups: each workspace names a member group and a manager group.
- Workspace configuration (cells, people, limits, policies, field mapping) lives in a
  `.gemba.yml` file in that workspace's repository, so changes are reviewed and versioned.
- Registering a workspace is a deployment-admin action. A settings screen is out of the first release.
- Theme and branding are per deployment, not per workspace.

### Sign-in and authorisation

- Azure Container Apps built-in authentication handles Entra sign-in in front of the app.
- The Go service reads the identity and group claims the platform passes in, behind an
  interface, so local development and tests can supply a fake identity.
- The service refuses requests with no identity unless demo mode is explicitly on.
- Roles per workspace: member (read) and manager (triage, reorder, trade-off).

### Deployment

- Multi-stage Dockerfile: build the React app, build the Go binary, copy into a minimal image.
- Bicep templates for Container Apps, Postgres Flexible Server, Key Vault and the Entra
  authentication settings.
- GitHub Actions for tests on every pull request and image build on main.

## 7. Quality plan

| Layer | What is tested | How |
|---|---|---|
| Flow engine | Every rule in section 4, including edge cases: empty cell, one-person cell, zero throughput, weekend boundaries | Table-driven Go tests with a fixed clock |
| Store contract | Reads, the allowed writes, refusal of writes to fields Gemba does not own | One suite run against the demo store and the GitHub adapter (recorded responses) |
| Actions | Triage, reorder against a queue that changed underneath, trade-off with and without a pause, mirrored against non-mirrored | Go tests on the demo store |
| Tenancy | A member of A cannot read or write B through any endpoint | A test that walks every route with a foreign workspace |
| API | Status codes, read-only users, missing identity | Go HTTP tests |
| UI | The four screens, drag reorder, the trade-off flow, keyboard use | Playwright against demo mode |
| Accessibility | Contrast in both themes, focus order, a non-drag way to reorder | Automated checks plus a manual pass |

Risks QA should watch:

1. **Stale order.** Two managers reorder the same queue. The second write must be refused, not merged silently.
2. **Sync races.** The ticket sync and Gemba write the same issue at the same moment. Field ownership must hold.
3. **Missed webhooks.** The cache drifts from GitHub. The reconcile must correct it and the UI must show data age.
4. **Rate limits.** A large reorder or first import exhausts the GitHub quota.
5. **Tenant leakage.** Any query missing its workspace filter.
6. **Time.** Time zones, weekends and holidays in "since last standup" and in thresholds.
7. **Thin history.** A new workspace has no throughput yet. The capacity line must say so instead of showing zero.

## 8. Milestones

Each milestone ends with something that can be shown.

| | Milestone | Done when |
|---|---|---|
| M1 | Flow engine and demo store in Go | All section 4 rules pass their tests; a JSON API serves the four screens from fictional data |
| M2 | React UI on demo mode | All four screens work end to end in a browser with no credentials; themes switch from config |
| M3 | GitHub adapter, read-only | A real project renders on the board through a GitHub App, with field mapping from `.gemba.yml` |
| M4 | Writes and freshness | Triage, reorder and trade-off write to GitHub; webhooks and reconcile keep the cache current |
| M5 | Postgres, tenancy, Entra | Two workspaces run side by side with isolated data and group-based roles |
| M6 | Azure deployment | One command deploys the container, database and sign-in to a clean subscription |
| M7 | Flow summary | The one-page view for directors, built on the history collected since M4 |

M1 and M2 need nothing from any employer environment. M3 onward is where the app is adapted to a real one.

## 9. To verify before building the affected part

These are assumptions, not confirmed facts:

1. The GitHub GraphQL API exposes everything needed to read and set project number and
   single-select fields from a GitHub App, and the webhook events needed for project item changes. (M3, M4)
2. Container Apps built-in authentication passes Entra group claims to the app, and how
   it behaves for users in very many groups. (M5)
3. Whether per-person WIP should count items assigned to several people once or once each. (M1)
4. Whether thresholds need business hours and a holiday calendar. (M1, revisit after use)
5. Whether items need a size class before throughput is trustworthy. (after M4, from real data)

## 10. Out of scope for now

- Any connector to a ticketing system. Gemba only requires that mirrored issues carry a
  marker and that something else keeps their status current.
- Work that crosses cells as a routed sequence. Use sub-issues, one per cell.
- Estimation, time tracking, and any per-person reporting.
- A workspace settings screen and self-service onboarding.

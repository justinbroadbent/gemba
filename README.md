# Gemba

> Continuing development? Start with [docs/HANDOFF.md](docs/HANDOFF.md): what this is, how it is built, what GitHub must provide, and what is left to do.

A lean flow board for operations teams, layered on top of GitHub Issues and Projects.

GitHub stays the system of record, and people keep working items where they live: planned
work in GitHub, tickets in the ticketing system. Gemba is where everyone comes to see what
matters, then clicks through to do it.

- **Now.** One person's answer to "what should I be doing?": what is in their hands, the one
  thing to pull next and why, and what changed since they last looked.
- **Floor.** The team at a glance: who holds what, who has a free hand, what needs help, and
  each cell's queue as a line through time. It lists exceptions, so everything else can be left alone.
- **Queues.** Every queued item, ranked, with the capacity line. Built to hold hundreds.
- **Projects.** Progress and a pace-based forecast for each project, rolled up from its items.
- **Triage.** An inbox holding only the work that needs a decision.
- **Trade-off.** Pick a new priority, see what it displaces, and record who authorized it.

What makes it more than a board:

- Rank becomes time: every queued item shows when it is likely to start, from the measured pace.
- Conflicts are caught early: dated work that will miss its date at its current position is flagged.
- Work is typed without tagging: anything from the ticketing system or expedited is unplanned;
  the rest is enterprise project, department project or maintenance.
- "Aging" is fair: an item is compared with how long work of its own type usually takes.
- Engineers enter nothing. Gemba reads status from GitHub and writes back only the cell, kind,
  class of service, rank, and decision comments.

**Status: working prototype.** See [docs/PLAN.md](docs/PLAN.md) for the full plan and what is still to do.

## Try it

You need Go 1.24+ and Node 20+.

```sh
npm --prefix web ci
npm --prefix web run build
go run ./cmd/gemba
```

Open http://127.0.0.1:8080. With no configuration it runs two demo workspaces on fictional
data, held in memory and reset on restart.

Or with Docker:

```sh
docker build -t gemba .
docker run --rm -p 8080:8080 -e GEMBA_REQUIRE_AUTH=0 gemba
```

## Connect a real GitHub project

1. Copy [examples/github.yml](examples/github.yml) and fill in the owner, repository and project number.
2. Add the fields listed at the top of that file to the project.
3. Run with a token that has the `repo` and `project` scopes:

```sh
GEMBA_CONFIG=my-config.yml GITHUB_TOKEN=... go run ./cmd/gemba
```

To fill an empty test project with fictional issues:

```sh
GEMBA_CONFIG=my-config.yml GITHUB_TOKEN=... go run ./cmd/seedgithub <workspace-slug>
```

## Configuration

Everything environment-specific is in one YAML file. The built-in default,
[internal/config/default.yml](internal/config/default.yml), documents every setting.

| Section | What it controls |
|---|---|
| `brand` | The name, and optionally your own logo in place of the built-in mark |
| `theme` | Every colour, for light and dark. Change these to rebrand |
| `workspaces` | One per team: cells, people and WIP limits, policies, who may manage it, and where its items live |

Environment variables:

| Variable | Meaning |
|---|---|
| `GEMBA_CONFIG` | Path to the configuration file. Default: the built-in demo |
| `GEMBA_ADDR` | Listen address. Default `127.0.0.1:8080` |
| `GEMBA_REQUIRE_AUTH` | Set to `1` to refuse requests with no signed-in identity |
| `GITHUB_TOKEN` | Token for workspaces with `store: github` |

## Sign-in and access

Gemba does not handle sign-in itself. It expects a platform in front of it to authenticate the
user and pass the sign-in name in the `X-MS-CLIENT-PRINCIPAL-NAME` header, which is what Azure
Container Apps built-in authentication does with Microsoft Entra ID.

- A user sees only the workspaces that list them under `managers` or `members`.
- Managers can triage, reorder and record trade-offs. Members are read-only.
- With `GEMBA_REQUIRE_AUTH=1`, a request with no identity is refused. Without it, the app runs
  in local mode, where everyone is a manager of everything. Never deploy in local mode.
- Do not expose the container directly: the header is only trustworthy behind the platform.

## Development

```sh
go test ./...                 # flow rules, actions, API and workspace isolation
go run ./cmd/gemba            # API on :8080
npm --prefix web run dev      # web app with hot reload, forwarding /api to :8080
```

Layout:

```
cmd/gemba          the server
cmd/seedgithub     fills a test project with fictional issues
internal/flow      the flow rules; pure, no I/O
internal/actions   triage, queue order, trade-offs
internal/store     the store contract, the demo store, the GitHub store
internal/api       HTTP handlers and view models
internal/config    configuration loading and the documented default
web                the React app
```

## License

MIT

## Proving it works

Three layers, each answering a different question.

| Question | How | Command |
|---|---|---|
| Are the rules right? | Unit tests on the flow rules, the actions and the API's access control | `go test ./...` |
| Does it really write to GitHub? | Drives every write through the API, then reads the result back from GitHub with an independent query. Creates and removes its own issues. | `GEMBA_E2E=1 GEMBA_CONFIG=/abs/path/config.yml GITHUB_TOKEN=... go test ./e2e -v` |
| Do the pages work when a person uses them? | Clicks, drags and types through the built app in a real browser, against the demo floor | `npm --prefix web run e2e` |

The GitHub test needs a project you can afford to change, with the fields in
`examples/github.yml` plus a "Waiting" status, a "Quick" class of service and a
triage issue. The browser test uses Edge or Chrome already on the machine
(`BROWSER=chrome` to choose).

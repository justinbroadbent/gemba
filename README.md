# Gemba 現場

A lean flow board for operations teams, layered on top of GitHub Issues and Projects.

GitHub stays the system of record. Gemba adds what GitHub does not have:

- **Board.** One swim lane per discipline, with each person's work in progress shown against their limit.
- **Triage.** An inbox holding only the work that needs a decision. Quick tickets resolve without reaching it.
- **Queues.** A ranked queue per discipline, with a line showing what fits in the next two weeks at the measured pace.
- **Trade-off.** Pick a new priority, see what it displaces, and record who authorized it on the affected issues.

Engineers enter nothing in Gemba. It reads status from GitHub and writes back only the cell,
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
| `brand` | The name and the one or two characters in the seal |
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

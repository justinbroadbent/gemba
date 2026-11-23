# Gemba 現場

A lean flow board for operations teams, layered on top of GitHub Issues and Projects.

GitHub stays the system of record. Gemba adds what GitHub does not have: work cells with
ranked queues, WIP limits per person, a triage inbox, a capacity line based on measured
throughput, and a recorded answer to "if this is the new priority, what gets displaced?"

**Status:** planning. See [docs/PLAN.md](docs/PLAN.md).

- Backend: Go
- Frontend: React + TypeScript
- Runs as one container with Postgres; sign-in through Microsoft Entra ID
- Multi-tenant: one deployment serves several teams, each with its own workspace

## License

MIT

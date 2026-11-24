# For agents working on this repository

Read `docs/HANDOFF.md` first. It says what Gemba is, how it is built, what it
needs from GitHub, how to run the tests, and what is left to do.

The rules that matter most:

- GitHub is the system of record. Gemba has no database; do not add stored
  state without a reason that survives section 1 of the handoff.
- Flow rules live in `internal/flow` only. The web app renders view models and
  applies no rules of its own.
- Gemba writes only the fields listed under "Field ownership".
- A team is described by GitHub (project field options, teams) and by the
  settings file `.gemba.yml` in its work-items repository. Access comes only
  from the deployment config; the settings file must never be able to grant it.
- There is no settings page and no database. Do not add either to hold
  something GitHub or the settings file could hold.
- Nothing in the repository may name or describe a specific employer.
- Before finishing a change, run `go test ./...` and `npm --prefix web run build`.
  For a change to a write or an interaction, also run the matching end-to-end test.

# HOW TO: TUI Add + Renewal Workflow

This is the operator-facing TUI workflow for adding a patent and checking renewal
obligations. It assumes the daemon is running and the TUI can reach it.

Related docs:

- [03_USPTO_CONFIG_LOADING.md](./03_USPTO_CONFIG_LOADING.md) — source keys and
  `source.mode`.
- [04_TUI_ADD_FLOW.md](./04_TUI_ADD_FLOW.md) — detailed `:add` execution flow.
- [14_TUI_RENEWALS.md](./14_TUI_RENEWALS.md) — renewal data model and caveats.
- [22_BUILD_DEPLOY_SECRETS.md](./22_BUILD_DEPLOY_SECRETS.md) — local vs VPS
  secret setup.

---

## 1. Start The Daemon And TUI

Local workstation:

```bash
patentmine serve
patentmine tui
```

VPS / deploy daemon:

```bash
ssh patentmine@your-vps
patentmine tui
```

Useful check:

```bash
patentmine paths
```

`patentmine paths` shows the active database, socket, patent cache, logs, and
credential directory. On a VPS, confirm `PATENTMINE_HOME` points to the deploy
home, for example `/var/lib/patentmine`, and credentials point to
`/etc/patentmine/secrets`.

---

## 2. Pick The Source Policy

In the TUI command prompt, set or inspect source mode:

```text
:source.mode
:source.mode uspto-first
```

Recommended defaults:

- `uspto-first` for normal US prosecution/renewal work.
- `compare` when you want USPTO and Google side-by-side and are willing to wait
  longer.
- `google-only` only for foreign or legacy records where USPTO is not expected.
- `uspto-only` when Google fallback would hide a USPTO lookup problem.

---

## 3. Add A Patent

From a project/matter view, add a patent:

```text
:add US11611785B2
```

Force a specific source when needed:

```text
:add.uspto US11611785B2
:add.google US11611785B2
```

What happens:

- PatentMine normalizes the number.
- If source mode allows USPTO, it attempts USPTO resolution first.
- If multiple USPTO candidates match, the TUI opens a candidate picker.
- The record is added to the active project and a background crawl starts.
- The patent list refreshes when the daemon announces the database change.

Bulk add from a file:

```text
:add.file /path/to/patents.txt
```

The file must use the PatentMine added-patents format described in
[04_TUI_ADD_FLOW.md](./04_TUI_ADD_FLOW.md).

---

## 4. Check Basic Patent Data

After the add finishes, open the patent detail from the catalog selection. Useful
commands from a selected patent:

```text
:source.bibs
:source.compare
:patent.expiration-date
:patent.expiration-date refresh
```

Use `:source.bibs` to see what USPTO vs Google reported. Use
`:source.compare` when divergent source values need review. Use expiration-date
analysis to compute USPTO statutory expiration and compare it with Google-derived
or estimated dates.

---

## 5. Track US Maintenance Fees

For a granted US utility patent:

```text
:renewal.track US11611785B2
```

Optionally set entity size:

```text
:renewal.track US11611785B2 large
:renewal.track US11611785B2 small
:renewal.track US11611785B2 micro
```

This creates the US maintenance-fee deadlines from the grant date:

- 3.5 years from grant.
- 7.5 years from grant.
- 11.5 years from grant.

Each has:

- a 6-month normal payment window before the due date,
- a due date,
- a 6-month surcharge/grace period after the due date.

Stop tracking:

```text
:renewal.untrack US11611785B2
```

Important: this is a docketing assistant, not a system of record. Verify dates
against the official USPTO maintenance fee system.

---

## 6. Check The Renewal Docket

Open the unified deadline board:

```text
:deadline.show
```

The board includes office-action response deadlines and renewal/maintenance
deadlines.

In the deadline overlay:

- `p` marks the selected deadline done.
- `x` dismisses the selected deadline.
- Renewal-focused tabs/filters can be used from the overlay when available.

Reminder emails are optional. Without SMTP configuration, the in-app docket still
works.

---

## 7. EP / National Validation Countries

EP renewals are different from US maintenance fees. Before grant, EPO annuities
are based on the EP application. After grant, national renewal obligations depend
on validated countries.

Fetch EPO OPS legal-status data for an EP patent:

```text
:renewal.validation.fetch EP1234567B1
```

Show stored validation-country state:

```text
:renewal.validation.list EP1234567B1
```

Manually set a country after review or agent/national-register confirmation:

```text
:renewal.validation.set EP1234567B1 DE validated
:renewal.validation.set EP1234567B1 FR potential
:renewal.validation.set EP1234567B1 GB lapsed
```

Status meanings:

- `potential`: designated or possible, but not confirmed as validated.
- `validated`: confirmed national phase/validation exists.
- `lapsed`: validation existed or was possible but is no longer in force.
- `unknown`: not enough information.

Guardrail: a designated EP state is not the same as a validated country. PatentMine
does not treat `potential` as a payable national renewal obligation unless you
manually confirm it or legal-status data supports it.

---

## 8. Typical Workflows

US patent renewal check:

```text
:source.mode uspto-first
:add.uspto US11611785B2
:patent.expiration-date refresh
:renewal.track US11611785B2 small
:deadline.show
```

EP patent country-phase check:

```text
:source.mode google-only
:add.google EP1234567B1
:renewal.validation.fetch EP1234567B1
:renewal.validation.list EP1234567B1
:renewal.validation.set EP1234567B1 DE validated
:deadline.show
```

Portfolio review:

```text
:deadline.show
:renewal.validation.list EP1234567B1
:source.bibs
```

---

## 9. Troubleshooting

`:add.uspto` fails:

- Check `PATENTMINE_USPTO_API_KEY` and run `patentmine check uspto`.
- If using a VPS, verify the daemon can read `/etc/patentmine/secrets/uspto_odp_key`.

`:renewal.validation.fetch` fails:

- Check `PATENTMINE_EPO_OPS_CONSUMER_KEY` and
  `PATENTMINE_EPO_OPS_CONSUMER_SECRET`.
- Confirm both are `file:` references in the daemon `.env` or direct environment.
- Confirm the daemon user can read the files.

No renewal deadlines appear:

- US: confirm the patent has a grant date and is a US utility patent.
- EP: confirm validation countries are `validated`; `potential` countries are not
  treated as active obligations by default.
- Check `:deadline.show` after tracking or validation updates.

Source values disagree:

- Use `:source.bibs` and `:source.compare`.
- Prefer official authority data over Google estimates when available.

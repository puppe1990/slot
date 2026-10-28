# slot — AI Conventions

Amarra-cais app (Go + server-rendered HTML + SQLite). Primary reader is often an LLM agent: cheap greps, small modules, headless tests.

## Rule #1: TDD is mandatory

1. Write the test (`*_test.go`), using `gofakeit` for seeded data when a scenario needs variety.
2. Run it focused and confirm it **fails for the right reason**.
3. Write the **minimal** code to pass.
4. `go test ./... -count=1`, then `make ci` before calling it done.

Bug fix ⇒ regression test in the same commit.

## Invariants — do not "simplify" these away

- **Hold atomicity**: `CreateHold` purges expired holds, checks overlap, and inserts inside one transaction. SQLite runs with `MaxOpenConns(1)`; the partial unique index `idx_bookings_active_start` is the second line of defence. Never split that sequence across requests.
- **Buffers apply on both sides**: `blockedIntervals*` expands existing bookings by _their_ meeting type buffers, and `collides` expands the candidate by _its_ buffers. Changing one side without the other silently double-books.
- **Times are UTC at rest** (`INTEGER` unix seconds). The owner timezone only decides which wall-clock windows exist. `time.Date` per day is what makes DST correct — do not normalize by adding `24h`.
- **Guest tokens are hashed at rest**. The database stores `guest_token_hash` / `hold_token_hash`; the raw token lives in the URL (manage) or an HttpOnly cookie (hold). Never log or persist the raw value.
- **Cookies**: `slot_hold` is HttpOnly, SameSite=Lax, 10-minute MaxAge, path `/book`.
- **Rooms are private**: never put `booking.meeting_url` in a page, email or `.ics`. Link the join routes (`/manage/:token/join`, `/bookings/:id/join`) — they mint the Daily meeting token; the raw URL only works with one.
- **State changes are POST**: RSVP and cancel are POST because mail scanners prefetch GET links in email bodies.
- **Audit inside the transaction**: state change and `audit_logs` row commit together.

## Layout

| Path                     | Responsibility                                                        |
| ------------------------ | --------------------------------------------------------------------- |
| `internal/scheduling/`   | Pure slot engine (no I/O)                                             |
| `internal/store/`        | SQLite, migrations, transactions                                      |
| `internal/guesttoken/`   | HMAC sign/parse + hashing                                             |
| `internal/calendar/`     | Provider interface, fake, Google adapter                              |
| `internal/notify/`       | Notifier interface + outbox                                           |
| `internal/ics/`          | RFC 5545 builder                                                      |
| `internal/handlers/`     | HTTP handlers, view models, formatting                                |
| `internal/app/`          | Bootstrap, adapter selection, routes                                  |
| `web/templates/layouts/` | `app.html` (owner, with the account menu) and `booking.html` (public) |
| `web/templates/pages/`   | HTML pages (`{{ define "content" }}`)                                 |

Handler constructors take `Deps` (store, views, site, config, calendar, notifier, signer, clock). Inject them; never reach for a global or `time.Now()` inside domain logic.

## Clean code for agents

| Priority | Rule                                                                           |
| -------- | ------------------------------------------------------------------------------ |
| 1        | Small units — functions 4–20 lines; files target 200–300, hard cap ~500        |
| 2        | SRP — one reason to change per file                                            |
| 3        | Greppable names — avoid `data`, `handler`, `Manager`, `util`                   |
| 4        | Comments explain WHY (race safety, SQLite, DST, tokens), never WHAT            |
| 5        | Early returns; max ~2 nesting levels                                           |
| 6        | Errors include the value: `fmt.Errorf("hold %s-%s: %w", start, end, err)`      |
| 7        | Headless tests only — SQLite `:memory:`, `httptest`, fake calendar, fake clock |
| 8        | `gofmt` + Prettier without debate                                              |

## Languages

Every user-facing string goes through a catalog: templates use `{{ t "key" }}`, Go code uses `h.localeFor(r).t("key")` (or the `localeText` methods for dates, clocks, durations, statuses and locations). Never hardcode copy in a page — a missing key prints itself, which is the only "translation" you will notice in review.

- Catalogs: `internal/i18n/en.go` and `pt.go`. Add the key to **both** in the same commit.
- Formatting: `internal/handlers/locale_format.go`. English is 12h, pt-BR is 24h; long dates and weekday labels are localized.
- The header toggle (`web/templates/components/locale-toggle.html`) posts to `/locale` and does a full navigation (`data-amarra-skip`).
- Tests that assert copy run under the English default; pt render tests wrap the handler in `i18n.LocaleMiddleware` (see `internal/handlers/i18n_pages_test.go`).

## Design system — Calm Conviction

Warm alabaster canvas, deep pine for action, muted clay for destructive states, Hanken Grotesk for prose, JetBrains Mono for every temporal string.

- Palette lives in `input.css` CSS variables (`--amarra-ink/foam/copper/tide/coral`) so the amarra kit keeps working; `tailwind.config.js` maps them plus the role tokens.

### Color roles (do not blend them)

| Role                        | Tokens                                                                              | Use for                                                                                                   |
| --------------------------- | ----------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------- |
| Canvas / Surface / Elevated | `canvas` `#FAF9F6`, `surface` `#F4F3EF`, `elevated` `#FFF`, `container-low…highest` | Page, cards, and wells inside cards. Never color text with these.                                         |
| Primary                     | `pine`, `pine-hover`, `moss`, `moss-dim`                                            | Primary buttons, active nav/date, confirmed/live/verified markers, the hold banner. Nothing navigational. |
| Secondary                   | `slate`, `slate-soft`, `slate-tint`                                                 | Ghost buttons, navigational links and paths, disclosure labels, paused state.                             |
| Tertiary                    | `clay`, `clay-soft`, `clay-tint`                                                    | Cancellation, destructive actions, closed/unavailable, validation errors.                                 |
| Structure                   | `line`, `line-strong`, `outline`                                                    | 1px borders and dividers. Text never uses these.                                                          |
| Text                        | `foam` (ink), `muted`, `outline`                                                    | Prose and numbers in ink; metadata in muted; `outline` only for strokes.                                  |

Rule of thumb: if a pine element does not perform an action or confirm a commitment, it is slate. Status pills pair a tint with its text color (`bg-moss/70 text-pine`, `bg-clay-tint/50 text-clay`, `bg-slate-tint text-slate-soft`).

- The app is **light-first**: `:root` and `html.light` carry the same values, and there is no theme toggle.
- Cards: `rounded-xl border border-line bg-surface`; inputs `rounded-lg border border-line bg-canvas`; buttons 44–48px tall.
- Times, durations and timezones are always mono; never mix them into prose spacing.
- Rebuild CSS after template changes: `npx tailwindcss -i input.css -o web/static/css/styles.css --minify`.

## Amarra HTML contract

- Handlers call `view.Write` via `base.page`; pages define `{{ define "content" }}`.
- Pipeline: `view.Load` once at boot (`layouts/*.html`, `pages/*.html`, `partials/*.html` flat only).
- Drive morphs `#amarra-main`; plain forms/links need no attribute. Opt out with `data-amarra-skip`.
- CSRF: double-submit cookie + `csrf_token` field (`.form` injects it; inline forms must include it manually).
- Flash: `flash.Set(w, kind, msg, cfg.CookieSecure())`, read through `.flash` in the layout.
- Inline `<script>` needs `nonce="{{ .CSPNonce }}"`; it does not re-run on a Drive morph.
- `.Site.LoggedIn` flips the `data-amarra-shell` marker so auth screens do a full navigation.

## Routes

`internal/app/routes.go`. Owner routes are wrapped with `middleware.RequireAuthFunc("/login", …)`. Public booking routes are rate-limited with `middleware.NewRateLimiter`.

## Commands

```bash
amarra-cais dev            # tailwind watch + air + template reload
amarra-cais test           # go test ./...
make ci                    # test + lint + prettier check
amarra-cais doctor         # verify amarra.js, layouts, PWA
amarra-cais db seed        # demo owner, formats and bookings
amarra-cais pwa --bump     # after HTML changes when testing on a phone
amarra-cais upgrade        # bump the framework + print the migration checklist
```

## Do not

- Parse templates per request
- Mock the database (use SQLite `:memory:`)
- Add a JS framework, Vite, or a second source of truth for slot math
- Grow files past ~500 lines
- Ship a feature without a headless test
- Log raw hold/guest tokens

## Machine surface

- `internal/handlers/admin_api.go` — JSON under `/api/admin/*`, behind `middleware.AdminAuth` (Bearer `ADMIN_TOKEN`). It acts as `FirstOwner()`: single-owner app, no per-token scopes.
- `internal/slotcli` — HTTP client used by both `cmd/slot` and `cmd/slot-mcp`. Never duplicate request code in the binaries.
- Booking side effects (calendar, room, notices) live in `base.finalizeBooking` / `cancelBooking` / `rescheduleBooking` so the web flow, the CLI and the MCP cannot drift.
- MCP stdio = newline-delimited JSON-RPC. Notifications (no `id`) never get a reply; tool failures come back as `result.isError`, not as RPC errors.

## Recurring blocks

- `internal/scheduling/recurring.go` — `ExpandRecurringBlocks` turns "weekdays + clock + every N weeks + anchor date" into busy intervals; the gap check then reuses the normal collision path.
- Weeks are counted from **Monday of the anchor week** and the day difference uses noon UTC, so a DST transition cannot flip the parity.
- Blocks are busy windows, not extra availability: they subtract from the weekly hours. If "open hours on the off week" is ever needed, that is a new feature, not a flag.
- `buildSlots` is the only place that expands them; the public page, the dashboard and the admin API all inherit the behaviour.

## Holidays

- `internal/holidays` — the fixed national calendar (8 dates), `NextOnOrAfter` for the "Next:" label. Movable observances (Carnival, Good Friday, Corpus Christi) are out of scope.
- `holidayClosures` (handlers) turns enabled holidays inside the horizon into `IsUnavailable` date overrides. A holiday with no stored row is **on**: the calendar starts closed.
- A manual date override for the same day wins — the owner explicitly opened it.
- Holidays are computed per request, never stored as overrides: changing a switch changes every future occurrence at once.

## Framework upgrades

- `amarra-cais upgrade` bumps `go.mod`, runs `go mod tidy` and `doctor`, and prints a per-version checklist. Follow it before merging — the checklist is where breaking changes live.
- v0.24.0: `tailwind.config.js` must scan the shipped kit (`shippedKitContent()` via `go list -m`). Without it Tailwind prunes kit-only utilities (#372). The overrides in `web/templates/components/` are now **design choices**, not a CSS workaround.
- v0.25.0: CSP emits `frame-src 'self'`. `CSP_FRAME_SRC` allows third-party iframes (amarra-cais#379); `frame-ancestors` stays `none`.
- Always run `amarra-cais pwa --bump` after an upgrade so `web/static/js/amarra.js` matches `go.mod`, then `amarra-cais css`.

## Reminders and consent

- `internal/handlers/reminders.go` — in-process ticker started by `App.RunContext` (one binary on this deploy, no separate worker). `REMINDERS=off` disables it; `REMINDER_LEAD_HOURS` (default 24) sets the window.
- Idempotency is the point: `ClaimReminder` stamps `bookings.reminder_sent_at` **before** the send, and `ReleaseReminder` clears it when the notifier fails, so a restart cannot duplicate or lose a reminder.
- The manage URL is rebuilt with `deps.Signer.Sign(booking.ID)` — the signer is deterministic and the DB still stores only the hash.
- `bookings.guest_locale` remembers the language of the booking; the reminder picks that catalog instead of the app default, so a Brazilian guest does not get an English reminder because `LOCALE=en`.
- Consent: the confirm form requires the checkbox (`confirm.error_consent`), and `ConfirmHold` writes `consent_at`. Existing rows keep an empty value; the column is the audit trail, not a gate.

## Google calendar credentials

- Service account wins over OAuth: `GOOGLE_SA_JSON` + `GOOGLE_CALENDAR_ID` (the owner's calendar id, never `primary` — a service account has no primary calendar). No consent screen, no refresh token, no 7-day expiry.
- `internal/calendar/serviceaccount.go` signs the RS256 assertion with stdlib crypto and caches the access token; the Google adapter only receives an `AccessToken` function, so both credential styles share the REST code.
- A service account on a personal calendar must not send `attendees`: Google answers `403 forbiddenForServiceAccounts` ("cannot invite attendees without Domain-Wide Delegation"). `Google.InviteAttendees` is false for the service-account path; the guest still gets the Slot e-mail and the `.ics`.
- The calendar must be shared with the service account e-mail ("Make changes to events"). An unreadable calendar comes back as `notFound` with an empty busy list — the adapter turns that into an **error**, never "free": failing closed is what prevents a silent double-booking. The booking page then errors instead of offering colliding slots, so enable the credential only after the share is confirmed.

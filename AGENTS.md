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
- **Audit inside the transaction**: state change and `audit_logs` row commit together.

## Layout

| Path                     | Responsibility                                 |
| ------------------------ | ---------------------------------------------- |
| `internal/scheduling/`   | Pure slot engine (no I/O)                      |
| `internal/store/`        | SQLite, migrations, transactions               |
| `internal/guesttoken/`   | HMAC sign/parse + hashing                      |
| `internal/calendar/`     | Provider interface, fake, Google adapter       |
| `internal/notify/`       | Notifier interface + outbox                    |
| `internal/ics/`          | RFC 5545 builder                               |
| `internal/handlers/`     | HTTP handlers, view models, formatting         |
| `internal/app/`          | Bootstrap, adapter selection, routes           |
| `web/templates/layouts/` | `app.html` (owner) and `booking.html` (public) |
| `web/templates/pages/`   | HTML pages (`{{ define "content" }}`)          |

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

## Design system — Calm Conviction

Warm alabaster canvas, deep pine for action, muted clay for destructive states, Hanken Grotesk for prose, JetBrains Mono for every temporal string.

- Palette lives in `input.css` CSS variables (`--amarra-ink/foam/copper/tide/coral`) so the amarra kit keeps working; `tailwind.config.js` maps them plus semantic aliases (`canvas`, `surface`, `pine`, `clay`, `line`, `muted`, `moss`).
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
```

## Do not

- Parse templates per request
- Mock the database (use SQLite `:memory:`)
- Add a JS framework, Vite, or a second source of truth for slot math
- Grow files past ~500 lines
- Ship a feature without a headless test
- Log raw hold/guest tokens

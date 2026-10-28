# Slot

A deliberately focused Calendly alternative for an independent consultant: 30- and 60-minute sessions, true timezone arithmetic, ten-minute slot holds, and signed guest links that reschedule or cancel without an account.

Built with [Amarra-cais](https://github.com/puppe1990/amarra-cais) — Go 1.26, server-rendered HTML (Amarra Views + Drive), Tailwind, SQLite. No SPA, no Vite, no Redis.

## What works

- **Real availability math** — weekly windows in the owner's IANA timezone, per-date overrides, minimum notice, booking horizon, pre/post buffers, external busy blocks, and existing bookings. DST spring-forward and fall-back are covered by tests.
- **Two-week rotas** — recurring blocked hours: close the same window every N weeks (a morning shift one week, a night shift the next), anchored on a date so the parity never drifts.
- **National holidays** — the eight fixed Brazilian holidays, each with a switch; an enabled holiday closes the whole day, every year, and a manual date override still wins.
- **No double bookings** — a guest picks a slot and gets a 10-minute `HOLD`; the overlap check and the insert share one SQLite transaction, and a partial unique index on `(meeting_type_id, start_time_utc)` for active rows backs it up. Confirming converts the hold to `CONFIRMED` atomically.
- **Timezone-aware UI** — the guest page detects the browser zone (`?tz=`), shows guest and host clocks side by side, and renders every slot in both.
- **Signed guest management** — `/manage/{token}` is HMAC-signed and only the SHA-256 hash is stored, so a DB backup cannot be replayed as a booking link.
- **Calendar + email adapters** — a deterministic in-memory calendar (default) and a testable Google Calendar REST adapter; an in-memory outbox records every notification for assertions.
- **ICS download** — RFC 5545 output with CRLF folding and escaping.

## Quick start

```bash
export PATH="$HOME/go/bin:$PATH"
amarra-cais install        # npm install + go mod tidy + Tailwind build
amarra-cais dev            # http://localhost:8080
```

Development seeds a demo desk: owner `demo@example.com` / `password`, formats `30min` and `60min`, and three upcoming bookings.

```bash
amarra-cais test           # go test ./...
make ci                    # tests + golangci-lint + prettier check
```

## Routes

| Route                                                      | Purpose                                                        |
| ---------------------------------------------------------- | -------------------------------------------------------------- |
| `/`                                                        | Owner → `/dashboard`; visitor → first published format         |
| `/login`, `/signup`, `/forgot-password`, `/reset-password` | Owner auth (session + password)                                |
| `/dashboard`                                               | Upcoming bookings, volume, committed time, public link         |
| `/settings`                                                | Account: display name, home timezone, password                 |
| `/setup`                                                   | Weekly hours, date overrides, calendar provider                |
| `/meeting-types`                                           | List/create formats, pause/publish                             |
| `/meeting-types/:id/edit`                                  | Duration, buffers, notice, horizon, modality                   |
| `/book/:slug`                                              | Public booking page (date strip + slots)                       |
| `POST /book/:slug/hold`                                    | Create a 10-minute hold; token goes to an HttpOnly cookie      |
| `/book/:slug/confirm`                                      | Guest form for the held slot                                   |
| `POST /book/:slug/confirm`                                 | Atomically converts the hold into a confirmed booking          |
| `/manage/:token`                                           | Guest self-serve reschedule/cancel                             |
| `GET /manage/:token/invite.ics`                            | Calendar file                                                  |
| `/api/health`                                              | DB probe JSON                                                  |
| `/api/admin/*`                                             | JSON API for the CLI and the MCP server (Bearer `ADMIN_TOKEN`) |

## CLI

```bash
go install github.com/puppe1990/slot/cmd/slot@latest

export SLOT_URL=https://slot.apps.gestaobem.com
export SLOT_TOKEN=<ADMIN_TOKEN>     # the app's ADMIN_TOKEN

slot whoami                        # which account the token talks to
slot blocks                        # recurring blocked hours
slot blocks add --title "Morning Icaro Time" --days 1,2,3,4,5 \
  --start 07:00 --end 11:00 --every 2 --anchor 2026-11-02
slot blocks rm 3
slot formats
slot slots --format 30min --days 7 --tz America/Sao_Paulo
slot book --format 30min --slot 2026-10-28T13:00:00Z \
  --name "Marina Costa" --email marina@example.com --whatsapp "+5511999998888"
slot bookings --status upcoming --days 30
slot cancel 12 --reason "conflict"
slot reschedule 12 --slot 2026-10-29T15:00:00Z
```

Every command accepts `--json` for scripting and `--url` / `--token` to override the env.

## MCP

`slot-mcp` speaks MCP over stdio and calls the same admin API, so an agent can read the agenda, find a free slot, book, move or cancel sessions.

```json
{
  "mcpServers": {
    "slot": {
      "command": "slot-mcp",
      "env": {
        "SLOT_URL": "https://slot.apps.gestaobem.com",
        "SLOT_TOKEN": "<ADMIN_TOKEN>"
      }
    }
  }
}
```

Tools: `slot_formats_list`, `slot_slots_list`, `slot_bookings_list`, `slot_booking_create`, `slot_booking_reschedule`, `slot_booking_cancel`.

Both clients are thin: the authorisation, the double-booking rules and the e-mail/video side effects all live in the app.

## Data model

`users` · `calendar_connections` · `meeting_types` · `availability_rules` · `date_overrides` · `bookings` · `audit_logs`

Guests can leave an optional WhatsApp on the confirm form. The number is normalized to `+<digits>`, stored on the booking, shown to the owner as a `wa.me` link on the dashboard and included in the new-booking email.

Booking lifecycle:

```
EMPTY SLOT ──hold──▶ HOLD (10 min TTL) ──confirm──▶ CONFIRMED
                         │                             │
                         └── expiry/abandon ──▶ purged  ├── reschedule ──▶ RESCHEDULED
                                                       └── cancel ─────▶ CANCELLED
```

Every transition writes an `audit_logs` row (`HOLD_CREATED`, `BOOKING_CONFIRMED`, `BOOKING_RESCHEDULED`, `BOOKING_CANCELLED`) inside the same transaction as the state change.

## Code map

| Path                   | Responsibility                                            |
| ---------------------- | --------------------------------------------------------- |
| `internal/scheduling/` | Pure slot engine: windows, buffers, notice, horizon, DST  |
| `internal/store/`      | SQLite store, migrations, hold/confirm transactions       |
| `internal/guesttoken/` | HMAC tokens + SHA-256-at-rest hashing                     |
| `internal/calendar/`   | `Provider` interface, in-memory fake, Google REST adapter |
| `internal/notify/`     | Notifier interface + in-memory outbox                     |
| `internal/ics/`        | RFC 5545 builder                                          |
| `internal/handlers/`   | HTTP handlers + view models                               |
| `internal/app/`        | Bootstrap, adapters, routes                               |
| `web/templates/`       | `app` (owner) and `booking` (public) layouts + pages      |

## TDD with faker

Tests come first. `github.com/brianvoe/gofakeit/v7` generates seeded guest identities and a property test:

```bash
go test ./... -count=1
go test ./internal/scheduling/ -run TestAvailableSlotsNeverCollideWithFakedBusyIntervals -v
```

The scheduling suite runs 40 faker-seeded scenarios and asserts that no returned slot overlaps a busy interval after buffers — including DST transition days. Store tests use SQLite `:memory:` and never mock the database.

## Languages

The UI ships in **English** and **Portuguese (pt-BR)**. The header toggle posts to `/locale`, which stores the choice in the `cais_locale` cookie and reloads.

- Static copy lives in `internal/i18n/en.go` / `pt.go`; templates call `{{ t "key" }}`.
- Copy built in Go (statuses, locations, durations, dates) goes through `localeFor(r)` in `internal/handlers/locale_format.go`.
- Clocks follow the locale: 12h in English, 24h in pt-BR. Weekdays and months are localized too, so the date strip reads `seg, ter, qua`.

## Email

Transactional mail goes through a `Notifier` picked by env:

| `EMAIL_PROVIDER`   | Needs                           | Use                                  |
| ------------------ | ------------------------------- | ------------------------------------ |
| `outbox` (default) | —                               | memory only; development and tests   |
| `resend`           | `RESEND_API_KEY` + `EMAIL_FROM` | real delivery through the Resend API |

Every notice goes out as **plain text and HTML**. The HTML part uses the Calm Conviction palette (canvas, stone, pine, clay) with a table layout and inline styles only — Gmail strips `<style>` and ignores web fonts, so the fallbacks are system fonts. The header badge follows the tone: moss/pine for confirmed, slate for moved, clay for cancelled.

`EMAIL_FROM` must sit on a domain verified in Resend (e.g. `Slot <slot@gestaobem.com>`). Confirmation, reschedule and cancellation notices are composed in `internal/handlers/notices.go` and follow the locale of the request that triggered them — a guest who booked in pt-BR gets a Portuguese email. A delivery failure logs and never fails the booking.

## Video rooms

Each confirmed booking can get a real room instead of a dead "Google Meet" label. The provider is chosen by env and falls back to `none` (label only) when unconfigured.

| `CALL_PROVIDER`  | Needs           | Room                                                    |
| ---------------- | --------------- | ------------------------------------------------------- |
| `none` (default) | —               | none; the manage page shows only the modality           |
| `jitsi`          | —               | `https://meet.jit.si/slot-<random>`, no API call        |
| `daily`          | `DAILY_API_KEY` | `POST /v1/rooms` with an expiry at the session end + 1h |

The room URL is stored on the booking (`meeting_url`) but never linked directly: **Join room** points at the signed route, which asks Daily for a short-lived meeting token (`is_owner` for the consultant) and redirects with it. A forwarded email therefore cannot walk into someone else's room. The `.ics` carries the join route, not the room. Cancelling the booking calls `EndRoom` (Daily deletes the room; Jitsi is a no-op). Guests can also RSVP ("I'll be there") from the manage page; the answer is stored on `bookings.attendance` and shows as a badge on the dashboard. A vendor failure logs and never loses the booking — same rule as the calendar event.

Adding a provider is one file in `internal/call/` plus a case in `call.New`; the handlers only know the interface.

## Environment

| Variable                                    | Default         | Description                                           |
| ------------------------------------------- | --------------- | ----------------------------------------------------- |
| `PORT`                                      | `:8080`         | Listen address                                        |
| `DB_PATH`                                   | `./data/app.db` | SQLite file                                           |
| `ENV`                                       | `development`   | `production` gates cookies, seeds and the boot checks |
| `APP_URL`                                   | —               | Absolute URLs in emails and OG tags                   |
| `GUEST_TOKEN_SECRET`                        | dev fallback    | HMAC secret; **required** when `ENV=production`       |
| `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` | —               | Selects the Google adapter instead of the fake        |
| `GOOGLE_CALENDAR_ID`                        | `primary`       | Calendar target                                       |

Fonts load from Google Fonts; keep `CSP_STYLE_SRC` and `CSP_FONT_SRC` in `.env` or drop the `<link>` tags for a strict-CSP deployment.

## Google Calendar

`internal/calendar/google.go` speaks the Calendar REST API (`freeBusy`, event create/patch/delete) with an injectable access token and base URL, and is covered by `httptest` tests. The OAuth authorization-code exchange is intentionally not wired yet: without credentials the app boots on the deterministic fake, and `/setup` records which provider you intend to use.

## Deploy

```bash
amarra-cais css
amarra-cais build --os linux --arch amd64 -o bin/server-linux
tar czf release.tar.gz bin/server-linux web/static
```

Set `ENV=production`, `APP_URL`, `PORT`, `GUEST_TOKEN_SECRET` and `ADMIN_TOKEN`. Demo seeds never run in production.

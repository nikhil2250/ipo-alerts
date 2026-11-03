# IPO Closing Alerts

A Go application that checks IPOWatch for Mainboard IPOs closing today, joins each IPO's current Retail subscription and GMP, then emails a report. SME IPOs are excluded. Dates are evaluated in `Asia/Kolkata` by default.

## Configure email

The application reads SMTP settings from environment variables:

| Variable | Required | Example |
| --- | --- | --- |
| `SMTP_HOST` | Yes | `smtp.gmail.com` |
| `SMTP_PORT` | No | `587` |
| `SMTP_USERNAME` | For authenticated SMTP | Your SMTP username |
| `SMTP_PASSWORD` | For authenticated SMTP | Your SMTP password or app password |
| `MAIL_FROM` | Yes | Sender email address |
| `MAIL_TO` | Yes | Recipient email address; multiple addresses may be comma-separated |
| `TIMEZONE` | No | `Asia/Kolkata` |

For Gmail, use an app password rather than your account password. Do not commit credentials.

## Run locally

The app loads `.env` from the current directory automatically. Add these entries:

```dotenv
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USERNAME=your-address@example.com
SMTP_PASSWORD=your-app-password
MAIL_FROM=your-address@example.com
MAIL_TO=recipient@example.com,another@example.com
```

Keep `.env` private; it is ignored by Git. Variables already exported in your shell take precedence over values in `.env`.

```sh
go run .
```

Run the test suite with `go test ./...`.

Set `DRY_RUN=true` to print the report locally without connecting to SMTP.

## GitHub Actions

The workflow runs daily at 11:00 and 13:00 India Standard Time. GitHub Actions schedules use UTC, so these are configured as 05:30 and 07:30 UTC. Scheduled workflows run from the repository's default branch; GitHub may delay scheduled starts during busy periods.

Add these repository Actions secrets:

- `SMTP_USERNAME`
- `SMTP_PASSWORD`
- `MAIL_FROM`
- `MAIL_TO`

Optionally add repository Actions variables `SMTP_HOST` and `SMTP_PORT`; they default to `smtp.gmail.com` and `587`. Use **Actions > IPO closing alerts > Run workflow** to test a run manually.

## Data notes

The app reads the IPOWatch subscription-status page and GMP page, selecting Mainboard IPOs with a closing date matching today in `TIMEZONE`. It joins the data by IPO name. If no Mainboard IPOs close today, it skips sending email. `MAIL_TO` accepts multiple comma-separated email addresses. If an IPO has no GMP row, the email shows `Not listed`. IPOWatch describes GMP as unofficial and subject to change; the report is informational, not investment advice.
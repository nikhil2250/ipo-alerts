# IPO Closing Alerts

A Go application that emails Mainboard IPOs closing today, with Retail subscription, sNII subscription, GMP, and GMP percentage. SME IPOs and IPOs closing on other dates are excluded.

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

Each run reads the GMP page and keeps its first three Mainboard rows in listed order, regardless of open or closed status. Retail subscription comes from IPOWatch's subscription table. sNII subscription is read from each IPO's detail page; unavailable values are reported as `Not available`. `MAIL_TO` accepts multiple comma-separated email addresses. IPOWatch describes GMP as unofficial and subject to change; the report is informational, not investment advice.
Each run selects all Mainboard IPOs whose closing date is today in `TIMEZONE`, regardless of their Open or Closed status on the GMP page. Retail subscription comes from IPOWatch's subscription table. sNII subscription is read from each IPO's detail page; unavailable values are reported as `Not available`. `MAIL_TO` accepts multiple comma-separated email addresses. IPOWatch describes GMP as unofficial and subject to change; the report is informational, not investment advice.
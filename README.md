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

## Data extraction

The application fetches the IPOWatch's current subscription page (/subscription/) and GMP page, converts visible page markup into plain text, and sends that text to NVIDIA NIM for structured IPO extraction. It does not use HTML DOM parsing, table parsing, or fixed column positions.

The subscription page is the source for IPO name, category, opening/closing dates, Retail, sNII, bNII, QIB and total subscription wherever those values are published. The GMP page enriches matching IPO records with GMP and GMP percentage. For closing-today Mainboard IPOs with missing sNII or bNII, the application fetches the IPO-specific subscription-detail page and asks NVIDIA to extract the missing fields.

The model must return source evidence for each extracted field. Go checks that evidence is present in the fetched page text, validates dates and numeric values, matches records by normalized IPO name, and then applies the Mainboard + closing-today filter using the configured timezone. Missing values remain null; the report displays them as Not available. GMP is unofficial and may change; verify figures against exchange disclosures before making investment decisions.

## NVIDIA AI configuration

Add these GitHub Actions settings:

- Secret: NVIDIA_API_KEY
- Variable: NVIDIA_MODEL (optional; defaults to nvidia/nemotron-3.5-lightning-30b-a3b)

The NVIDIA endpoint is https://integrate.api.nvidia.com/v1/chat/completions. AI extraction is the primary extraction path and is required for the scheduled workflow; there is no deterministic table-parser fallback.

Use Actions > IPO closing alerts > Run workflow with dry_run enabled to fetch and extract data without sending an email. The workflow also runs go test ./... before the report job.


## No-active-IPO days

An empty `ipos` array from NVIDIA on the subscription or GMP listing page is treated as a valid no-current-record result. The scheduled job logs the condition, skips the report email, and exits successfully. Network failures, NVIDIA API failures, malformed JSON, and invalid non-empty IPO records still fail visibly.

# IPO Closing Alerts

A Go application that checks current Indian IPO listings, identifies **Mainboard IPOs closing today**, and sends a consolidated email with subscription and grey-market information.

The report includes Retail, sNII, bNII, QIB and total subscription multiples when the source publishes them, plus GMP and GMP percentage when available. SME IPOs and IPOs closing on other dates are excluded.

## How it works

1. Fetch the current IPOWatch subscription listing: https://ipowatch.in/subscription/
2. Convert the fetched page markup to readable text. The application does not use an HTML DOM parser, table parser or fixed table-column positions.
3. Send the page text to NVIDIA NIM for structured IPO extraction.
4. Validate each result against its complete source-row evidence (`record_evidence`), including category, closing date and any extracted numeric values.
5. Keep only Mainboard IPOs whose closing date is today in the configured timezone.
6. Enrich matching IPOs with GMP from https://ipowatch.in/ipo-grey-market-premium-latest-ipo-gmp/.
7. When any subscription metric is unavailable, fetch the IPO-specific subscription-detail page and ask NVIDIA to extract the subscription metrics shown there; returned values are used to enrich the IPO record.
8. Send the report using SMTP, unless there are no qualifying IPOs or DRY_RUN is enabled.

**Important data rules:** A combined NII value is not treated as sNII or bNII. Missing values remain unavailable; the model must not estimate or invent them. GMP is unofficial and may change, so verify it independently before making investment decisions.

## Configuration

The application reads a `.env` file from the current directory when run locally. In GitHub Actions, configure the following repository secrets and variables.

| Name | Type | Required | Default / purpose |
| --- | --- | --- | --- |
| `NVIDIA_API_KEY` | Secret | Yes | NVIDIA NIM authentication key |
| `NVIDIA_MODEL` | Variable | No | `nvidia/nemotron-3.5-lightning-30b-a3b` |
| `TIMEZONE` | Variable / environment | No | `Asia/Kolkata` |
| `SMTP_HOST` | Variable | For email | `smtp.gmail.com` in the workflow |
| `SMTP_PORT` | Variable | No | `587` |
| `SMTP_USERNAME` | Secret | For authenticated SMTP | SMTP username |
| `SMTP_PASSWORD` | Secret | For authenticated SMTP | SMTP password or Gmail app password |
| `MAIL_FROM` | Secret | When sending email | Sender address |
| `MAIL_TO` | Secret | When sending email | Comma-separated recipient addresses |
| `DRY_RUN` | Environment | No | `true` prints a report instead of sending email |

Do not commit API keys, SMTP passwords or `.env` files. For Gmail, use an app password rather than your account password.

## Run locally

Create a `.env` file:

```dotenv
NVIDIA_API_KEY=your-nvidia-api-key
NVIDIA_MODEL=nvidia/nemotron-3.5-lightning-30b-a3b
TIMEZONE=Asia/Kolkata
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USERNAME=your-address@example.com
SMTP_PASSWORD=your-app-password
MAIL_FROM=your-address@example.com
MAIL_TO=recipient@example.com,another@example.com
```

Run the application:

```sh
go run .
```

Run tests:

```sh
go test ./...
```

Set `DRY_RUN=true` in the environment to print a non-empty report without sending email. When no qualifying IPOs are found, the application skips SMTP and exits successfully.

## GitHub Actions schedule

The workflow is `.github/workflows/ipo-alerts.yml` and runs automatically every day:

| UTC cron | Scheduled time (IST) |
| --- | --- |
| `30 5 * * *` | **11:00 AM IST** |
| `30 7 * * *` | **1:00 PM IST** |

GitHub Actions schedules use UTC. Scheduled runs set `TIMEZONE=Asia/Kolkata`, run `go test ./...`, and then execute `go run .`. GitHub may delay scheduled starts during periods of high platform load.

To test manually, open **Actions → IPO closing alerts → Run workflow**. The `dry_run` input defaults to `true`, so a manual run does not send email unless you explicitly turn dry-run off. Scheduled runs use `DRY_RUN=false`.

## NVIDIA AI extraction

The application uses NVIDIA's OpenAI-compatible chat-completions endpoint:

```text
https://integrate.api.nvidia.com/v1/chat/completions
```

NVIDIA receives readable text from the fetched IPOWatch pages and returns structured JSON. For each IPO, it must provide `record_evidence`, a complete source-row excerpt. Go checks that the excerpt belongs to the IPO and that extracted category, date and numeric values are supported by that row. There is no deterministic HTML table-parser fallback.

Subscription-page fields are IPO name, category, opening/closing dates, Retail, QIB, total and sNII/bNII only when separately published. The detail-page lookup is triggered when any subscription metric is unavailable; values explicitly returned from that page can enrich the record. Combined NII is never substituted for sNII or bNII.

## No-active-IPO days and errors

If NVIDIA returns an empty `ipos` list for the subscription or GMP listing page, the application treats it as an empty dataset. When the subscription list is empty or no Mainboard IPO closes today, it skips the email and exits successfully.

Actual page-fetch/network failures, NVIDIA HTTP/API failures, malformed JSON and invalid non-empty IPO records remain errors and cause the run to fail visibly.
# Project Instructions

- Keep this project as a small Go CLI for scheduled IPO closing email alerts.
- Preserve the Asia/Kolkata default and UTC-converted GitHub Actions schedule unless the user asks to change them.
- Keep scraping limited to IPOWatch and avoid logging SMTP credentials or other secrets.
- Run `go test ./...` after changes to parsing, matching, or email formatting.
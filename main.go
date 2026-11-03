package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/joho/godotenv"
	"golang.org/x/net/html"
)

const (
	gmpURL          = "https://ipowatch.in/ipo-grey-market-premium-latest-ipo-gmp/"
	subscriptionURL = "https://ipowatch.in/ipo-subscription-status-today/"
	defaultTimezone = "Asia/Kolkata"
)

type IPO struct {
	Name               string
	Category           string
	RetailSubscription string
	GreyMarketPremium  string
	ClosingDate        time.Time
}

func main() {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatalf("load .env: %v", err)
	}
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	locationName := envOrDefault("TIMEZONE", defaultTimezone)
	location, err := time.LoadLocation(locationName)
	if err != nil {
		return fmt.Errorf("load timezone %q: %w", locationName, err)
	}

	subscriptionHTML, err := fetchPage(subscriptionURL)
	if err != nil {
		return fmt.Errorf("fetch subscription data: %w", err)
	}
	subscriptions, err := parseSubscriptionTable(subscriptionHTML)
	if err != nil {
		return err
	}

	gmpHTML, err := fetchPage(gmpURL)
	if err != nil {
		return fmt.Errorf("fetch GMP data: %w", err)
	}
	premiums, err := parseGMPTable(gmpHTML)
	if err != nil {
		return err
	}

	today := time.Now().In(location)
	closingToday := selectClosingToday(subscriptions, premiums, today)
	message := buildEmail(closingToday, today)
	if strings.EqualFold(os.Getenv("DRY_RUN"), "true") {
		fmt.Print(message)
		return nil
	}
	return sendReport(closingToday, today)
}

func fetchPage(url string) ([]byte, error) {
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "ipo-alerts/1.0 (scheduled IPO data summary)")
	client := &http.Client{Timeout: 25 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("GET %s returned %s", url, response.Status)
	}
	return io.ReadAll(response.Body)
}

func parseSubscriptionTable(page []byte) ([]IPO, error) {
	doc, err := html.Parse(bytes.NewReader(page))
	if err != nil {
		return nil, fmt.Errorf("parse subscription page: %w", err)
	}

	var ipos []IPO
	for _, row := range tableRows(doc) {
		if len(row) < 8 {
			continue
		}
		closingDate, err := parseClosingDate(row[2])
		if err != nil || row[0] == "" {
			continue
		}
		ipos = append(ipos, IPO{
			Name:               row[0],
			Category:           row[1],
			ClosingDate:        closingDate,
			RetailSubscription: row[5],
		})
	}
	if len(ipos) == 0 {
		return nil, fmt.Errorf("no IPO rows found on subscription page; the page layout may have changed")
	}
	return ipos, nil
}

func parseGMPTable(page []byte) (map[string]string, error) {
	doc, err := html.Parse(bytes.NewReader(page))
	if err != nil {
		return nil, fmt.Errorf("parse GMP page: %w", err)
	}

	premiums := make(map[string]string)
	for _, row := range tableRows(doc) {
		if len(row) < 6 || !strings.Contains(strings.ToLower(row[0]), "open") && !strings.Contains(strings.ToLower(row[0]), "closed") {
			continue
		}
		name := strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(row[0], " Open"), " Closed"))
		if name == row[0] {
			lower := strings.ToLower(row[0])
			name = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(lower, " open"), " closed"))
		}
		if key := normalizeName(name); key != "" {
			premiums[key] = row[1]
		}
	}
	if len(premiums) == 0 {
		return nil, fmt.Errorf("no IPO rows found on GMP page; the page layout may have changed")
	}
	return premiums, nil
}

func tableRows(doc *html.Node) [][]string {
	var rows [][]string
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "tr" {
			var cells []string
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				if child.Type == html.ElementNode && (child.Data == "td" || child.Data == "th") {
					cells = append(cells, nodeText(child))
				}
			}
			if len(cells) > 0 {
				rows = append(rows, cells)
			}
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(doc)
	return rows
}

func nodeText(node *html.Node) string {
	var parts []string
	var visit func(*html.Node)
	visit = func(current *html.Node) {
		if current.Type == html.TextNode {
			if text := strings.TrimSpace(current.Data); text != "" {
				parts = append(parts, text)
			}
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(node)
	return strings.Join(parts, " ")
}

func parseClosingDate(value string) (time.Time, error) {
	for _, layout := range []string{"January 2, 2006", "January 2 2006", "Jan 2, 2006", "Jan 2 2006"} {
		if parsed, err := time.ParseInLocation(layout, strings.TrimSpace(value), time.Local); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized closing date %q", value)
}

func normalizeName(value string) string {
	var normalized strings.Builder
	for _, character := range strings.ToLower(value) {
		if unicode.IsLetter(character) || unicode.IsNumber(character) {
			normalized.WriteRune(character)
		}
	}
	return normalized.String()
}

func sameDate(left, right time.Time) bool {
	return left.Year() == right.Year() && left.Month() == right.Month() && left.Day() == right.Day()
}

func selectClosingToday(subscriptions []IPO, premiums map[string]string, today time.Time) []IPO {
	closingToday := make([]IPO, 0)
	for _, ipo := range subscriptions {
		if !strings.EqualFold(strings.TrimSpace(ipo.Category), "Mainboard") || !sameDate(ipo.ClosingDate, today) {
			continue
		}
		ipo.GreyMarketPremium = premiums[normalizeName(ipo.Name)]
		if ipo.GreyMarketPremium == "" {
			ipo.GreyMarketPremium = "Not listed"
		}
		closingToday = append(closingToday, ipo)
	}
	return closingToday
}

func buildEmail(ipos []IPO, today time.Time) string {
	var body strings.Builder
	fmt.Fprintf(&body, "IPOs closing on %s\n\n", today.Format("2 January 2006"))
	if len(ipos) == 0 {
		body.WriteString("No IPOs are scheduled to close today.\n")
		return body.String()
	}
	for _, ipo := range ipos {
		fmt.Fprintf(&body, "IPO Name: %s\nRetail Subscription: %s x\nGMP: %s\n\n", ipo.Name, ipo.RetailSubscription, ipo.GreyMarketPremium)
	}
	body.WriteString("Source: IPOWatch. GMP is unofficial and may change.\n")
	return body.String()
}

func sendReport(ipos []IPO, today time.Time) error {
	if len(ipos) == 0 {
		log.Print("No IPOs are scheduled to close today; skipping email.")
		return nil
	}
	return sendEmail(buildEmail(ipos, today), today)
}

func sendEmail(body string, today time.Time) error {
	host := os.Getenv("SMTP_HOST")
	port := envOrDefault("SMTP_PORT", "587")
	username := os.Getenv("SMTP_USERNAME")
	password := os.Getenv("SMTP_PASSWORD")
	from := os.Getenv("MAIL_FROM")
	recipients := parseRecipients(os.Getenv("MAIL_TO"))
	if host == "" || strings.TrimSpace(from) == "" || len(recipients) == 0 {
		return fmt.Errorf("SMTP_HOST, MAIL_FROM, and MAIL_TO must be configured")
	}
	if _, err := strconv.Atoi(port); err != nil {
		return fmt.Errorf("SMTP_PORT must be a number: %w", err)
	}

	message := strings.Join([]string{
		"From: " + from,
		"To: " + strings.Join(recipients, ", "),
		"Subject: IPOs closing today - " + today.Format("2 Jan 2006"),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		body,
	}, "\r\n")

	var auth smtp.Auth
	if username != "" || password != "" {
		if username == "" || password == "" {
			return fmt.Errorf("SMTP_USERNAME and SMTP_PASSWORD must both be set")
		}
		auth = smtp.PlainAuth("", username, password, host)
	}
	if err := smtp.SendMail(host+":"+port, auth, from, recipients, []byte(message)); err != nil {
		return fmt.Errorf("send email: %w", err)
	}
	return nil
}

func parseRecipients(value string) []string {
	return trimRecipients(strings.Split(value, ","))
}

func trimRecipients(recipients []string) []string {
	trimmed := make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		if recipient = strings.TrimSpace(recipient); recipient != "" {
			trimmed = append(trimmed, recipient)
		}
	}
	return trimmed
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

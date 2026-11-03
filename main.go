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
	"regexp"
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

var gmpPercentPattern = regexp.MustCompile(`([+-]?[0-9]+[.]?[0-9]*)%`)

type IPO struct {
	Name                      string
	Category                  string
	RetailSubscription        string
	SmallNIISubscription      string
	GreyMarketPremium         string
	GreyMarketPremiumPercent  string
	ClosingDate               time.Time
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
	gmpIPOs, err := parseGMPTable(gmpHTML)
	if err != nil {
		return err
	}

	today := time.Now().In(location)
	closingIPOs := selectClosingToday(subscriptions, gmpIPOs, today)
	for index := range closingIPOs {
		detailHTML, err := fetchPage(ipoDetailsURL(closingIPOs[index].Name))
		if err != nil {
			log.Printf("fetch sNII subscription for %s: %v", closingIPOs[index].Name, err)
			closingIPOs[index].SmallNIISubscription = "Not available"
			continue
		}
		closingIPOs[index].SmallNIISubscription, err = parseSmallNIISubscription(detailHTML)
		if err != nil {
			log.Printf("parse sNII subscription for %s: %v", closingIPOs[index].Name, err)
			closingIPOs[index].SmallNIISubscription = "Not available"
		}
	}
	message := buildEmail(closingIPOs, today)
	if strings.EqualFold(os.Getenv("DRY_RUN"), "true") {
		fmt.Print(message)
		return nil
	}
	return sendReport(closingIPOs, today)
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

func parseGMPTable(page []byte) ([]IPO, error) {
	doc, err := html.Parse(bytes.NewReader(page))
	if err != nil {
		return nil, fmt.Errorf("parse GMP page: %w", err)
	}

	var ipos []IPO
	for _, row := range mainboardTableRows(doc) {
		if len(row) < 6 {
			continue
		}
		name := stripIPOStatus(row[0])
		if name == "" {
			continue
		}
		ipo := IPO{
			Name:              name,
			Category:          "Mainboard",
			GreyMarketPremium: strings.TrimSpace(row[1]),
		}
		if match := gmpPercentPattern.FindStringSubmatch(row[4]); len(match) == 2 {
			ipo.GreyMarketPremiumPercent = match[1] + "%"
		} else {
			ipo.GreyMarketPremiumPercent = "Not available"
		}
		ipos = append(ipos, ipo)
	}
	if len(ipos) == 0 {
		return nil, fmt.Errorf("no Mainboard IPO rows found on GMP page; the page layout may have changed")
	}
	return ipos, nil
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

func mainboardTableRows(doc *html.Node) [][]string {
	var rows [][]string
	inMainboardSection := false
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.ElementNode && len(node.Data) == 2 && node.Data[0] == 'h' && node.Data[1] >= '1' && node.Data[1] <= '6' {
			heading := strings.ToLower(strings.TrimSpace(nodeText(node)))
			if strings.Contains(heading, "current mainboard ipo") {
				inMainboardSection = true
			} else if strings.Contains(heading, "current sme ipo") {
				inMainboardSection = false
			}
		}
		if node.Type == html.ElementNode && node.Data == "tr" {
			if inMainboardSection {
				var cells []string
				for child := node.FirstChild; child != nil; child = child.NextSibling {
					if child.Type == html.ElementNode && (child.Data == "td" || child.Data == "th") {
						cells = append(cells, nodeText(child))
					}
				}
				if len(cells) > 0 {
					rows = append(rows, cells)
				}
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

func stripIPOStatus(value string) string {
	value = strings.TrimSpace(value)
	for _, status := range []string{" Open", " Closed"} {
		if strings.HasSuffix(strings.ToLower(value), strings.ToLower(status)) {
			return strings.TrimSpace(value[:len(value)-len(status)])
		}
	}
	return ""
}

func ipoDetailsURL(name string) string {
	var slug strings.Builder
	separatorPending := false
	for _, character := range strings.ToLower(name) {
		if unicode.IsLetter(character) || unicode.IsNumber(character) {
			if separatorPending && slug.Len() > 0 {
				slug.WriteByte('-')
			}
			slug.WriteRune(character)
			separatorPending = false
		} else if unicode.IsSpace(character) || character == '-' {
			separatorPending = true
		}
	}
	return "https://ipowatch.live/" + slug.String() + "-ipo-details/"
}

func parseSmallNIISubscription(page []byte) (string, error) {
	doc, err := html.Parse(bytes.NewReader(page))
	if err != nil {
		return "", fmt.Errorf("parse IPO detail page: %w", err)
	}
	for _, row := range tableRows(doc) {
		if len(row) < 2 || !strings.HasPrefix(normalizeName(row[0]), "snii") {
			continue
		}
		value := strings.TrimSpace(row[1])
		if strings.HasSuffix(strings.ToLower(value), "x") {
			return value, nil
		}
	}
	return "Not available", nil
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

func selectClosingToday(subscriptions, gmpIPOs []IPO, today time.Time) []IPO {
	gmpByName := make(map[string]IPO, len(gmpIPOs))
	for _, ipo := range gmpIPOs {
		gmpByName[normalizeName(ipo.Name)] = ipo
	}

	closingIPOs := make([]IPO, 0)
	for _, subscription := range subscriptions {
		if !strings.EqualFold(strings.TrimSpace(subscription.Category), "Mainboard") || !sameDate(subscription.ClosingDate, today) {
			continue
		}
		subscription.RetailSubscription = strings.TrimSpace(subscription.RetailSubscription)
		subscription.GreyMarketPremium = "Not listed"
		subscription.GreyMarketPremiumPercent = "Not available"
		if gmp, ok := findIPOByName(subscription.Name, gmpByName); ok {
			subscription.Name = gmp.Name
			subscription.GreyMarketPremium = gmp.GreyMarketPremium
			subscription.GreyMarketPremiumPercent = gmp.GreyMarketPremiumPercent
		}
		closingIPOs = append(closingIPOs, subscription)
	}
	return closingIPOs
}

func findIPOByName(name string, iposByName map[string]IPO) (IPO, bool) {
	key := normalizeName(name)
	if ipo, ok := iposByName[key]; ok {
		return ipo, true
	}

	var match IPO
	found := false
	for candidateName, ipo := range iposByName {
		if len(candidateName) < 8 || len(key) < 8 {
			continue
		}
		if strings.HasPrefix(key, candidateName) || strings.HasPrefix(candidateName, key) {
			if found {
				return IPO{}, false
			}
			match = ipo
			found = true
		}
	}
	return match, found
}

func buildEmail(ipos []IPO, today time.Time) string {
	var body strings.Builder
	fmt.Fprintf(&body, "Mainboard IPOs closing on %s\n\n", today.Format("2 January 2006"))
	if len(ipos) == 0 {
		body.WriteString("No Mainboard IPOs are listed today.\n")
		return body.String()
	}
	for _, ipo := range ipos {
		fmt.Fprintf(&body, "IPO Name: %s\nRetail Subscription: %s x\nsNII Subscription: %s\nGMP: %s (%s)\n\n", ipo.Name, ipo.RetailSubscription, ipo.SmallNIISubscription, ipo.GreyMarketPremium, ipo.GreyMarketPremiumPercent)
	}
	body.WriteString("Source: IPOWatch. GMP is unofficial and may change.\n")
	return body.String()
}

func sendReport(ipos []IPO, today time.Time) error {
	if len(ipos) == 0 {
		log.Print("No Mainboard IPOs are scheduled to close today; skipping email.")
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
		"Subject: Mainboard IPOs closing today - " + today.Format("2 Jan 2006"),
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

package main

import (
	"strings"
	"testing"
	"time"
)

func TestParseSubscriptionTable(t *testing.T) {
	page := []byte(`<table><tbody><tr><td>Test IPO</td><td>SME</td><td>November 3, 2026</td><td>1.0</td><td>2.0</td><td>3.25</td><td>2.5</td><td>12:00</td></tr></tbody></table>`)
	ipos, err := parseSubscriptionTable(page)
	if err != nil {
		t.Fatal(err)
	}
	if len(ipos) != 1 || ipos[0].Name != "Test IPO" || ipos[0].Category != "SME" || ipos[0].RetailSubscription != "3.25" {
		t.Fatalf("unexpected IPO data: %#v", ipos)
	}
	if got := ipos[0].ClosingDate.Format("2006-01-02"); got != "2026-11-03" {
		t.Fatalf("closing date = %s, want 2026-11-03", got)
	}
}

func TestParseGMPTable(t *testing.T) {
	page := []byte(`<table><tbody><tr><td>Test IPO Open</td><td>₹15</td><td>🟢</td><td>₹94</td><td>₹109 (15.96%)</td><td>2-4 Nov</td></tr></tbody></table>`)
	premiums, err := parseGMPTable(page)
	if err != nil {
		t.Fatal(err)
	}
	if got := premiums[normalizeName("Test IPO")]; got != "₹15" {
		t.Fatalf("GMP = %q, want ₹15", got)
	}
}

func TestBuildEmailForNoClosingIPOs(t *testing.T) {
	message := buildEmail(nil, time.Date(2026, time.November, 3, 12, 0, 0, 0, time.FixedZone("IST", 5*60*60+30*60)))
	if !strings.Contains(message, "No IPOs are scheduled to close today") {
		t.Fatalf("unexpected empty report: %s", message)
	}
}

func TestSendReportSkipsEmailWhenNoIPOsClose(t *testing.T) {
	today := time.Date(2026, time.November, 3, 12, 0, 0, 0, time.FixedZone("IST", 5*60*60+30*60))
	if err := sendReport(nil, today); err != nil {
		t.Fatalf("empty report should skip SMTP without requiring configuration: %v", err)
	}
}

func TestParseRecipients(t *testing.T) {
	recipients := parseRecipients("first@example.com, second@example.com ,")
	if len(recipients) != 2 || recipients[0] != "first@example.com" || recipients[1] != "second@example.com" {
		t.Fatalf("unexpected recipients: %#v", recipients)
	}
}

func TestSelectClosingToday(t *testing.T) {
	today := time.Date(2026, time.November, 3, 12, 0, 0, 0, time.FixedZone("IST", 5*60*60+30*60))
	ipos := []IPO{
		{Name: "Closing Mainboard IPO", Category: "Mainboard", ClosingDate: today, RetailSubscription: "3.25"},
		{Name: "Closing SME IPO", Category: "SME", ClosingDate: today},
		{Name: "Tomorrow Mainboard IPO", Category: "Mainboard", ClosingDate: today.AddDate(0, 0, 1)},
	}
	selected := selectClosingToday(ipos, map[string]string{normalizeName("Closing Mainboard IPO"): "₹15"}, today)
	if len(selected) != 1 || selected[0].Name != "Closing Mainboard IPO" || selected[0].GreyMarketPremium != "₹15" {
		t.Fatalf("unexpected closing IPOs: %#v", selected)
	}
}

func TestNormalizeNameIgnoresPunctuationAndCase(t *testing.T) {
	if normalizeName("M.K. Sons Fine Jewels") != normalizeName("m k sons fine-jewels") {
		t.Fatal("equivalent IPO names should normalize to the same key")
	}
}

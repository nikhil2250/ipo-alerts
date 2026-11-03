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
	page := []byte(`<h3>Current Mainboard IPO</h3><table><tbody><tr><td>Closed IPO Closed</td><td>₹9</td><td>🟢</td><td>₹82</td><td>₹91 (10.98%)</td><td>29-2 Nov</td></tr></tbody></table><h3>Current SME IPO</h3><table><tbody><tr><td>SME IPO Open</td><td>₹99</td><td>🟢</td><td>₹100</td><td>₹199 (99.00%)</td><td>2-4 Nov</td></tr></tbody></table>`)
	ipos, err := parseGMPTable(page)
	if err != nil {
		t.Fatal(err)
	}
	if len(ipos) != 1 || ipos[0].Name != "Closed IPO" || ipos[0].GreyMarketPremium != "₹9" || ipos[0].GreyMarketPremiumPercent != "10.98%" {
		t.Fatalf("unexpected Mainboard IPO data: %#v", ipos)
	}
}

func TestParseSmallNIISubscription(t *testing.T) {
	page := []byte(`<table><tr><td>sNII &lt; ₹10L</td><td>8,50,000</td><td>0.00%</td></tr></table><table><tr><td>sNII &lt; ₹10L</td><td>11.71x</td><td>8,50,000</td><td>99,49,576</td><td>₹161.18 Cr</td></tr></table>`)
	got, err := parseSmallNIISubscription(page)
	if err != nil {
		t.Fatal(err)
	}
	if got != "11.71x" {
		t.Fatalf("sNII subscription = %q, want 11.71x", got)
	}
}

func TestIPODetailsURL(t *testing.T) {
	if got := ipoDetailsURL("M.K.Sons Fine Jewels"); got != "https://ipowatch.live/mksons-fine-jewels-ipo-details/" {
		t.Fatalf("detail URL = %q", got)
	}
}

func TestBuildEmailForNoMainboardIPOs(t *testing.T) {
	message := buildEmail(nil, time.Date(2026, time.November, 3, 12, 0, 0, 0, time.FixedZone("IST", 5*60*60+30*60)))
	if !strings.Contains(message, "No Mainboard IPOs are listed today") {
		t.Fatalf("unexpected empty report: %s", message)
	}
}

func TestBuildEmailIncludesRequestedMetrics(t *testing.T) {
	ipo := IPO{
		Name:                     "Test IPO",
		RetailSubscription:       "3.08",
		SmallNIISubscription:     "11.71x",
		GreyMarketPremium:        "₹9",
		GreyMarketPremiumPercent: "10.98%",
	}
	message := buildEmail([]IPO{ipo}, time.Date(2026, time.November, 3, 12, 0, 0, 0, time.UTC))
	for _, expected := range []string{"Retail Subscription: 3.08 x", "sNII Subscription: 11.71x", "GMP: ₹9 (10.98%)"} {
		if !strings.Contains(message, expected) {
			t.Errorf("email missing %q: %s", expected, message)
		}
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
	tomorrow := today.AddDate(0, 0, 1)
	subscriptions := []IPO{
		{Name: "First Closed IPO", Category: "Mainboard", ClosingDate: today, RetailSubscription: "1.25"},
		{Name: "SFC Environmental", Category: "Mainboard", ClosingDate: today, RetailSubscription: "2.71"},
		{Name: "Third Closing IPO", Category: "Mainboard", ClosingDate: today, RetailSubscription: "3.50"},
		{Name: "Fourth Closing IPO", Category: "Mainboard", ClosingDate: today, RetailSubscription: "4.75"},
		{Name: "SME IPO", Category: "SME", ClosingDate: today, RetailSubscription: "50.00"},
		{Name: "Tomorrow IPO", Category: "Mainboard", ClosingDate: tomorrow, RetailSubscription: "9.00"},
	}
	gmpIPOs := []IPO{
		{Name: "First Closed IPO", Category: "Mainboard", GreyMarketPremium: "₹9", GreyMarketPremiumPercent: "10.98%"},
		{Name: "SFC Environmental Technologies", Category: "Mainboard", GreyMarketPremium: "₹15", GreyMarketPremiumPercent: "4.55%"},
		{Name: "Third Closing IPO", Category: "Mainboard", GreyMarketPremium: "₹5", GreyMarketPremiumPercent: "5.00%"},
		{Name: "Fourth Closing IPO", Category: "Mainboard", GreyMarketPremium: "₹7", GreyMarketPremiumPercent: "7.00%"},
	}
	selected := selectClosingToday(subscriptions, gmpIPOs, today)
	if len(selected) != 4 {
		t.Fatalf("got %d IPOs, want all 4 Mainboard IPOs closing today: %#v", len(selected), selected)
	}
	if selected[1].Name != "SFC Environmental Technologies" || selected[1].RetailSubscription != "2.71" || selected[1].GreyMarketPremium != "₹15" || selected[1].GreyMarketPremiumPercent != "4.55%" {
		t.Fatalf("IPO metrics were not joined correctly: %#v", selected[1])
	}
	if selected[3].Name != "Fourth Closing IPO" {
		t.Fatalf("closing IPOs were unexpectedly limited: %#v", selected)
	}
}

func TestNormalizeNameIgnoresPunctuationAndCase(t *testing.T) {
	if normalizeName("M.K. Sons Fine Jewels") != normalizeName("m k sons fine-jewels") {
		t.Fatal("equivalent IPO names should normalize to the same key")
	}
}

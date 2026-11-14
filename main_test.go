package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func floatPtr(value float64) *float64 { return &value }

func TestHTMLToTextRemovesMarkupAndNonVisibleBlocks(t *testing.T) {
	page := []byte("<html><style>.x{display:none}</style><script>alert('x')</script><!--comment--><body><h1>Test IPO</h1><p>Retail &amp; NII 3.42x</p></body></html>")
	got := htmlToText(page)
	for _, want := range []string{"Test IPO", "Retail & NII 3.42x"} {
		if !strings.Contains(got, want) { t.Fatalf("text %q does not contain %q", got, want) }
	}
	for _, unwanted := range []string{"alert", "display:none", "comment", "<h1>"} {
		if strings.Contains(got, unwanted) { t.Fatalf("text still contains %q: %s", unwanted, got) }
	}
}

func TestExtractedIPOJSONUnmarshal(t *testing.T) {
	data := []byte("{\"ipos\":[{\"name\":\"Test IPO\",\"category\":\"Mainboard\",\"closing_date\":\"2026-11-14\",\"retail_subscription\":3.42,\"snii_subscription\":null,\"evidence\":{\"name\":\"Test IPO\"}}]}")
	var response aiPageResponse
	if err := json.Unmarshal(data, &response); err != nil { t.Fatal(err) }
	if len(response.IPOs) != 1 { t.Fatalf("got %d IPOs, want 1", len(response.IPOs)) }
	got := response.IPOs[0]
	if got.Name != "Test IPO" || got.Category != "Mainboard" || got.ClosingDate != "2026-11-14" { t.Fatalf("unexpected IPO: %#v", got) }
	if got.Retail == nil || *got.Retail != 3.42 { t.Fatalf("retail = %v, want 3.42", got.Retail) }
	if got.SmallNII != nil { t.Fatalf("missing sNII should be nil, got %v", *got.SmallNII) }
}

func validSubscriptionIPO() (ExtractedIPO, string) {
	source := "Apex Technologies IPO Mainboard opening date 12 November 2026 closing date 14 November 2026 Retail subscription 3.42x sNII subscription 8.15x bNII subscription 11.20x QIB subscription 5.60x total subscription 6.75x"
	row := source
	item := ExtractedIPO{
		Name: "Apex Technologies IPO",
		Category: "Mainboard",
		OpeningDate: "2026-11-12",
		ClosingDate: "2026-11-14",
		Retail: floatPtr(3.42),
		SmallNII: floatPtr(8.15),
		BigNII: floatPtr(11.20),
		QIB: floatPtr(5.60),
		Total: floatPtr(6.75),
		Evidence: map[string]string{
			"name": row,
			"category": row,
			"opening_date": row,
			"closing_date": row,
			"retail_subscription": row,
			"snii_subscription": row,
			"bnii_subscription": row,
			"qib_subscription": row,
			"total_subscription": row,
		},
	}
	return item, source
}

func TestValidateExtractedIPOAcceptsVerifiableSubscription(t *testing.T) {
	item, source := validSubscriptionIPO()
	if err := validateExtractedIPO(item, source, "subscription"); err != nil { t.Fatalf("valid data rejected: %v", err) }
}

func TestValidateExtractedIPORejectsUnsupportedEvidence(t *testing.T) {
	item, source := validSubscriptionIPO()
	item.Retail = floatPtr(9.99)
	if err := validateExtractedIPO(item, source, "subscription"); err == nil || !strings.Contains(err.Error(), "evidence") {
		t.Fatalf("expected evidence mismatch, got %v", err)
	}
}

func TestValidateExtractedIPORejectsTotalNIIAsSNIIWithoutEvidence(t *testing.T) {
	item, source := validSubscriptionIPO()
	item.SmallNII = floatPtr(11.20)
	if err := validateExtractedIPO(item, source, "subscription"); err == nil {
		t.Fatal("expected mismatched sNII evidence to be rejected")
	}
}

func TestConvertSubscriptionRows(t *testing.T) {
	item, _ := validSubscriptionIPO()
	location := time.FixedZone("IST", 5*60*60+30*60)
	got, err := convertSubscriptionRows([]ExtractedIPO{item}, location)
	if err != nil { t.Fatal(err) }
	if len(got) != 1 || got[0].Name != item.Name { t.Fatalf("unexpected result: %#v", got) }
	if got[0].RetailSubscription != "3.42" || got[0].SmallNIISubscription != "8.15" || got[0].BigNIISubscription != "11.2" {
		t.Fatalf("unexpected subscription values: %#v", got[0])
	}
}

func TestSelectClosingTodayKeepsOnlyMainboard(t *testing.T) {
	ist := time.FixedZone("IST", 5*60*60+30*60)
	today := time.Date(2026, time.November, 14, 12, 0, 0, 0, ist)
	tomorrow := today.AddDate(0, 0, 1)
	subscriptions := []IPO{
		{Name: "Apex Technologies IPO", Category: "Mainboard", ClosingDate: today},
		{Name: "SME Example IPO", Category: "SME", ClosingDate: today},
		{Name: "Tomorrow IPO", Category: "Mainboard", ClosingDate: tomorrow},
	}
	gmp := []ExtractedIPO{{Name: "Apex Technologies IPO", GMP: floatPtr(25), GMPPercent: floatPtr(12.5)}}
	got := selectClosingToday(subscriptions, gmp, today, ist)
	if len(got) != 1 { t.Fatalf("got %d records, want 1: %#v", len(got), got) }
	if got[0].GreyMarketPremium != "₹25" || got[0].GreyMarketPremiumPercent != "12.5%" { t.Fatalf("GMP not joined: %#v", got[0]) }
}

func TestFindExtractedIPORejectsAmbiguousPrefix(t *testing.T) {
	rows := []ExtractedIPO{{Name: "Apex Technologies"},{Name: "Apex Technologies Limited"}}
	if _, ok := findExtractedIPO("Apex Technologies", rows); ok { t.Fatal("ambiguous match should be rejected") }
}

func TestBuildEmailIncludesAllMetricsAndSources(t *testing.T) {
	today := time.Date(2026, time.November, 14, 12, 0, 0, 0, time.UTC)
	ipo := IPO{
		Name: "Apex Technologies IPO",
		ClosingDate: today,
		RetailSubscription: "3.42",
		SmallNIISubscription: "8.15",
		BigNIISubscription: "11.2",
		QIBSubscription: "5.6",
		TotalSubscription: "6.75",
		GreyMarketPremium: "₹25",
		GreyMarketPremiumPercent: "12.5%",
		SubscriptionSourceURL: subscriptionURL,
		GMPSourceURL: gmpURL,
	}
	message := buildEmail([]IPO{ipo}, today)
	for _, want := range []string{"Retail Subscription: 3.42x","sNII Subscription: 8.15x","bNII Subscription: 11.2x","QIB Subscription: 5.6x","Total Subscription: 6.75x","GMP: ₹25 (12.5%)",subscriptionURL,gmpURL} {
		if !strings.Contains(message, want) { t.Errorf("email missing %q: %s", want, message) }
	}
}

func TestBuildEmailForNoIPOs(t *testing.T) {
	message := buildEmail(nil, time.Now())
	if !strings.Contains(message, "No Mainboard IPOs are scheduled to close today") { t.Fatalf("unexpected empty report: %s", message) }
}

func TestSendReportSkipsEmailWhenNoIPOsClose(t *testing.T) {
	if err := sendReport(nil, time.Now()); err != nil { t.Fatalf("empty report should skip SMTP: %v", err) }
}

func TestParseRecipients(t *testing.T) {
	recipients := parseRecipients("first@example.com, second@example.com ,")
	if len(recipients) != 2 || recipients[0] != "first@example.com" || recipients[1] != "second@example.com" { t.Fatalf("unexpected recipients: %#v", recipients) }
}

func TestIPODetailsURL(t *testing.T) {
	if got := ipoDetailsURL("M.K.Sons Fine Jewels"); got != "https://ipowatch.live/mksons-fine-jewels-ipo-details/" { t.Fatalf("detail URL = %q", got) }
}

func TestExtractIPOsWithAIMockedNVIDIA(t *testing.T) {
	t.Setenv("NVIDIA_API_KEY", "test-key")
	t.Setenv("NVIDIA_MODEL", "test-model")
	originalURL := nvidiaChatCompletionsURL
	defer func() { nvidiaChatCompletionsURL = originalURL }()

	item, source := validSubscriptionIPO()
	responseJSON, err := json.Marshal(map[string]any{
		"ipos": []any{map[string]any{
			"name": item.Name, "category": item.Category, "opening_date": item.OpeningDate,
			"closing_date": item.ClosingDate, "retail_subscription": item.Retail,
			"snii_subscription": item.SmallNII, "bnii_subscription": item.BigNII,
			"qib_subscription": item.QIB, "total_subscription": item.Total,
			"evidence": item.Evidence,
		}},
	})
	if err != nil { t.Fatal(err) }

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost { t.Errorf("method = %s, want POST", r.Method) }
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" { t.Errorf("authorization = %q", got) }
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil { t.Errorf("decode NVIDIA request: %v", err) }
		if request["model"] != "test-model" { t.Errorf("model = %v, want test-model", request["model"]) }
		_ = source
		_, _ = fmt.Fprint(w, string(mustJSON(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": string(responseJSON)}}}})))
	}))
	defer server.Close()
	nvidiaChatCompletionsURL = server.URL

	rows, err := extractIPOsWithAI(context.Background(), "subscription", "", subscriptionURL, []byte(source))
	if err != nil { t.Fatalf("AI extraction failed: %v", err) }
	if len(rows) != 1 || rows[0].Name != item.Name { t.Fatalf("unexpected extraction: %#v", rows) }
}

func TestExtractIPOsWithAIRejectsInvalidEvidence(t *testing.T) {
	t.Setenv("NVIDIA_API_KEY", "test-key")
	t.Setenv("NVIDIA_MODEL", "test-model")
	originalURL := nvidiaChatCompletionsURL
	defer func() { nvidiaChatCompletionsURL = originalURL }()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content := "{\"ipos\":[{\"name\":\"Wrong IPO\",\"category\":\"Mainboard\",\"closing_date\":\"2026-11-14\",\"evidence\":{\"name\":\"Wrong IPO\",\"category\":\"Wrong IPO Mainboard\",\"closing_date\":\"Wrong IPO closing 14 November 2026\"}}]}"
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}}}})
	}))
	defer server.Close()
	nvidiaChatCompletionsURL = server.URL
	_, err := extractIPOsWithAI(context.Background(), "subscription", "", subscriptionURL, []byte("Page says Other IPO Mainboard closes 14 November 2026 with enough descriptive text to exceed the minimum characters."))
	if err == nil { t.Fatal("expected source validation error") }
}

func mustJSON(value any) []byte {
	data, err := json.Marshal(value)
	if err != nil { panic(err) }
	return data
}

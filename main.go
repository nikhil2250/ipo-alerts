package main

import (
 "bytes"
 "context"
 stdhtml "html"
 "encoding/json"
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
)

const (
 gmpURL = "https://ipowatch.in/ipo-grey-market-premium-latest-ipo-gmp/"
 subscriptionURL = "https://ipowatch.in/ipo-subscription-status-today/"
 defaultTimezone = "Asia/Kolkata"
 defaultNVIDIAModel = "nvidia/nemotron-3.5-lightning-30b-a3b"
)

var (
 nvidiaChatCompletionsURL = "https://integrate.api.nvidia.com/v1/chat/completions"
 scriptRE = regexp.MustCompile("(?is)<script\\b[^>]*>.*?</script\\s*>")
 styleRE = regexp.MustCompile("(?is)<style\\b[^>]*>.*?</style\\s*>")
 commentRE = regexp.MustCompile("(?s)<!--.*?-->")
 tagRE = regexp.MustCompile("<[^>]+>")
 spaceRE = regexp.MustCompile("\\s+")
 numberRE = regexp.MustCompile("-?[0-9]+(?:,[0-9]{3})*(?:\\.[0-9]+)?")
)

type IPO struct {
 Name string
 Category string
 OpeningDate time.Time
 ClosingDate time.Time
 RetailSubscription string
 SmallNIISubscription string
 BigNIISubscription string
 QIBSubscription string
 TotalSubscription string
 GreyMarketPremium string
 GreyMarketPremiumPercent string
 SubscriptionSourceURL string
 GMPSourceURL string
 DetailSourceURL string
}

type ExtractedIPO struct {
 Name string
 Category string
 OpeningDate string
 ClosingDate string
 Retail *float64
 SmallNII *float64
 BigNII *float64
 QIB *float64
 Total *float64
 GMP *float64
 GMPPercent *float64
 Evidence map[string]string
}

type aiPageResponse struct { IPOs []ExtractedIPO }

func (item *ExtractedIPO) UnmarshalJSON(data []byte) error {
 var fields map[string]json.RawMessage
 if err := json.Unmarshal(data, &fields); err != nil { return err }
 readString := func(key string, target *string) error {
  if raw, ok := fields[key]; ok && string(raw) != "null" { return json.Unmarshal(raw, target) }
  return nil
 }
 readNumber := func(key string, target **float64) error {
  if raw, ok := fields[key]; ok && string(raw) != "null" {
   var value float64
   if err := json.Unmarshal(raw, &value); err != nil { return fmt.Errorf("%s must be numeric or null: %w", key, err) }
   *target = &value
  }
  return nil
 }
 for key, target := range map[string]*string{
  "name": &item.Name, "category": &item.Category,
  "opening_date": &item.OpeningDate, "closing_date": &item.ClosingDate,
 } {
  if err := readString(key, target); err != nil { return err }
 }
 for key, target := range map[string]**float64{
  "retail_subscription": &item.Retail, "snii_subscription": &item.SmallNII,
  "bnii_subscription": &item.BigNII, "qib_subscription": &item.QIB,
  "total_subscription": &item.Total, "gmp": &item.GMP,
  "gmp_percentage": &item.GMPPercent,
 } {
  if err := readNumber(key, target); err != nil { return err }
 }
 if raw, ok := fields["evidence"]; ok {
  if err := json.Unmarshal(raw, &item.Evidence); err != nil { return fmt.Errorf("decode evidence: %w", err) }
 }
 return nil
}

func main() {
 if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) { log.Fatalf("load .env: %v", err) }
 if err := run(); err != nil { log.Fatal(err) }
}

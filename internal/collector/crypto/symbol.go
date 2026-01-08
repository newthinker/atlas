package crypto

import (
	"fmt"
	"regexp"
	"strings"
)

// Common quote currencies in order of priority for detection
var quoteCurrencies = []string{"USDT", "BUSD", "USDC", "BTC", "ETH", "BNB"}

// validSymbol matches crypto trading pairs
var validCryptoSymbol = regexp.MustCompile(`^[A-Za-z0-9]{2,20}$`)

// NormalizeSymbol converts various input formats to standard format (e.g., BTCUSDT)
// Input formats: "BTC", "btc", "BTC-USDT", "BTC/USDT", "btcusdt"
// Output: "BTCUSDT"
func NormalizeSymbol(input string, defaultQuote string) string {
	if input == "" {
		return ""
	}

	// Uppercase and remove common separators
	s := strings.ToUpper(input)
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "/", "")
	s = strings.ReplaceAll(s, "_", "")

	// Check if already contains a quote currency
	// Ensure there's a base currency left (symbol must be longer than quote)
	for _, quote := range quoteCurrencies {
		if strings.HasSuffix(s, quote) && len(s) > len(quote) {
			return s
		}
	}

	// No quote currency found, append default
	return s + strings.ToUpper(defaultQuote)
}

// ParseSymbol extracts base and quote from a normalized symbol
// "BTCUSDT" -> ("BTC", "USDT")
func ParseSymbol(symbol string) (base, quote string) {
	s := strings.ToUpper(symbol)

	// Try to find known quote currency
	// Ensure there's a base currency left (symbol must be longer than quote)
	for _, q := range quoteCurrencies {
		if strings.HasSuffix(s, q) && len(s) > len(q) {
			return strings.TrimSuffix(s, q), q
		}
	}

	// Fallback: assume last 4 chars are quote (USDT, BUSD, etc.)
	if len(s) > 4 {
		return s[:len(s)-4], s[len(s)-4:]
	}

	return s, ""
}

// FormatDisplay converts internal format to display format
// "BTCUSDT" -> "BTC/USDT"
func FormatDisplay(symbol string) string {
	base, quote := ParseSymbol(symbol)
	if quote == "" {
		return base
	}
	return base + "/" + quote
}

// ValidateCryptoSymbol checks if a symbol has valid format
func ValidateCryptoSymbol(symbol string) error {
	if symbol == "" {
		return fmt.Errorf("symbol cannot be empty")
	}
	if len(symbol) > 30 {
		return fmt.Errorf("symbol too long: %s", symbol)
	}

	// Remove separators for validation
	s := strings.ReplaceAll(symbol, "-", "")
	s = strings.ReplaceAll(s, "/", "")
	s = strings.ReplaceAll(s, "_", "")

	if !validCryptoSymbol.MatchString(s) {
		return fmt.Errorf("invalid symbol format: %s", symbol)
	}
	return nil
}

// internal/context/news_test.go
package context

import (
	"context"
	"testing"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

func TestStaticNewsProvider_GetNews(t *testing.T) {
	news := []NewsItem{
		{
			Title:       "Stock rises",
			Symbols:     []string{"AAPL"},
			PublishedAt: time.Now().Add(-24 * time.Hour),
		},
		{
			Title:       "Old news",
			Symbols:     []string{"AAPL"},
			PublishedAt: time.Now().Add(-30 * 24 * time.Hour),
		},
		{
			Title:       "Other stock",
			Symbols:     []string{"GOOG"},
			PublishedAt: time.Now().Add(-24 * time.Hour),
		},
	}

	provider := NewStaticNewsProvider(news)
	ctx := context.Background()

	result, err := provider.GetNews(ctx, "AAPL", 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 1 {
		t.Errorf("expected 1 news item, got %d", len(result))
	}
	if result[0].Title != "Stock rises" {
		t.Errorf("expected 'Stock rises', got %s", result[0].Title)
	}
}

func TestStaticNewsProvider_GetMarketNews(t *testing.T) {
	news := []NewsItem{
		{
			Title:       "Market update",
			Symbols:     []string{}, // Market-wide
			PublishedAt: time.Now().Add(-24 * time.Hour),
		},
		{
			Title:       "Stock specific",
			Symbols:     []string{"AAPL"},
			PublishedAt: time.Now().Add(-24 * time.Hour),
		},
	}

	provider := NewStaticNewsProvider(news)
	ctx := context.Background()

	result, err := provider.GetMarketNews(ctx, core.MarketUS, 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 1 {
		t.Errorf("expected 1 market news item, got %d", len(result))
	}
}

func TestCachedNewsProvider(t *testing.T) {
	callCount := 0
	baseProvider := &countingNewsProvider{
		count: &callCount,
	}

	cached := NewCachedNewsProvider(baseProvider, time.Hour)
	ctx := context.Background()

	// First call
	_, err := cached.GetNews(ctx, "AAPL", 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}

	// Second call should use cache
	_, err = cached.GetNews(ctx, "AAPL", 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected still 1 call (cached), got %d", callCount)
	}

	// Different symbol should make new call
	_, err = cached.GetNews(ctx, "GOOG", 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls, got %d", callCount)
	}
}

// Helper provider for testing
type countingNewsProvider struct {
	count *int
}

func (p *countingNewsProvider) GetNews(ctx context.Context, symbol string, days int) ([]NewsItem, error) {
	*p.count++
	return []NewsItem{}, nil
}

func (p *countingNewsProvider) GetMarketNews(ctx context.Context, market core.Market, days int) ([]NewsItem, error) {
	*p.count++
	return []NewsItem{}, nil
}

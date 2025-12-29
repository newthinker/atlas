// internal/context/news.go
package context

import (
	"context"
	"time"

	"github.com/newthinker/atlas/internal/core"
)

// StaticNewsProvider is a simple news provider that returns configured news.
// In production, this would be replaced with actual news API integration.
type StaticNewsProvider struct {
	news []NewsItem
}

// NewStaticNewsProvider creates a news provider with static news items.
func NewStaticNewsProvider(news []NewsItem) *StaticNewsProvider {
	return &StaticNewsProvider{news: news}
}

// GetNews returns news items for the given symbol.
func (p *StaticNewsProvider) GetNews(ctx context.Context, symbol string, days int) ([]NewsItem, error) {
	cutoff := time.Now().AddDate(0, 0, -days)
	var result []NewsItem

	for _, item := range p.news {
		if item.PublishedAt.Before(cutoff) {
			continue
		}
		for _, s := range item.Symbols {
			if s == symbol {
				result = append(result, item)
				break
			}
		}
	}

	return result, nil
}

// GetMarketNews returns news items for the given market.
func (p *StaticNewsProvider) GetMarketNews(ctx context.Context, market core.Market, days int) ([]NewsItem, error) {
	cutoff := time.Now().AddDate(0, 0, -days)
	var result []NewsItem

	for _, item := range p.news {
		if item.PublishedAt.Before(cutoff) {
			continue
		}
		// If no symbols specified, treat as market-wide news
		if len(item.Symbols) == 0 {
			result = append(result, item)
		}
	}

	return result, nil
}

// CachedNewsProvider wraps a news provider with caching.
type CachedNewsProvider struct {
	provider NewsProvider
	cache    map[string][]NewsItem
	cacheAt  map[string]time.Time
	ttl      time.Duration
}

// NewCachedNewsProvider creates a cached news provider.
func NewCachedNewsProvider(provider NewsProvider, ttl time.Duration) *CachedNewsProvider {
	return &CachedNewsProvider{
		provider: provider,
		cache:    make(map[string][]NewsItem),
		cacheAt:  make(map[string]time.Time),
		ttl:      ttl,
	}
}

// GetNews returns cached news or fetches from the underlying provider.
func (p *CachedNewsProvider) GetNews(ctx context.Context, symbol string, days int) ([]NewsItem, error) {
	key := symbol
	if cached, ok := p.cache[key]; ok {
		if time.Since(p.cacheAt[key]) < p.ttl {
			return cached, nil
		}
	}

	news, err := p.provider.GetNews(ctx, symbol, days)
	if err != nil {
		return nil, err
	}

	p.cache[key] = news
	p.cacheAt[key] = time.Now()
	return news, nil
}

// GetMarketNews returns cached market news or fetches from the underlying provider.
func (p *CachedNewsProvider) GetMarketNews(ctx context.Context, market core.Market, days int) ([]NewsItem, error) {
	key := string(market)
	if cached, ok := p.cache[key]; ok {
		if time.Since(p.cacheAt[key]) < p.ttl {
			return cached, nil
		}
	}

	news, err := p.provider.GetMarketNews(ctx, market, days)
	if err != nil {
		return nil, err
	}

	p.cache[key] = news
	p.cacheAt[key] = time.Now()
	return news, nil
}

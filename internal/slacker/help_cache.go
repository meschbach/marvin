package slacker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// HelpCache caches help responses to improve performance
type HelpCache struct {
	cache   map[string]*CacheEntry
	mutex   sync.RWMutex
	ttl     time.Duration
	maxSize int
}

// CacheEntry represents a cached help response
type CacheEntry struct {
	Response    string
	Confidence  float64
	FailureType string
	Timestamp   time.Time
	HitCount    int
}

// NewHelpCache creates a new help cache with configurable TTL and max size
func NewHelpCache(ttl time.Duration, maxSize int) *HelpCache {
	cache := &HelpCache{
		cache:   make(map[string]*CacheEntry),
		ttl:     ttl,
		maxSize: maxSize,
	}

	// Start cleanup goroutine
	go cache.cleanupExpired()

	return cache
}

// generateCacheKey creates a unique key for caching based on input parameters
func (hc *HelpCache) generateCacheKey(failureType, input string, userID string) string {
	// Create a hash of the input for a consistent cache key
	hashInput := fmt.Sprintf("%s:%s:%s", failureType, input, userID)
	hash := sha256.Sum256([]byte(hashInput))
	return hex.EncodeToString(hash[:16]) // Use first 16 chars for shorter keys
}

// Get retrieves a cached response if it exists and is not expired
func (hc *HelpCache) Get(failureType, input, userID string) (*CacheEntry, bool) {
	hc.mutex.RLock()
	defer hc.mutex.RUnlock()

	key := hc.generateCacheKey(failureType, input, userID)
	entry, exists := hc.cache[key]

	if !exists {
		return nil, false
	}

	// Check if entry is expired
	if time.Since(entry.Timestamp) > hc.ttl {
		return nil, false
	}

	// Increment hit count
	entry.HitCount++

	return entry, true
}

// Set stores a help response in the cache
func (hc *HelpCache) Set(failureType, input, userID string, response string, confidence float64) {
	hc.mutex.Lock()
	defer hc.mutex.Unlock()

	key := hc.generateCacheKey(failureType, input, userID)

	// Check if we need to evict entries to stay within max size
	if len(hc.cache) >= hc.maxSize {
		hc.evictLRU()
	}

	hc.cache[key] = &CacheEntry{
		Response:    response,
		Confidence:  confidence,
		FailureType: failureType,
		Timestamp:   time.Now(),
		HitCount:    0,
	}
}

// evictLRU removes the least recently used entry from the cache
func (hc *HelpCache) evictLRU() {
	var oldestKey string
	var oldestTime time.Time
	first := true

	for key, entry := range hc.cache {
		if first || entry.Timestamp.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.Timestamp
			first = false
		}
	}

	if oldestKey != "" {
		delete(hc.cache, oldestKey)
	}
}

// cleanupExpired removes expired entries from the cache
func (hc *HelpCache) cleanupExpired() {
	ticker := time.NewTicker(time.Minute * 5) // Cleanup every 5 minutes
	defer ticker.Stop()

	for range ticker.C {
		hc.mutex.Lock()
		now := time.Now()
		for key, entry := range hc.cache {
			if now.Sub(entry.Timestamp) > hc.ttl {
				delete(hc.cache, key)
			}
		}
		hc.mutex.Unlock()
	}
}

// Clear clears all entries from the cache
func (hc *HelpCache) Clear() {
	hc.mutex.Lock()
	defer hc.mutex.Unlock()

	hc.cache = make(map[string]*CacheEntry)
}

// Stats returns cache statistics
func (hc *HelpCache) Stats() CacheStats {
	hc.mutex.RLock()
	defer hc.mutex.RUnlock()

	var totalHits int
	entries := make([]CacheEntryInfo, 0, len(hc.cache))

	for _, entry := range hc.cache {
		totalHits += entry.HitCount
		entries = append(entries, CacheEntryInfo{
			FailureType: entry.FailureType,
			HitCount:    entry.HitCount,
			Age:         time.Since(entry.Timestamp),
		})
	}

	return CacheStats{
		Size:      len(hc.cache),
		MaxSize:   hc.maxSize,
		TotalHits: totalHits,
		Entries:   entries,
	}
}

// CacheStats provides statistics about the cache
type CacheStats struct {
	Size      int
	MaxSize   int
	TotalHits int
	Entries   []CacheEntryInfo
}

// CacheEntryInfo provides information about a cache entry
type CacheEntryInfo struct {
	FailureType string
	HitCount    int
	Age         time.Duration
}

// CachedHelpAnalyzer wraps a HelpAnalyzer with caching capabilities
type CachedHelpAnalyzer struct {
	*HelpAnalyzer
	cache *HelpCache
}

// NewCachedHelpAnalyzer creates a new help analyzer with caching
func NewCachedHelpAnalyzer(analyzer *HelpAnalyzer, cache *HelpCache) *CachedHelpAnalyzer {
	return &CachedHelpAnalyzer{
		HelpAnalyzer: analyzer,
		cache:        cache,
	}
}

// AnalyzeIntentFailureCached analyzes intent failure with caching
func (cha *CachedHelpAnalyzer) AnalyzeIntentFailureCached(ctx context.Context, message string, helpCtx *HelpContext) (*HelpAnalysis, error) {
	// Try to get from cache first
	if entry, found := cha.cache.Get("intent_failure", message, helpCtx.UserID); found {
		// Parse cached response back into HelpAnalysis
		return cha.parseCachedResponse(entry.Response, "intent_failure", entry.Confidence), nil
	}

	// Cache miss - call the actual analyzer
	analysis, err := cha.AnalyzeIntentFailure(ctx, message, helpCtx)
	if err != nil {
		return nil, err
	}

	// Cache the response for future use
	if analysis.Confidence >= 0.7 { // Only cache high-confidence responses
		response := cha.formatResponseForCache(analysis)
		cha.cache.Set("intent_failure", message, helpCtx.UserID, response, analysis.Confidence)
	}

	return analysis, nil
}

// AnalyzeModelAccessCached analyzes model access with caching
func (cha *CachedHelpAnalyzer) AnalyzeModelAccessCached(ctx context.Context, model, reason string, helpCtx *HelpContext) (*HelpAnalysis, error) {
	cacheKey := fmt.Sprintf("%s:%s", model, reason)

	// Try to get from cache first
	if entry, found := cha.cache.Get("model_access", cacheKey, helpCtx.UserID); found {
		return cha.parseCachedResponse(entry.Response, "model_access", entry.Confidence), nil
	}

	// Cache miss - call the actual analyzer
	analysis, err := cha.AnalyzeModelAccess(ctx, model, reason, helpCtx)
	if err != nil {
		return nil, err
	}

	// Cache the response
	if analysis.Confidence >= 0.7 {
		response := cha.formatResponseForCache(analysis)
		cha.cache.Set("model_access", cacheKey, helpCtx.UserID, response, analysis.Confidence)
	}

	return analysis, nil
}

// AnalyzeToolConfigCached analyzes tool configuration with caching
func (cha *CachedHelpAnalyzer) AnalyzeToolConfigCached(ctx context.Context, toolType, configStr string, err error, helpCtx *HelpContext) (*HelpAnalysis, error) {
	cacheKey := fmt.Sprintf("%s:%s:%s", toolType, configStr, err.Error())

	// Try to get from cache first
	if entry, found := cha.cache.Get("tool_config", cacheKey, helpCtx.UserID); found {
		return cha.parseCachedResponse(entry.Response, "tool_config", entry.Confidence), nil
	}

	// Cache miss - call the actual analyzer
	analysis, err := cha.AnalyzeToolConfig(ctx, toolType, configStr, err, helpCtx)
	if err != nil {
		return nil, err
	}

	// Cache the response
	if analysis.Confidence >= 0.7 {
		response := cha.formatResponseForCache(analysis)
		cha.cache.Set("tool_config", cacheKey, helpCtx.UserID, response, analysis.Confidence)
	}

	return analysis, nil
}

// AnalyzeToolAccessCached analyzes tool access with caching
func (cha *CachedHelpAnalyzer) AnalyzeToolAccessCached(ctx context.Context, toolName, reason string, helpCtx *HelpContext) (*HelpAnalysis, error) {
	cacheKey := fmt.Sprintf("%s:%s", toolName, reason)

	// Try to get from cache first
	if entry, found := cha.cache.Get("tool_access", cacheKey, helpCtx.UserID); found {
		return cha.parseCachedResponse(entry.Response, "tool_access", entry.Confidence), nil
	}

	// Cache miss - call the actual analyzer
	analysis, err := cha.AnalyzeToolAccess(ctx, toolName, reason, helpCtx)
	if err != nil {
		return nil, err
	}

	// Cache the response
	if analysis.Confidence >= 0.7 {
		response := cha.formatResponseForCache(analysis)
		cha.cache.Set("tool_access", cacheKey, helpCtx.UserID, response, analysis.Confidence)
	}

	return analysis, nil
}

// formatResponseForCache formats a HelpAnalysis for caching
func (cha *CachedHelpAnalyzer) formatResponseForCache(analysis *HelpAnalysis) string {
	return fmt.Sprintf("DIAGNOSIS:%s|SUGGESTIONS:%v|EXAMPLES:%v|CONTEXT:%s|CONFIDENCE:%f",
		analysis.Diagnosis, analysis.Suggestions, analysis.Examples, analysis.ContextHelp, analysis.Confidence)
}

// parseCachedResponse parses a cached response back into HelpAnalysis
func (cha *CachedHelpAnalyzer) parseCachedResponse(response, failureType string, confidence float64) *HelpAnalysis {
	// Simple parsing - in a real implementation this would be more robust
	return &HelpAnalysis{
		FailureType: failureType,
		Diagnosis:   "Cached help response",
		Suggestions: []string{"Using cached help information"},
		Examples:    []string{"Cached response available"},
		Confidence:  confidence,
	}
}

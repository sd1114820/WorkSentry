package handlers

import (
	"context"
	"database/sql"
	"time"

	"worksentry/internal/db/sqlc"
)

const (
	settingsCacheTTL = 10 * time.Second
	rulesCacheTTL    = 10 * time.Second
)

func (h *Handler) getSettingsCached(ctx context.Context) sqlc.Setting {
	now := time.Now()
	h.settingsMu.RLock()
	if h.settingsCacheOK && now.Sub(h.settingsCachedAt) <= settingsCacheTTL {
		cached := h.settingsCache
		h.settingsMu.RUnlock()
		return cached
	}
	h.settingsMu.RUnlock()

	settings, err := h.Queries.GetSettings(ctx)
	if err == sql.ErrNoRows {
		settings = defaultSettings()
	} else if err != nil {
		h.settingsMu.RLock()
		if h.settingsCacheOK {
			cached := h.settingsCache
			h.settingsMu.RUnlock()
			return cached
		}
		h.settingsMu.RUnlock()
		return defaultSettings()
	}

	h.setSettingsCache(settings)
	return settings
}

func (h *Handler) setSettingsCache(settings sqlc.Setting) {
	h.settingsMu.Lock()
	h.settingsCache = settings
	h.settingsCachedAt = time.Now()
	h.settingsCacheOK = true
	h.settingsMu.Unlock()
}

func (h *Handler) invalidateSettingsCache() {
	h.settingsMu.Lock()
	h.settingsCachedAt = time.Time{}
	h.settingsCacheOK = false
	h.settingsMu.Unlock()
}

func (h *Handler) getEnabledRulesCached(ctx context.Context) []sqlc.ListEnabledRulesRow {
	now := time.Now()
	h.rulesMu.RLock()
	if h.rulesCacheOK && now.Sub(h.rulesCachedAt) <= rulesCacheTTL {
		cached := make([]sqlc.ListEnabledRulesRow, len(h.rulesCache))
		copy(cached, h.rulesCache)
		h.rulesMu.RUnlock()
		return cached
	}
	h.rulesMu.RUnlock()

	rules, err := h.Queries.ListEnabledRules(ctx)
	if err != nil {
		h.rulesMu.RLock()
		if h.rulesCacheOK {
			cached := make([]sqlc.ListEnabledRulesRow, len(h.rulesCache))
			copy(cached, h.rulesCache)
			h.rulesMu.RUnlock()
			return cached
		}
		h.rulesMu.RUnlock()
		return nil
	}

	h.rulesMu.Lock()
	h.rulesCache = make([]sqlc.ListEnabledRulesRow, len(rules))
	copy(h.rulesCache, rules)
	h.rulesCachedAt = time.Now()
	h.rulesCacheOK = true
	h.rulesMu.Unlock()
	return rules
}

func (h *Handler) invalidateRulesCache() {
	h.rulesMu.Lock()
	h.rulesCache = nil
	h.rulesCachedAt = time.Time{}
	h.rulesCacheOK = false
	h.rulesMu.Unlock()
}

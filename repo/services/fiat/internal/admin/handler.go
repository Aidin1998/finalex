package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

type FeeConfig struct {
	ID         string  `json:"id"`
	Pair       string  `json:"pair"`
	UserGroup  string  `json:"userGroup"`
	TierMin    float64 `json:"tierMin"`
	TierMax    float64 `json:"tierMax"`
	FeePercent float64 `json:"feePercent"`
	Active     bool    `json:"active"`
	CreatedAt  string  `json:"createdAt"`
	UpdatedAt  string  `json:"updatedAt"`
}

type FeeConfigRepo interface {
	List(ctx context.Context) ([]FeeConfig, error)
	Get(ctx context.Context, id string) (*FeeConfig, error)
	Create(ctx context.Context, cfg *FeeConfig) error
	Update(ctx context.Context, cfg *FeeConfig) error
	Delete(ctx context.Context, id string) error
}

type AdminHandler struct {
	Repo         FeeConfigRepo
	Configs      atomic.Value // []FeeConfig
	AuditLogFunc func(action, user, details string)
}

func (h *AdminHandler) Authz(next http.HandlerFunc, requiredScope string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// JWT parse & RBAC check (pseudo)
		token := r.Header.Get("Authorization")
		if !strings.HasPrefix(token, "Bearer ") || !strings.Contains(token, requiredScope) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func (h *AdminHandler) ListFeeConfigs(w http.ResponseWriter, r *http.Request) {
	cfgs, _ := h.Repo.List(r.Context())
	json.NewEncoder(w).Encode(cfgs)
}

func (h *AdminHandler) CreateFeeConfig(w http.ResponseWriter, r *http.Request) {
	var cfg FeeConfig
	json.NewDecoder(r.Body).Decode(&cfg)
	cfg.ID = time.Now().Format("20060102150405")
	cfg.CreatedAt = time.Now().Format(time.RFC3339)
	cfg.UpdatedAt = cfg.CreatedAt
	h.Repo.Create(r.Context(), &cfg)
	h.reloadConfigs()
	if h.AuditLogFunc != nil {
		h.AuditLogFunc("create", "admin", cfg.ID)
	}
	w.WriteHeader(http.StatusCreated)
}

func (h *AdminHandler) UpdateFeeConfig(w http.ResponseWriter, r *http.Request) {
	var cfg FeeConfig
	json.NewDecoder(r.Body).Decode(&cfg)
	cfg.UpdatedAt = time.Now().Format(time.RFC3339)
	h.Repo.Update(r.Context(), &cfg)
	h.reloadConfigs()
	if h.AuditLogFunc != nil {
		h.AuditLogFunc("update", "admin", cfg.ID)
	}
	w.WriteHeader(http.StatusOK)
}

func (h *AdminHandler) DeleteFeeConfig(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	h.Repo.Delete(r.Context(), id)
	h.reloadConfigs()
	if h.AuditLogFunc != nil {
		h.AuditLogFunc("delete", "admin", id)
	}
	w.WriteHeader(http.StatusOK)
}

func (h *AdminHandler) reloadConfigs() {
	cfgs, _ := h.Repo.List(context.Background())
	h.Configs.Store(cfgs)
}

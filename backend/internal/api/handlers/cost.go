package handlers

import (
	"net/http"

	"github.com/kvn8888/codegym/backend/internal/api/response"
	"github.com/kvn8888/codegym/backend/internal/usage"
)

// CostHandler serves GenAI token and cost aggregates.
type CostHandler struct {
	usage *usage.Service
}

// NewCostHandler builds a cost handler. usage may be nil (returns empty aggregate).
func NewCostHandler(usageService *usage.Service) *CostHandler {
	return &CostHandler{usage: usageService}
}

// Cost handles GET /api/v1/cost — aggregate tokens in/out and estimated USD cost
// for the authenticated workspace user.
func (h *CostHandler) Cost(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.usage == nil {
		response.JSON(w, http.StatusOK, usage.Aggregate{
			Currency:    "USD",
			PricingAsOf: usage.PricingAsOf,
			Rates:       append([]usage.Rate(nil), usage.DefaultRates...),
		})
		return
	}

	agg, err := h.usage.Aggregate(r.Context())
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "cost_unavailable", "Could not load GenAI cost aggregate.")
		return
	}
	response.JSON(w, http.StatusOK, agg)
}

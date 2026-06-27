package auth

type Principal struct {
	UserID          string       `json:"user_id"`
	DefaultTenantID string       `json:"default_tenant_id"`
	TenantIDs       []string     `json:"tenant_ids"`
	UserMetadata    UserMetadata `json:"user_metadata,omitempty"`
}

type UserMetadata struct {
	Email       string `json:"email,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
}

func (p Principal) HasTenant(tenantID string) bool {
	for _, candidate := range p.TenantIDs {
		if candidate == tenantID {
			return true
		}
	}
	return false
}

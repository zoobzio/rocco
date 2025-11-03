package rocco

// Identity represents an authenticated user or service account.
// Users must implement this interface with their own identity type.
type Identity interface {
	// ID returns the unique identifier for this identity (e.g., user ID, service account ID).
	ID() string

	// TenantID returns the tenant/organization this identity belongs to.
	// Return empty string if not applicable.
	TenantID() string

	// HasScope checks if this identity has the given scope/permission.
	HasScope(scope string) bool

	// HasRole checks if this identity has the given role.
	HasRole(role string) bool

	// Stats returns usage metrics for rate limiting.
	// Keys are metric names (e.g., "requests_today", "api_calls_this_hour").
	// Values are current counts.
	Stats() map[string]int
}

// NoIdentity represents the absence of authentication.
// Used for public endpoints that don't require authentication.
type NoIdentity struct{}

// ID implements Identity.
func (NoIdentity) ID() string {
	return ""
}

// TenantID implements Identity.
func (NoIdentity) TenantID() string {
	return ""
}

// HasScope implements Identity.
func (NoIdentity) HasScope(_ string) bool {
	return false
}

// HasRole implements Identity.
func (NoIdentity) HasRole(_ string) bool {
	return false
}

// Stats implements Identity.
func (NoIdentity) Stats() map[string]int {
	return nil
}

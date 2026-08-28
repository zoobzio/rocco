package rocco

// Identity represents an authenticated user or service account.
// Users must implement this interface with their own identity type.
type Identity interface {
	// ID returns the unique identifier for this identity (e.g., user ID, service account ID).
	ID() string

	// TenantID returns the tenant/organization this identity belongs to.
	// Return empty string if not applicable.
	TenantID() string

	// Email returns the email address associated with this identity.
	// Return empty string if not available.
	Email() string

	// Scopes returns all scopes/permissions granted to this identity.
	Scopes() []string

	// Roles returns all roles assigned to this identity.
	Roles() []string

	// HasScope checks if this identity has the given scope/permission.
	HasScope(scope string) bool

	// HasRole checks if this identity has the given role.
	HasRole(role string) bool

	// Stats returns usage metrics for rate limiting.
	// Keys are metric names (e.g., "requests_today", "api_calls_this_hour").
	// Values are current counts.
	Stats() map[string]int
}

// satisfiesRequirements checks an identity against scope and role requirement
// groups. Semantics: OR within each group, AND across groups. Scope groups are
// checked before role groups. Returns the first unsatisfied group (for error
// reporting) and ok=false, or ok=true when all groups are satisfied.
//
// This is the single implementation of authorization semantics, shared by the
// engine's authorization middleware and OpenAPI handler filtering.
func satisfiesRequirements(identity Identity, scopeGroups, roleGroups [][]string) (failedScopes, failedRoles []string, ok bool) {
	for _, scopeGroup := range scopeGroups {
		hasAnyScope := false
		for _, scope := range scopeGroup {
			if identity.HasScope(scope) {
				hasAnyScope = true
				break
			}
		}
		if !hasAnyScope {
			return scopeGroup, nil, false
		}
	}

	for _, roleGroup := range roleGroups {
		hasAnyRole := false
		for _, role := range roleGroup {
			if identity.HasRole(role) {
				hasAnyRole = true
				break
			}
		}
		if !hasAnyRole {
			return nil, roleGroup, false
		}
	}

	return nil, nil, true
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

// Email implements Identity.
func (NoIdentity) Email() string {
	return ""
}

// Scopes implements Identity.
func (NoIdentity) Scopes() []string {
	return nil
}

// Roles implements Identity.
func (NoIdentity) Roles() []string {
	return nil
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

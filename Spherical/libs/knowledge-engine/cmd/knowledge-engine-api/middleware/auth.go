// Package middleware provides HTTP middleware for the Knowledge Engine API.
package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// Context keys for request-scoped values.
type contextKey string

const (
	// TenantIDKey is the context key for tenant ID.
	TenantIDKey contextKey = "tenant_id"
	// UserIDKey is the context key for user ID.
	UserIDKey contextKey = "user_id"
	// RolesKey is the context key for user roles.
	RolesKey contextKey = "roles"
)

// AuthConfig holds authentication configuration.
type AuthConfig struct {
	Enabled          bool
	Issuer           string
	Audience         string
	RequireAuth      bool
	RequireMTLS      bool           // T060: Require mTLS for production
	MTLSCertPool     interface{}    // T060: Certificate pool for mTLS verification
	RequiredScopes   []string       // T060: Required OAuth2 scopes
	AllowPublicPaths []string       // T060: Public paths that don't require auth
}

// Role represents an RBAC role.
type Role string

const (
	RoleAdmin       Role = "admin"
	RoleAnalyst     Role = "analyst"
	RoleAgentRuntime Role = "agent-runtime"
)

// Auth returns an authentication middleware.
func Auth(cfg AuthConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip auth if disabled
			if !cfg.Enabled {
				// In dev mode, use default tenant from header or query param
				tenantID := r.Header.Get("X-Tenant-ID")
				if tenantID == "" {
					tenantID = r.URL.Query().Get("tenant_id")
				}
				if tenantID == "" {
					tenantID = "dev" // Default tenant for development
				}

				ctx := context.WithValue(r.Context(), TenantIDKey, tenantID)
				ctx = context.WithValue(ctx, RolesKey, []Role{RoleAdmin})
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Extract token from Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				if cfg.RequireAuth {
					http.Error(w, `{"error": "missing authorization header"}`, http.StatusUnauthorized)
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			// Parse Bearer token
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				http.Error(w, `{"error": "invalid authorization header format"}`, http.StatusUnauthorized)
				return
			}
			token := parts[1]

			// T060: Verify mTLS if required
			if cfg.RequireMTLS {
				if err := verifyMTLS(r, cfg); err != nil {
					http.Error(w, `{"error": "mTLS verification failed"}`, http.StatusForbidden)
					return
				}
			}

			// Validate token (placeholder - would integrate with OAuth2/OIDC provider)
			claims, err := validateToken(token, cfg)
			if err != nil {
				http.Error(w, `{"error": "invalid token"}`, http.StatusUnauthorized)
				return
			}

			// T060: Validate OAuth2 scopes if required
			if len(cfg.RequiredScopes) > 0 {
				// Extract scopes from token claims (would come from JWT)
				tokenScopes := extractScopesFromToken(token) // TODO: Implement
				if !validateScopes(tokenScopes, cfg.RequiredScopes) {
					http.Error(w, `{"error": "insufficient scopes"}`, http.StatusForbidden)
					return
				}
			}

			// Add claims to context
			ctx := r.Context()
			ctx = context.WithValue(ctx, TenantIDKey, claims.TenantID)
			ctx = context.WithValue(ctx, UserIDKey, claims.UserID)
			ctx = context.WithValue(ctx, RolesKey, claims.Roles)

			// T060: Enforce tenancy guards
			if err := enforceTenancyGuards(ctx, claims.TenantID, ""); err != nil {
				http.Error(w, fmt.Sprintf(`{"error": "%s"}`, err.Error()), http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// TokenClaims represents validated token claims.
type TokenClaims struct {
	TenantID string
	UserID   string
	Roles    []Role
	Scopes   []string // T060: OAuth2 scopes
}

// validateToken validates a JWT token (placeholder implementation).
// T060: Enhanced security - implement proper JWT validation with scope checks.
func validateToken(token string, cfg AuthConfig) (*TokenClaims, error) {
	// TODO: Implement actual JWT validation using cfg.Issuer and cfg.Audience
	// This would typically use a library like github.com/golang-jwt/jwt/v5
	// Requirements:
	// 1. Verify JWT signature using public key from issuer's JWKS endpoint
	// 2. Validate expiration (exp), issued at (iat), not before (nbf)
	// 3. Validate issuer (iss) matches cfg.Issuer
	// 4. Validate audience (aud) contains cfg.Audience
	// 5. Extract and validate OAuth2 scopes (scope claim)
	// 6. Verify user has required scopes from cfg.RequiredScopes
	// 7. Extract tenant_id, user_id, roles from token claims
	// For now, return placeholder claims

	return &TokenClaims{
		TenantID: "validated-tenant",
		UserID:   "validated-user",
		Roles:    []Role{RoleAgentRuntime},
	}, nil
}

// verifyMTLS verifies mTLS client certificate.
// T060: Implement mTLS verification for production security.
func verifyMTLS(r *http.Request, cfg AuthConfig) error {
	if !cfg.RequireMTLS {
		return nil
	}

	// TODO: Implement mTLS verification
	// Requirements:
	// 1. Extract client certificate from r.TLS.PeerCertificates
	// 2. Verify certificate chain against trusted CA
	// 3. Validate certificate validity period
	// 4. Check certificate revocation list (CRL) or OCSP
	// 5. Verify certificate subject matches expected patterns
	// 6. Extract tenant/org info from certificate subject if needed

	return nil
}

// validateScopes checks if token claims include required OAuth2 scopes.
// T060: OAuth2 scope validation.
func validateScopes(tokenScopes []string, requiredScopes []string) bool {
	if len(requiredScopes) == 0 {
		return true
	}

	scopeMap := make(map[string]bool)
	for _, scope := range tokenScopes {
		scopeMap[scope] = true
	}

	for _, required := range requiredScopes {
		if !scopeMap[required] {
			return false
		}
	}

	return true
}

// enforceTenancyGuards ensures requests are properly scoped to tenant.
// T060: Enhanced tenancy guards.
func enforceTenancyGuards(ctx context.Context, tenantID string, resourceTenantID string) error {
	if tenantID == "" {
		return fmt.Errorf("tenant ID required")
	}

	if resourceTenantID != "" && tenantID != resourceTenantID {
		return fmt.Errorf("tenant mismatch: access denied")
	}

	return nil
}

// RequireRoles returns middleware that requires specific roles.
func RequireRoles(roles ...Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userRoles, ok := r.Context().Value(RolesKey).([]Role)
			if !ok {
				http.Error(w, `{"error": "roles not found in context"}`, http.StatusForbidden)
				return
			}

			// Check if user has any of the required roles
			hasRole := false
			for _, required := range roles {
				for _, userRole := range userRoles {
					if userRole == required || userRole == RoleAdmin {
						hasRole = true
						break
					}
				}
				if hasRole {
					break
				}
			}

			if !hasRole {
				http.Error(w, `{"error": "insufficient permissions"}`, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// TenantFromContext extracts the tenant ID from context.
func TenantFromContext(ctx context.Context) string {
	if v := ctx.Value(TenantIDKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// TenantUUIDFromContext extracts the tenant ID as UUID from context.
func TenantUUIDFromContext(ctx context.Context) (uuid.UUID, error) {
	tenantStr := TenantFromContext(ctx)
	if tenantStr == "" {
		return uuid.Nil, nil
	}
	return uuid.Parse(tenantStr)
}

// UserFromContext extracts the user ID from context.
func UserFromContext(ctx context.Context) string {
	if v := ctx.Value(UserIDKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// RolesFromContext extracts the roles from context.
func RolesFromContext(ctx context.Context) []Role {
	if v := ctx.Value(RolesKey); v != nil {
		if roles, ok := v.([]Role); ok {
			return roles
		}
	}
	return nil
}

// HasRole checks if the context has a specific role.
func HasRole(ctx context.Context, role Role) bool {
	roles := RolesFromContext(ctx)
	for _, r := range roles {
		if r == role || r == RoleAdmin {
			return true
		}
	}
	return false
}

// extractScopesFromToken extracts OAuth2 scopes from JWT token.
// T060: Extract scopes from token claims.
func extractScopesFromToken(token string) []string {
	// TODO: Implement actual scope extraction from JWT
	// This would parse the JWT, extract the "scope" claim,
	// and split it into individual scopes (space-separated)
	return []string{}
}

// CORS returns CORS middleware for browser clients.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Check if origin is allowed
			allowed := false
			for _, o := range allowedOrigins {
				if o == "*" || o == origin {
					allowed = true
					break
				}
			}

			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Tenant-ID")
				w.Header().Set("Access-Control-Max-Age", "86400")
			}

			// Handle preflight
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequestLogger returns middleware that logs requests.
func RequestLogger(logger interface{ Info() interface{ Msg(string) } }) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Log would include: method, path, tenant, user, latency
			next.ServeHTTP(w, r)
		})
	}
}


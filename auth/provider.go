// Tencent is pleased to support the open source community by making tRPC available.
//
// Copyright (C) 2025 THL A29 Limited, a Tencent company.
// All rights reserved.
//
// If you have downloaded a copy of the tRPC source code from Tencent,
// please note that tRPC source code is licensed under the  Apache 2.0 License,
// A copy of the Apache 2.0 License is included in this file.

// Package auth provides authentication and authorization functionality for the A2A protocol.
package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ContextKey type for context values.
type ContextKey string

// Context keys.
const (
	// AuthUserKey is the key used to store the authenticated user in the context.
	AuthUserKey ContextKey = "auth_user"
	// AuthHeaderName is the default name of the header containing the authentication token.
	AuthHeaderName = "Authorization"
)

// User represents an authenticated user.
type User struct {
	// ID is the unique identifier of the user.
	ID string
	// Claims contains any additional JWT claims.
	Claims jwt.MapClaims
}

// Provider is an interface for authentication providers.
type Provider interface {
	// Authenticate authenticates a request.
	Authenticate(r *http.Request) (*User, error)
}

// JWTAuthProvider authenticates requests using JWT.
type JWTAuthProvider struct {
	// Secret is the secret key for validating JWT tokens.
	Secret []byte
	// Audience is the expected audience of the JWT.
	Audience string
	// Issuer is the expected issuer of the JWT.
	Issuer string
	// TokenLifetime is the duration for which a token is valid.
	TokenLifetime time.Duration
}

// NewJWTAuthProvider creates a new JWT authentication provider.
func NewJWTAuthProvider(secret []byte, audience, issuer string, lifetime time.Duration) *JWTAuthProvider {
	if lifetime == 0 {
		lifetime = 24 * time.Hour
	}
	return &JWTAuthProvider{
		Secret:        secret,
		Audience:      audience,
		Issuer:        issuer,
		TokenLifetime: lifetime,
	}
}

// Authenticate validates a JWT from the request's Authorization header.
func (p *JWTAuthProvider) Authenticate(r *http.Request) (*User, error) {
	// Extract token from Authorization header.
	authHeader := r.Header.Get(AuthHeaderName)
	if authHeader == "" {
		return nil, ErrMissingToken
	}
	// Example: Authorization: Bearer <token>
	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || !strings.EqualFold(parts[0], string(TokenTypeBearer)) {
		return nil, ErrInvalidAuthHeader
	}
	tokenString := parts[1]
	// Parse and validate the token.
	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(
		tokenString,
		claims,
		func(token *jwt.Token) (interface{}, error) {
			// Validate the token signing method.
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %T", token.Method)
			}
			return p.Secret, nil
		},
		jwt.WithAudience(p.Audience),
		jwt.WithIssuer(p.Issuer),
	)
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, ErrInvalidToken
	}
	// Extract user ID from claims.
	subject, err := token.Claims.GetSubject()
	if err != nil {
		return nil, fmt.Errorf("missing subject claim: %w", err)
	}
	// Create and return the authenticated user.
	user := &User{
		ID:     subject,
		Claims: claims,
	}
	return user, nil
}

// CreateToken creates a new JWT token.
func (p *JWTAuthProvider) CreateToken(userID string, customClaims map[string]interface{}) (string, error) {
	now := time.Now()
	expiresAt := now.Add(p.TokenLifetime)
	const (
		subKey = "sub"
		iatKey = "iat"
		expKey = "exp"
		audKey = "aud"
		issKey = "iss"
	)
	// Create standard claims.
	claims := jwt.MapClaims{
		subKey: userID,
		iatKey: now.Unix(),
		expKey: expiresAt.Unix(),
	}
	// Add audience and issuer if set.
	if p.Audience != "" {
		claims[audKey] = p.Audience
	}
	if p.Issuer != "" {
		claims[issKey] = p.Issuer
	}
	// Add custom claims.
	for k, v := range customClaims {
		claims[k] = v
	}
	// Create and sign the token.
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(p.Secret)
	if err != nil {
		return "", err
	}
	return tokenString, nil
}

// APIKeyAuthProvider authenticates requests using API keys.
type APIKeyAuthProvider struct {
	// KeyMap maps API keys to user IDs.
	KeyMap map[string]string
	// HeaderName is the name of the header containing the API key.
	HeaderName string
}

// NewAPIKeyAuthProvider creates a new API key authentication provider.
func NewAPIKeyAuthProvider(keyMap map[string]string, headerName string) *APIKeyAuthProvider {
	const defaultHeaderName = "X-API-Key"
	if headerName == "" {
		headerName = defaultHeaderName
	}
	return &APIKeyAuthProvider{
		KeyMap:     keyMap,
		HeaderName: headerName,
	}
}

// Authenticate validates an API key from the request's header.
func (p *APIKeyAuthProvider) Authenticate(r *http.Request) (*User, error) {
	apiKey := r.Header.Get(p.HeaderName)
	if apiKey == "" {
		return nil, ErrMissingToken
	}
	userID, ok := p.KeyMap[apiKey]
	if !ok {
		return nil, ErrInvalidToken
	}
	user := &User{
		ID:     userID,
		Claims: jwt.MapClaims{},
	}
	return user, nil
}

// ChainAuthProvider chains multiple authentication providers.
type ChainAuthProvider struct {
	providers []Provider
}

// NewChainAuthProvider creates a new chain authentication provider.
func NewChainAuthProvider(providers ...Provider) *ChainAuthProvider {
	return &ChainAuthProvider{
		providers: providers,
	}
}

// Authenticate tries each authentication provider in order, returning the first success.
func (p *ChainAuthProvider) Authenticate(r *http.Request) (*User, error) {
	var lastErr error
	for _, provider := range p.providers {
		user, err := provider.Authenticate(r)
		if err == nil {
			return user, nil
		}
		lastErr = err
	}

	return nil, lastErr
}

// Middleware wraps an HTTP handler with authentication.
type Middleware struct {
	provider Provider
}

// NewMiddleware creates a new authentication middleware.
func NewMiddleware(provider Provider) *Middleware {
	return &Middleware{
		provider: provider,
	}
}

// Wrap adds authentication to an HTTP handler.
func (m *Middleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := m.provider.Authenticate(r)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		// Add the authenticated user to the request context.
		ctx := context.WithValue(r.Context(), AuthUserKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

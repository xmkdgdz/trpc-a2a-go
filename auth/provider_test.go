// Tencent is pleased to support the open source community by making tRPC available.
//
// Copyright (C) 2025 THL A29 Limited, a Tencent company.
// All rights reserved.
//
// If you have downloaded a copy of the tRPC source code from Tencent,
// please note that tRPC source code is licensed under the  Apache 2.0 License,
// A copy of the Apache 2.0 License is included in this file.

package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"trpc.group/trpc-go/a2a-go/auth"
)

func TestJWTAuthProvider(t *testing.T) {
	// Setup test data
	secret := []byte("test-secret-key-for-jwt-provider")
	audience := "test-audience"
	issuer := "test-issuer"
	userID := "user123"
	customClaims := map[string]interface{}{
		"role": "admin",
	}

	// Create provider
	provider := auth.NewJWTAuthProvider(secret, audience, issuer, 1*time.Hour)

	// Test token creation
	t.Run("CreateToken", func(t *testing.T) {
		token, err := provider.CreateToken(userID, customClaims)
		require.NoError(t, err)
		require.NotEmpty(t, token)
	})

	// Test authentication with valid token
	t.Run("Authenticate_ValidToken", func(t *testing.T) {
		// Create a token
		token, err := provider.CreateToken(userID, customClaims)
		require.NoError(t, err)

		// Create a request with the token
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set(auth.AuthHeaderName, "Bearer "+token)

		// Authenticate
		user, err := provider.Authenticate(req)
		require.NoError(t, err)
		assert.Equal(t, userID, user.ID)
		assert.Equal(t, "admin", user.Claims["role"])
	})

	// Test authentication with missing token
	t.Run("Authenticate_MissingToken", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)

		user, err := provider.Authenticate(req)
		assert.ErrorIs(t, err, auth.ErrMissingToken)
		assert.Nil(t, user)
	})

	// Test authentication with invalid header format
	t.Run("Authenticate_InvalidHeader", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set(auth.AuthHeaderName, "InvalidFormat token123")

		user, err := provider.Authenticate(req)
		assert.ErrorIs(t, err, auth.ErrInvalidAuthHeader)
		assert.Nil(t, user)
	})

	// Test authentication with expired token
	t.Run("Authenticate_ExpiredToken", func(t *testing.T) {
		// Create provider with very short token lifetime
		shortProvider := auth.NewJWTAuthProvider(secret, audience, issuer, 1*time.Millisecond)
		token, err := shortProvider.CreateToken(userID, nil)
		require.NoError(t, err)

		// Wait for token to expire
		time.Sleep(5 * time.Millisecond)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set(auth.AuthHeaderName, "Bearer "+token)

		user, err := shortProvider.Authenticate(req)
		assert.Error(t, err)
		assert.Nil(t, user)
		assert.Contains(t, err.Error(), "expired")
	})
}

func TestAPIKeyAuthProvider(t *testing.T) {
	// Setup test data
	keyMap := map[string]string{
		"api-key-1": "user1",
		"api-key-2": "user2",
	}
	headerName := "X-Custom-API-Key"

	// Test with custom header name
	t.Run("Authenticate_CustomHeader", func(t *testing.T) {
		provider := auth.NewAPIKeyAuthProvider(keyMap, headerName)

		// Valid request
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set(headerName, "api-key-1")

		user, err := provider.Authenticate(req)
		require.NoError(t, err)
		assert.Equal(t, "user1", user.ID)
	})

	// Test with default header name
	t.Run("Authenticate_DefaultHeader", func(t *testing.T) {
		provider := auth.NewAPIKeyAuthProvider(keyMap, "")

		// Valid request
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-API-Key", "api-key-2")

		user, err := provider.Authenticate(req)
		require.NoError(t, err)
		assert.Equal(t, "user2", user.ID)
	})

	// Test missing API key
	t.Run("Authenticate_MissingKey", func(t *testing.T) {
		provider := auth.NewAPIKeyAuthProvider(keyMap, headerName)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)

		user, err := provider.Authenticate(req)
		assert.ErrorIs(t, err, auth.ErrMissingToken)
		assert.Nil(t, user)
	})

	// Test invalid API key
	t.Run("Authenticate_InvalidKey", func(t *testing.T) {
		provider := auth.NewAPIKeyAuthProvider(keyMap, headerName)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set(headerName, "invalid-key")

		user, err := provider.Authenticate(req)
		assert.ErrorIs(t, err, auth.ErrInvalidToken)
		assert.Nil(t, user)
	})
}

func TestChainAuthProvider(t *testing.T) {
	// Setup JWT provider
	jwtProvider := auth.NewJWTAuthProvider(
		[]byte("test-secret"),
		"test-audience",
		"test-issuer",
		1*time.Hour,
	)

	// Setup API key provider
	keyMap := map[string]string{
		"test-api-key": "api-user",
	}
	apiKeyProvider := auth.NewAPIKeyAuthProvider(keyMap, "X-API-Key")

	// Create chain provider
	chainProvider := auth.NewChainAuthProvider(jwtProvider, apiKeyProvider)

	// Test JWT authentication success
	t.Run("Authenticate_JWT_Success", func(t *testing.T) {
		token, err := jwtProvider.CreateToken("jwt-user", nil)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set(auth.AuthHeaderName, "Bearer "+token)

		user, err := chainProvider.Authenticate(req)
		require.NoError(t, err)
		assert.Equal(t, "jwt-user", user.ID)
	})

	// Test API key authentication success
	t.Run("Authenticate_APIKey_Success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-API-Key", "test-api-key")

		user, err := chainProvider.Authenticate(req)
		require.NoError(t, err)
		assert.Equal(t, "api-user", user.ID)
	})

	// Test authentication failure when no provider succeeds
	t.Run("Authenticate_AllFail", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)

		user, err := chainProvider.Authenticate(req)
		assert.Error(t, err)
		assert.Nil(t, user)
	})
}

func TestAuthMiddleware(t *testing.T) {
	// Setup a simple API key provider for testing
	keyMap := map[string]string{
		"valid-key": "test-user",
	}
	provider := auth.NewAPIKeyAuthProvider(keyMap, "X-API-Key")

	// Create middleware
	middleware := auth.NewMiddleware(provider)

	// Create a test handler that checks for authenticated user
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := r.Context().Value(auth.AuthUserKey).(*auth.User)
		if !ok {
			http.Error(w, "No user in context", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(user.ID))
	})

	// Wrap the test handler with the auth middleware
	wrappedHandler := middleware.Wrap(testHandler)

	// Test successful authentication
	t.Run("Middleware_Success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-API-Key", "valid-key")
		recorder := httptest.NewRecorder()

		wrappedHandler.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Equal(t, "test-user", recorder.Body.String())
	})

	// Test authentication failure
	t.Run("Middleware_Failure", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-API-Key", "invalid-key")
		recorder := httptest.NewRecorder()

		wrappedHandler.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	})
}

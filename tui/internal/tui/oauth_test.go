package tui

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGeneratePKCE(t *testing.T) {
	verifier, challenge := generatePKCE()

	if verifier == "" {
		t.Fatal("verifier must not be empty")
	}
	if challenge == "" {
		t.Fatal("challenge must not be empty")
	}

	// Verify challenge == base64url(sha256(verifier)).
	h := sha256.Sum256([]byte(verifier))
	expected := base64.RawURLEncoding.EncodeToString(h[:])
	if challenge != expected {
		t.Fatalf("challenge mismatch:\n  got:  %s\n  want: %s", challenge, expected)
	}

	// Two calls should produce different values (randomness check).
	v2, _ := generatePKCE()
	if verifier == v2 {
		t.Fatal("two calls returned the same verifier — randomness failure")
	}
}

func TestGenerateState(t *testing.T) {
	state := generateState()

	if len(state) != 64 {
		t.Fatalf("state length = %d, want 64", len(state))
	}

	// Must be valid hex.
	if _, err := hex.DecodeString(state); err != nil {
		t.Fatalf("state is not valid hex: %v", err)
	}

	// Two calls should differ.
	s2 := generateState()
	if state == s2 {
		t.Fatal("two calls returned the same state — randomness failure")
	}
}

func TestExchangeCodeForTokens(t *testing.T) {
	// Build a fake JWT with the OpenAI profile claim for the id_token.
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payloadObj := map[string]interface{}{
		"sub": "user-123",
		"https://api.openai.com/profile": map[string]string{
			"email": "test@example.com",
		},
	}
	payloadJSON, _ := json.Marshal(payloadObj)
	payload := base64.RawURLEncoding.EncodeToString(payloadJSON)
	fakeJWT := fmt.Sprintf("%s.%s.sig", header, payload)

	// Mock token server.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}

		// Verify all expected form params.
		checks := map[string]string{
			"grant_type":    "authorization_code",
			"client_id":     codexClientID,
			"code":          "test-auth-code",
			"code_verifier": "test-verifier",
			"redirect_uri":  "http://127.0.0.1:1455/auth/callback",
		}
		for k, want := range checks {
			got := r.FormValue(k)
			if got != want {
				t.Errorf("form param %s = %q, want %q", k, got, want)
			}
		}

		resp := map[string]interface{}{
			"access_token":  "acc-tok-123",
			"refresh_token": "ref-tok-456",
			"id_token":      fakeJWT,
			"expires_in":    3600,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	tokens, err := exchangeCodeForTokens(
		server.URL,
		"test-auth-code",
		"test-verifier",
		"http://127.0.0.1:1455/auth/callback",
	)
	if err != nil {
		t.Fatalf("exchangeCodeForTokens failed: %v", err)
	}

	if tokens.AccessToken != "acc-tok-123" {
		t.Errorf("AccessToken = %q, want %q", tokens.AccessToken, "acc-tok-123")
	}
	if tokens.RefreshToken != "ref-tok-456" {
		t.Errorf("RefreshToken = %q, want %q", tokens.RefreshToken, "ref-tok-456")
	}
	if tokens.Email != "test@example.com" {
		t.Errorf("Email = %q, want %q", tokens.Email, "test@example.com")
	}
	if tokens.ExpiresAt == 0 {
		t.Error("ExpiresAt should be non-zero")
	}
}

func TestExtractEmailFromJWT(t *testing.T) {
	tests := []struct {
		name  string
		jwt   string
		want  string
	}{
		{
			name: "valid jwt with email",
			jwt: func() string {
				h := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
				p := map[string]interface{}{
					"sub": "u-1",
					"https://api.openai.com/profile": map[string]string{
						"email": "alice@example.com",
					},
				}
				pj, _ := json.Marshal(p)
				return fmt.Sprintf("%s.%s.sig", h, base64.RawURLEncoding.EncodeToString(pj))
			}(),
			want: "alice@example.com",
		},
		{
			name: "missing profile claim",
			jwt: func() string {
				h := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
				p := map[string]interface{}{"sub": "u-2"}
				pj, _ := json.Marshal(p)
				return fmt.Sprintf("%s.%s.sig", h, base64.RawURLEncoding.EncodeToString(pj))
			}(),
			want: "",
		},
		{
			name: "not a jwt",
			jwt:  "not-a-jwt",
			want: "",
		},
		{
			name: "empty string",
			jwt:  "",
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractEmailFromJWT(tc.jwt)
			if got != tc.want {
				t.Errorf("extractEmailFromJWT() = %q, want %q", got, tc.want)
			}
		})
	}
}

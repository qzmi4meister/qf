package auth

import (
	"strings"
	"testing"
)

// TestOIDCConfig_String_MasksSecret verifies that String() never exposes the
// raw ClientSecret value, preventing accidental leakage via fmt/slog.
func TestOIDCConfig_String_MasksSecret(t *testing.T) {
	secret := "super-secret-value-abc123"
	cfg := OIDCConfig{
		Issuer:       "https://accounts.example.com",
		ClientID:     "my-client-id",
		ClientSecret: secret,
		RedirectURL:  "https://cp.example.com/auth/oidc/callback",
	}

	s := cfg.String()

	if strings.Contains(s, secret) {
		t.Errorf("String() leaked ClientSecret: %q", s)
	}
	if !strings.Contains(s, "<masked>") {
		t.Errorf("String() should contain '<masked>' placeholder, got: %q", s)
	}
	if !strings.Contains(s, cfg.Issuer) {
		t.Errorf("String() should contain Issuer, got: %q", s)
	}
}

// TestOIDCConfig_String_NoSecret verifies output when ClientSecret is empty.
func TestOIDCConfig_String_NoSecret(t *testing.T) {
	cfg := OIDCConfig{Issuer: "https://accounts.example.com", ClientID: "id"}
	s := cfg.String()
	if strings.Contains(s, "<masked>") {
		t.Errorf("String() should not show <masked> when no secret set: %q", s)
	}
	if !strings.Contains(s, "<not set>") {
		t.Errorf("String() should show '<not set>' when ClientSecret is empty, got: %q", s)
	}
}

// TestOIDCConfig_String_ZeroValue verifies disabled OIDC config is safe to print.
func TestOIDCConfig_String_ZeroValue(t *testing.T) {
	var cfg OIDCConfig
	s := cfg.String()
	if s == "" {
		t.Error("String() should not return empty string")
	}
}

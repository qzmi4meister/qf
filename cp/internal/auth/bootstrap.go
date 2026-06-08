package auth

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgtype"
	storegen "github.com/qf/qf/cp/internal/store/gen"
	"golang.org/x/crypto/bcrypt"
)

// EnsureAdminUser creates the admin user from QF_ADMIN_EMAIL + QF_ADMIN_PASSWORD +
// QF_ADMIN_USERNAME env vars if no user with that username exists yet. Idempotent.
func EnsureAdminUser(ctx context.Context, q *storegen.Queries, tenantID pgtype.UUID) error {
	email := os.Getenv("QF_ADMIN_EMAIL")
	password := os.Getenv("QF_ADMIN_PASSWORD")
	username := os.Getenv("QF_ADMIN_USERNAME")
	if email == "" || password == "" || username == "" {
		return nil
	}

	_, err := q.GetUserByUsername(ctx, storegen.GetUserByUsernameParams{
		TenantID: tenantID,
		Username: username,
	})
	if err == nil {
		return nil // already exists
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("bcrypt: %w", err)
	}
	hashStr := string(hash)

	user, err := q.CreateUser(ctx, storegen.CreateUserParams{
		TenantID:     tenantID,
		Email:        email,
		Username:     username,
		PasswordHash: &hashStr,
		OidcSubject:  nil,
	})
	if err != nil {
		return fmt.Errorf("create admin user: %w", err)
	}

	if err := q.UpsertUserRole(ctx, storegen.UpsertUserRoleParams{
		UserID:   user.ID,
		TenantID: tenantID,
		Role:     "admin",
	}); err != nil {
		return fmt.Errorf("set admin role: %w", err)
	}

	slog.Info("admin user created", "username", username, "email", email)
	return nil
}

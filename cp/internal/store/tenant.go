package store

import (
	"context"
	"fmt"

	storegen "github.com/qf/qf/cp/internal/store/gen"
)

// EnsureDefaultTenant returns the default tenant (slug "default"), creating it if absent.
func EnsureDefaultTenant(ctx context.Context, q *storegen.Queries) (storegen.Tenant, error) {
	t, err := q.GetTenantBySlug(ctx, "default")
	if err == nil {
		return t, nil
	}
	t, err = q.CreateTenant(ctx, storegen.CreateTenantParams{
		Slug: "default",
		Name: "Default",
	})
	if err != nil {
		return storegen.Tenant{}, fmt.Errorf("create default tenant: %w", err)
	}
	return t, nil
}

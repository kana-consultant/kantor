package rbac

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	repository "github.com/kana-consultant/kantor/backend/internal/repository"
	"github.com/kana-consultant/kantor/backend/internal/tenant"
)

type ModuleRole struct {
	RoleID   string `json:"role_id"`
	RoleSlug string `json:"role_slug"`
	RoleName string `json:"role_name"`
}

type CachedPermissions struct {
	IsSuperAdmin bool                `json:"is_super_admin"`
	ModuleRoles  map[string]ModuleRole `json:"module_roles"`
	Permissions  map[string]bool     `json:"permissions"`
	CachedAt     time.Time           `json:"cached_at"`
	TTL          time.Duration       `json:"ttl"`
}

func (c *CachedPermissions) PermissionList() []string {
	items := make([]string, 0, len(c.Permissions))
	for permission := range c.Permissions {
		items = append(items, permission)
	}
	sort.Strings(items)
	return items
}

type PermissionCache struct {
	mu    sync.RWMutex
	store map[string]*CachedPermissions
	db    repository.DBTX
	ttl   time.Duration
}

func NewPermissionCache(db repository.DBTX, ttl time.Duration) *PermissionCache {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}

	return &PermissionCache{
		store: make(map[string]*CachedPermissions),
		db:    db,
		ttl:   ttl,
	}
}

func (c *PermissionCache) cacheKey(ctx context.Context, userID string) string {
	if info, ok := tenant.FromContext(ctx); ok {
		return info.ID + ":" + userID
	}
	return userID
}

func (c *PermissionCache) Get(ctx context.Context, userID string) *CachedPermissions {
	key := c.cacheKey(ctx, userID)
	c.mu.RLock()
	cached := c.store[key]
	c.mu.RUnlock()

	if cached == nil {
		return nil
	}

	if time.Since(cached.CachedAt) > cached.TTL {
		c.InvalidateKey(key)
		return nil
	}

	return cached
}

func (c *PermissionCache) Load(ctx context.Context, userID string) (*CachedPermissions, error) {
	if cached := c.Get(ctx, userID); cached != nil {
		return cached, nil
	}

	db := repository.DB(ctx, c.db)

	var isSuperAdmin bool
	if err := db.QueryRow(ctx, `SELECT is_super_admin FROM users WHERE id = $1::uuid`, userID).Scan(&isSuperAdmin); err != nil {
		return nil, fmt.Errorf("load super admin flag: %w", err)
	}

	cached := &CachedPermissions{
		IsSuperAdmin: isSuperAdmin,
		ModuleRoles:  make(map[string]ModuleRole),
		Permissions:  make(map[string]bool),
		CachedAt:     time.Now().UTC(),
		TTL:          c.ttl,
	}

	if !isSuperAdmin {
		roleRows, err := db.Query(ctx, `
			SELECT umr.module_id, r.id::text, r.slug, r.name
			FROM user_module_roles umr
			INNER JOIN roles r ON r.id = umr.role_id
			WHERE umr.user_id = $1::uuid
				AND r.is_active = TRUE
		`, userID)
		if err != nil {
			return nil, fmt.Errorf("load module roles: %w", err)
		}

		for roleRows.Next() {
			var moduleID string
			var role ModuleRole
			if err := roleRows.Scan(&moduleID, &role.RoleID, &role.RoleSlug, &role.RoleName); err != nil {
				roleRows.Close()
				return nil, fmt.Errorf("scan module role: %w", err)
			}
			cached.ModuleRoles[moduleID] = role
		}
		if err := roleRows.Err(); err != nil {
			roleRows.Close()
			return nil, fmt.Errorf("iterate module roles: %w", err)
		}
		roleRows.Close()

		permissionRows, err := db.Query(ctx, `
			SELECT DISTINCT p.id
			FROM user_module_roles umr
			INNER JOIN roles r ON r.id = umr.role_id
			INNER JOIN role_permissions rp ON rp.role_id = r.id
			INNER JOIN permissions p ON p.id = rp.permission_id
			WHERE umr.user_id = $1::uuid
				AND r.is_active = TRUE
				AND p.module_id = umr.module_id
		`, userID)
		if err != nil {
			return nil, fmt.Errorf("load permissions: %w", err)
		}

		for permissionRows.Next() {
			var permissionID string
			if err := permissionRows.Scan(&permissionID); err != nil {
				permissionRows.Close()
				return nil, fmt.Errorf("scan permission: %w", err)
			}
			cached.Permissions[permissionID] = true
		}
		if err := permissionRows.Err(); err != nil {
			permissionRows.Close()
			return nil, fmt.Errorf("iterate permissions: %w", err)
		}
		permissionRows.Close()
	}

	key := c.cacheKey(ctx, userID)
	c.mu.Lock()
	c.store[key] = cached
	c.mu.Unlock()

	return cached, nil
}

func (c *PermissionCache) Invalidate(ctx context.Context, userID string) {
	c.InvalidateKey(c.cacheKey(ctx, userID))
}

func (c *PermissionCache) InvalidateKey(key string) {
	c.mu.Lock()
	delete(c.store, key)
	c.mu.Unlock()
}

func (c *PermissionCache) InvalidateByRole(roleID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for userID, cached := range c.store {
		for _, role := range cached.ModuleRoles {
			if role.RoleID == roleID {
				delete(c.store, userID)
				break
			}
		}
	}
}

func (c *PermissionCache) InvalidateAll() {
	c.mu.Lock()
	c.store = make(map[string]*CachedPermissions)
	c.mu.Unlock()
}

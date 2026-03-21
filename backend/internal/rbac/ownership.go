package rbac

func CanViewAll(perms *CachedPermissions, viewAllPermission string) bool {
	if perms == nil {
		return false
	}

	return perms.IsSuperAdmin || perms.Permissions[viewAllPermission]
}

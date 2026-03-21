ALTER TABLE IF EXISTS role_permissions
    RENAME TO role_permissions_v2_tmp;

ALTER TABLE IF EXISTS roles
    RENAME TO roles_v2_tmp;

ALTER TABLE IF EXISTS permissions
    RENAME TO permissions_v2_tmp;

ALTER TABLE IF EXISTS roles_deprecated
    RENAME TO roles;

ALTER TABLE IF EXISTS permissions_deprecated
    RENAME TO permissions;

ALTER TABLE IF EXISTS user_roles_deprecated
    RENAME TO user_roles;

ALTER TABLE IF EXISTS role_permissions_deprecated
    RENAME TO role_permissions;

DROP TABLE IF EXISTS user_module_roles;
DROP TABLE IF EXISTS system_settings;
DROP TABLE IF EXISTS role_permissions_v2_tmp;
DROP TABLE IF EXISTS permissions_v2_tmp;
DROP TABLE IF EXISTS roles_v2_tmp;
DROP TABLE IF EXISTS modules;

DROP INDEX IF EXISTS idx_users_is_super_admin;

ALTER TABLE users
    DROP COLUMN IF EXISTS is_super_admin;

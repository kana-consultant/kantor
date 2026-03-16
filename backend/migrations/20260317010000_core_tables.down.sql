DROP INDEX IF EXISTS idx_audit_logs_created_at;
DROP INDEX IF EXISTS idx_audit_logs_module;
DROP INDEX IF EXISTS idx_audit_logs_user_id;
DROP INDEX IF EXISTS idx_permissions_action;
DROP INDEX IF EXISTS idx_permissions_resource;
DROP INDEX IF EXISTS idx_permissions_module;
DROP INDEX IF EXISTS idx_roles_module;
DROP INDEX IF EXISTS idx_roles_name;

DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS role_permissions;
DROP TABLE IF EXISTS user_roles;
DROP TABLE IF EXISTS permissions;
DROP TABLE IF EXISTS roles;
DROP TABLE IF EXISTS users;

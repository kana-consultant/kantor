ALTER TABLE users
    ADD COLUMN IF NOT EXISTS is_super_admin BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE modules (
    id VARCHAR(50) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    display_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE permissions_v2 (
    id VARCHAR(100) PRIMARY KEY,
    module_id VARCHAR(50) NOT NULL REFERENCES modules (id) ON DELETE CASCADE,
    resource VARCHAR(50) NOT NULL,
    action VARCHAR(50) NOT NULL,
    description TEXT,
    is_sensitive BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_permissions_v2_module_resource_action UNIQUE (module_id, resource, action)
);

CREATE TABLE roles_v2 (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    slug VARCHAR(50) NOT NULL UNIQUE,
    description TEXT,
    is_system BOOLEAN NOT NULL DEFAULT FALSE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    hierarchy_level INT NOT NULL DEFAULT 50,
    created_by UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE role_permissions_v2 (
    role_id UUID NOT NULL REFERENCES roles_v2 (id) ON DELETE CASCADE,
    permission_id VARCHAR(100) NOT NULL REFERENCES permissions_v2 (id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);

CREATE TABLE user_module_roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    module_id VARCHAR(50) NOT NULL REFERENCES modules (id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES roles_v2 (id) ON DELETE CASCADE,
    assigned_by UUID REFERENCES users (id) ON DELETE SET NULL,
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, module_id)
);

CREATE TABLE system_settings (
    key VARCHAR(100) PRIMARY KEY,
    value JSONB NOT NULL,
    description TEXT,
    updated_by UUID REFERENCES users (id) ON DELETE SET NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_modules_display_order ON modules (display_order);
CREATE INDEX idx_rbac_permissions_module_id ON permissions_v2 (module_id);
CREATE INDEX idx_rbac_permissions_resource ON permissions_v2 (resource);
CREATE INDEX idx_rbac_permissions_action ON permissions_v2 (action);
CREATE INDEX idx_rbac_roles_slug ON roles_v2 (slug);
CREATE INDEX idx_rbac_roles_active ON roles_v2 (is_active);
CREATE INDEX idx_user_module_roles_user_id ON user_module_roles (user_id);
CREATE INDEX idx_user_module_roles_module_id ON user_module_roles (module_id);
CREATE INDEX idx_user_module_roles_role_id ON user_module_roles (role_id);
CREATE INDEX idx_users_is_super_admin ON users (is_super_admin);

ALTER TABLE role_permissions
    RENAME TO role_permissions_deprecated;

ALTER TABLE user_roles
    RENAME TO user_roles_deprecated;

ALTER TABLE permissions
    RENAME TO permissions_deprecated;

ALTER TABLE roles
    RENAME TO roles_deprecated;

ALTER TABLE permissions_v2
    RENAME TO permissions;

ALTER TABLE roles_v2
    RENAME TO roles;

ALTER TABLE role_permissions_v2
    RENAME TO role_permissions;

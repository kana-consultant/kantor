CREATE TABLE employees (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID UNIQUE REFERENCES users (id) ON DELETE SET NULL,
    full_name TEXT NOT NULL,
    email TEXT NOT NULL,
    phone TEXT,
    position TEXT NOT NULL,
    department TEXT,
    date_joined DATE NOT NULL,
    employment_status TEXT NOT NULL DEFAULT 'active',
    address TEXT,
    emergency_contact TEXT,
    avatar_url TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_employees_email UNIQUE (email),
    CONSTRAINT chk_employees_employment_status CHECK (employment_status IN ('active', 'probation', 'resigned', 'terminated'))
);

CREATE TABLE departments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    description TEXT,
    head_id UUID REFERENCES employees (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_departments_name UNIQUE (name)
);

CREATE INDEX idx_employees_user_id ON employees (user_id);
CREATE INDEX idx_employees_department ON employees (department);
CREATE INDEX idx_employees_employment_status ON employees (employment_status);
CREATE INDEX idx_employees_date_joined ON employees (date_joined);
CREATE INDEX idx_departments_head_id ON departments (head_id);

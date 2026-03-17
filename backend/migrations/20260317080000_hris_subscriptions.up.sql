CREATE TABLE subscriptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    vendor TEXT NOT NULL,
    description TEXT,
    cost_amount BIGINT NOT NULL,
    cost_currency TEXT NOT NULL DEFAULT 'IDR',
    billing_cycle TEXT NOT NULL,
    start_date DATE NOT NULL,
    renewal_date DATE NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    pic_employee_id UUID REFERENCES employees (id) ON DELETE SET NULL,
    category TEXT NOT NULL,
    login_credentials_encrypted TEXT,
    notes TEXT,
    created_by UUID NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_subscriptions_billing_cycle CHECK (billing_cycle IN ('monthly', 'quarterly', 'yearly')),
    CONSTRAINT chk_subscriptions_status CHECK (status IN ('active', 'cancelled', 'expired'))
);

CREATE TABLE subscription_alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    subscription_id UUID NOT NULL REFERENCES subscriptions (id) ON DELETE CASCADE,
    alert_type TEXT NOT NULL,
    is_read BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_subscription_alert_type CHECK (alert_type IN ('30_days', '7_days', '1_day'))
);

CREATE INDEX idx_subscriptions_status ON subscriptions (status);
CREATE INDEX idx_subscriptions_renewal_date ON subscriptions (renewal_date);
CREATE INDEX idx_subscriptions_pic_employee_id ON subscriptions (pic_employee_id);
CREATE INDEX idx_subscription_alerts_is_read ON subscription_alerts (is_read);
CREATE INDEX idx_subscription_alerts_subscription_id ON subscription_alerts (subscription_id);

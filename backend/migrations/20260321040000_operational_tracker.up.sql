CREATE TABLE activity_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    date DATE NOT NULL,
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ,
    total_active_seconds INT NOT NULL DEFAULT 0,
    total_idle_seconds INT NOT NULL DEFAULT 0,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_activity_sessions_user_date ON activity_sessions(user_id, date DESC);
CREATE INDEX idx_activity_sessions_is_active ON activity_sessions(is_active);

CREATE TABLE activity_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL REFERENCES activity_sessions(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    url TEXT NOT NULL,
    domain VARCHAR(255) NOT NULL,
    page_title VARCHAR(500),
    category VARCHAR(50) NOT NULL DEFAULT 'uncategorized',
    duration_seconds INT NOT NULL DEFAULT 0,
    started_at TIMESTAMPTZ NOT NULL,
    ended_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_activity_entries_session_id ON activity_entries(session_id);
CREATE INDEX idx_activity_entries_user_id_started_at ON activity_entries(user_id, started_at DESC);
CREATE INDEX idx_activity_entries_domain ON activity_entries(domain);
CREATE INDEX idx_activity_entries_category ON activity_entries(category);

CREATE TABLE domain_categories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    domain_pattern VARCHAR(255) NOT NULL UNIQUE,
    category VARCHAR(50) NOT NULL,
    is_productive BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_domain_categories_category ON domain_categories(category);

CREATE TABLE activity_consents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    consented BOOLEAN NOT NULL DEFAULT FALSE,
    consented_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    ip_address VARCHAR(45),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO domain_categories (domain_pattern, category, is_productive) VALUES
    ('github.com', 'development', TRUE),
    ('gitlab.com', 'development', TRUE),
    ('bitbucket.org', 'development', TRUE),
    ('docs.google.com', 'documentation', TRUE),
    ('notion.so', 'documentation', TRUE),
    ('confluence.atlassian.com', 'documentation', TRUE),
    ('slack.com', 'communication', TRUE),
    ('discord.com', 'communication', TRUE),
    ('teams.microsoft.com', 'communication', TRUE),
    ('figma.com', 'design', TRUE),
    ('canva.com', 'design', TRUE),
    ('youtube.com', 'entertainment', FALSE),
    ('netflix.com', 'entertainment', FALSE),
    ('twitch.tv', 'entertainment', FALSE),
    ('facebook.com', 'social_media', FALSE),
    ('instagram.com', 'social_media', FALSE),
    ('twitter.com', 'social_media', FALSE),
    ('tiktok.com', 'social_media', FALSE),
    ('mail.google.com', 'communication', TRUE),
    ('outlook.com', 'communication', TRUE),
    ('localhost', 'development', TRUE),
    ('127.0.0.1', 'development', TRUE);

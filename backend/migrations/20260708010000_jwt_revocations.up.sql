-- jwt_revocations is the shared store for revoked JWT identifiers (JTIs).
-- It is intentionally global — NOT RLS-scoped — because:
--   1. The blacklist is consulted by AuthMiddleware before tenant resolution
--      can happen, so a tenant-scoped query would be wrong-shaped.
--   2. The JTI itself is globally unique; the original token's tenant_id is
--      recorded as advisory metadata only, used for diagnostics and audit
--      queries like "which tokens did tenant X revoke this week".
--
-- Writes come from internal/auth.RevokeAccessToken (logout, admin revoke,
-- change-password cascade). Reads come from every authenticated request via
-- AuthMiddleware. Reads are O(1) thanks to the primary key on jti and the
-- index on expires_at keeps the periodic purge cheap.
CREATE TABLE IF NOT EXISTS jwt_revocations (
    jti        TEXT        PRIMARY KEY,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    user_id    UUID,
    tenant_id  UUID
);

CREATE INDEX IF NOT EXISTS jwt_revocations_expires_at_idx
    ON jwt_revocations (expires_at);

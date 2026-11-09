-- Set by the Coordinator for directory accounts that need a paid subscription.
-- 0 for every existing account, so nothing changes until the Coordinator locks one.
ALTER TABLE users ADD COLUMN subscription_locked INTEGER NOT NULL DEFAULT 0;

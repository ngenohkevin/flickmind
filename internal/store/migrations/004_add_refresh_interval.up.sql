ALTER TABLE user_config ADD COLUMN IF NOT EXISTS refresh_interval TEXT DEFAULT '2h';

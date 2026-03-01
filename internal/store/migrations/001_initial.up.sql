CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS user_config (
    user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    groq_key TEXT DEFAULT '',
    deepseek_key TEXT DEFAULT '',
    gemini_key TEXT DEFAULT '',
    trakt_access_token TEXT DEFAULT '',
    trakt_refresh_token TEXT DEFAULT '',
    trakt_expires_at TIMESTAMP,
    genres TEXT DEFAULT '',
    content_types TEXT DEFAULT 'movie,series',
    language TEXT DEFAULT 'en',
    mood TEXT DEFAULT '',
    min_rating REAL DEFAULT 0,
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS recommendation_cache (
    cache_key TEXT PRIMARY KEY,
    data TEXT NOT NULL,
    expires_at TIMESTAMP NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_cache_expires ON recommendation_cache(expires_at);

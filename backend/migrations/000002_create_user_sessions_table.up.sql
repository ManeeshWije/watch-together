CREATE TABLE IF NOT EXISTS user_sessions (
    uuid UUID PRIMARY KEY NOT NULL,
    user_uuid UUID REFERENCES users(uuid) NOT NULL,
    created_at TIMESTAMP,
    expires_at TIMESTAMP,
    CONSTRAINT FK_user_session FOREIGN KEY(user_uuid)
        REFERENCES users(uuid)
);

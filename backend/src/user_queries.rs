use crate::types::{User, UserSession};
use sqlx::PgPool;
use std::time::Duration;
use uuid::Uuid;

pub async fn fetch_user_by_email(pool: &PgPool, email: &str) -> Result<User, sqlx::Error> {
    let user = sqlx::query_as!(
        User,
        "
        SELECT * FROM users
        WHERE email = $1
        ",
        email
    )
    .fetch_one(pool)
    .await?;

    Ok(user)
}

pub async fn fetch_user_by_session_uuid(
    pool: &PgPool,
    session_uuid: Uuid,
) -> Result<User, sqlx::Error> {
    let user = sqlx::query_as!(
        User,
        "
        SELECT u.uuid, u.username, u.email, u.created_at, u.num_uploads, u.is_admin
        FROM users AS u
        LEFT JOIN user_sessions AS s ON u.uuid = s.user_uuid
        WHERE s.uuid = $1 AND s.expires_at > $2
        ",
        session_uuid,
        chrono::Utc::now().naive_utc()
    )
    .fetch_one(pool)
    .await?;

    Ok(user)
}

pub async fn fetch_user_session_by_user_uuid(
    pool: &PgPool,
    user_uuid: Uuid,
) -> Result<UserSession, sqlx::Error> {
    let user_session = sqlx::query_as!(
        UserSession,
        "
        SELECT * FROM user_sessions
        WHERE user_uuid = $1
        ",
        user_uuid
    )
    .fetch_one(pool)
    .await?;

    Ok(user_session)
}

pub async fn create_user(
    pool: &PgPool,
    uuid: Uuid,
    username: &str,
    email: &str,
) -> Result<User, sqlx::Error> {
    let created_at_timestamp = chrono::offset::Utc::now().naive_utc();
    let user = sqlx::query_as!(
        User,
        "
        INSERT INTO users (uuid, username, email, created_at, num_uploads, is_admin)
        VALUES ($1, $2, $3, $4, 0, false)
        RETURNING *
        ",
        uuid,
        username,
        email,
        created_at_timestamp,
    )
    .fetch_one(pool)
    .await?;

    Ok(user)
}

pub async fn create_user_session(
    pool: &PgPool,
    user_uuid: Uuid,
    session_duration: Duration,
) -> Result<UserSession, sqlx::Error> {
    let uuid = Uuid::new_v4();
    let created_at_timestamp = chrono::offset::Utc::now().naive_utc();
    let expires_at_timestamp =
        created_at_timestamp + chrono::Duration::seconds(session_duration.as_secs() as i64);

    sqlx::query_as!(
        UserSession,
        "
        INSERT INTO user_sessions (uuid, user_uuid, created_at, expires_at)
        VALUES ($1, $2, $3, $4)
        ",
        uuid,
        user_uuid,
        created_at_timestamp,
        expires_at_timestamp,
    )
    .execute(pool)
    .await?;

    let user_session = sqlx::query_as!(
        UserSession,
        "
        SELECT * FROM user_sessions
        WHERE uuid = $1
        ",
        uuid
    )
    .fetch_one(pool)
    .await?;

    Ok(user_session)
}

pub async fn _delete_user(pool: &PgPool, uuid: Uuid) -> Result<(), sqlx::Error> {
    sqlx::query!(
        "
        DELETE FROM users
        WHERE uuid = $1
        ",
        uuid
    )
    .execute(pool)
    .await?;

    Ok(())
}

pub async fn delete_user_session(pool: &PgPool, session_uuid: Uuid) -> Result<(), sqlx::Error> {
    sqlx::query!(
        "
        DELETE FROM user_sessions
        WHERE uuid = $1
        ",
        session_uuid
    )
    .execute(pool)
    .await?;

    Ok(())
}

pub async fn delete_expired_sessions(pool: &PgPool) -> Result<(), sqlx::Error> {
    sqlx::query!(
        "
        DELETE FROM user_sessions
        WHERE expires_at < $1
        ",
        chrono::offset::Utc::now().naive_utc()
    )
    .execute(pool)
    .await?;

    Ok(())
}

pub async fn increment_uploads(pool: &PgPool, user_uuid: Uuid) -> Result<bool, sqlx::Error> {
    // Fetch the user to check the current number of uploads and if the user is an admin
    let user = sqlx::query_as!(
        User,
        "
        SELECT *
        FROM users
        WHERE uuid = $1
        ",
        user_uuid
    )
    .fetch_one(pool)
    .await?;

    // If the user is an admin, allow the upload without incrementing
    if user.is_admin.unwrap_or(false) {
        return Ok(true);
    }

    // If num_uploads is already at 5, deny the upload without incrementing
    if user.num_uploads.unwrap_or(0) >= 5 {
        return Ok(false);
    }

    // Increment num_uploads
    let _ = sqlx::query!(
        "
        UPDATE users
        SET num_uploads = num_uploads + 1
        WHERE uuid = $1
        ",
        user_uuid
    )
    .execute(pool)
    .await?;

    Ok(true)
}

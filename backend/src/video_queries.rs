use crate::types::Video;
use chrono::NaiveDateTime;
use sqlx::PgPool;

pub async fn list_videos(pool: &PgPool) -> Result<Vec<Video>, sqlx::Error> {
    let videos = sqlx::query_as!(Video, "SELECT url, title, size, created_at FROM videos")
        .fetch_all(pool)
        .await?;

    Ok(videos)
}

pub async fn get_video(pool: &PgPool, url: &str) -> Result<Option<Video>, sqlx::Error> {
    let video = sqlx::query_as!(
        Video,
        "SELECT url, title, size, created_at FROM videos WHERE url = $1",
        url,
    )
    .fetch_optional(pool)
    .await?;

    Ok(video)
}

pub async fn delete_video(pool: &PgPool, title: &str) -> Result<(), sqlx::Error> {
    let result = sqlx::query!("DELETE FROM videos WHERE title = $1", title)
        .execute(pool)
        .await?;

    if result.rows_affected() == 0 {
        return Err(sqlx::Error::RowNotFound);
    }

    Ok(())
}

pub async fn create_video(
    pool: &PgPool,
    url: &str,
    title: &str,
    size: i32,
    created_at: NaiveDateTime,
) -> Result<(), sqlx::Error> {
    sqlx::query!(
        "INSERT INTO videos (url, title, size, created_at) VALUES ($1, $2, $3, $4)",
        url,
        title,
        size,
        created_at
    )
    .execute(pool)
    .await?;

    Ok(())
}

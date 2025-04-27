use crate::{
    aws,
    types::{AddVideoRequest, AppState},
    user_handler, user_queries, video_queries,
};
use axum::{
    body::Bytes,
    extract::{ws::Message, Path, State},
    http::StatusCode,
    response::{IntoResponse, Json, Response},
};
use axum_extra::extract::CookieJar;
use chrono::Utc;
use std::time::Duration;
use tokio::{sync::mpsc::Sender, time::sleep};

pub async fn get_video(
    cookies: CookieJar,
    Path(video_key): Path<String>,
    State(app_state): State<AppState>,
) -> Response {
    println!("Requested video: {:?}", video_key);

    let user_uuid = match user_handler::get_user_from_session(cookies, &app_state).await {
        Ok(uuid) => uuid,
        Err(response) => return response,
    };

    // Check if the WebSocket client exists before attempting any S3 operations
    let sender = {
        let clients = app_state.web_socket_clients.lock().await;
        clients.get(&user_uuid.to_string()).cloned()
    };

    if sender.is_none() {
        return (StatusCode::NOT_FOUND, Json("WebSocket client not found")).into_response();
    }

    let sender = sender.unwrap();

    // Spawn a separate task for downloading from S3 and streaming
    let video_key_clone = video_key.clone();
    tokio::spawn(async move {
        println!("Started background task to fetch video: {}", video_key);

        match aws::get_object(
            &app_state.aws_client,
            &app_state.aws_s3_bucket,
            video_key_clone,
        )
        .await
        {
            Ok(video_data) => {
                println!("Fetched video: {} ({} bytes)", video_key, video_data.len());
                send_video_in_chunks(sender, video_data).await;
            }
            Err(e) => {
                println!("Failed to fetch video: {:?}", e);
                let _ = sender
                    .send(Message::Text(format!(
                        "ERROR: Failed to retrieve video: {}",
                        e
                    )))
                    .await;
            }
        }
    });

    // Return immediate response to client that streaming has started
    (
        StatusCode::ACCEPTED,
        Json("Video streaming started via WebSocket"),
    )
        .into_response()
}

// Function to send the video in small chunks to avoid flooding WebSocket
async fn send_video_in_chunks(sender: Sender<Message>, video_bytes: Bytes) {
    let chunk_size = 5 * 1024 * 1024; // 5MB chunks
    let total_size = video_bytes.len();
    let mut sent_bytes = 0;

    while sent_bytes < total_size {
        let end = (sent_bytes + chunk_size).min(total_size);
        let chunk = &video_bytes[sent_bytes..end];

        if sender.send(Message::Binary(chunk.to_vec())).await.is_err() {
            println!("WebSocket client disconnected.");
            break;
        }

        sent_bytes = end;

        // Sleep briefly to avoid overwhelming the WebSocket connection
        sleep(Duration::from_millis(10)).await;
    }

    // Send a completion message after all chunks are sent
    if sent_bytes >= total_size {
        let _ = sender
            .send(Message::Text("VIDEO_SEND_COMPLETED".to_string()))
            .await;
        println!("Sent completion message");
    }

    println!("Finished streaming video");
}

pub async fn add_video(
    cookies: CookieJar,
    State(app_state): State<AppState>,
    Json(payload): Json<AddVideoRequest>,
) -> impl IntoResponse {
    let AddVideoRequest { url } = payload;

    // Fetch the video metadata, including title
    let video_details = match aws::get_video_metadata(&url).await {
        Ok(details) => details,
        Err(_) => {
            return (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json("Failed to fetch video metadata"),
            )
                .into_response();
        }
    };

    let title = video_details.title.clone();

    // Ensure video is not longer than 1 hour
    let length = video_details.length_seconds;
    if length > 3600 {
        return (
            StatusCode::PAYLOAD_TOO_LARGE,
            Json("Video size is too large"),
        )
            .into_response();
    }

    // Check upload limit
    let user_uuid = match user_handler::get_user_from_session(cookies, &app_state).await {
        Ok(uuid) => uuid,
        Err(response) => return response,
    };

    let can_upload = match user_queries::increment_uploads(&app_state.pool, user_uuid).await {
        Ok(can) => can,
        Err(_) => {
            return (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json("Error checking can_upload authority"),
            )
                .into_response();
        }
    };

    if !can_upload {
        return (
            StatusCode::INTERNAL_SERVER_ERROR,
            Json("Cannot upload more videos, please contact an admin"),
        )
            .into_response();
    }

    // Check if video already exists
    match video_queries::get_video(&app_state.pool, &url).await {
        Ok(Some(_)) => {
            return (StatusCode::BAD_REQUEST, Json("Video already exists")).into_response();
        }
        Ok(None) => {}
        Err(_) => {
            return (StatusCode::INTERNAL_SERVER_ERROR, Json("Database error")).into_response();
        }
    }

    // Now we spin off the S3 upload in a background task
    tokio::spawn(async move {
        match aws::download_video_upload_s3(
            &app_state.aws_client,
            &app_state.aws_s3_bucket,
            &url,
            &title,
            app_state.web_socket_clients.clone(),
            user_uuid.to_string(),
        )
        .await
        {
            Ok(file_size) => {
                let created_at = Utc::now().naive_utc();
                if let Err(err) = video_queries::create_video(
                    &app_state.pool,
                    &url,
                    &title,
                    file_size as i32,
                    created_at,
                )
                .await
                {
                    eprintln!("Failed to insert video metadata: {:?}", err);
                } else {
                    println!("Successfully uploaded and inserted video: {}", title);
                    // Send VIDEO_UPLOADED event to client
                    let clients = app_state.web_socket_clients.lock().await;
                    if let Some(sender) = clients.get(&user_uuid.to_string()) {
                        let _ = sender
                            .send(Message::Text("VIDEO_UPLOADED".to_string()))
                            .await;
                        println!("Sent VIDEO_UPLOADED to client: {}", user_uuid.to_string());
                    } else {
                        println!("Client not connected for user {}", user_uuid.to_string());
                    }
                }
            }
            Err(e) => {
                eprintln!("Failed to download/upload video: {:?}", e);
            }
        }
    });

    // Return early to user without waiting for upload
    (StatusCode::OK, Json("Video upload started")).into_response()
}

pub async fn delete_video(
    State(app_state): State<AppState>,
    Path(title): Path<String>,
) -> impl IntoResponse {
    // Delete video from the database
    match video_queries::delete_video(&app_state.pool, &title).await {
        Ok(_) => {
            // If successful, delete the video from S3
            match app_state
                .aws_client
                .delete_object()
                .bucket(&app_state.aws_s3_bucket)
                .key(&title)
                .send()
                .await
            {
                Ok(_) => (StatusCode::OK, Json("Video deleted successfully")).into_response(),
                Err(_e) => (
                    StatusCode::INTERNAL_SERVER_ERROR,
                    Json("Failed to delete video from S3"),
                )
                    .into_response(),
            }
        }
        Err(_) => (StatusCode::NOT_FOUND, Json("Video not found")).into_response(),
    }
}

pub async fn list_videos(State(app_state): State<AppState>) -> impl IntoResponse {
    // Get the list of videos from the database
    match video_queries::list_videos(&app_state.pool).await {
        Ok(videos) => {
            let response = Json(videos);
            (StatusCode::OK, response).into_response()
        }
        Err(_) => (
            StatusCode::INTERNAL_SERVER_ERROR,
            Json("Failed to retrieve videos"),
        )
            .into_response(),
    }
}

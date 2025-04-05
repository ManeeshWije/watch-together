use std::collections::HashSet;

use crate::constants::COOKIE_AUTH_SESSION;
use crate::types::{self, AppState, User};
use crate::user_queries;
use axum::extract::State;
use axum::http::StatusCode;
use axum::response::{IntoResponse, Json, Response};
use axum_extra::extract::CookieJar;
use uuid::Uuid;

pub async fn get_user_from_session(
    cookies: CookieJar,
    app_state: &AppState,
) -> Result<uuid::Uuid, Response> {
    // Extract the session UUID from the cookie
    let session_uuid = match cookies.get(COOKIE_AUTH_SESSION) {
        Some(cookie) => cookie.value().to_string(),
        None => {
            // Return a proper Response with an error message and StatusCode
            return Err(Response::builder()
                .status(StatusCode::BAD_REQUEST)
                .header("Content-Type", "application/json")
                .body(
                    Json("Session cookie missing or invalid")
                        .into_response()
                        .into_body(),
                )
                .unwrap());
        }
    };
    // Convert session_uuid string to Uuid
    let session_uuid = match Uuid::parse_str(&session_uuid) {
        Ok(uuid) => uuid,
        Err(_) => {
            // Return a proper Response with an error message and StatusCode
            return Err(Response::builder()
                .status(StatusCode::BAD_REQUEST)
                .header("Content-Type", "application/json")
                .body(
                    Json("Invalid session UUID format")
                        .into_response()
                        .into_body(),
                )
                .unwrap());
        }
    };

    // Fetch the user associated with the session UUID
    let user = match user_queries::fetch_user_by_session_uuid(&app_state.pool, session_uuid).await {
        Ok(user) => user,
        Err(_) => {
            // Return a proper Response with an error message and StatusCode
            return Err(Response::builder()
                .status(StatusCode::NOT_FOUND)
                .header("Content-Type", "application/json")
                .body(
                    Json("User not found for session UUID")
                        .into_response()
                        .into_body(),
                )
                .unwrap());
        }
    };

    Ok(user.uuid)
}

pub async fn get_connected_users(State(app_state): State<AppState>) -> Json<Vec<types::User>> {
    // Get the set of connected client IDs from the WebSocket clients map
    let connected_client_ids: HashSet<String> = {
        let clients = app_state.web_socket_clients.lock().await;
        println!("WebSocket clients map contains {} clients", clients.len());
        clients.keys().cloned().collect()
    };
    
    println!("Found {} connected client IDs", connected_client_ids.len());
    
    // Prepare the result vector
    let mut connected_users = Vec::new();
    
    // For each connected client ID (which is a user UUID string)
    for client_id in connected_client_ids {
        println!("Processing client ID: {}", client_id);
        
        // Parse the client ID string into a UUID
        match Uuid::parse_str(&client_id) {
            Ok(user_uuid) => {
                // Fetch user details from the database
                match user_queries::fetch_user_by_uuid(&app_state.pool, user_uuid).await {
                    Ok(user) => {
                        println!("Found user: {} ({})", user.username, user_uuid);
                        connected_users.push(User {
                            uuid: user.uuid,
                            username: user.username,
                            email: user.email,
                            is_admin: user.is_admin,
                            created_at: user.created_at,
                            num_uploads: Some(user.num_uploads.unwrap_or_default()),
                        });
                    },
                    Err(e) => println!("Failed to fetch user {}: {:?}", user_uuid, e),
                }
            },
            Err(e) => println!("Failed to parse UUID from client ID {}: {:?}", client_id, e),
        }
    }
    
    println!("Returning {} connected users", connected_users.len());
    Json(connected_users)
}

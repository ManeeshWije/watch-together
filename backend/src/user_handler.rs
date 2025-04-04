use crate::constants::COOKIE_AUTH_SESSION;
use crate::types::AppState;
use crate::user_queries;
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

    // Return the user UUID (or the user object if needed)
    Ok(user.uuid)
}

use aws_sdk_s3::Client;
use axum::extract::ws::Message;
use chrono::NaiveDateTime;
use serde::{Deserialize, Serialize};
use sqlx::PgPool;
use std::{collections::HashMap, sync::Arc};
use tokio::sync::{broadcast::Sender, mpsc, Mutex};
use uuid::Uuid;

#[derive(Debug, Serialize, Deserialize)]
pub struct User {
    pub uuid: Uuid,
    pub username: String,
    pub email: String,
    pub created_at: Option<NaiveDateTime>,
    pub num_uploads: Option<i32>,
    pub is_admin: Option<bool>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct UserSession {
    pub uuid: Uuid,
    pub user_uuid: Uuid,
    pub created_at: Option<NaiveDateTime>,
    pub expires_at: Option<NaiveDateTime>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct Video {
    pub url: String,
    pub title: String,
    pub size: Option<i32>,
    pub created_at: Option<NaiveDateTime>,
}

#[derive(Debug, Clone)]
pub struct AppState {
    pub broadcast_tx: Arc<Sender<Message>>,
    pub web_socket_clients: Arc<Mutex<HashMap<String, mpsc::Sender<Message>>>>,
    pub aws_s3_bucket: String,
    pub aws_client: Client,
    pub pool: PgPool,
}

// What we get back from Google
#[derive(Default, Debug, serde::Serialize, serde::Deserialize)]
pub struct GoogleUser {
    pub sub: String,
    pub name: String,
    pub email: String,
}

// What we send to Google
#[derive(Debug, serde::Serialize, serde::Deserialize)]
pub struct AuthRequest {
    pub code: String,
    pub state: String,
}

#[derive(Serialize)]
pub struct AuthResponse {
    pub authenticated: bool,
    pub user: Option<User>,
}

#[derive(Deserialize)]
pub struct AddVideoRequest {
    pub url: String,
}

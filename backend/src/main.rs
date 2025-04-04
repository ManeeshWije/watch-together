mod auth;
mod aws;
mod connection;
mod constants;
mod types;
mod user_handler;
mod user_queries;
mod video_handler;
mod video_queries;

use aws_config::load_from_env;
use aws_sdk_s3::Client;
use axum::{
    body::Body,
    extract::{
        ws::{Message, WebSocket},
        State, WebSocketUpgrade,
    },
    response::{Redirect, Response},
    routing::{get, post},
    Router,
};
use axum_extra::extract::CookieJar;
use dotenv::dotenv;
use futures::SinkExt;
use futures_util::StreamExt;
use http::{header::{CONNECTION, CONTENT_TYPE, UPGRADE}, HeaderValue, Method};
use sqlx::PgPool;
use std::{collections::HashMap, fs, time::Duration};
use std::{env, net::SocketAddr, sync::Arc};
use tokio::{
    sync::{
        broadcast::{self},
        mpsc, Mutex,
    },
    time::interval,
};
use tower_http::trace::{DefaultMakeSpan, TraceLayer};
use tower_http::{cors::CorsLayer, services::ServeDir};
use tracing_subscriber::{layer::SubscriberExt, util::SubscriberInitExt};

#[tokio::main]
async fn main() {
    dotenv().ok();
    tracing_subscriber::registry()
        .with(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| "tower_http=debug".into()),
        )
        .with(tracing_subscriber::fmt::layer())
        .init();

    if let Err(e) = fs::create_dir_all("downloads") {
        eprintln!("Failed to create downloads directory: {}", e);
    } else {
        println!("Downloads directory created or already exists.");
    }

    let aws_s3_bucket = env::var("AWS_S3_BUCKET").unwrap_or_default();
    let pool = connection::connect(env::var("DATABASE_URL").unwrap_or_default())
        .await
        .unwrap_or_else(|err| {
            eprintln!("Database connection failed: {:?}", err);
            std::process::exit(1);
        });
    let cfg = load_from_env().await;
    let s3 = Client::new(&cfg);

    tokio::spawn(delete_expired_sessions_task(pool.clone()));

    let (tx, rx) = broadcast::channel(32);

    let web_socket_clients = Arc::new(Mutex::new(HashMap::new()));

    let app_state = types::AppState {
        broadcast_tx: Arc::new(tx),
        _broadcast_rx: Arc::new(rx),
        web_socket_clients,
        aws_s3_bucket,
        aws_client: s3,
        pool,
    };

    let dist_dir = if cfg!(debug_assertions) {
        "../frontend/dist/"
    } else {
        "./dist"
    };

    let cors_origin = env::var("CLIENT_URL")
        .unwrap()
        .as_str()
        .parse::<HeaderValue>()
        .unwrap();

    let cors_middleware = CorsLayer::new()
        .allow_methods([Method::GET, Method::POST, Method::DELETE, Method::PUT])
        .allow_origin(cors_origin)
        .allow_headers(vec![CONTENT_TYPE, UPGRADE, CONNECTION])
        .allow_credentials(true);

    // build our application with some routes
    let app = Router::new()
        .nest_service("/", ServeDir::new(dist_dir))
        .route("/video", get(video_redirect))
        .route("/ws", get(websocket_handler))
        .route("/add-video", post(video_handler::add_video))
        .route("/get-video/:title", get(video_handler::get_video))
        .route("/list-videos", get(video_handler::list_videos))
        .route("/delete-video/:title", post(video_handler::delete_video))
        .route("/auth/logout", get(auth::logout))
        .route("/auth/session", get(auth::session))
        .route("/auth/google/login", get(auth::login))
        .route("/auth/google/callback", get(auth::callback))
        .with_state(app_state)
        .layer(cors_middleware)
        .layer(
            TraceLayer::new_for_http()
                .make_span_with(DefaultMakeSpan::default().include_headers(true)),
        );

    // run it with hyper
    let listener = tokio::net::TcpListener::bind("0.0.0.0:8080").await.unwrap();
    tracing::debug!("listening on {}", listener.local_addr().unwrap());
    axum::serve(
        listener,
        app.into_make_service_with_connect_info::<SocketAddr>(),
    )
    .await
    .unwrap();
}

async fn video_redirect() -> Redirect {
    Redirect::to("/")
}

async fn websocket_handler(
    ws: WebSocketUpgrade,
    cookies: CookieJar,
    State(app_state): State<types::AppState>,
) -> Response<Body> {
    ws.on_upgrade(|socket| handle_socket(socket, app_state, cookies))
}

async fn handle_socket(socket: WebSocket, app_state: types::AppState, cookies: CookieJar) {
    let user_uuid = match user_handler::get_user_from_session(cookies, &app_state).await {
        Ok(uuid) => uuid,
        Err(_) => return, // Handle error within the helper function
    };

    // Create client ID from user UUID
    let client_id = user_uuid.to_string();

    // Split the WebSocket
    let (mut client_tx, mut client_rx) = socket.split();

    // Create a channel for this specific client
    let (tx, mut rx) = mpsc::channel::<Message>(100);

    // Keep track of the last message to avoid duplicates
    let last_message = Arc::new(Mutex::new(None::<String>));
    let last_message_clone = last_message.clone();

    // Add this client to our clients map
    app_state
        .web_socket_clients
        .lock()
        .await
        .insert(client_id.clone(), tx);

    // Subscribe to the broadcast channel
    let mut broadcast_rx = app_state.broadcast_tx.subscribe();

    // Task 1: Forward messages from the mpsc channel to the client's WebSocket
    let forward_task = tokio::spawn(async move {
        while let Some(message) = rx.recv().await {
            if client_tx.send(message).await.is_err() {
                break;
            }
        }
    });

    // Task 2: Process messages from the client, record last message sent, and broadcast
    let broadcast_tx = Arc::clone(&app_state.broadcast_tx);
    let client_id_clone = client_id.clone();
    let clients = Arc::clone(&app_state.web_socket_clients);
    let receive_task = tokio::spawn(async move {
        while let Some(Ok(msg)) = client_rx.next().await {
            // Process the received message
            if let Message::Text(text) = &msg {
                println!("Received message from user {}: {}", client_id_clone, text);

                // Store this message as the last one sent by this client
                *last_message.lock().await = Some(text.clone());
            }

            // Broadcast to all clients (including self, we'll filter on receive)
            if broadcast_tx.send(msg).is_err() {
                break;
            }
        }

        // Remove client from the list on disconnection
        clients.lock().await.remove(&client_id_clone);
    });

    // Task 3: Receive broadcast messages, filter out duplicates for this client
    let clients_clone = Arc::clone(&app_state.web_socket_clients);
    let broadcast_task = tokio::spawn(async move {
        while let Ok(msg) = broadcast_rx.recv().await {
            // Check if this is our own last sent message (to avoid duplicates)
            let should_forward = match &msg {
                Message::Text(text) => {
                    let last_msg = last_message_clone.lock().await;
                    !last_msg.as_ref().is_some_and(|last| last == text)
                }
                _ => true, // Always forward binary messages
            };

            // Forward to client if it's not a duplicate of our last sent message
            if should_forward {
                if let Some(sender) = clients_clone.lock().await.get(&client_id) {
                    if sender.send(msg).await.is_err() {
                        break;
                    }
                }
            }
        }
    });

    // Wait for any task to finish
    tokio::select! {
        _ = forward_task => {},
        _ = receive_task => {},
        _ = broadcast_task => {},
    }
}

async fn delete_expired_sessions_task(pool: PgPool) {
    let mut interval = interval(Duration::from_secs(60 * 60 * 24)); // Run once a day
    loop {
        interval.tick().await;
        if let Err(err) = user_queries::delete_expired_sessions(&pool).await {
            eprintln!("Failed to delete expired sessions: {}", err);
        } else {
            println!("Expired sessions deleted successfully.");
        }
    }
}

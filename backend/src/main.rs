mod auth;
mod aws;
mod connection;
mod constants;
mod rate_limiter;
mod types;
mod user_handler;
mod user_queries;
mod video_handler;
mod video_queries;

use aws_config::load_from_env;
use aws_sdk_s3::Client;
use axum::{
    Router,
    body::Body,
    extract::{
        State, WebSocketUpgrade,
        ws::{Message, WebSocket},
    },
    middleware::from_fn_with_state,
    response::{Redirect, Response},
    routing::{get, post},
};
use axum_extra::extract::CookieJar;
use dotenv::dotenv;
use futures::SinkExt;
use futures_util::StreamExt;
use http::{
    HeaderValue, Method,
    header::{CONNECTION, CONTENT_TYPE, UPGRADE},
};
use sqlx::PgPool;
use std::{collections::HashMap, time::Duration};
use std::{env, net::SocketAddr, sync::Arc};
use tokio::{
    sync::{
        Mutex,
        broadcast::{self},
        mpsc,
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

    let aws_s3_bucket = env::var("AWS_S3_BUCKET").unwrap_or_default();
    let pool = connection::connect(env::var("DATABASE_URL").unwrap_or_default())
        .await
        .unwrap_or_else(|err| {
            eprintln!("Database connection failed: {:?}", err);
            std::process::exit(1);
        });
    let cfg = load_from_env().await;
    let s3 = Client::new(&cfg);
    let rate_limiter = rate_limiter::create_rate_limiter(50, 5);

    tokio::spawn(delete_expired_sessions_task(pool.clone()));
    tokio::spawn(rate_limiter::cleanup_rate_limiters(rate_limiter.clone()));

    let (tx, _) = broadcast::channel(32);

    let web_socket_clients = Arc::new(Mutex::new(HashMap::new()));

    let app_state = types::AppState {
        broadcast_tx: Arc::new(tx),
        web_socket_clients,
        aws_s3_bucket,
        aws_client: s3,
        pool,
        rate_limiter,
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
        .route("/users", get(user_handler::get_connected_users))
        .route("/auth/logout", get(auth::logout))
        .route("/auth/session", get(auth::session))
        .route("/auth/google/login", get(auth::login))
        .route("/auth/google/callback", get(auth::callback))
        .with_state(app_state.clone())
        .layer(from_fn_with_state(
            app_state,
            rate_limiter::rate_limit_middleware,
        ))
        .layer(cors_middleware)
        .layer(
            TraceLayer::new_for_http()
                .make_span_with(DefaultMakeSpan::default().include_headers(true)),
        );

    let port = env::var("PORT")
        .unwrap_or_else(|_| "8080".to_string())
        .parse::<u16>()
        .expect("PORT must be a valid u16");

    // run it with hyper
    let listener = tokio::net::TcpListener::bind(SocketAddr::from(([0, 0, 0, 0], port)))
        .await
        .unwrap();
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

    // Split the socket into sender and receiver parts
    let (mut sender, mut receiver) = socket.split();

    // Create a channel for this specific user
    let (tx, mut rx) = mpsc::channel::<Message>(100);

    // Add the user to the connected clients map
    {
        let mut clients = app_state.web_socket_clients.lock().await;
        clients.insert(user_uuid.clone().to_string(), tx);
    }

    // Broadcast that a new user has connected
    broadcast_to_others(
        &app_state,
        &user_uuid.to_string(),
        format!("USER_CONNECTED:{}", user_uuid),
    )
    .await;

    // Handle incoming messages from this client
    let mut recv_task = tokio::spawn(async move {
        while let Some(Ok(message)) = receiver.next().await {
            match message {
                Message::Text(text) => {
                    // Broadcast the message to all other clients
                    broadcast_to_others(&app_state, &user_uuid.to_string(), text).await;
                }
                Message::Close(_) => {
                    break;
                }
                _ => {} // Ignore other message types
            }
        }

        // User disconnected, remove from clients map and broadcast disconnect
        {
            let mut clients = app_state.web_socket_clients.lock().await;
            clients.remove(&user_uuid.to_string());
        }
        broadcast_to_others(
            &app_state,
            &user_uuid.to_string(),
            format!("USER_DISCONNECTED:{}", user_uuid),
        )
        .await;
    });

    // Handle outgoing messages to this client
    let mut send_task = tokio::spawn(async move {
        while let Some(message) = rx.recv().await {
            if sender.send(message).await.is_err() {
                break;
            }
        }
    });

    // Wait for either task to finish
    tokio::select! {
        _ = &mut recv_task => send_task.abort(),
        _ = &mut send_task => recv_task.abort(),
    }
}

// Helper function to broadcast messages to all clients except the sender
async fn broadcast_to_others(app_state: &types::AppState, sender_uuid: &str, message: String) {
    let clients = app_state.web_socket_clients.lock().await;

    for (client_uuid, tx) in clients.iter() {
        // Don't send the message back to the original sender
        if client_uuid != sender_uuid {
            // Clone the message for each client
            let msg = Message::Text(message.clone());
            // It's ok if sending fails (client might have disconnected)
            let _ = tx.send(msg).await;
        }
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

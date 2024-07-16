use aws_config::load_from_env;
use aws_sdk_s3::{types::Object, Client, Error};
use axum::{
    body::{Body, Bytes},
    extract::{
        ws::{Message, WebSocket},
        State, WebSocketUpgrade,
    },
    http::{HeaderName, StatusCode},
    response::{Html, IntoResponse, Response},
    routing::{get, post},
    Form, Router,
};
use futures::SinkExt;
use futures_util::{
    stream::{SplitSink, SplitStream},
    StreamExt,
};
use handlebars::Handlebars;
use serde::Deserialize;
use std::{collections::HashMap, time::Instant};
use std::{env, net::SocketAddr, path::PathBuf, sync::Arc};
use tokio::sync::{
    broadcast::{self, Receiver, Sender},
    Mutex,
};
use tower_http::cors::{Any, CorsLayer};
use tower_http::{
    services::ServeDir,
    trace::{DefaultMakeSpan, TraceLayer},
};
use tracing_subscriber::{layer::SubscriberExt, util::SubscriberInitExt};

#[derive(Debug, Clone)]
struct AppState {
    broadcast_tx: Arc<Mutex<Sender<Message>>>,
    handlebars: Arc<Handlebars<'static>>,
    aws_s3_bucket: String,
    password: String,
}

#[derive(Deserialize)]
struct FormData {
    password: String,
}

#[tokio::main]
async fn main() {
    tracing_subscriber::registry()
        .with(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| "tower_http=debug".into()),
        )
        .with(tracing_subscriber::fmt::layer())
        .init();

    let client_dir = PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("client");

    let aws_s3_bucket = env::var("AWS_S3_BUCKET").unwrap_or_default();
    let password = env::var("PASSWORD").unwrap_or_default();

    let mut handlebars = Handlebars::new();
    handlebars
        .register_template_file("index", "./src/views/index.hbs")
        .expect("ERROR: could not register template file - index");
    handlebars
        .register_template_file("header", "./src/views/header.hbs")
        .expect("ERROR: could not register template file - header");
    handlebars
        .register_template_file("login", "./src/views/login.hbs")
        .expect("ERROR: could not register template file - login");
    handlebars
        .register_template_file("video", "./src/views/video.hbs")
        .expect("ERROR: could not register template file - video");

    let handlebars = Arc::new(handlebars);

    let (tx, _) = broadcast::channel(32);

    let app_state = AppState {
        broadcast_tx: Arc::new(Mutex::new(tx)),
        handlebars,
        aws_s3_bucket,
        password,
    };

    let cors = CorsLayer::new()
        .allow_methods(Any)
        .allow_origin(Any)
        .allow_headers(vec![
            HeaderName::from_static("upgrade"),
            HeaderName::from_static("connection"),
        ]);

    // build our application with some routes
    let app = Router::new()
        .fallback_service(ServeDir::new(client_dir))
        .route("/", get(root_handler))
        .route("/ws", get(websocket_handler))
        .route("/submit", post(submit_handler))
        .with_state(app_state)
        .layer(cors)
        // logging so we can see whats going on
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

async fn root_handler(State(app_state): State<AppState>) -> impl IntoResponse {
    let handlebars = &app_state.handlebars;
    let index = handlebars
        .render("index", &HashMap::<String, String>::new())
        .unwrap();
    (StatusCode::OK, Html(index))
}

async fn submit_handler(
    State(app_state): State<AppState>,
    Form(form): Form<FormData>,
) -> impl IntoResponse {
    let handlebars = &app_state.handlebars;

    if form.password == app_state.password {
        let video = handlebars
            .render("video", &HashMap::<String, String>::new())
            .unwrap();
        (StatusCode::OK, Html(video))
    } else {
        let login = handlebars
            .render("login", &HashMap::<String, String>::new())
            .unwrap();
        (StatusCode::UNAUTHORIZED, Html(login))
    }
}

async fn websocket_handler(
    ws: WebSocketUpgrade,
    State(app_state): State<AppState>,
) -> Response<Body> {
    ws.on_upgrade(|socket| handle_socket(socket, app_state))
}

async fn handle_socket(mut ws: WebSocket, app_state: AppState) {
    let start = Instant::now();
    let video_bytes = fetch_video(&app_state).await;
    // Send the initial video content to the client
    if let Err(e) = ws
        .send(Message::Binary(
            video_bytes
                .expect("ERROR: failed to send video bytes")
                .to_vec(),
        ))
        .await
    {
        eprintln!("Failed to send initial video content: {}", e);
        return;
    }

    let end_time = Instant::now();
    let duration = end_time - start;
    println!("Time taken to send video bytes: {:?}", duration);

    let (ws_tx, ws_rx) = ws.split();
    let ws_tx = Arc::new(Mutex::new(ws_tx));

    let broadcast_rx = app_state.broadcast_tx.lock().await.subscribe();
    tokio::spawn(async move {
        recv_broadcast(ws_tx, broadcast_rx).await;
    });

    recv_from_client(ws_rx, app_state.broadcast_tx).await;
}

async fn recv_from_client(
    mut client_rx: SplitStream<WebSocket>,
    broadcast_tx: Arc<Mutex<Sender<Message>>>,
) {
    while let Some(Ok(msg)) = client_rx.next().await {
        match msg {
            Message::Binary(_) => {}
            _ => {}
        }

        if broadcast_tx.lock().await.send(msg.clone()).is_err() {
            println!("Failed to broadcast a message");
        }
    }
}

async fn recv_broadcast(
    client_tx: Arc<Mutex<SplitSink<WebSocket, Message>>>,
    mut broadcast_rx: Receiver<Message>,
) {
    while let Ok(msg) = broadcast_rx.recv().await {
        if client_tx.lock().await.send(msg).await.is_err() {
            return; // disconnected.
        }
    }
}

async fn list_objects(client: &Client, bucket: &str) -> Result<Vec<Object>, Error> {
    let mut objects = Vec::new();

    let response = client
        .list_objects_v2()
        .bucket(bucket.to_owned())
        .max_keys(10)
        .send()
        .await?;

    for object in response.contents.unwrap_or_else(Vec::new) {
        println!(" - {}", object.key.as_deref().unwrap_or("Unknown"));
        objects.push(object);
    }

    Ok(objects)
}

async fn get_object(client: &Client, bucket: &str, object: Object) -> Result<Bytes, anyhow::Error> {
    let obj = client
        .get_object()
        .bucket(bucket.to_owned())
        .key(object.key().unwrap())
        .send()
        .await?;

    let bytes = obj.body.collect().await?.into_bytes();
    println!("bytes - {:?}", bytes.len());

    Ok(bytes)
}

async fn fetch_video(app_state: &AppState) -> Result<Bytes, anyhow::Error> {
    let cfg = load_from_env().await;
    let s3 = Client::new(&cfg);

    let objects = match list_objects(&s3, &app_state.aws_s3_bucket).await {
        Ok(objects) => objects,
        Err(err) => {
            eprintln!("Failed to list objects from S3: {}", err);
            return Err(err.into());
        }
    };

    let mut video_bytes = None;
    for object in objects {
        match get_object(&s3, &app_state.aws_s3_bucket, object).await {
            Ok(bytes) => {
                video_bytes = Some(bytes);
                break;
            }
            Err(err) => {
                eprintln!("Failed to fetch video object from S3: {}", err);
            }
        }
    }

    video_bytes.ok_or_else(|| anyhow::anyhow!("No video content found in S3 bucket"))
}

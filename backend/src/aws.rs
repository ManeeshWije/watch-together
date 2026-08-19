use aws_sdk_s3::{
    types::{CompletedMultipartUpload, CompletedPart},
    Client,
};
use axum::{body::Bytes, extract::ws::Message};
use std::fs;
use std::{collections::HashMap, sync::Arc};
use tokio::fs::File;
use tokio::io::AsyncReadExt;
use tokio::process::Command;
use tokio::sync::{mpsc, Mutex};

const PART_SIZE: usize = 5 * 1024 * 1024; // 5MB

pub async fn get_object(
    client: &Client,
    bucket: &str,
    key: String,
) -> Result<Bytes, anyhow::Error> {
    println!("Calling get_object with key: {:?}", key);
    match client.get_object().bucket(bucket).key(&key).send().await {
        Ok(obj) => {
            let bytes = obj.body.collect().await?.into_bytes();
            println!("Fetched {} bytes from S3", bytes.len());
            Ok(bytes)
        }
        Err(err) => {
            eprintln!("❌ Failed to fetch from S3: {:?}", err);
            Err(anyhow::anyhow!("AWS S3 fetch error: {:?}", err))
        }
    }
}

async fn upload_file(
    client: &Client,
    bucket: String,
    video_key: String,
    mut file: File,
    file_size: u64,
    websocket_clients: Arc<Mutex<HashMap<String, mpsc::Sender<Message>>>>,
    user_uuid: String,
) -> Result<(), anyhow::Error> {
    let create_resp = client
        .create_multipart_upload()
        .bucket(&bucket)
        .key(&video_key)
        .content_type("video/mp4")
        .send()
        .await?;
    let upload_id = create_resp.upload_id().unwrap().to_string();
    let mut completed_parts = Vec::new();
    let mut buffer = vec![0; PART_SIZE];
    let mut part_number = 1;
    let mut uploaded_bytes: u64 = 0;

    // For all but the last part
    while uploaded_bytes + PART_SIZE as u64 <= file_size {
        let bytes_read = file.read_exact(&mut buffer).await?;

        let upload_resp = client
            .upload_part()
            .bucket(&bucket)
            .key(&video_key)
            .upload_id(&upload_id)
            .part_number(part_number)
            .body(buffer.clone().into())
            .send()
            .await?;

        completed_parts.push(
            CompletedPart::builder()
                .set_e_tag(upload_resp.e_tag().map(|s| s.to_string()))
                .set_part_number(Some(part_number))
                .build(),
        );

        uploaded_bytes += bytes_read as u64;
        part_number += 1;
        let progress = (uploaded_bytes as f64 / file_size as f64) * 100.0;
        let progress_message = format!("PROGRESS: {:.2}%", progress);

        // Send the progress message via WebSocket
        let clients = websocket_clients.lock().await;
        if let Some(client_tx) = clients.get(&user_uuid) {
            let _ = client_tx.send(Message::Text(progress_message)).await;
        }
    }

    // Handle the last part, which may be smaller than 5MB
    if uploaded_bytes < file_size {
        let remaining = (file_size - uploaded_bytes) as usize;
        let mut last_buffer = vec![0; remaining];
        file.read_exact(&mut last_buffer).await?;

        let upload_resp = client
            .upload_part()
            .bucket(&bucket)
            .key(&video_key)
            .upload_id(&upload_id)
            .part_number(part_number)
            .body(last_buffer.into())
            .send()
            .await?;

        completed_parts.push(
            CompletedPart::builder()
                .set_e_tag(upload_resp.e_tag().map(|s| s.to_string()))
                .set_part_number(Some(part_number))
                .build(),
        );

        // Update progress to 100%
        let clients = websocket_clients.lock().await;
        if let Some(client_tx) = clients.get(&user_uuid) {
            let _ = client_tx
                .send(Message::Text("PROGRESS: 100.00%".into()))
                .await;
        }
    }

    completed_parts.sort_by_key(|p| p.part_number.unwrap_or(0));

    client
        .complete_multipart_upload()
        .bucket(&bucket)
        .key(&video_key)
        .upload_id(&upload_id)
        .multipart_upload(
            CompletedMultipartUpload::builder()
                .set_parts(Some(completed_parts))
                .build(),
        )
        .send()
        .await?;

    Ok(())
}

fn yt_dlp_command(
    cookie_path: &str,
    pot_provider_url: &str,
) -> Command {
    let mut cmd = Command::new("yt-dlp");

    cmd.arg("--cookies")
        .arg(cookie_path)
        .arg("--extractor-args")
        .arg("youtube:player_client=mweb")
        .arg("--extractor-args")
        .arg(format!(
            "youtubepot-bgutilhttp:base_url={}",
            pot_provider_url
        ));

    cmd
}

pub async fn download_video_upload_s3(
    client: &Client,
    bucket: &str,
    url: &str,
    websocket_clients: Arc<Mutex<HashMap<String, mpsc::Sender<Message>>>>,
    user_uuid: String,
) -> Result<(u64, String, String), anyhow::Error> {
    println!("Starting yt-dlp download for URL: {:?}", url);

    use base64::{engine::general_purpose, Engine as _};

    let encoded = std::env::var("YT_COOKIES_B64")?;
    let pot_provider_url = std::env::var("YT_POT_PROVIDER_URL")?;

    let decoded = general_purpose::STANDARD.decode(encoded)?;

    let cookie_path = "/tmp/youtube-cookies.txt";

    tokio::fs::write(cookie_path, decoded).await?;

    let title_output = yt_dlp_command(
        cookie_path,
        &pot_provider_url,
    )
        .arg("--get-title")
        .arg(url)
        .output()
        .await?;

    if !title_output.status.success() {
        return Err(anyhow::anyhow!(
            "yt-dlp --get-title failed: {}",
            String::from_utf8_lossy(&title_output.stderr)
        ));
    }

    let duration_output = yt_dlp_command(
        cookie_path,
        &pot_provider_url,
    )
        .arg("--print")
        .arg("duration")
        .arg(url)
        .output()
        .await?;

    if !duration_output.status.success() {
        return Err(anyhow::anyhow!(
            "yt-dlp --print duration failed: {}",
            String::from_utf8_lossy(&duration_output.stderr)
        ));
    }

    let title = String::from_utf8_lossy(&title_output.stdout)
        .trim()
        .to_string();

    let duration = String::from_utf8_lossy(&duration_output.stdout)
        .trim()
        .to_string();

    let output_file = format!("{}.mp4", title);

    // let output = Command::new("yt-dlp")
    //     .arg("--cookies")
    //     .arg(&cookie_path)
    //     .arg("-v")
    //     .arg("--no-playlist")
    //     .arg("-o")
    //     .arg(&output_file)
    //     .arg("-f")
    //     .arg(format)
    //     .arg("--merge-output-format")
    //     .arg("mp4")
    //     .arg("--postprocessor-args")
    //     .arg("ffmpeg:-movflags +faststart")
    //     .arg(url)
    //     .output()
    //     .await?;
let high_quality = yt_dlp_command(
    cookie_path,
    &pot_provider_url,
)
    .arg("-v")
    .arg("--no-playlist")
    .arg("-o")
    .arg(&output_file)
    .arg("-f")
    .arg("bv*+ba/b")
    .arg("-S")
    .arg("res:720,br")
    .arg("--merge-output-format")
    .arg("mp4")
    .arg("--postprocessor-args")
    .arg("ffmpeg:-movflags +faststart")
    .arg(url)
    .output()
    .await?;

let output = if high_quality.status.success() {
    high_quality
} else {
    eprintln!(
        "High-quality download failed, falling back to 360p:\n{}",
        String::from_utf8_lossy(&high_quality.stderr)
    );

    let _ = tokio::fs::remove_file(&output_file).await;
    let _ = tokio::fs::remove_file(format!("{}.part", &output_file)).await;

    yt_dlp_command(
        cookie_path,
        &pot_provider_url,
    )
        .arg("-v")
        .arg("--no-playlist")
        .arg("-o")
        .arg(&output_file)
        .arg("-f")
        .arg("18")
        .arg("--postprocessor-args")
        .arg("ffmpeg:-movflags +faststart")
        .arg(url)
        .output()
        .await?
};

    println!(
        "yt-dlp stdout:\n{}",
        String::from_utf8_lossy(&output.stdout)
    );

    eprintln!(
        "yt-dlp stderr:\n{}",
        String::from_utf8_lossy(&output.stderr)
    );

    if !output.status.success() {
        return Err(anyhow::anyhow!(
            "yt-dlp failed with status {:?}\n{}",
            output.status.code(),
            String::from_utf8_lossy(&output.stderr)
        ));
    }

    println!("Download complete: {:?}", &title);

let probe = Command::new("ffprobe")
    .args([
        "-v",
        "error",
        "-select_streams",
        "v:0",
        "-show_entries",
        "stream=width,height,codec_name,bit_rate",
        "-of",
        "default=noprint_wrappers=1",
        &output_file,
    ])
    .output()
    .await?;

println!(
    "FINAL VIDEO INFO:\n{}",
    String::from_utf8_lossy(&probe.stdout)
);

    let file_size = fs::metadata(&output_file)?.len();
    let file = File::open(&output_file).await?;

    let upload_result = upload_file(
        client,
        bucket.to_string(),
        title.to_owned(),
        file,
        file_size,
        websocket_clients,
        user_uuid,
    )
    .await;

    if upload_result.is_ok() {
        if let Err(err) = tokio::fs::remove_file(&output_file).await {
            eprintln!("Failed to delete file {}: {:?}", output_file, err);
        } else {
            println!("Deleted local file: {}", output_file);
        }
    }

    upload_result?;

    Ok((file_size, title, duration))
}

use aws_sdk_s3::{
    types::{CompletedMultipartUpload, CompletedPart},
    Client,
};
use axum::{body::Bytes, extract::ws::Message};
use rustube::{Id, VideoDetails, VideoFetcher};
use std::fs;
use std::sync::Arc;
use tokio::fs::File;
use tokio::process::Command;
use tokio::{io::AsyncReadExt, sync::broadcast::Sender};

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
    broadcaster: Arc<Sender<Message>>,
) -> Result<(), anyhow::Error> {
    let create_resp = client
        .create_multipart_upload()
        .bucket(&bucket)
        .key(&video_key)
        .content_type("video/webm")
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
        if let Err(e) = broadcaster.send(Message::Text(progress_message)) {
            eprintln!("Failed to send progress message: {}", e);
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
        if let Err(e) = broadcaster.send(Message::Text("PROGRESS: 100.00%".into())) {
            eprintln!("Failed to send progress message: {}", e);
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

pub async fn get_video_metadata(url: &str) -> Result<VideoDetails, anyhow::Error> {
    let id = Id::from_raw(url)?;
    let descrambler = VideoFetcher::from_id(id.into_owned())?.fetch().await?;
    Ok(descrambler.video_details().clone())
}

pub async fn download_video_upload_s3(
    client: &Client,
    bucket: &str,
    url: &str,
    title: &str,
    broadcaster: Arc<Sender<Message>>,
) -> Result<u64, anyhow::Error> {
    println!("Starting yt-dlp download for URL: {:?}", url);
    // Spawn yt-dlp process
    // we are always gonna prefer a slightly less quality video to perserve space
    let status = Command::new("yt-dlp")
        .arg("-o")
        .arg(format!("{}", &title)) // Set output file name
        .arg("-f")
        .arg("bestvideo[height<=720]+bestaudio/best[height<=720]")
        .arg(url)
        .spawn()? // Spawn the process
        .wait()
        .await?; // Wait for it to complete

    if !status.success() {
        return Err(anyhow::anyhow!("yt-dlp failed with exit code {:?}", status));
    }

    println!("Download complete: {:?}", &title);

    // Define the output file name
    let output_file = format!("{}.webm", title);
    // Get file size
    let file_size = fs::metadata(&output_file)?.len();
    let file = File::open(&output_file).await?;

    // Upload to S3
    let upload_result = upload_file(
        client,
        bucket.to_string(),
        title.to_owned(),
        file,
        file_size,
        broadcaster,
    )
    .await;

    // Delete file after upload
    if upload_result.is_ok() {
        if let Err(err) = tokio::fs::remove_file(&output_file).await {
            eprintln!("Failed to delete file {}: {:?}", output_file, err);
        } else {
            println!("Deleted local file: {}", output_file);
        }
    }

    Ok(file_size)
}

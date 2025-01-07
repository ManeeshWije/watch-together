package utils

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"log/slog"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gorilla/websocket"
	"github.com/kkdai/youtube/v2"
	"github.com/kkdai/youtube/v2/downloader"
)

var videoRegexpList = []*regexp.Regexp{
	regexp.MustCompile(`(?:v|embed|shorts|watch\?v)(?:=|/)([^"&?/=%]{11})`),
	regexp.MustCompile(`(?:=|/)([^"&?/=%]{11})`),
	regexp.MustCompile(`([^"&?/=%]{11})`),
}

// ExtractVideoID extracts the videoID from the given string
// CREDITS: https://github.com/kkdai/youtube/blob/master/video_id.go
func ExtractVideoID(videoID string) (string, error) {
	if strings.Contains(videoID, "youtu") || strings.ContainsAny(videoID, "\"?&/<%=") {
		for _, re := range videoRegexpList {
			if isMatch := re.MatchString(videoID); isMatch {
				subs := re.FindStringSubmatch(videoID)
				videoID = subs[1]
			}
		}
	}

	if strings.ContainsAny(videoID, "?&/<%=") {
		return "", fmt.Errorf("ERROR: Invalid video ID")
	}

	if len(videoID) < 10 {
		return "", fmt.Errorf("ERROR: Invalid num chars")
	}

	return videoID, nil
}

func GetVideoMetadata(client youtube.Client, videoID string) (*youtube.Video, error) {
	video, err := client.GetVideo(videoID)
	if err != nil {
		return nil, fmt.Errorf("ERROR: Could not fetch video metadata %s", err)
	}
	return video, nil
}

func StreamToS3(client youtube.Client, video *youtube.Video, bucketName, s3Key string, s3Client s3.Client, ws *websocket.Conn) error {
	dl := &downloader.Downloader{
		Client: client,
	}
	tempFile, err := os.CreateTemp("", "video-*.mp4")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tempFile.Name())

	// Send DOWNLOADING message
	progressMessage := "DOWNLOADING"
	err = ws.WriteMessage(websocket.TextMessage, []byte(progressMessage))
	if err != nil {
		slog.Error("Error sending downloading message to client", "error", err)
		return err
	}

	ctx := context.Background()
	if err := dl.DownloadComposite(ctx, tempFile.Name(), video, "hd720", "", ""); err != nil {
		return fmt.Errorf("download video: %w", err)
	}

	file, err := os.Open(tempFile.Name())
	if err != nil {
		return fmt.Errorf("open temp file: %w", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("get file info: %w", err)
	}

	// Send DOWNLOADED message
	progressMessage = "DOWNLOADED"
	err = ws.WriteMessage(websocket.TextMessage, []byte(progressMessage))
	if err != nil {
		slog.Error("Error sending downloaded message to client", "error", err)
		return err
	}

	return UploadFile(s3Client, bucketName, &s3Key, file, fileInfo.Size(), ws)
}

package utils

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"

	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/gorilla/websocket"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

const PartSize = 5 * 1024 * 1024 // 5 MB minimum part size

func ListObjects(s3Client s3.Client, bucket string) ([]*string, error) {
	output, err := s3Client.ListObjectsV2(context.TODO(), &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
	})

	if err != nil {
		slog.Error("Unable to list objects", "error", err)
		return nil, err
	}

	var objects []*string
	for _, object := range output.Contents {
		objects = append(objects, object.Key)
	}

	return objects, nil
}

func GetObject(s3Client s3.Client, bucket string, videoKey string) ([]byte, error) {
	resp, err := s3Client.GetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(videoKey),
	})
	if err != nil {
		slog.Error("Error fetching object from S3", "bucket", bucket, "key", videoKey, "error", err)
		return nil, err
	}
	defer resp.Body.Close()

	videoContent, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("Error reading object content", "key", videoKey, "error", err)
		return nil, err
	}

	return videoContent, nil
}

func UploadFile(s3Client s3.Client, bucket string, videoKey *string, data io.ReadCloser, size int64, ws *websocket.Conn) error {
	defer data.Close()

	// Start multipart upload
	createResp, err := s3Client.CreateMultipartUpload(context.TODO(), &s3.CreateMultipartUploadInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(*videoKey),
		ContentType: aws.String("video/mp4"),
	})
	if err != nil {
		slog.Error("Failed to initiate multipart upload", "bucket", bucket, "key", *videoKey, "error", err)
		return err
	}

	uploadID := createResp.UploadId
	slog.Info("Started multipart upload", "uploadID", *uploadID)

	var completedParts []types.CompletedPart
	buffer := make([]byte, PartSize)
	partNumber := int32(1)
	var uploadedBytes int64

	for {
		n, readErr := io.ReadFull(data, buffer)
		if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
			slog.Error("Failed to read data", "error", readErr)
			abortUpload(s3Client, bucket, *videoKey, *uploadID)
			return readErr
		}

		if n == 0 {
			break
		}

		currentPartNumber := partNumber
		uploadResp, err := s3Client.UploadPart(context.TODO(), &s3.UploadPartInput{
			Bucket:        aws.String(bucket),
			Key:           aws.String(*videoKey),
			UploadId:      uploadID,
			PartNumber:    &currentPartNumber,
			Body:          bytes.NewReader(buffer[:n]),
			ContentLength: aws.Int64(int64(n)),
		})
		if err != nil {
			slog.Error("Failed to upload part", "partNumber", currentPartNumber, "error", err)
			abortUpload(s3Client, bucket, *videoKey, *uploadID)
			return err
		}

		completedParts = append(completedParts, types.CompletedPart{
			ETag:       uploadResp.ETag,
			PartNumber: aws.Int32(currentPartNumber),
		})

		slog.Info("Uploaded part", "partNumber", currentPartNumber, "size", n)
		partNumber++

		uploadedBytes += int64(n)

		progress := float64(uploadedBytes) / float64(size) * 100
		progressMessage := fmt.Sprintf("Progress: %.2f%%", progress)
		err = ws.WriteMessage(websocket.TextMessage, []byte(progressMessage))
		if err != nil {
			slog.Error("Error sending progress to client", "progress", progress, "error", err)
			return err
		}

		if readErr == io.EOF {
			break
		}
	}

	sort.Slice(completedParts, func(i, j int) bool {
		return aws.ToInt32(completedParts[i].PartNumber) < aws.ToInt32(completedParts[j].PartNumber)
	})

	_, err = s3Client.CompleteMultipartUpload(context.TODO(), &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(bucket),
		Key:      aws.String(*videoKey),
		UploadId: uploadID,
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: completedParts,
		},
	})
	if err != nil {
		slog.Error("Failed to complete multipart upload", "bucket", bucket, "key", *videoKey, "error", err)
		return err
	}

	slog.Info("Completed multipart upload", "key", *videoKey)
	return nil
}

func abortUpload(s3Client s3.Client, bucket string, key string, uploadID string) {
	_, err := s3Client.AbortMultipartUpload(context.TODO(), &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
	})
	if err != nil {
		slog.Error("Failed to abort multipart upload", "key", key, "uploadID", uploadID, "error", err)
	} else {
		slog.Info("Aborted multipart upload", "key", key)
	}
}

func DeleteObject(s3Client s3.Client, bucket string, objectKey string, ws *websocket.Conn) error {
	_, err := s3Client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		slog.Error("Could not delete object from S3", "bucket", bucket, "key", objectKey, "error", err)
		return err
	}

	slog.Info("Deleted object from S3", "bucket", bucket, "key", objectKey)
	return nil
}

func CreateS3Client() (*s3.Client, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		slog.Error("Unable to load SDK config", "error", err)
		return nil, err
	}
	s3Client := s3.NewFromConfig(cfg)
	return s3Client, nil
}

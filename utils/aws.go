package utils

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"sort"

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
		log.Fatalf("unable to list objects, %v", err)
		return nil, err
	}

	var objects []*string

	for _, object := range output.Contents {
		objects = append(objects, object.Key)
	}

	return objects, nil
}

func GetObject(s3Client s3.Client, bucket string, videoKey *string) ([]byte, error) {
	// Call the GetObject API to retrieve the video content.
	resp, err := s3Client.GetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(*videoKey),
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	videoContent, err := io.ReadAll(resp.Body)
	if err != nil {
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
		log.Printf("ERROR: Failed to initiate multipart upload: %s", err)
		return err
	}

	uploadID := createResp.UploadId
	log.Printf("Started multipart upload with ID: %s", *uploadID)

	var completedParts []types.CompletedPart
	buffer := make([]byte, PartSize)
	partNumber := int32(1)
	var uploadedBytes int64 = 0

	for {
		n, readErr := io.ReadFull(data, buffer)
		if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
			log.Printf("ERROR: Failed to read data: %s", readErr)
			abortUpload(s3Client, bucket, *videoKey, *uploadID)
			return readErr
		}

		if n == 0 {
			break
		}

		currentPartNumber := partNumber // Create a copy for the closure
		uploadResp, err := s3Client.UploadPart(context.TODO(), &s3.UploadPartInput{
			Bucket:        aws.String(bucket),
			Key:           aws.String(*videoKey),
			UploadId:      uploadID,
			PartNumber:    &currentPartNumber,
			Body:          bytes.NewReader(buffer[:n]),
			ContentLength: aws.Int64(int64(n)),
		})
		if err != nil {
			log.Printf("ERROR: Failed to upload part %d: %s", currentPartNumber, err)
			abortUpload(s3Client, bucket, *videoKey, *uploadID)
			return err
		}

		completedParts = append(completedParts, types.CompletedPart{
			ETag:       uploadResp.ETag,
			PartNumber: aws.Int32(currentPartNumber),
		})

		log.Printf("Uploaded part %d with size %d", currentPartNumber, n)
		partNumber++

		// Update the uploaded bytes count
		uploadedBytes += int64(n)

		// Send the progress update
		progress := float64(uploadedBytes) / float64(size) * 100
		progressMessage := fmt.Sprintf("Progress: %.2f%%", progress)
		err = ws.WriteMessage(websocket.TextMessage, []byte(progressMessage))
		if err != nil {
			log.Println("Error sending progress to client:", err)
			return err
		}

		if readErr == io.EOF {
			break
		}
	}

	// Complete the upload
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
		log.Printf("ERROR: Failed to complete multipart upload: %s", err)
		return err
	}

	// Send reload message
	progressMessage := "RELOAD"
	err = ws.WriteMessage(websocket.TextMessage, []byte(progressMessage))
	if err != nil {
		log.Println("Error sending reload message to client:", err)
		return err
	}

	log.Printf("Successfully uploaded %s to bucket %s", *videoKey, bucket)
	return nil
}

func abortUpload(s3Client s3.Client, bucket string, key string, uploadID string) {
	_, err := s3Client.AbortMultipartUpload(context.TODO(), &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
	})
	if err != nil {
		log.Printf("ERROR: Failed to abort multipart upload: %s", err)
	}
	log.Printf("Aborted multipart upload for %s", key)
}

func DeleteObject(s3Client s3.Client, bucket string, objectKey string, ws *websocket.Conn) error {
	_, err := s3Client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		log.Printf("ERROR: Could not delete object from S3: %v", err)
		return err
	}

	// Send reload message
	progressMessage := "RELOAD"
	err = ws.WriteMessage(websocket.TextMessage, []byte(progressMessage))
	if err != nil {
		log.Println("Error sending reload message to client:", err)
		return err
	}
	log.Printf("Successfully deleted object %s from bucket %s", objectKey, bucket)
	return nil
}

func CreateS3Client() (*s3.Client, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.Fatalf("unable to load SDK config, %v", err)
		return nil, err
	}
	s3Client := s3.NewFromConfig(cfg)
	return s3Client, nil
}

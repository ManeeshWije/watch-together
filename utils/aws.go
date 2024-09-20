package utils

import (
	"context"
	"errors"
	"io"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func listObjects(s3Client s3.Client, bucket string) ([]*string, error) {
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

func getObject(s3Client s3.Client, bucket string, videoKey *string) ([]byte, error) {
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

func FetchVideo() ([]byte, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.Fatalf("unable to load SDK config, %v", err)
		return nil, err
	}

	bucket, exists := os.LookupEnv("AWS_S3_BUCKET")
	if !exists {
		err := errors.New("AWS_S3_BUCKET env var not set")
		log.Print(err)
		return nil, err
	}

	s3Client := s3.NewFromConfig(cfg)
	objects, _ := listObjects(*s3Client, bucket)
	for _, object := range objects {
        // fmt.Println(*object)
		if object != nil {
			bytes, _ := getObject(*s3Client, bucket, object)
			return bytes, nil
		}
	}
	fetchErr := errors.New("Could not fetch bytes of video")
	log.Print(fetchErr)
	return nil, fetchErr
}

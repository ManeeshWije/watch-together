package utils

import (
	"context"
	"io"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

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

func CreateS3Client() (*s3.Client, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.Fatalf("unable to load SDK config, %v", err)
		return nil, err
	}
	s3Client := s3.NewFromConfig(cfg)
	return s3Client, nil
}

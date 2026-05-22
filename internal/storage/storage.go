package storage

import (
	"context"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3Storage struct {
	S3      *s3.Client
	TM      *transfermanager.Client
	Presign *s3.PresignClient
}

func New(ctx context.Context) (*S3Storage, error) {
	env := os.Getenv("APP_ENV")
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}

	s3c := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if env == "local" || env == "development" {
			endpoint := os.Getenv("S3_ENDPOINT")
			if endpoint == "" {
				endpoint = os.Getenv("AWS_ENDPOINT")
			}
			o.BaseEndpoint = &endpoint
			o.UsePathStyle = true
		}
	})
	tmc := transfermanager.New(s3c)
	pc := s3.NewPresignClient(s3c)

	return &S3Storage{
		S3:      s3c,
		TM:      tmc,
		Presign: pc,
	}, nil
}

func (s *S3Storage) DeleteObject(ctx context.Context, bucket string, key string) error {
	//nolint:exhaustruct // too many fields
	_, err := s.S3.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})

	return err
}

func (s *S3Storage) CopyObject(ctx context.Context, bucket string, sourceKey string, destKey string) error {
	//nolint:exhaustruct // too many fields
	_, err := s.S3.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(bucket),
		CopySource: aws.String(bucket + "/" + sourceKey),
		Key:        aws.String(destKey),
	})

	return err
}

func (s *S3Storage) PresignGetObject(ctx context.Context, bucket string, key string, lifetime time.Duration) (string, error) {
	//nolint:exhaustruct // too many fields
	req, err := s.Presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(lifetime))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

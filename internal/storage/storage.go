package storage

import (
	"context"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3Storage struct {
	S3 *s3.Client
	TM *transfermanager.Client
}

func New(ctx context.Context) (*S3Storage, error) {
	env := os.Getenv("APP_ENV")
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}

	s3c := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if env == "local" || env == "development" {
			endpoint := os.Getenv("AWS_ENDPOINT")
			o.BaseEndpoint = &endpoint
			o.UsePathStyle = true
		}
	})
	tmc := transfermanager.New(s3c)

	return &S3Storage{
		S3: s3c,
		TM: tmc,
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

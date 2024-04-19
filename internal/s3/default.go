package s3

import "github.com/linkxzhou/LessDB/internal/utils"

var (
	s3Endpoint  = utils.GetEnviron("S3Endpoint")
	s3Region    = utils.GetEnviron("S3Region")
	s3AccessKey = utils.GetEnviron("S3AccessKey")
	s3SecretKey = utils.GetEnviron("S3SecretKey")
	s3Bucket    = utils.GetEnviron("S3Bucket")

	defaultClient *S3Client
)

func DefaultS3Client() *S3Client {
	if defaultClient == nil {
		defaultClient = NewS3Client(s3Endpoint, s3Region, s3AccessKey, s3SecretKey, s3Bucket)
	}

	return defaultClient
}

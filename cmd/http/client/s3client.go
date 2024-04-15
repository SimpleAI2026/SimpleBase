package client

import (
	"github.com/linkxzhou/LessDB/internal/s3"
	"github.com/linkxzhou/LessDB/internal/utils"
)

var (
	s3Endpoint  = utils.GetEnviron("S3Endpoint")
	s3Region    = utils.GetEnviron("S3Region")
	s3AccessKey = utils.GetEnviron("S3AccessKey")
	s3SecretKey = utils.GetEnviron("S3SecretKey")
	s3Bucket    = utils.GetEnviron("S3Bucket")

	s3Client *s3.S3Client
)

func init() {
	s3Client = s3.NewS3Client(s3Endpoint, s3Region, s3AccessKey, s3SecretKey, s3Bucket)
	if s3Client == nil {
		panic("NewS3Client failed!")
	}
}

func GetS3() *s3.S3Client {
	return s3Client
}

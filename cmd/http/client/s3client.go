package client

import (
	"fmt"
	"os"

	"github.com/linkxzhou/LessDB/internal/s3"
	"github.com/linkxzhou/LessDB/internal/sqlite3vfs"
	"github.com/linkxzhou/LessDB/internal/utils"
	"github.com/linkxzhou/LessDB/internal/vfsextend"
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

	var vfs = &vfsextend.HttpVFS{
		CacheHandler: vfsextend.NewDiskCache(
			func(fileName string) (*os.File, error) {
				return os.OpenFile(fmt.Sprintf("vfscache_%v", fileName), os.O_RDWR|os.O_CREATE, 0644)
			},
			vfsextend.DefaultNoCacheSize),
	}
	if err := sqlite3vfs.RegisterVFS("httpvfs", vfs); err != nil {
		panic("HttpVFS err: " + err.Error())
	}
}

func GetS3() *s3.S3Client {
	return s3Client
}

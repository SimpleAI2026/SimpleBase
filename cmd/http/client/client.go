package client

import (
	"github.com/linkxzhou/LessDB/internal/s3"
	"github.com/linkxzhou/LessDB/internal/sqlite3vfs"
	"github.com/linkxzhou/LessDB/internal/vfsextend"

	"fmt"
	"os"
)

var s3client *s3.S3Client

func init() {
	s3client = s3.DefaultS3Client()
	if s3client == nil {
		panic("NewS3Client failed!")
	}

	var vfs = &vfsextend.HttpVFS{
		CacheHandler: vfsextend.NewDiskCache(
			func(fileName string) (*os.File, error) {
				return os.OpenFile(fmt.Sprintf("vfscache_%v", fileName), os.O_RDWR|os.O_CREATE, 0644)
			},
			vfsextend.DefaultNoCacheSize),
		URIHandler: s3.S3URIHandler{Client: s3client},
	}
	if err := sqlite3vfs.RegisterVFS("httpvfs", vfs); err != nil {
		panic("HttpVFS err: " + err.Error())
	}
}

func S3() *s3.S3Client {
	return s3client
}
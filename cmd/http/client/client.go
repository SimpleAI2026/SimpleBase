package client

import (
	"github.com/linkxzhou/LessDB/internal/s3"
	"github.com/linkxzhou/LessDB/internal/sqlite3vfs"
	"github.com/linkxzhou/LessDB/internal/vfsextend"

	"fmt"
	"os"
)

var S3Client *s3.S3Client

func init() {
	S3Client := s3.DefaultS3Client()
	if S3Client == nil {
		panic("NewS3Client failed!")
	}

	var vfs = &vfsextend.HttpVFS{
		CacheHandler: vfsextend.NewDiskCache(
			func(fileName string) (*os.File, error) {
				return os.OpenFile(fmt.Sprintf("vfscache_%v", fileName), os.O_RDWR|os.O_CREATE, 0644)
			},
			vfsextend.DefaultNoCacheSize),
		URIHandler: s3.S3URIHandler{Client: S3Client},
	}
	if err := sqlite3vfs.RegisterVFS("httpvfs", vfs); err != nil {
		panic("HttpVFS err: " + err.Error())
	}
}

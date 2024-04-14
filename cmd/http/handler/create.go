package handler

import (
	"context"
	"io"
	"net/http"
	"os"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/TamiDB/internal/s3"
	"github.com/linkxzhou/TamiDB/internal/utils"
)

var (
	S3Endpoint  = utils.GetEnviron("S3Endpoint")
	S3Region    = utils.GetEnviron("S3Region")
	S3AccessKey = utils.GetEnviron("S3AccessKey")
	S3SecretKey = utils.GetEnviron("S3SecretKey")
	S3Bucket    = utils.GetEnviron("S3Bucket")

	s3Client *s3.S3Client
)

type AuthResp struct {
	ReadKey  string `json:"readkey"`
	WriteKey string `json:"writekey"`
}

func init() {
	s3Client = s3.NewS3Client(S3Endpoint, S3Region, S3AccessKey, S3SecretKey, S3Bucket)
	if s3Client == nil {
		panic("NewS3Client failed!")
	}
}

// UploadDB for upload sqlite3 file and to S3
func UploadDB(c echo.Context) error {
	// Source
	file, err := c.FormFile("file")
	if err != nil {
		c.Logger().Error("FormFile err: ", err)
		return err
	}
	srcFile, err := file.Open()
	if err != nil {
		c.Logger().Error("OpenFile err: ", err)
		return err
	}
	defer srcFile.Close()

	// Destination
	dbName := utils.NewRandomName()
	dstFile, err := os.Create(dbName)
	if err != nil {
		c.Logger().Error("Failed to create local file err: ", err)
		return err
	}
	defer func() {
		dstFile.Close()
		// Remove tempfile
		os.Remove(dbName)
	}()

	if _, err = io.Copy(dstFile, srcFile); err != nil {
		c.Logger().Error("Failed to save file locally err: ", err)
		return err
	}

	db, err := GetFileDB(dbName)
	if err != nil {
		c.Logger().Error("sql.Open err: ", err)
		return err
	}
	defer db.Close()

	writeKey := utils.NewRandomKey()
	err = initSQLWithSystem(c, db, dbName, writeKey)
	if err != nil {
		c.Logger().Error("InitSQL err: ", err)
		return err
	}

	// Seek to start and upload S3
	dstFile.Seek(0, io.SeekStart)
	c.Logger().Info("s3Client: ", s3Client.String(dbName))
	err = s3Client.Upload(context.TODO(), dbName, dstFile)
	if err != nil {
		c.Logger().Error("S3 UploadFile err: ", err)
		return err
	}

	return c.JSON(http.StatusOK, DataResp{
		Code:    0,
		Message: "OK",
		Data: AuthResp{
			ReadKey:  dbName,
			WriteKey: writeKey,
		},
	})
}

type CMDListParams struct {
	List []SQLExecuteCommandArgs `json:"list"`
}

// CreateDB for use sql create sqlite3 file
func CreateDB(c echo.Context) error {
	ep := new(CMDListParams)
	if err := c.Bind(ep); err != nil || ep.List == nil || len(ep.List) < 0 {
		return c.String(http.StatusBadRequest, "bad request")
	}

	dbName := utils.NewRandomName()
	db, err := GetFileDB(dbName)
	if err != nil {
		c.Logger().Error("sql.Open err: ", err)
		return err
	}
	defer db.Close()

	writeKey := utils.NewRandomKey()
	err = initSQLWithSystem(c, db, dbName, writeKey)
	if err != nil {
		c.Logger().Error("InitSQL err: ", err)
		return err
	}

	err = ExecuteSQLWithFile(c, db, ep.List)
	if err != nil {
		c.Logger().Error("ExecuteSQL err: ", err)
		return err
	}

	dbFile, err := os.Open(dbName)
	if err != nil {
		c.Logger().Error("OpenFile err: ", err)
		return err
	}
	defer func() {
		dbFile.Close()
		// Remove tempfile
		os.Remove(dbName)
	}()

	c.Logger().Info("s3Client: ", s3Client.String(dbName))
	err = s3Client.Upload(context.TODO(), dbName, dbFile)
	if err != nil {
		c.Logger().Error("S3 UploadFile err: ", err)
		return err
	}

	return c.JSON(http.StatusOK, DataResp{
		Code:    0,
		Message: "OK",
		Data: AuthResp{
			ReadKey:  dbName,
			WriteKey: writeKey,
		},
	})
}

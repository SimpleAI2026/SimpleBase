package handler

import (
	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/LessDB/cmd/http/client"
	"github.com/linkxzhou/LessDB/internal/utils"

	"context"
	"io"
	"net/http"
	"os"
)

type AuthResp struct {
	ReadKey  string `json:"readkey"`
	WriteKey string `json:"writekey"`
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
	readKey, dbName, err := utils.NewRandomName()
	if err != nil {
		c.Logger().Error("NewRandomName err: ", err)
		return err
	}

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

	db, err := client.GetFileDB(dbName)
	if err != nil {
		c.Logger().Error("sql.Open err: ", err)
		return err
	}
	defer db.Close()

	writeKey, _, err := utils.NewRandomKey()
	if err != nil {
		c.Logger().Error("NewRandomKey err: ", err)
		return err
	}

	err = client.SysTableInit(c, db, readKey, writeKey)
	if err != nil {
		c.Logger().Error("InitSQL err: ", err)
		return err
	}

	// Seek to start and upload S3
	dstFile.Seek(0, io.SeekStart)
	c.Logger().Info("S3Client: ", client.S3().String(dbName))
	err = client.S3().Upload(context.TODO(), dbName, dstFile)
	if err != nil {
		c.Logger().Error("S3 UploadFile err: ", err)
		return err
	}

	return c.JSON(http.StatusOK, newOKResp(AuthResp{
			ReadKey:  readKey,
			WriteKey: writeKey,
		}))
}

type CMDListParams struct {
	List []client.SQLExecuteCommandArgs `json:"list"`
}

// CreateDB for use sql create sqlite3 file
func CreateDB(c echo.Context) error {
	ep := new(CMDListParams)
	if err := c.Bind(ep); err != nil || ep.List == nil || len(ep.List) < 0 {
		return c.String(http.StatusBadRequest, "bad request")
	}

	readKey, dbName, err := utils.NewRandomName()
	if err != nil {
		c.Logger().Error("NewRandomName err: ", err)
		return err
	}

	db, err := client.GetFileDB(dbName)
	if err != nil {
		c.Logger().Error("sql.Open err: ", err)
		return err
	}
	defer db.Close()

	writeKey, _, err := utils.NewRandomKey()
	if err != nil {
		c.Logger().Error("NewRandomKey err: ", err)
		return err
	}

	err = client.SysTableInit(c, db, readKey, writeKey)
	if err != nil {
		c.Logger().Error("InitSQL err: ", err)
		return err
	}

	err = client.ExecuteSQLWithFile(c, db, ep.List)
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

	c.Logger().Info("S3Client: ", client.S3().String(dbName))
	err = client.S3().Upload(context.TODO(), dbName, dbFile)
	if err != nil {
		c.Logger().Error("S3 UploadFile err: ", err)
		return err
	}

	return c.JSON(http.StatusOK, newOKResp(AuthResp{
			ReadKey:  readKey,
			WriteKey: writeKey,
		}))
}

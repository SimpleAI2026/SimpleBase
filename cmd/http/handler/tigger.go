package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/LessDB/cmd/http/client"
)

type (
	TiggerReq struct {
		Events []TiggerEvents `json:"events"`
	}

	TiggerEvents struct {
		S3Bucket string `json:"s3bucket"`
		S3Key    string `json:"s3key"`
	}
)

func TiggerS3Events(c echo.Context) error {
	req := new(TiggerReq)
	if err := c.Bind(req); err != nil {
		return c.String(http.StatusBadRequest, "bad request")
	}

	for _, event := range req.Events {
		redologStr, err := client.S3Client.DownloadString(context.TODO(), event.S3Key)
		if err != nil {
			c.Logger().Error("Download err: ", err)
			return err
		}

		var redolog UploadS3Redolog
		err = json.Unmarshal(redologStr, &redolog)
		if err != nil {
			c.Logger().Error("Unmarshal err: ", err)
			return err
		}

		dbName := redolog.DBName
		dbFile, err := os.Create(dbName)
		if err != nil {
			c.Logger().Error("OpenFile err: ", err)
			return err
		}
		defer func() {
			dbFile.Close()
			// Remove tempfile
			os.Remove(dbName)
		}()

		err = client.S3Client.Download(context.TODO(), dbName, dbFile)
		if err != nil {
			c.Logger().Error("Download err: ", err)
			return err
		}

		db, err := client.GetFileDB(dbName)
		if err != nil {
			c.Logger().Error("sql.Open err: ", err)
			return err
		}
		defer db.Close()

		// Insert into redolog result to system table
		err = client.SysTableInsertStatus(c, db, ExecStatusPending, event.S3Key, "Pending")
		if err != nil {
			c.Logger().Error("SysTableInsertStatus err: ", err)
			return err
		}

		// Execute redolog list on sqlite3 file
		execStatus := ExecStatusOK
		execMessage := "OK"
		err = client.ExecuteSQLWithFile(c, db, redolog.List)
		if err != nil {
			c.Logger().Error("ExecuteSQL err: ", err)
			execStatus = ExecStatusFailed
			execMessage = err.Error()
		}
		// Update redolog error to system table
		err = client.SysTableUpdateStatus(c, db, execStatus, event.S3Key, execMessage)
		if err != nil {
			c.Logger().Error("SysTableUpdateStatus err: ", err)
			return err
		}

		dbFile.Seek(0, io.SeekStart)
		c.Logger().Info("S3Client Upload: ", client.S3Client.String(dbName))
		err = client.S3Client.Upload(context.TODO(), dbName, dbFile)
		if err != nil {
			c.Logger().Error("S3 UploadFile err: ", err)
			return err
		}
	}

	return c.JSON(http.StatusOK, DataResp{
		Code:    0,
		Message: "OK",
	})
}

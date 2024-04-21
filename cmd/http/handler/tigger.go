package handler

import (
	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/LessDB/cmd/http/client"

	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"
)

type (
	TiggerReq struct {
		Events []TiggerEvents `json:"events"`
		Sync   bool           `json:"sync"`
		TiggerExecuteCommandArgs
	}

	TiggerEvents struct {
		S3Key string `json:"s3key"`
	}

	TiggerExecuteCommandArgs struct {
		DBName string                         `json:"dbname"`
		List   []client.SQLExecuteCommandArgs `json:"list"`
		S3Key  string                         `json:"s3key"`
	}
)

func TiggerS3Events(c echo.Context) error {
	req := new(TiggerReq)
	if err := c.Bind(req); err != nil {
		return c.String(http.StatusBadRequest, "bad request")
	}

	var commandList []TiggerExecuteCommandArgs
	// Download file from S3
	for _, event := range req.Events {
		redologStr, err := client.S3().DownloadString(context.TODO(), event.S3Key)
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

		commandList = append(commandList, TiggerExecuteCommandArgs{
			DBName: redolog.DBName,
			List:   redolog.List,
			S3Key:  event.S3Key,
		})
	}

	if req.List != nil {
		commandList = append(commandList, TiggerExecuteCommandArgs{
			DBName: req.DBName,
			List:   req.List,
			S3Key:  fmt.Sprintf("custom-%v", time.Now().UnixNano()),
		})
	}

	// Execute SQL on sqlite3 file
	for _, command := range commandList {
		lessdbName := fmt.Sprintf("%v.lessdb", command.DBName)
		// exist db file
		if _, err := os.Stat(lessdbName); os.IsNotExist(err) {
			dbFile, err := os.Create(lessdbName)
			if err != nil {
				c.Logger().Error("OpenFile err: ", err)
				return err
			}
			defer dbFile.Close()

			err = client.S3().Download(context.TODO(), command.DBName, dbFile)
			if err != nil {
				c.Logger().Error("Download err: ", err)
				return err
			}
		}

		db, err := client.GetFileDB(lessdbName)
		if err != nil {
			c.Logger().Error("sql.Open err: ", err)
			return err
		}
		defer db.Close()

		execStatus := ExecStatusPending
		execMessage := "Pending"
		// Insert into redolog result to system table
		err = client.SysTableInsertStatus(c, db, execStatus, command.S3Key, execMessage)
		if err != nil {
			c.Logger().Error("SysTableInsertStatus err: ", err)
			return err
		}

		// Execute redolog list on sqlite3 file
		execStatus = ExecStatusOK
		execMessage = "OK"
		execErr := client.ExecuteSQLWithFile(c, db, command.List)
		if execErr != nil {
			c.Logger().Error("ExecuteSQL err: ", execErr)
			execStatus = ExecStatusFailed
			execMessage = execErr.Error()
		}

		// Update redolog error to system table
		err = client.SysTableUpdateStatus(c, db, execStatus, command.S3Key, execMessage)
		if err != nil {
			c.Logger().Error("SysTableUpdateStatus err: ", err)
			return err
		}

		if execErr != nil && req.Sync {
			return c.JSON(http.StatusOK, newFailResp(-101, execErr.Error()))
		}
	}

	// Upload result to S3
	for _, command := range commandList {
		lessdbName := fmt.Sprintf("%v.lessdb", command.DBName)
		if _, err := os.Stat(lessdbName); os.IsNotExist(err) {
			return errors.New("DB file not found")
		}

		dbFile, err := os.Open(lessdbName)
		if err != nil {
			c.Logger().Error("OpenFile err: ", err)
			return err
		}
		defer dbFile.Close()

		c.Logger().Info("S3Client Upload: ", client.S3().String(command.DBName))
		err = client.S3().Upload(context.TODO(), command.DBName, dbFile)
		if err != nil {
			c.Logger().Error("S3 UploadFile err: ", err)
			return err
		}
	}

	return c.JSON(http.StatusOK, newOKResp(nil))
}

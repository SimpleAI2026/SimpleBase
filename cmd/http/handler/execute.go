package handler

import (
	"github.com/linkxzhou/LessDB/cmd/http/client"
	"github.com/linkxzhou/LessDB/internal/utils"

	"github.com/labstack/echo/v4"

	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type (
	ExecuteDBParams struct {
		QueryParams
		CMDListParams
		WriteKey string `json:"writekey" form:"writekey" query:"writekey" param:"writekey"`
	}

	ExecuteLogParams struct {
		QueryParams
		SeqID string `json:"seqid" form:"seqid" query:"seqid" param:"seqid"`
	}

	UploadS3Redolog struct {
		DBName        string                         `json:"dbname"`
		ReadKey       string                         `json:"readkey"`
		WriteKey      string                         `json:"writekey"`
		NanoTimestamp int64                          `json:"nanotimestamp"`
		List          []client.SQLExecuteCommandArgs `json:"list"`
	}

	ResultRedolog struct {
		SeqID   string `json:"seqid"`
		Message string `json:"message"`
		Status  int    `json:"status"`
	}
)

const (
	ErrNoAuthWrite = "attempt to write a readonly database"
	// Execute sql has status on sqlite3
	ExecStatusPending int = iota
	ExecStatusOK
	ExecStatusFailed
	ExecStatusPartialFailed
)

var tiggerURL = utils.GetEnviron("LESSDB_TIGGERURL")

// ExecuteDB for exec sql on sqlite3 file
func ExecuteDB(c echo.Context) error {
	edbp := new(ExecuteDBParams)
	if err := c.Bind(edbp); err != nil || edbp.List == nil || len(edbp.List) < 0 {
		return c.String(http.StatusBadRequest, "bad request")
	}

	c.Logger().Info("ExecuteDB edbp: ", edbp)

	// Check writeKey
	if _, writeKeyOK := utils.VerifyKey(edbp.WriteKey); !writeKeyOK {
		return c.JSON(http.StatusOK, newNoWriteAuthResp())
	}

	readKey := edbp.ReadKey
	dbName, authOK := utils.VerifyKey(readKey)
	if !authOK || dbName == "" {
		return c.JSON(http.StatusOK, newNoAuthResp())
	}

	db, uri, err := client.GetVFSDB(dbName)
	if err != nil {
		c.Logger().Error("getVFSDB err: ", err)
		return err
	}
	defer db.Close()

	c.Logger().Info("S3 GetFileLink: ", uri, ", dbName: ", dbName)

	var s3key string
	var execMessage string
	var execStatus int = ExecStatusPending

	// Upload redolog to S3
	if tiggerURL == utils.EmptyNil {
		var redologs []client.SQLExecuteCommandArgs
		for _, cmd := range edbp.List {
			if err := client.ExecuteSQLWithHTTPVFS(c, db, cmd); err != nil {
				if !strings.Contains(err.Error(), ErrNoAuthWrite) {
					c.Logger().Error("ExecuteSQL err: ", err)
					return err
				}
				redologs = append(redologs, cmd)
			}
		}

		nanoTimestamp := time.Now().UnixNano()
		uploadS3Redolog := UploadS3Redolog{
			DBName:        dbName,
			NanoTimestamp: nanoTimestamp,
			ReadKey:       readKey,
			WriteKey:      edbp.WriteKey,
			List:          redologs,
		}
		jsons, err := json.Marshal(uploadS3Redolog)
		if err != nil {
			c.Logger().Error("Marshal err: ", err)
			return err
		}

		s3key = fmt.Sprintf("%v-%v.redolog", readKey, nanoTimestamp)
		err = client.S3().UploadString(context.TODO(), s3key, jsons)
		if err != nil {
			c.Logger().Error("S3 UploadFile err: ", err)
			return err
		}
		execMessage = "Execute pending"
		execStatus = ExecStatusPending
	} else {
		s3key = fmt.Sprintf("%v-%v.sync", readKey, time.Now().UnixNano())
		if err := requestTigger(dbName, TiggerExecuteCommandArgs{
			DBName: dbName,
			List:   edbp.List,
			S3Key:  s3key,
		}); err != nil {
			c.Logger().Error("Sync requestTigger err: ", err)
			return err
		}
		execMessage = "Execute finished"
		execStatus = ExecStatusOK
	}

	return c.JSON(http.StatusOK, newOKResp(ResultRedolog{
		SeqID:   s3key,
		Message: execMessage,
		Status:  execStatus,
	}))
}

func requestTigger(dbName string, args TiggerExecuteCommandArgs) error {
	resp := DataResp{Code: -1}
	err := httpRequest(tiggerURL, TiggerReq{
		TiggerExecuteCommandArgs: args,
		Sync:                     true,
	}, &resp)
	if err != nil || resp.Code != 0 {
		return errors.New("Request failed, err: " + resp.Message)
	}
	return nil
}

// ExecuteLog for query redolog on sqlite3 file
func ExecuteLog(c echo.Context) error {
	elp := new(ExecuteLogParams)
	if err := c.Bind(elp); err != nil {
		return c.String(http.StatusBadRequest, "bad request")
	}

	c.Logger().Info("ExecuteLogParams elp: ", elp)

	readKey := elp.ReadKey
	dbName, authOK := utils.VerifyKey(readKey)
	if !authOK || dbName == "" {
		return c.JSON(http.StatusOK, newNoAuthResp())
	}

	db, uri, err := client.GetVFSDB(dbName)
	if err != nil {
		c.Logger().Error("getVFSDB err: ", err)
		return err
	}
	defer db.Close()

	c.Logger().Info("S3 GetFileLink: ", uri, ", dbName: ", dbName)
	_, values, _, _, err := client.QuerySQLWithHTTPVFS(c, db,
		client.SQLExecuteCommandArgs{
			CMD:  client.SysTableQuerySQL(),
			Args: []interface{}{elp.SeqID},
		})
	if err != nil {
		c.Logger().Error("ExecuteSQL err: ", err)
		return err
	}

	// TODO: fix values to redolog

	return c.JSON(http.StatusOK, newOKResp(values))
}

// QueryDB for query data on sqlite3 file
func QueryDB(c echo.Context) error {
	edbp := new(ExecuteDBParams)
	if err := c.Bind(edbp); err != nil || edbp.List == nil || len(edbp.List) < 0 {
		return c.String(http.StatusBadRequest, "bad request")
	}

	c.Logger().Info("QueryDB edbp: ", edbp)

	var result []DBValuesResp
	readKey := edbp.ReadKey
	dbName, authOK := utils.VerifyKey(readKey)
	if !authOK || dbName == "" {
		return c.JSON(http.StatusOK, newNoAuthResp())
	}

	db, uri, err := client.GetVFSDB(dbName)
	if err != nil {
		c.Logger().Error("getVFSDB err: ", err)
		return err
	}
	defer db.Close()

	c.Logger().Info("S3 GetFileLink: ", uri, ", dbName: ", dbName)
	for _, cmd := range edbp.List {
		startTime := time.Now()
		columns, values, types, count, err :=
			client.QuerySQLWithHTTPVFS(c, db, cmd)
		if err != nil {
			c.Logger().Error("ExecuteSQL err: ", err)
			return err
		}
		result = append(result, DBValuesResp{
			Columns:  columns,
			Values:   values,
			Types:    types,
			Count:    count,
			TimeCost: float64(time.Since(startTime).Microseconds()) / 1e3,
		})
	}

	return c.JSON(http.StatusOK, newOKResp(result))
}

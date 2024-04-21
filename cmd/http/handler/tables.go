package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/linkxzhou/LessDB/cmd/http/client"
	"github.com/linkxzhou/LessDB/internal/utils"
)

const defaultLimitSize int64 = 100

type (
	QueryParams struct {
		ReadKey   string `json:"ReadKey" form:"ReadKey" query:"ReadKey" param:"ReadKey"`
		TableName string `json:"tableName" form:"tableName" query:"tableName" param:"tableName"`
		Limit     int64  `json:"limit" form:"limit" query:"limit" param:"limit"`
		Offset    int64  `json:"offset" form:"offset" query:"offset" param:"offset"`
	}

	DBValuesResp struct {
		Columns  interface{} `json:"columns"`
		Values   interface{} `json:"values"`
		Types    interface{} `json:"types"`
		Count    int         `json:"count"`
		TimeCost float64     `json:"cost"`
	}

	DataResp struct {
		Code    int         `json:"code"`
		Message string      `json:"message"`
		Data    interface{} `json:"data"`
	}
)

func GetTables(c echo.Context) (err error) {
	q := new(QueryParams)
	if c.Bind(q); err != nil {
		return c.String(http.StatusBadRequest, "bad request")
	}

	readKey := q.ReadKey
	dbName, authOK := utils.VerifyKey(readKey)
	if !authOK || dbName == "" {
		return c.JSON(http.StatusOK, newNoAuthResp())
	}

	limit := defaultLimitSize
	if q.Limit > 0 {
		limit = q.Limit
	}

	c.Logger().Info("GetTables q: ", q)

	startTime := time.Now()
	db, uri, err := client.GetVFSDB(dbName)
	if err != nil {
		c.Logger().Error("getVFSDB err: ", err)
		return newBadRequestResp(c, err)
	}
	defer db.Close()

	c.Logger().Info("S3 GetFileLink: ", uri, ", dbName: ", dbName)
	columns, values, types, count, err := client.QuerySQLWithHTTPVFS(c, db,
		client.SQLExecuteCommandArgs{
			CMD: `SELECT name FROM sqlite_master 
				WHERE type='table' limit ? offset ?`,
			Args: []interface{}{limit, q.Offset * limit},
		})

	if err != nil {
		return c.String(http.StatusBadRequest, "query err: "+err.Error())
	}

	return c.JSON(http.StatusOK, newOKResp(DBValuesResp{
		Columns:  columns,
		Values:   values,
		Types:    types,
		Count:    count,
		TimeCost: float64(time.Since(startTime).Microseconds()) / 1e3,
	}))
}

func GetRows(c echo.Context) (err error) {
	q := new(QueryParams)
	if c.Bind(q); err != nil {
		return c.String(http.StatusBadRequest, "bad request")
	}

	readKey := q.ReadKey
	dbName, authOK := utils.VerifyKey(readKey)
	if !authOK || dbName == "" {
		return c.JSON(http.StatusOK, newNoAuthResp())
	}

	limit := defaultLimitSize
	if q.Limit > 0 {
		limit = q.Limit
	}

	c.Logger().Info("GetRows q: ", q)

	startTime := time.Now()
	db, uri, err := client.GetVFSDB(dbName)
	if err != nil {
		c.Logger().Error("getVFSDB err: ", err)
		return newBadRequestResp(c, err)
	}
	defer db.Close()

	c.Logger().Info("S3 GetFileLink: ", uri, ", dbName: ", dbName)
	columns, values, types, count, err := client.QuerySQLWithHTTPVFS(c, db,
		client.SQLExecuteCommandArgs{
			CMD:  fmt.Sprintf(`SELECT * FROM %v limit ? offset ?`, q.TableName),
			Args: []interface{}{limit, q.Offset * limit},
		})
	if err != nil {
		c.Logger().Info("ExecuteSQL err: ", err)
		return newBadRequestResp(c, err)
	}

	return c.JSON(http.StatusOK, newOKResp(DBValuesResp{
		Columns:  columns,
		Values:   values,
		Types:    types,
		Count:    count,
		TimeCost: float64(time.Since(startTime).Microseconds()) / 1e3,
	}))
}

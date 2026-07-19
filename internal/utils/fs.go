package utils

import (
	"fmt"
	"os"
)

// FSCreateMultiDir 调用os.MkdirAll递归创建文件夹
func FSCreateMultiDir(filePath string) error {
	if !FSPathIsExist(filePath) {
		err := os.MkdirAll(filePath, os.ModePerm)
		if err != nil {
			fmt.Println("创建文件夹失败,error info:", err)
			return err
		}
		return err
	}
	return nil
}

// FSPathIsExist 判断所给路径文件/文件夹是否存在(返回true是存在)
func FSPathIsExist(path string) bool {
	_, err := os.Stat(path) //os.Stat获取文件信息
	if err != nil {
		if os.IsExist(err) {
			return true
		}
		return false
	}
	return true
}

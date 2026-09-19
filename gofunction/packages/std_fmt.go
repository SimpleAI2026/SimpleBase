package packages

import (
	"fmt"

	"github.com/linkxzhou/SimpleBase/gofunction/importer"
)

// stdFmt fmt 包注册清单
func stdFmt() []*importer.Object {
	return []*importer.Object{
		funcObj("Sprintf", fmt.Sprintf),
		funcObj("Sprint", fmt.Sprint),
		funcObj("Sprintln", fmt.Sprintln),
		funcObj("Sscanf", fmt.Sscanf),
		funcObj("Sscan", fmt.Sscan),
		funcObj("Sscanln", fmt.Sscanln),
		funcObj("Printf", fmt.Printf),
		funcObj("Println", fmt.Println),
		funcObj("Print", fmt.Print),
		funcObj("Errorf", fmt.Errorf),
	}
}

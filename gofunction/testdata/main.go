package testdata

type __testSet struct{}

// TestSet 将测试用例全部定位为TestSet结构体的方法，可以通过反射的方式来遍历调用所有的测试用例
var TestSet = __testSet{}

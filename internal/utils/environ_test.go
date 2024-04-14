package utils

// import (
// 	"fmt"
// 	"testing"
// )

// // TODO:
// func splitEnviron() {
// 	k1, v1, err := SplitEnviron("aaa")
// 	fmt.Println("SplitEnviron: ", k1, v1, err)
// }

// func Test_env(t *testing.T) {
// 	splitEnviron()
// 	fmt.Println("GetEnvironInfo: ", GetEnvironInfo())
// 	fmt.Println("GetEnvironInfo: ", GetEnviron("PDR_APPID"))
// }

// func TestEnv(t *testing.T) {
// 	tests := []struct {
// 		name     string
// 		input    SHA256Sum
// 		expected string
// 	}{
// 		{
// 			name:     "empty hash",
// 			input:    SHA256Sum([]byte{}),
// 			expected: "",
// 		},
// 		{
// 			name:     "non-empty hash",
// 			input:    SHA256Sum([]byte{0x12, 0x34, 0x56}),
// 			expected: "123456",
// 		},
// 	}

// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			output := tt.input.String()
// 			if output != tt.expected {
// 				t.Errorf("Expected: %s, got: %s", tt.expected, output)
// 			}
// 		})
// 	}
// }

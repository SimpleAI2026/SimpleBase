package cli

import "testing"

func TestRepeatDetector(t *testing.T) {
	d := newRepeatDetector(3, 2) // 窗口3，同签名最多2次
	if warn := d.record("shell", `ls -la`); warn != "" {
		t.Errorf("第1次调用不应告警：%s", warn)
	}
	if warn := d.record("shell", `ls -la`); warn != "" {
		t.Errorf("第2次调用（阈值内）不应告警：%s", warn)
	}
	// 第3次超限
	if warn := d.record("shell", `ls -la`); warn == "" {
		t.Error("第3次调用应告警")
	}
	// 不同工具不计入
	if warn := d.record("write_file", `{"path":"a.txt"}`); warn != "" {
		t.Errorf("不同工具不应告警：%s", warn)
	}
	// 不同参数不计入
	if warn := d.record("shell", `pwd`); warn != "" {
		t.Errorf("相同工具不同参数不应告警：%s", warn)
	}
}

func TestRepeatDetectorWindow(t *testing.T) {
	d := newRepeatDetector(2, 1) // 窗口2，同签名最多1次（即第2次就告警）
	d.record("a", "1")
	d.record("b", "2")
	// 窗口满（只保留最近 2 次），第一个条目 "a"/"1" 被挤掉。
	// 再次调用 "a"/"1" 时，它已经不在窗口内，计数是 1（首次），不告警。
	if warn := d.record("a", "1"); warn != "" {
		t.Errorf("窗口满后旧条目被挤掉，重新出现时不应立即告警：%s", warn)
	}
	// 现在窗口是 ["b"/"2", "a"/"1"]，再调一次 "a"/"1" 就会是第 2 次，触发告警。
	if warn := d.record("a", "1"); warn == "" {
		t.Error("窗口内第2次重复应告警")
	}
}

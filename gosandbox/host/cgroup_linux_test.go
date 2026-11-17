//go:build linux

package host

import (
	"os"
	"path/filepath"
	"testing"
)

// newTestCgroup 在临时目录冒充 cgroup 根，返回子组与路径。
// 不依赖真实 /sys/fs/cgroup：这里测的是文件读写与规则，不是内核。
func newTestCgroup(t *testing.T, name string) (*cgroupGroup, string) {
	t.Helper()
	old := cgroupRoot
	cgroupRoot = t.TempDir()
	t.Cleanup(func() { cgroupRoot = old })
	// delegatedBase 要读 cgroup.subtree_control，先造出委派子树。
	base := filepath.Join(cgroupRoot, "box")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "cgroup.subtree_control"),
		[]byte("memory cpu pids\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// currentCgroupDir 读 /proc/self/cgroup，测试里用环境变量覆盖不了，
	// 所以直接构造 cgroupGroup，指向临时目录下的子组。
	dir := filepath.Join(base, CgroupNamePrefix+name)
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return &cgroupGroup{dir: dir, keep: name != ""}, dir
}

// 限额文件按预期写入；软限与 oom.group 也在。
func TestCgroupApplyWritesLimits(t *testing.T) {
	g, dir := newTestCgroup(t, "lim")
	p := basePolicy()
	p.MemoryMaxBytes = 512 << 20
	p.PidsMax = 64
	p.CPUMax = "100000 100000"
	if err := g.apply(p); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"memory.max":       "536870912",
		"memory.high":      "483183820", // 90%
		"memory.swap.max":  "0",
		"memory.oom.group": "1",
		"pids.max":         "64",
		"cpu.max":          "100000 100000",
	} {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if string(got) != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

// 未配置的限额不写文件：不加限额的沙盒不该被塞进默认值。
func TestCgroupApplySkipsUnsetLimits(t *testing.T) {
	g, dir := newTestCgroup(t, "unset")
	if err := g.apply(basePolicy()); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"memory.max", "cpu.max", "pids.max"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s must not be written when unset", name)
		}
	}
}

// freeze 写 cgroup.freeze；解冻写 0。
func TestCgroupFreeze(t *testing.T) {
	g, dir := newTestCgroup(t, "frz")
	if err := g.freeze(true); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "cgroup.freeze")); string(b) != "1" {
		t.Fatalf("freeze = %q", b)
	}
	if err := g.freeze(false); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "cgroup.freeze")); string(b) != "0" {
		t.Fatalf("thaw = %q", b)
	}
}

// usage 读到的值进 Metrics；缺失的文件按 0 处理，不让整个 metrics 失败。
func TestCgroupUsageReadsFiles(t *testing.T) {
	g, dir := newTestCgroup(t, "use")
	files := map[string]string{
		"memory.current": "1048576",
		"memory.peak":    "2097152",
		"pids.current":   "3",
		"cpu.stat":       "usage_usec 12345\nuser_usec 10000\nsystem_usec 2345\n",
		"io.stat":        "8:0 rbytes=4096 wbytes=8192 rios=1 wios=2\n8:16 rbytes=1024 wbytes=0 rios=1 wios=0\n",
		"memory.events":  "low 0\nhigh 4\nmax 2\noom 1\noom_kill 1\n",
	}
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	u := g.usage()
	if u.MemoryBytes != 1048576 || u.MemoryPeak != 2097152 || u.Pids != 3 {
		t.Fatalf("usage = %+v", u)
	}
	if u.CPUUsec != 12345 {
		t.Errorf("cpu usec = %d", u.CPUUsec)
	}
	// io.stat 每设备一行，按 key 累加。
	if u.DiskReadByte != 4096+1024 || u.DiskWriteByt != 8192 {
		t.Errorf("io = %d/%d", u.DiskReadByte, u.DiskWriteByt)
	}
	if u.OOMKills != 1 {
		t.Errorf("oom kills = %d", u.OOMKills)
	}
}

// 缺文件时 usage 全 0，不 panic。
func TestCgroupUsageMissingFiles(t *testing.T) {
	g, _ := newTestCgroup(t, "empty")
	if u := g.usage(); u.MemoryBytes != 0 || u.CPUUsec != 0 || u.Pids != 0 {
		t.Fatalf("usage = %+v", u)
	}
}

// 固定子组不被 remove 删掉：它要留给 Metrics / Freeze。
func TestCgroupKeepSurvivesRemove(t *testing.T) {
	g, dir := newTestCgroup(t, "keep")
	g.remove()
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("kept cgroup must survive remove: %v", err)
	}
	// 随机子组（一次性命令）照常删。
	tmp, err := os.MkdirTemp(filepath.Dir(dir), CgroupNamePrefix)
	if err != nil {
		t.Fatal(err)
	}
	(&cgroupGroup{dir: tmp}).remove()
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatal("ephemeral cgroup must be removed")
	}
}

// cgroupPathFor 只接受带前缀的名字，避免误操作别人的 cgroup。
func TestCgroupPathForRejectsForeignNames(t *testing.T) {
	old := cgroupRoot
	cgroupRoot = t.TempDir()
	t.Cleanup(func() { cgroupRoot = old })
	for _, bad := range []string{"", "system.slice", "docker", "gosandbox"} {
		if got := cgroupPathFor(bad); got != "" {
			t.Errorf("cgroupPathFor(%q) = %q, want empty", bad, got)
		}
	}
}

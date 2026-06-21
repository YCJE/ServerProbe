package collector

import (
	"os"
)

// FileReader 抽象文件读取，便于测试
type FileReader interface {
	ReadFile(path string) ([]byte, error)
}

// OSFileReader 使用 os.ReadFile 实现
type OSFileReader struct{}

// ReadFile 读取文件内容
func (r *OSFileReader) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// ProcPath /proc 路径，可被测试覆盖
var ProcPath = "/proc"

// Collector 采集器接口
type Collector interface {
	Collect() (interface{}, error)
	Name() string
}

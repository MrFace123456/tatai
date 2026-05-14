package manager

import (
	"io"
	"os"
	"runtime"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// LogResult 返回给前端的日志结构
type LogResult struct {
	Content    string `json:"content"`
	NextOffset int64  `json:"next_offset"`
	FileSize   int64  `json:"file_size"`
}

// ReadLogByOffset 读取指定位置之后的日志
func ReadLogByOffset(filePath string, offset int64, limit int64) (*LogResult, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	stat, _ := file.Stat()
	fileSize := stat.Size()

	// 如果没有传 offset (第一次读取)，默认读取最后 1000 字节
	if offset == 0 && fileSize > 1000 {
		offset = fileSize - 1000
	}

	// 移动文件指针
	_, err = file.Seek(offset, io.SeekStart)
	if err != nil {
		return nil, err
	}

	// 读取内容
	content := make([]byte, limit)
	n, err := file.Read(content)
	if err != nil && err != io.EOF {
		return nil, err
	}

	data := content[:n]
	result := &LogResult{
		NextOffset: offset + int64(n),
		FileSize:   fileSize,
	}

	// 根据操作系统和内容编码进行转换
	result.Content = decodeLogData(data)

	return result, nil
}
func decodeLogData(data []byte) string {
	return DecodeLogData(data)
}

// decodeLogData 解码日志数据
func DecodeLogData(data []byte) string {
	// 检查是否是有效的 UTF-8
	if utf8.Valid(data) {
		return string(data)
	}

	// 只在 Windows 环境下尝试 GBK 转码
	if runtime.GOOS == "windows" {
		utf8Content, err := gbkToUtf8(data)
		if err == nil {
			return utf8Content
		}
	}

	// 其他情况（Linux 或转换失败），返回原始数据的字符串表示
	// 替换掉无效字符
	return string(bytesToValidUTF8(data))
}

// gbkToUtf8 将 GBK 编码的字节转换为 UTF-8 字符串
func gbkToUtf8(data []byte) (string, error) {
	decoder := simplifiedchinese.GBK.NewDecoder()
	utf8Data, _, err := transform.Bytes(decoder, data)
	if err != nil {
		return "", err
	}
	return string(utf8Data), nil
}

// bytesToValidUTF8 将字节转换为有效的 UTF-8 字符串（替换无效字符）
func bytesToValidUTF8(data []byte) []byte {
	var result []byte
	for i := 0; i < len(data); i++ {
		b := data[i]
		// 跳过无效的 UTF-8 字节序列
		if b < 0x80 {
			result = append(result, b)
		} else if b >= 0xC2 && b <= 0xDF && i+1 < len(data) {
			// 2 字节 UTF-8
			result = append(result, b, data[i+1])
			i++
		} else if b >= 0xE0 && b <= 0xEF && i+2 < len(data) {
			// 3 字节 UTF-8
			result = append(result, b, data[i+1], data[i+2])
			i += 2
		} else {
			// 无效字符，替换为 '?'
			result = append(result, '?')
		}
	}
	return result
}

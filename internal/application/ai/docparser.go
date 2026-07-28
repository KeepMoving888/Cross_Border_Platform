package ai

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/cb-platform/internal/pkg/logger"
)

// DocumentParser 文档解析器抽象
// 支持多模态文档上传:PDF / Word(.docx) / Markdown / 纯文本
// 根据 content-type 或文件扩展名自动选择解析器
type DocumentParser interface {
	// Parse 解析文档内容,返回纯文本
	Parse(data []byte, filename string) (string, error)
	// SupportedExtensions 支持的文件扩展名
	SupportedExtensions() []string
}

// ============== 多模态解析器(组合模式) ==============

// MultiModalParser 组合多个解析器,根据文件类型分发
type MultiModalParser struct {
	parsers map[string]DocumentParser // key: 小写扩展名(不含点)
}

// NewMultiModalParser 创建多模态解析器
// 优先使用纯 Go 实现,避免 CGO 依赖
func NewMultiModalParser() *MultiModalParser {
	m := &MultiModalParser{parsers: make(map[string]DocumentParser)}
	// 纯文本 / Markdown(始终可用)
	m.Register("txt", &TextParser{})
	m.Register("md", &TextParser{})
	m.Register("markdown", &TextParser{})
	// PDF / Word:尝试注册,若依赖不可用则跳过(运行时降级)
	m.Register("pdf", &PDFParser{})
	m.Register("docx", &DocxParser{})
	return m
}

// Register 注册解析器
func (m *MultiModalParser) Register(ext string, p DocumentParser) {
	m.parsers[strings.ToLower(ext)] = p
}

// Parse 根据文件扩展名选择解析器
func (m *MultiModalParser) Parse(data []byte, filename string) (string, error) {
	ext := extractExt(filename)
	if ext == "" {
		// 无扩展名,默认按纯文本处理
		return string(data), nil
	}
	parser, ok := m.parsers[ext]
	if !ok {
		// 不支持的类型,降级为纯文本(可能产生乱码,但保证可用)
		logger.Get().Warnf("no parser for ext %s, fallback to raw text", ext)
		return string(data), nil
	}
	return parser.Parse(data, filename)
}

// SupportedExtensions 所有已注册解析器支持的扩展名
func (m *MultiModalParser) SupportedExtensions() []string {
	exts := make([]string, 0, len(m.parsers))
	for ext := range m.parsers {
		exts = append(exts, ext)
	}
	return exts
}

// extractExt 提取文件扩展名(小写,不含点)
func extractExt(filename string) string {
	idx := strings.LastIndex(filename, ".")
	if idx < 0 || idx == len(filename)-1 {
		return ""
	}
	return strings.ToLower(filename[idx+1:])
}

// ============== 纯文本 / Markdown 解析器 ==============

// TextParser 纯文本和 Markdown 解析器
// Markdown 直接作为文本处理(LLM 能理解 Markdown 语法)
type TextParser struct{}

func (p *TextParser) Parse(data []byte, filename string) (string, error) {
	return string(data), nil
}

func (p *TextParser) SupportedExtensions() []string {
	return []string{"txt", "md", "markdown"}
}

// ============== PDF 解析器 ==============

// PDFParser PDF 文档解析器
// 实现策略:
//  1. 优先使用纯 Go 的 pdf 解析库(避免 CGO 依赖)
//  2. 若库不可用,降级为二进制文本提取(提取可读 ASCII 片段)
//  3. 生产环境建议集成外部服务(如 Apache Tika)处理复杂 PDF
type PDFParser struct{}

func (p *PDFParser) Parse(data []byte, filename string) (string, error) {
	// 尝试使用纯 Go PDF 文本提取
	if text, err := extractPDFText(data); err == nil && strings.TrimSpace(text) != "" {
		return text, nil
	}
	// 降级:提取 PDF 中的文本流(简单版,基于 BT/ET 文本块)
	text := extractPDFTextFallback(data)
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("无法从 PDF 提取文本,可能为扫描件(需 OCR),建议使用文本格式上传")
	}
	return text, nil
}

func (p *PDFParser) SupportedExtensions() []string {
	return []string{"pdf"}
}

// extractPDFText 尝试使用外部库提取 PDF 文本
// 这里保留扩展点,实际集成时引入如 github.com/ledongthuc/pdf
func extractPDFText(data []byte) (string, error) {
	// TODO: 集成纯 Go PDF 库(如 github.com/ledongthuc/pdf)
	// 当前返回 error 触发降级
	return "", fmt.Errorf("pdf library not integrated")
}

// extractPDFTextFallback PDF 文本提取降级方案
// 从 PDF 字节流中提取 BT...ET 块内的文本操作符(Tj/TJ)
// 这是一种简化的 PDF 文本提取,不处理字体编码和压缩,仅适用于未压缩的简单 PDF
func extractPDFTextFallback(data []byte) string {
	var sb strings.Builder
	content := string(data)

	// PDF 文本对象以 BT 开始,ET 结束
	// 文本显示操作符:Tj(显示字符串)、TJ(显示字符串数组)、'(移动到下一行显示)、"(设置字间距并显示)
	inText := false
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "BT" {
			inText = true
			continue
		}
		if trimmed == "ET" {
			inText = false
			sb.WriteString("\n")
			continue
		}
		if !inText {
			continue
		}
		// 提取 (text) Tj 格式的文本
		text := extractParenthesesText(trimmed)
		if text != "" {
			sb.WriteString(text)
			sb.WriteString(" ")
		}
	}

	result := sb.String()
	// 清理 PDF 转义字符
	result = strings.ReplaceAll(result, "\\(", "(")
	result = strings.ReplaceAll(result, "\\)", ")")
	result = strings.ReplaceAll(result, "\\n", "\n")
	result = strings.ReplaceAll(result, "\\r", "")
	result = strings.ReplaceAll(result, "\\", "")
	return strings.TrimSpace(result)
}

// extractParenthesesText 提取括号内的文本
// PDF 中 (Hello) Tj 表示显示 "Hello"
func extractParenthesesText(s string) string {
	var sb strings.Builder
	inParen := false
	escape := false
	for _, r := range s {
		if escape {
			sb.WriteRune(r)
			escape = false
			continue
		}
		if r == '\\' {
			escape = true
			continue
		}
		if r == '(' {
			inParen = true
			continue
		}
		if r == ')' {
			if inParen {
				sb.WriteString(" ")
			}
			inParen = false
			continue
		}
		if inParen {
			sb.WriteRune(r)
		}
	}
	return strings.TrimSpace(sb.String())
}

// ============== Word(.docx) 解析器 ==============

// DocxParser Word .docx 文档解析器
// .docx 本质是 ZIP 压缩包,核心文本在 word/document.xml 中
// 这里使用标准库 archive/zip 提取 XML,再解析 <w:t> 标签获取文本
type DocxParser struct{}

func (p *DocxParser) Parse(data []byte, filename string) (string, error) {
	return extractDocxText(data)
}

func (p *DocxParser) SupportedExtensions() []string {
	return []string{"docx"}
}

// extractDocxText 从 .docx 提取文本
// .docx 是 ZIP 包,word/document.xml 包含正文
func extractDocxText(data []byte) (string, error) {
	// 使用标准库 archive/zip 读取
	// 避免引入第三方依赖,保持构建简单
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("open docx zip: %w", err)
	}

	var documentXML []byte
	for _, file := range reader.File {
		if file.Name == "word/document.xml" {
			rc, err := file.Open()
			if err != nil {
				return "", fmt.Errorf("open document.xml: %w", err)
			}
			documentXML, err = io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return "", fmt.Errorf("read document.xml: %w", err)
			}
			break
		}
	}

	if documentXML == nil {
		return "", fmt.Errorf("document.xml not found in docx")
	}

	// 解析 <w:t> 标签提取文本
	text := extractWtText(string(documentXML))
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("no text extracted from docx")
	}
	return text, nil
}

// extractWtText 从 Word XML 提取 <w:t> 标签内容
// <w:t> 是 WordprocessingML 中的文本运行标签
// <w:p> 是段落,遇到段落分隔符插入换行
func extractWtText(xml string) string {
	var sb strings.Builder
	inWt := false
	inWp := false
	tagBuf := strings.Builder{}
	inTag := false

	for i := 0; i < len(xml); i++ {
		c := xml[i]
		if c == '<' {
			inTag = true
			tagBuf.Reset()
			tagBuf.WriteByte(c)
			continue
		}
		if c == '>' {
			tagBuf.WriteByte(c)
			inTag = false
			tag := tagBuf.String()
			// 段落开始
			if strings.HasPrefix(tag, "<w:p ") || tag == "<w:p>" || strings.HasPrefix(tag, "<w:p>") {
				if inWp {
					sb.WriteString("\n")
				}
				inWp = true
			}
			// 文本运行开始
			if strings.HasPrefix(tag, "<w:t ") || tag == "<w:t>" || strings.HasPrefix(tag, "<w:t>") {
				inWt = true
			}
			// 文本运行结束
			if tag == "</w:t>" {
				inWt = false
			}
			// 段落结束
			if tag == "</w:p>" {
				sb.WriteString("\n")
				inWp = false
			}
			continue
		}
		if inTag {
			tagBuf.WriteByte(c)
			continue
		}
		if inWt {
			sb.WriteByte(c)
		}
	}
	return strings.TrimSpace(sb.String())
}

// ============== 解析器工厂 ==============

// defaultParser 默认多模态解析器(单例)
var defaultParser = NewMultiModalParser()

// GetDefaultParser 获取默认解析器
func GetDefaultParser() *MultiModalParser {
	return defaultParser
}

// ParseDocument 解析文档的便捷函数
// 支持类型:txt, md, markdown, pdf, docx
func ParseDocument(data []byte, filename string) (string, error) {
	return defaultParser.Parse(data, filename)
}

// IsSupportedFile 检查文件类型是否支持解析
func IsSupportedFile(filename string) bool {
	ext := extractExt(filename)
	if ext == "" {
		return false
	}
	_, ok := defaultParser.parsers[ext]
	return ok
}

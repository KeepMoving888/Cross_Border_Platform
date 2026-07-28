package ai

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// ============== TextParser 测试 ==============

func TestTextParser_Parse(t *testing.T) {
	p := &TextParser{}
	text, err := p.Parse([]byte("hello world"), "test.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "hello world" {
		t.Errorf("expected 'hello world', got %q", text)
	}
}

func TestTextParser_SupportedExtensions(t *testing.T) {
	p := &TextParser{}
	exts := p.SupportedExtensions()
	if len(exts) != 3 {
		t.Errorf("expected 3 extensions, got %d", len(exts))
	}
}

// ============== MultiModalParser 测试 ==============

func TestMultiModalParser_ParseText(t *testing.T) {
	m := NewMultiModalParser()
	text, err := m.Parse([]byte("plain text"), "doc.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "plain text" {
		t.Errorf("expected 'plain text', got %q", text)
	}
}

func TestMultiModalParser_ParseMarkdown(t *testing.T) {
	m := NewMultiModalParser()
	md := "# Title\n\nThis is **bold** text."
	text, err := m.Parse([]byte(md), "readme.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != md {
		t.Errorf("markdown should be parsed as raw text")
	}
}

func TestMultiModalParser_ParseUnsupportedType(t *testing.T) {
	m := NewMultiModalParser()
	// 不支持的类型应降级为 raw text(不报错)
	text, err := m.Parse([]byte("raw data"), "file.xyz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "raw data" {
		t.Errorf("expected raw text fallback, got %q", text)
	}
}

func TestMultiModalParser_ParseNoExtension(t *testing.T) {
	m := NewMultiModalParser()
	text, err := m.Parse([]byte("no ext"), "filename")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "no ext" {
		t.Errorf("expected raw text for no extension, got %q", text)
	}
}

func TestMultiModalParser_SupportedExtensions(t *testing.T) {
	m := NewMultiModalParser()
	exts := m.SupportedExtensions()
	if len(exts) < 5 {
		t.Errorf("expected >=5 extensions, got %d", len(exts))
	}
}

// ============== DocxParser 测试 ==============

func TestDocxParser_ParseValidDocx(t *testing.T) {
	// 构造一个最小的 .docx (ZIP 包含 word/document.xml)
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	docXML := `<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p><w:r><w:t>Hello</w:t></w:r></w:p>
    <w:p><w:r><w:t>World</w:t></w:r></w:p>
  </w:body>
</w:document>`
	f, err := w.Create("word/document.xml")
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	f.Write([]byte(docXML))
	w.Close()

	p := &DocxParser{}
	text, err := p.Parse(buf.Bytes(), "test.docx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(text, "Hello") {
		t.Errorf("expected text to contain 'Hello', got %q", text)
	}
	if !strings.Contains(text, "World") {
		t.Errorf("expected text to contain 'World', got %q", text)
	}
}

func TestDocxParser_ParseInvalidZip(t *testing.T) {
	p := &DocxParser{}
	_, err := p.Parse([]byte("not a zip"), "test.docx")
	if err == nil {
		t.Error("expected error for invalid zip")
	}
}

func TestDocxParser_ParseNoDocumentXML(t *testing.T) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, _ := w.Create("other.xml")
	f.Write([]byte("<xml/>"))
	w.Close()

	p := &DocxParser{}
	_, err := p.Parse(buf.Bytes(), "test.docx")
	if err == nil {
		t.Error("expected error when document.xml missing")
	}
}

// ============== PDFParser 测试 ==============

func TestPDFParser_ParseEmptyPDF(t *testing.T) {
	p := &PDFParser{}
	// 空内容应返回错误(无法提取文本)
	_, err := p.Parse([]byte(""), "test.pdf")
	if err == nil {
		t.Error("expected error for empty PDF")
	}
}

func TestPDFParser_ParseSimplePDF(t *testing.T) {
	// 构造包含 BT...ET 文本块的伪 PDF
	pdfContent := `BT
(Hello World) Tj
ET
BT
(This is a test) Tj
ET`
	p := &PDFParser{}
	text, err := p.Parse([]byte(pdfContent), "test.pdf")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(text, "Hello World") {
		t.Errorf("expected text to contain 'Hello World', got %q", text)
	}
	if !strings.Contains(text, "This is a test") {
		t.Errorf("expected text to contain 'This is a test', got %q", text)
	}
}

// ============== 工具函数测试 ==============

func TestExtractExt(t *testing.T) {
	tests := []struct {
		filename string
		expected string
	}{
		{"file.txt", "txt"},
		{"file.PDF", "pdf"}, // 大写转小写
		{"archive.tar.gz", "gz"},
		{"noext", ""},
		{"trailing.", ""},
		{".hidden", "hidden"},
	}
	for _, tt := range tests {
		got := extractExt(tt.filename)
		if got != tt.expected {
			t.Errorf("extractExt(%q) = %q, expected %q", tt.filename, got, tt.expected)
		}
	}
}

func TestExtractParenthesesText(t *testing.T) {
	tests := []struct {
		input    string
		contains string
	}{
		{"(Hello) Tj", "Hello"},
		{"(Hello) Tj (World) Tj", "Hello"},
		{"no text here", ""},
		{"(escaped \\(paren\\) text) Tj", "escaped"},
	}
	for _, tt := range tests {
		got := extractParenthesesText(tt.input)
		if tt.contains != "" && !strings.Contains(got, tt.contains) {
			t.Errorf("extractParenthesesText(%q) = %q, expected to contain %q", tt.input, got, tt.contains)
		}
	}
}

func TestExtractWtText(t *testing.T) {
	xml := `<w:p><w:r><w:t>Hello</w:t></w:r></w:p><w:p><w:r><w:t>World</w:t></w:r></w:p>`
	text := extractWtText(xml)
	if !strings.Contains(text, "Hello") {
		t.Errorf("expected to contain 'Hello', got %q", text)
	}
	if !strings.Contains(text, "World") {
		t.Errorf("expected to contain 'World', got %q", text)
	}
}

func TestIsSupportedFile(t *testing.T) {
	supported := []string{"doc.txt", "readme.md", "manual.pdf", "report.docx"}
	for _, f := range supported {
		if !IsSupportedFile(f) {
			t.Errorf("expected %s to be supported", f)
		}
	}
	unsupported := []string{"image.png", "video.mp4", "noext"}
	for _, f := range unsupported {
		if IsSupportedFile(f) {
			t.Errorf("expected %s to be unsupported", f)
		}
	}
}

func TestParseDocument_TextFile(t *testing.T) {
	text, err := ParseDocument([]byte("hello"), "test.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "hello" {
		t.Errorf("expected 'hello', got %q", text)
	}
}

func TestParseDocument_UnsupportedFallback(t *testing.T) {
	// 不支持的类型应降级为 raw text
	text, err := ParseDocument([]byte("raw"), "file.xyz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "raw" {
		t.Errorf("expected raw fallback, got %q", text)
	}
}

func TestGetDefaultParser(t *testing.T) {
	p := GetDefaultParser()
	if p == nil {
		t.Fatal("expected non-nil default parser")
	}
	exts := p.SupportedExtensions()
	if len(exts) < 5 {
		t.Errorf("expected >=5 extensions, got %d", len(exts))
	}
}

package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestReadAndWriteDocument(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "测试.md")
	content := "# 标题\n\n你好，Markdown。"

	result, err := writeDocument(path, content)
	if err != nil {
		t.Fatalf("writeDocument failed: %v", err)
	}
	if result.Name != "测试.md" {
		t.Fatalf("unexpected name: %s", result.Name)
	}
	document, err := readDocument(path)
	if err != nil {
		t.Fatalf("readDocument failed: %v", err)
	}
	if document.Content != content {
		t.Fatalf("content mismatch: %q", document.Content)
	}
}

func TestReadDocumentRejectsInvalidUTF8(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.md")
	if err := os.WriteFile(path, []byte{0xff, 0xfe}, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readDocument(path); err == nil {
		t.Fatal("expected invalid UTF-8 error")
	}
}

func TestExportDefaultFilename(t *testing.T) {
	tests := []struct {
		name          string
		currentPath   string
		suggestedName string
		extension     string
		want          string
	}{
		{name: "current document", currentPath: filepath.Join("tmp", "报告.md"), suggestedName: "ignored.md", extension: "pdf", want: "报告.pdf"},
		{name: "unsaved document", suggestedName: "草稿.markdown", extension: "html", want: "草稿.html"},
		{name: "empty document", extension: "png", want: "未命名.png"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := exportDefaultFilename(test.currentPath, test.suggestedName, test.extension); got != test.want {
				t.Fatalf("expected %q, got %q", test.want, got)
			}
		})
	}
}

func TestEnsureExportExtension(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		extension string
		want      string
	}{
		{name: "missing", path: filepath.Join("tmp", "report"), extension: "html", want: filepath.Join("tmp", "report.html")},
		{name: "present", path: filepath.Join("tmp", "report.HTML"), extension: "html", want: filepath.Join("tmp", "report.HTML")},
		{name: "windows duplicate", path: filepath.Join("tmp", "report.html.html"), extension: "html", want: filepath.Join("tmp", "report.html")},
		{name: "Chinese IME duplicate", path: filepath.Join("tmp", "report。html.html"), extension: "html", want: filepath.Join("tmp", "report.html")},
		{name: "full width duplicate", path: filepath.Join("tmp", "report．html.html"), extension: "html", want: filepath.Join("tmp", "report.html")},
		{name: "repeated duplicate", path: filepath.Join("tmp", "report.pdf.pdf.pdf"), extension: ".pdf", want: filepath.Join("tmp", "report.pdf")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ensureExportExtension(test.path, test.extension); got != test.want {
				t.Fatalf("expected %q, got %q", test.want, got)
			}
		})
	}
}

func TestDecodeAndWriteExportPayload(t *testing.T) {
	pdf := []byte("%PDF-1.7\nexport test")
	encoded := base64.StdEncoding.EncodeToString(pdf)
	decoded, err := decodeExportPayload("pdf", encoded)
	if err != nil {
		t.Fatalf("decodeExportPayload failed: %v", err)
	}
	if string(decoded) != string(pdf) {
		t.Fatalf("decoded payload mismatch: %q", decoded)
	}

	path := filepath.Join(t.TempDir(), "report.pdf")
	result, err := writeExportFile(path, "pdf", decoded)
	if err != nil {
		t.Fatalf("writeExportFile failed: %v", err)
	}
	if result.Name != "report.pdf" {
		t.Fatalf("unexpected export name: %s", result.Name)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(written) != string(pdf) {
		t.Fatalf("written payload mismatch: %q", written)
	}
}

func TestDecodeExportPayloadRejectsInvalidData(t *testing.T) {
	if _, err := decodeExportPayload("pdf", "not base64"); err == nil {
		t.Fatal("expected invalid base64 error")
	}
	badPDF := base64.StdEncoding.EncodeToString([]byte("not a pdf"))
	if _, err := decodeExportPayload("pdf", badPDF); err == nil {
		t.Fatal("expected invalid PDF signature error")
	}
	badPNG := base64.StdEncoding.EncodeToString([]byte("not a png"))
	if _, err := decodeExportPayload("png", badPNG); err == nil {
		t.Fatal("expected invalid PNG signature error")
	}
	invalidUTF8 := base64.StdEncoding.EncodeToString([]byte{0xff, 0xfe})
	if _, err := decodeExportPayload("txt", invalidUTF8); err == nil {
		t.Fatal("expected invalid UTF-8 error")
	}
}

func TestEmptyPlainTextExportIsValid(t *testing.T) {
	payload, err := decodeExportPayload("txt", "")
	if err != nil {
		t.Fatalf("empty text export should be valid: %v", err)
	}
	if len(payload) != 0 {
		t.Fatalf("expected empty payload, got %q", payload)
	}
}

func TestWelcomeDocumentUsesRequestedLanguage(t *testing.T) {
	chinese := welcomeDocument("zh-CN")
	if chinese.Name != "README.md" || !chinese.Welcome || chinese.BuiltIn != "welcome" || !strings.Contains(chinese.Content, "# 墨笺 Markdown") {
		t.Fatalf("unexpected Chinese welcome document: %#v", chinese)
	}
	english := welcomeDocument("en")
	if english.Name != "README.md" || !english.Welcome || english.BuiltIn != "welcome" || !strings.Contains(english.Content, "# InkMark Markdown") {
		t.Fatalf("unexpected English welcome document: %#v", english)
	}
	if !strings.Contains(chinese.Content, "#inkmark-render-test") || !strings.Contains(english.Content, "#inkmark-render-test") {
		t.Fatal("both welcome documents must link to the rendering test")
	}
}

func TestRenderingTestDocumentIsEmbeddedAndBilingual(t *testing.T) {
	document := renderingTestDocument()
	if document.Path != "" || document.Welcome || document.BuiltIn != "render-test" {
		t.Fatalf("unexpected rendering test metadata: %#v", document)
	}
	if document.Name != "markdown-rendering-test.md" || !utf8.ValidString(document.Content) {
		t.Fatalf("invalid rendering test document: %#v", document)
	}
	for _, marker := range []string{
		"Markdown 综合渲染测试",
		"Comprehensive Rendering Test",
		"| 左对齐功能 / Left |",
		"$E=mc^2$",
		"> [!CAUTION]",
		"<details>",
		"<script>window.markdownUnsafeScriptExecuted",
		"#inkmark-welcome",
	} {
		if !strings.Contains(document.Content, marker) {
			t.Errorf("rendering test is missing %q", marker)
		}
	}
	if got := strings.Count(document.Content, "~~~mermaid"); got != 10 {
		t.Fatalf("expected 10 Mermaid diagrams, got %d", got)
	}
	if strings.Contains(document.Content, "/Users/") {
		t.Fatal("embedded sample must not contain a local absolute path")
	}
}

func TestInitialFileTakesPriorityOverWelcomeDocument(t *testing.T) {
	path := filepath.Join(t.TempDir(), "直接打开.md")
	if err := os.WriteFile(path, []byte("# Direct open"), 0o644); err != nil {
		t.Fatal(err)
	}
	app := &App{
		initPath: path,
		language: LanguageState{Mode: "auto", Locale: "zh-CN"},
	}
	document, err := app.LoadInitialDocument("zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	if document.Path != path || document.Welcome || document.Content != "# Direct open" {
		t.Fatalf("initial file was not loaded: %#v", document)
	}
	if recent := app.recentItemsSnapshot(); len(recent) != 1 || recent[0].Path != path || recent[0].ID == "" {
		t.Fatalf("initial file was not synchronously added to recent items: %#v", recent)
	}
}

func TestResolveDocumentArgument(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "带 空格.md")
	if err := os.WriteFile(path, []byte("# test"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := resolveDocumentArgument([]string{"--ignored", "带 空格.md"}, directory); got != path {
		t.Fatalf("expected %q, got %q", path, got)
	}
	if got := resolveDocumentArgument([]string{"missing.md"}, directory); got != "" {
		t.Fatalf("missing document should be ignored, got %q", got)
	}
}

func TestLanguageSettingsPersist(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "InkMark", "settings.json")
	app := &App{
		language:     LanguageState{Mode: "auto", Locale: "en"},
		settingsPath: settingsPath,
	}
	state, err := app.SetLanguage("zh-CN", "en")
	if err != nil {
		t.Fatal(err)
	}
	if state.Mode != "zh-CN" || state.Locale != "zh-CN" {
		t.Fatalf("manual language must override locale: %#v", state)
	}
	loaded := loadLanguageState(settingsPath)
	if loaded != state {
		t.Fatalf("persisted state mismatch: want %#v, got %#v", state, loaded)
	}
}

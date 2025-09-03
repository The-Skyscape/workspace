package controllers

import (
	"bytes"
	"html/template"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

// RenderMarkdown converts markdown content to HTML using goldmark
// This is used by templates to render markdown files like README.md
// The content parameter comes from file objects in templates
func (c *ReposController) RenderMarkdown(content string) template.HTML {
	// Create a new goldmark markdown processor with GitHub Flavored Markdown extensions
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,           // GitHub Flavored Markdown (tables, strikethrough, etc.)
			extension.Linkify,       // Auto-linkify URLs
			extension.TaskList,      // Task list support
			extension.Typographer,   // Smart punctuation
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(), // Auto-generate heading IDs for anchors
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(), // Preserve line breaks
			html.WithXHTML(),     // XHTML output
			// Note: WithUnsafe() would allow raw HTML, but we'll keep it safe for security
		),
	)

	// Convert markdown to HTML
	var buf bytes.Buffer
	if err := md.Convert([]byte(content), &buf); err != nil {
		// If conversion fails, return the original content escaped
		return template.HTML(template.HTMLEscapeString(content))
	}

	// Post-process the HTML to add Tailwind/DaisyUI classes for styling
	htmlStr := buf.String()
	
	// Add classes to elements for DaisyUI/Tailwind styling
	htmlStr = strings.ReplaceAll(htmlStr, "<pre>", `<pre class="bg-base-200 p-4 rounded overflow-x-auto">`)
	htmlStr = strings.ReplaceAll(htmlStr, "<code>", `<code class="bg-base-200 px-1 rounded text-sm">`)
	
	// Style tables with DaisyUI table classes
	htmlStr = strings.ReplaceAll(htmlStr, "<table>", `<table class="table table-zebra">`)
	htmlStr = strings.ReplaceAll(htmlStr, "<thead>", `<thead class="bg-base-200">`)
	
	// Style headings
	htmlStr = strings.ReplaceAll(htmlStr, "<h1", `<h1 class="text-3xl font-bold mt-6 mb-4"`)
	htmlStr = strings.ReplaceAll(htmlStr, "<h2", `<h2 class="text-2xl font-semibold mt-5 mb-3"`)
	htmlStr = strings.ReplaceAll(htmlStr, "<h3", `<h3 class="text-xl font-semibold mt-4 mb-2"`)
	htmlStr = strings.ReplaceAll(htmlStr, "<h4", `<h4 class="text-lg font-medium mt-3 mb-2"`)
	
	// Style lists
	htmlStr = strings.ReplaceAll(htmlStr, "<ul>", `<ul class="list-disc pl-6 space-y-1">`)
	htmlStr = strings.ReplaceAll(htmlStr, "<ol>", `<ol class="list-decimal pl-6 space-y-1">`)
	
	// Style blockquotes
	htmlStr = strings.ReplaceAll(htmlStr, "<blockquote>", `<blockquote class="border-l-4 border-primary pl-4 italic my-4">`)
	
	// Style links - make them look like DaisyUI links
	re := regexp.MustCompile(`<a href="([^"]+)">`)
	htmlStr = re.ReplaceAllString(htmlStr, `<a href="$1" class="link link-primary">`)
	
	// Style paragraphs with proper spacing
	htmlStr = strings.ReplaceAll(htmlStr, "<p>", `<p class="mb-4">`)
	
	// Handle task lists (checkboxes)
	htmlStr = strings.ReplaceAll(htmlStr, `<input checked="" disabled="" type="checkbox">`, 
		`<input type="checkbox" checked disabled class="checkbox checkbox-sm mr-2">`)
	htmlStr = strings.ReplaceAll(htmlStr, `<input disabled="" type="checkbox">`, 
		`<input type="checkbox" disabled class="checkbox checkbox-sm mr-2">`)
	
	// Add syntax highlighting classes for code blocks
	// This regex finds <pre><code class="language-xxx"> patterns
	codeBlockRe := regexp.MustCompile(`<pre class="[^"]+"><code class="language-(\w+)">`)
	htmlStr = codeBlockRe.ReplaceAllString(htmlStr, `<pre class="bg-base-200 p-4 rounded overflow-x-auto"><code class="language-$1">`)
	
	// Fix file extension detection for syntax highlighting
	// Goldmark might not detect the language, so we try to infer from common patterns
	if ext := filepath.Ext(content); ext != "" && !strings.Contains(htmlStr, "language-") {
		lang := getLanguageFromExt(ext)
		if lang != "" {
			htmlStr = strings.ReplaceAll(htmlStr, `<pre class="bg-base-200 p-4 rounded overflow-x-auto"><code>`, 
				`<pre class="bg-base-200 p-4 rounded overflow-x-auto"><code class="language-`+lang+`">`)
		}
	}
	
	return template.HTML(htmlStr)
}


// getLanguageFromExt maps file extensions to language names for syntax highlighting
func getLanguageFromExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".go":
		return "go"
	case ".js", ".javascript":
		return "javascript"
	case ".ts", ".typescript":
		return "typescript"
	case ".py", ".python":
		return "python"
	case ".rb", ".ruby":
		return "ruby"
	case ".java":
		return "java"
	case ".c":
		return "c"
	case ".cpp", ".cc", ".cxx":
		return "cpp"
	case ".cs":
		return "csharp"
	case ".php":
		return "php"
	case ".rs", ".rust":
		return "rust"
	case ".sh", ".bash":
		return "bash"
	case ".sql":
		return "sql"
	case ".html", ".htm":
		return "html"
	case ".css":
		return "css"
	case ".json":
		return "json"
	case ".xml":
		return "xml"
	case ".yaml", ".yml":
		return "yaml"
	case ".md", ".markdown":
		return "markdown"
	case ".dockerfile":
		return "dockerfile"
	default:
		return ""
	}
}
package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
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

// Notebook data structures
type NotebookData struct {
	Cells    []Cell                 `json:"cells"`
	Metadata map[string]interface{} `json:"metadata"`
	NBFormat int                    `json:"nbformat"`
}

type Cell struct {
	CellType       string                 `json:"cell_type"`
	Source         interface{}            `json:"source"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	ExecutionCount *int                   `json:"execution_count,omitempty"`
	Outputs        []Output               `json:"outputs,omitempty"`
}

type Output struct {
	OutputType     string                 `json:"output_type"`
	Text           interface{}            `json:"text,omitempty"`
	Data           map[string]interface{} `json:"data,omitempty"`
	ExecutionCount *int                   `json:"execution_count,omitempty"`
	Name           string                 `json:"name,omitempty"` // for stream
	Ename          string                 `json:"ename,omitempty"` // for error
	Evalue         string                 `json:"evalue,omitempty"` // for error
	Traceback      []string               `json:"traceback,omitempty"` // for error
}

// RenderNotebook converts notebook JSON content to HTML
func (c *ReposController) RenderNotebook(content string) template.HTML {
	var notebook NotebookData
	if err := json.Unmarshal([]byte(content), &notebook); err != nil {
		// If parsing fails, show error message
		return template.HTML(fmt.Sprintf(
			`<div class="alert alert-error">
				<svg xmlns="http://www.w3.org/2000/svg" class="stroke-current shrink-0 h-6 w-6" fill="none" viewBox="0 0 24 24">
					<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2m7-2a9 9 0 11-18 0 9 9 0 0118 0z" />
				</svg>
				<span>Failed to parse notebook: %s</span>
			</div>`, template.HTMLEscapeString(err.Error())))
	}

	var htmlBuilder strings.Builder
	
	// Notebook container
	htmlBuilder.WriteString(`<div class="notebook-container">`)
	
	// Render each cell
	for i, cell := range notebook.Cells {
		htmlBuilder.WriteString(c.renderCell(cell, i))
	}
	
	htmlBuilder.WriteString(`</div>`)
	
	return template.HTML(htmlBuilder.String())
}

// renderCell renders a single notebook cell
func (c *ReposController) renderCell(cell Cell, index int) string {
	var htmlBuilder strings.Builder
	
	// Cell container with DaisyUI v5 card styling
	htmlBuilder.WriteString(`<div class="card bg-base-100 mb-4 border border-base-300">`)
	htmlBuilder.WriteString(`<div class="card-body p-4">`)
	
	switch cell.CellType {
	case "markdown":
		htmlBuilder.WriteString(c.renderMarkdownCell(cell))
	case "code":
		htmlBuilder.WriteString(c.renderCodeCell(cell, index))
	case "raw":
		htmlBuilder.WriteString(c.renderRawCell(cell))
	default:
		htmlBuilder.WriteString(fmt.Sprintf(`<div class="text-base-content/60">Unknown cell type: %s</div>`, cell.CellType))
	}
	
	htmlBuilder.WriteString(`</div></div>`)
	
	return htmlBuilder.String()
}

// renderMarkdownCell renders a markdown cell
func (c *ReposController) renderMarkdownCell(cell Cell) string {
	source := c.extractSource(cell.Source)
	
	// Use the markdown rendering method
	renderedMarkdown := c.RenderMarkdown(source)
	
	return fmt.Sprintf(`
		<div class="prose prose-lg max-w-none">
			%s
		</div>
	`, renderedMarkdown)
}

// renderCodeCell renders a code cell with its outputs
func (c *ReposController) renderCodeCell(cell Cell, index int) string {
	var htmlBuilder strings.Builder
	
	source := c.extractSource(cell.Source)
	
	// Cell header with execution count
	if cell.ExecutionCount != nil {
		htmlBuilder.WriteString(fmt.Sprintf(`
			<div class="flex items-center gap-2 mb-2">
				<span class="badge badge-sm badge-ghost">In [%d]</span>
				<span class="text-xs text-base-content/60">Code</span>
			</div>
		`, *cell.ExecutionCount))
	}
	
	// Code input
	if source != "" {
		// Detect language from metadata or default to python
		lang := "python"
		if kernelSpec, ok := cell.Metadata["kernel_spec"].(map[string]interface{}); ok {
			if language, ok := kernelSpec["language"].(string); ok {
				lang = language
			}
		}
		
		// Use Prism.js compatible code block with syntax highlighting
		htmlBuilder.WriteString(fmt.Sprintf(`
			<div class="border border-base-300 rounded-lg overflow-hidden mb-3">
				<pre class="line-numbers"><code class="language-%s">%s</code></pre>
			</div>
		`, lang, template.HTMLEscapeString(source)))
	}
	
	// Outputs
	if len(cell.Outputs) > 0 {
		htmlBuilder.WriteString(`<div class="mt-3">`)
		for _, output := range cell.Outputs {
			htmlBuilder.WriteString(c.renderOutput(output))
		}
		htmlBuilder.WriteString(`</div>`)
	}
	
	return htmlBuilder.String()
}

// renderRawCell renders a raw cell
func (c *ReposController) renderRawCell(cell Cell) string {
	source := c.extractSource(cell.Source)
	
	return fmt.Sprintf(`
		<div class="text-xs text-base-content/60 mb-2">Raw</div>
		<pre class="bg-base-200 p-4 rounded overflow-x-auto">%s</pre>
	`, template.HTMLEscapeString(source))
}

// renderOutput renders a cell output
func (c *ReposController) renderOutput(output Output) string {
	var htmlBuilder strings.Builder
	
	switch output.OutputType {
	case "stream":
		text := c.extractSource(output.Text)
		streamClass := "bg-base-200"
		if output.Name == "stderr" {
			streamClass = "bg-error/10 text-error"
		}
		htmlBuilder.WriteString(fmt.Sprintf(`
			<div class="%s p-3 rounded mb-2">
				<div class="text-xs text-base-content/60 mb-1">%s</div>
				<pre class="whitespace-pre-wrap font-mono text-sm">%s</pre>
			</div>
		`, streamClass, output.Name, template.HTMLEscapeString(text)))
		
	case "execute_result", "display_data":
		// Handle different data formats
		if output.Data != nil {
			// Check for HTML content
			if htmlData, ok := output.Data["text/html"]; ok {
				htmlContent := c.extractSource(htmlData)
				htmlBuilder.WriteString(fmt.Sprintf(`
					<div class="output-html border border-base-300 rounded p-3 mb-2">
						%s
					</div>
				`, htmlContent))
			} else if textData, ok := output.Data["text/plain"]; ok {
				// Plain text output
				text := c.extractSource(textData)
				prefix := ""
				if output.ExecutionCount != nil {
					prefix = fmt.Sprintf(`<span class="badge badge-sm badge-ghost">Out [%d]</span> `, *output.ExecutionCount)
				}
				htmlBuilder.WriteString(fmt.Sprintf(`
					<div class="bg-base-200 p-3 rounded mb-2">
						%s
						<pre class="whitespace-pre-wrap font-mono text-sm">%s</pre>
					</div>
				`, prefix, template.HTMLEscapeString(text)))
			}
			
			// Handle image outputs
			if imgData, ok := output.Data["image/png"]; ok {
				imgBase64 := c.extractSource(imgData)
				htmlBuilder.WriteString(fmt.Sprintf(`
					<div class="mb-2">
						<img src="data:image/png;base64,%s" class="max-w-full" />
					</div>
				`, imgBase64))
			}
		}
		
	case "error":
		// Error output
		htmlBuilder.WriteString(fmt.Sprintf(`
			<div class="alert alert-error mb-2">
				<div>
					<div class="font-bold">%s: %s</div>
					<pre class="text-xs mt-2">%s</pre>
				</div>
			</div>
		`, template.HTMLEscapeString(output.Ename), 
		   template.HTMLEscapeString(output.Evalue),
		   template.HTMLEscapeString(strings.Join(output.Traceback, "\n"))))
	}
	
	return htmlBuilder.String()
}

// extractSource extracts string content from various source formats
func (c *ReposController) extractSource(source interface{}) string {
	if source == nil {
		return ""
	}
	
	switch v := source.(type) {
	case string:
		return v
	case []interface{}:
		// Join array of strings
		var lines []string
		for _, line := range v {
			if s, ok := line.(string); ok {
				lines = append(lines, s)
			}
		}
		return strings.Join(lines, "")
	default:
		// Try to convert to string
		return fmt.Sprintf("%v", v)
	}
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
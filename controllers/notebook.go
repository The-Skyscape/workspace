package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/The-Skyscape/devtools/pkg/application"
)

// Notebook controller factory function
func Notebook() (string, *NotebookController) {
	return "notebook", &NotebookController{}
}

// NotebookController handles Jupyter notebook rendering
type NotebookController struct {
	application.BaseController
	markdownController *MarkdownController
}

// Setup registers routes (none needed for notebook rendering)
func (c *NotebookController) Setup(app *application.App) {
	c.BaseController.Setup(app)
	// Initialize markdown controller for rendering markdown cells
	c.markdownController = &MarkdownController{}
	c.markdownController.Setup(app)
}

// Handle returns controller instance for request
func (c NotebookController) Handle(req *http.Request) application.Controller {
	c.Request = req
	return &c
}

// Notebook represents a Jupyter notebook structure
type NotebookData struct {
	Cells    []Cell                 `json:"cells"`
	Metadata map[string]interface{} `json:"metadata"`
	NBFormat int                    `json:"nbformat"`
	NBFormatMinor int               `json:"nbformat_minor"`
}

// Cell represents a notebook cell
type Cell struct {
	CellType       string      `json:"cell_type"`
	Source         interface{} `json:"source"` // string or []string
	Outputs        []Output    `json:"outputs,omitempty"`
	ExecutionCount *int        `json:"execution_count,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// Output represents cell output
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
func (c *NotebookController) RenderNotebook(content string) template.HTML {
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
func (c *NotebookController) renderCell(cell Cell, index int) string {
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
func (c *NotebookController) renderMarkdownCell(cell Cell) string {
	source := c.extractSource(cell.Source)
	
	// Use the markdown controller to render markdown
	renderedMarkdown := c.markdownController.RenderMarkdown(source)
	
	return fmt.Sprintf(`
		<div class="prose prose-lg max-w-none">
			%s
		</div>
	`, renderedMarkdown)
}

// renderCodeCell renders a code cell with its outputs
func (c *NotebookController) renderCodeCell(cell Cell, index int) string {
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
		
		// Use Prism.js compatible markup like in repo-file-view.html
		htmlBuilder.WriteString(fmt.Sprintf(`
			<div class="border border-base-300 rounded-lg overflow-hidden mb-3">
				<pre class="line-numbers"><code class="language-%s">%s</code></pre>
			</div>
		`, lang, template.HTMLEscapeString(source)))
	}
	
	// Render outputs
	if len(cell.Outputs) > 0 {
		htmlBuilder.WriteString(`<div class="cell-outputs">`)
		for _, output := range cell.Outputs {
			htmlBuilder.WriteString(c.renderOutput(output))
		}
		htmlBuilder.WriteString(`</div>`)
	}
	
	return htmlBuilder.String()
}

// renderRawCell renders a raw cell
func (c *NotebookController) renderRawCell(cell Cell) string {
	source := c.extractSource(cell.Source)
	
	return fmt.Sprintf(`
		<div class="bg-base-200 p-3 rounded">
			<pre>%s</pre>
		</div>
	`, template.HTMLEscapeString(source))
}

// renderOutput renders cell output based on type
func (c *NotebookController) renderOutput(output Output) string {
	switch output.OutputType {
	case "stream":
		return c.renderStreamOutput(output)
	case "execute_result", "display_data":
		return c.renderDataOutput(output)
	case "error":
		return c.renderErrorOutput(output)
	default:
		return fmt.Sprintf(`<div class="text-base-content/60">Unknown output type: %s</div>`, output.OutputType)
	}
}

// renderStreamOutput renders stream output (stdout/stderr)
func (c *NotebookController) renderStreamOutput(output Output) string {
	text := c.extractText(output.Text)
	
	// Different styling for stderr
	class := "bg-base-200"
	if output.Name == "stderr" {
		class = "bg-error/10 text-error"
	}
	
	return fmt.Sprintf(`
		<div class="%s p-3 rounded mb-2 overflow-x-auto">
			<pre class="text-sm">%s</pre>
		</div>
	`, class, template.HTMLEscapeString(text))
}

// renderDataOutput renders execute_result or display_data
func (c *NotebookController) renderDataOutput(output Output) string {
	var htmlBuilder strings.Builder
	
	// Output header with execution count if present
	if output.ExecutionCount != nil {
		htmlBuilder.WriteString(fmt.Sprintf(`
			<div class="flex items-center gap-2 mb-1">
				<span class="badge badge-sm badge-ghost">Out [%d]</span>
			</div>
		`, *output.ExecutionCount))
	}
	
	// Check for different data types
	if output.Data != nil {
		// Priority order for display
		if html, ok := output.Data["text/html"]; ok {
			htmlStr := c.extractText(html)
			// Wrap in div to contain the HTML
			htmlBuilder.WriteString(fmt.Sprintf(`
				<div class="bg-base-200 p-3 rounded mb-2 overflow-x-auto">
					%s
				</div>
			`, htmlStr))
		} else if imgPng, ok := output.Data["image/png"]; ok {
			// Handle base64 encoded PNG images
			imgStr := c.extractText(imgPng)
			htmlBuilder.WriteString(fmt.Sprintf(`
				<div class="bg-base-200 p-3 rounded mb-2">
					<img src="data:image/png;base64,%s" class="max-w-full" />
				</div>
			`, imgStr))
		} else if imgJpeg, ok := output.Data["image/jpeg"]; ok {
			// Handle base64 encoded JPEG images
			imgStr := c.extractText(imgJpeg)
			htmlBuilder.WriteString(fmt.Sprintf(`
				<div class="bg-base-200 p-3 rounded mb-2">
					<img src="data:image/jpeg;base64,%s" class="max-w-full" />
				</div>
			`, imgStr))
		} else if plainText, ok := output.Data["text/plain"]; ok {
			text := c.extractText(plainText)
			htmlBuilder.WriteString(fmt.Sprintf(`
				<div class="bg-base-200 p-3 rounded mb-2 overflow-x-auto">
					<pre class="text-sm">%s</pre>
				</div>
			`, template.HTMLEscapeString(text)))
		}
	}
	
	// Fallback to Text field if Data is empty
	if htmlBuilder.Len() == 0 && output.Text != nil {
		text := c.extractText(output.Text)
		htmlBuilder.WriteString(fmt.Sprintf(`
			<div class="bg-base-200 p-3 rounded mb-2 overflow-x-auto">
				<pre class="text-sm">%s</pre>
			</div>
		`, template.HTMLEscapeString(text)))
	}
	
	return htmlBuilder.String()
}

// renderErrorOutput renders error output
func (c *NotebookController) renderErrorOutput(output Output) string {
	var htmlBuilder strings.Builder
	
	htmlBuilder.WriteString(`<div class="alert alert-error mb-2">`)
	htmlBuilder.WriteString(`<div>`)
	
	// Error name and value
	if output.Ename != "" {
		htmlBuilder.WriteString(fmt.Sprintf(`<h3 class="font-bold">%s</h3>`, template.HTMLEscapeString(output.Ename)))
	}
	if output.Evalue != "" {
		htmlBuilder.WriteString(fmt.Sprintf(`<div class="text-sm">%s</div>`, template.HTMLEscapeString(output.Evalue)))
	}
	
	// Traceback
	if len(output.Traceback) > 0 {
		htmlBuilder.WriteString(`<details class="mt-2"><summary class="cursor-pointer text-sm">Traceback</summary>`)
		htmlBuilder.WriteString(`<pre class="text-xs mt-2 overflow-x-auto">`)
		for _, line := range output.Traceback {
			// Remove ANSI color codes if present
			cleanLine := stripANSI(line)
			htmlBuilder.WriteString(template.HTMLEscapeString(cleanLine))
			htmlBuilder.WriteString("\n")
		}
		htmlBuilder.WriteString(`</pre></details>`)
	}
	
	htmlBuilder.WriteString(`</div></div>`)
	
	return htmlBuilder.String()
}

// extractSource extracts source from string or []string
func (c *NotebookController) extractSource(source interface{}) string {
	switch s := source.(type) {
	case string:
		return s
	case []interface{}:
		var lines []string
		for _, line := range s {
			if str, ok := line.(string); ok {
				lines = append(lines, str)
			}
		}
		return strings.Join(lines, "")
	default:
		return ""
	}
}

// extractText extracts text from various formats
func (c *NotebookController) extractText(text interface{}) string {
	switch t := text.(type) {
	case string:
		return t
	case []interface{}:
		var lines []string
		for _, line := range t {
			if str, ok := line.(string); ok {
				lines = append(lines, str)
			}
		}
		return strings.Join(lines, "")
	default:
		return fmt.Sprintf("%v", text)
	}
}

// stripANSI removes ANSI escape codes from text
func stripANSI(text string) string {
	// Simple ANSI escape code removal
	var result bytes.Buffer
	inEscape := false
	
	for _, ch := range text {
		if ch == '\x1b' {
			inEscape = true
		} else if inEscape {
			if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') {
				inEscape = false
			}
		} else {
			result.WriteRune(ch)
		}
	}
	
	return result.String()
}
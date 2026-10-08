package server

import (
	"embed"
	"html/template"
)

//go:embed templates/layout.html templates/list.html templates/detail.html templates/form.html templates/error.html
var templateFS embed.FS

var pageTemplates = parsePages("list", "detail", "form", "error")

func parsePages(names ...string) map[string]*template.Template {
	pages := make(map[string]*template.Template, len(names))
	for _, name := range names {
		pages[name] = template.Must(template.ParseFS(templateFS, "templates/layout.html", "templates/"+name+".html"))
	}
	return pages
}

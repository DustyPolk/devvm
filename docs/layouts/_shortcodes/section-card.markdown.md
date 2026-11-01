{{- /* A section card in a page's Markdown copy: a list item linking the section's own copy, then its
   one-line Markdown. The build fails when no page has the path. */ -}}
{{- $path := .Get "path" -}}
{{- with site.GetPage (printf "/docs/%s" $path) -}}
- [{{ .LinkTitle }}]({{ with .OutputFormats.Get "markdown" }}{{ .RelPermalink }}{{ end }}): {{ trim $.Inner "\n" }}
{{- else -}}
  {{- errorf "section-card: no page docs/%s (%s)" $path .Position -}}
{{- end -}}

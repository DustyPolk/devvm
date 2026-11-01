{{- /* os-tabs in a page's Markdown copy: each tab's name, then its Markdown (_partials/os-sections.md). The
   os-tab shortcodes inside keep their Markdown for it, as they do for the HTML. */ -}}
{{- $_ := .Inner -}}
{{- partial "os-sections.md" (.Store.Get "tabs") -}}

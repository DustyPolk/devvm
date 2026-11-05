{{- /* The install commands in a page's Markdown copy: each system's name, then its commands
   (_partials/install-tabs.html, which the HTML's tabs use too). */ -}}
{{- partial "os-sections.md" (partial "install-tabs.html" (dict "page" .Page "position" .Position)) -}}

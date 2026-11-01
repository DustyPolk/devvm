{{- /* The tabs of os-tabs and install in a page's Markdown copy: each tab's name in bold, then its Markdown.
   Inside a list, the tabs' Markdown is indented as the list is, so each name gets the same indent. */ -}}
{{- $indent := "" -}}
{{- with index . 0 }}{{ $indent = index (findRE `^ *` .content) 0 }}{{ end -}}
{{- range $i, $tab := . -}}
{{- if $i }}

{{ $indent }}{{ end -}}
**{{ $tab.name }}**

{{ $indent }}{{ $tab.content | strings.TrimLeft " " | strings.TrimRight " \n" }}
{{- end -}}

{{- /* A page's Markdown copy: its title as the heading, then its Markdown with its shortcodes rendered, each
   through its .markdown.md version in _shortcodes/. Links to a section's _index.md point at index.md, the
   name of the section's own copy, so they work between the copies; a link written in code, after a
   backtick, stays as it is. */ -}}
# {{ .Title }}
{{ .RenderShortcodes | replaceRE "(^|[^`])(\\[[^\\]]*\\]\\((?:[^)\\s]*/)?)_index\\.md" "${1}${2}index.md" }}

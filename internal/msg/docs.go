package msg

// docs links messages to the docs page that says more: message ID to
// "page#anchor", a page in docs/content/docs and a heading on it. devvm
// prints the page's address on the line after the message's text.
// TestDocLinks checks every entry.
var docs = map[string]string{}

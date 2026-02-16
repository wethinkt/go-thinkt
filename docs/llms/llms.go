// Package llms embeds the llms.txt guide for use by the CLI.
package llms

import _ "embed"

//go:embed llms.txt
var Text string

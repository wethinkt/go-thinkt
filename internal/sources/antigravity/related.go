package antigravity

import "github.com/wethinkt/go-thinkt/internal/thinkt"

// pathArgs maps each path-bearing tool to the argument holding the path
// and the relation it records. Edit arguments name every file a CODE_ACTION
// result reports, so result text is not parsed.
var pathArgs = thinkt.ToolPaths{
	"view_file":                  {Arg: "AbsolutePath", Reason: thinkt.RelatedRead},
	"read_file":                  {Arg: "Path", Reason: thinkt.RelatedRead},
	"list_dir":                   {Arg: "DirectoryPath", Reason: thinkt.RelatedRead},
	"grep_search":                {Arg: "SearchPath", Reason: thinkt.RelatedRead},
	"replace_file_content":       {Arg: "TargetFile", Reason: thinkt.RelatedEdited},
	"multi_replace_file_content": {Arg: "TargetFile", Reason: thinkt.RelatedEdited},
	"write_to_file":              {Arg: "TargetFile", Reason: thinkt.RelatedEdited},
	"edit_file":                  {Arg: "Path", Reason: thinkt.RelatedEdited},
	"write_file":                 {Arg: "Path", Reason: thinkt.RelatedEdited},
	"run_command":                {Arg: "Cwd", Reason: thinkt.RelatedCommand},
}

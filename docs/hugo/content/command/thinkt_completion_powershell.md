---
title: "thinkt completion powershell"
---

## thinkt completion powershell

Generate the autocompletion script for powershell

### Synopsis

Generate the autocompletion script for powershell.

To load completions in your current shell session:

	thinkt completion powershell | Out-String | Invoke-Expression

To load completions for every new session, add the output of the above command
to your powershell profile.


```
thinkt completion powershell [flags]
```

### Options

```
  -h, --help              help for powershell
      --no-descriptions   disable completion descriptions
```

### Options inherited from parent commands

```
  -v, --verbose   verbose output
```

### SEE ALSO

* [thinkt completion](thinkt_completion.md)	 - Generate the autocompletion script for the specified shell


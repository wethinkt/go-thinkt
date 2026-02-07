---
title: "thinkt completion bash"
---

## thinkt completion bash

Generate the autocompletion script for bash

### Synopsis

Generate the autocompletion script for the bash shell.

This script depends on the 'bash-completion' package.
If it is not installed already, you can install it via your OS's package manager.

To load completions in your current shell session:

	source <(thinkt completion bash)

To load completions for every new session, execute once:

#### Linux:

	thinkt completion bash > /etc/bash_completion.d/thinkt

#### macOS:

	thinkt completion bash > $(brew --prefix)/etc/bash_completion.d/thinkt

You will need to start a new shell for this setup to take effect.


```
thinkt completion bash
```

### Options

```
  -h, --help              help for bash
      --no-descriptions   disable completion descriptions
```

### Options inherited from parent commands

```
  -v, --verbose   verbose output
```

### SEE ALSO

* [thinkt completion](thinkt_completion.md)	 - Generate the autocompletion script for the specified shell


Runs things

Uses dep for vendoring

# Example

```
$ cat runtags.sh
#!/bin/bash

notifyrun --exec "./dotags.sh" --ignore "core/tags" --ignore "core/tags.tmp" --ignoreEvent "CHMOD" core
```

```
$ cat dotags.sh
#!/bin/bash

do_tags() {
    ctags -f tags.tmp $(find . | grep ".*\.\(h\|c\|hpp\|cpp\|cc\|hh\)$")
    mv tags.tmp tags
}

cd core && do_tags
```

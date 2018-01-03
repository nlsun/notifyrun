Watches files and runs things in response.

Uses `dep` for vendoring.

It does not recursively watch directories, use shell commands to do that:
```
# Watch mydir
notifyrun --exec "echo hi" mydir

# Recursively watch mydir
notifyrun --exec "echo hi" $(find mydir -type d)
```

# Example

```
$ cat runtags.sh
#!/bin/bash

notifyrun --exec "./dotags.sh" --ignore "core/tags" --ignore "core/tags.tmp" --ignoreEvent "CHMOD" $(find core -type d)
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

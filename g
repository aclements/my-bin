#!/bin/zsh

# Print the file name and line number
ARGS=(-H -n)

FINDARGS=()
FINDPREDS=()

CASE=1
CASEOVERRIDE=0
REC=0
PATTERN=

while [[ -n $1 ]]; do
    arg=$1
    shift
    # Recursive?
    if [[ $arg == -r ]]; then
        REC=1
        continue
    fi
    # Case override?
    if [[ $arg == +i ]]; then
        CASEOVERRIDE=1
        continue
    fi
    # Pattern?
    if [[ $arg != -* ]]; then
        PATTERN=$arg
        # If the search pattern had capitalization, perform a
        # case-sensitive grep
        if [[ ${(L)arg} != $arg ]]; then
            CASE=0
        fi
        # Add predicates for any file globs following the pattern
        for arg in $*; do
            FINDPREDS=($FINDPREDS -name $arg)
        done
        break
    fi
    # Regular argument
    ARGS=($ARGS $arg)
done

# Add case sensitivity argument
if (( $CASE && ! $CASEOVERRIDE )); then
    ARGS=(-i $ARGS)
fi

# If -r was present, grep recursively (recursion is done with find, so
# this must be handled specially)
if (( $REC )); then
    FINDARGS=()
else
    FINDARGS=(-maxdepth 1)
fi

exec find . $FINDARGS \( \( -path "*/.svn" -o -name "*~" -o -path "*/build" -o -path "*/*.tmp" -o -path "*/sandbox" -o -name "semantic.cache" \) -prune \) -o \
    -type f $FINDPREDS -print0 | \
    xargs -0 -e grep $ARGS $PATTERN

#!/bin/zsh

# Print the file name and line number
ARGS=(-H -n)

FINDPATH=
FINDARGS=()
FINDPREDS=()

CASE=1
CASEOVERRIDE=0
REC=0
PATTERN=

while [[ -n $1 ]]; do
    arg=$1
    shift
    # Long argument?
    if [[ $arg == --* && $arg != -- ]]; then
        ARGS=($ARGS $arg)
        continue
    fi
    # Recursive?
    if [[ $arg == -r ]]; then
        REC=1
        continue
    fi
    if [[ $arg == -*r* ]]; then
        REC=1
        argv=(${arg/r/} $argv)
        continue
    fi
    # Case override?
    if [[ $arg == +i ]]; then
        CASEOVERRIDE=1
        continue
    fi
    # Pattern?
    if [[ $arg != -* || $arg == -- ]]; then
        if [[ $arg == -- ]]; then
            arg=$1
            shift
        fi
        PATTERN=$arg
        # If the search pattern had capitalization, perform a
        # case-sensitive grep
        if [[ ${(L)arg} != $arg ]]; then
            CASE=0
        fi
        # Add predicates for any file globs following the pattern
        for arg in $*; do
            if [[ -d $arg ]]; then
                if [[ -n $FINDPATH ]]; then
                    echo "Error: Multiple paths given" > /dev/stderr
                    exit 1
                fi
                FINDPATH=$arg
            else
                FINDPREDS=($FINDPREDS -name $arg)
            fi
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

if [[ -z $FINDPATH ]]; then
    FINDPATH=.
fi

exec find $FINDPATH $FINDARGS \( \( -path "*/.svn" -o -name "*~" -o -name ".#*" -o -path "*/build" -o -path "*/*.tmp" -o -path "*/sandbox" -o -name "semantic.cache" \) -prune \) -o \
    -type f $FINDPREDS -print0 | \
    xargs -0 -e grep $ARGS -- $PATTERN

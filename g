#!/bin/zsh

EXTRA=(-n)

CASE=1
for arg in $*; do
    if [[ $arg != -* && ${(L)arg} != $arg ]]; then
        CASE=0
        break
    fi
done
if (( $CASE )); then
    EXTRA=($EXTRA -i)
fi

exec grep $EXTRA $* *

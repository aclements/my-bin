#!/bin/zsh

EXTRA=(-n)
if [[ $#* == 1 && ${(L)1} == $1 ]]; then
    EXTRA=($EXTRA -i)
fi

exec grep $EXTRA $* *

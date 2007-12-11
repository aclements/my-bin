#!/bin/zsh

exec find . \( \( -path "*/.svn" -o -path "*/CVS" -o -path "*/build" -o -name "*~" -o -path "*/sandbox" \) \
               -prune \) -o $* -print

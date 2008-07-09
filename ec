#!/bin/zsh

if [[ -e "~/.emacs_server" ||
      -e "/tmp/esrv${UID}-$(hostname)" ||
      -e "/tmp/emacs$UID/server" ]]; then
    SERVER_RUNNING=1
else
    SERVER_RUNNING=
fi

if [[ -n $SSH_CONNECTION && -z $SERVER_RUNNING ]]; then
    echo "Forwarding to ${SSH_CONNECTION%% *}"
    exec ssh -ax ${SSH_CONNECTION%% *} \
        cd `pwd` \&\& SSH_CONNECTION= `which ec` $*
fi

typeset -A args

for arg in $*; do
    fname=$(echo $arg | sed -re 's/(.*):([0-9]+):?/\1/')
    lnum=$(echo $arg | sed -re 's/(.*):([0-9]+):?/+\2/')
    if [[ $fname == $lnum ]]; then
        lnum=
    fi
    args[$fname]=$lnum
done


if [[ -n $SERVER_RUNNING ]]; then
    # The emacs server appears to be running
    # Create new frame if requested
    if [[ `basename $0` == ecc ]]; then
        emacsclient -e '(new-frame)'
    fi
    # Open files
    for fname in ${(k)args}; do
        emacsclient -n $args[$fname] $fname > /dev/null
    done
else
    # Which emacs should I run?
    cmd=emacs
    if whence -p emacs21 &> /dev/null; then
        if [[ ! `whence -p emacs21` -ef `whence -p emacs` ]]; then
            cmd=emacs21
        fi
    fi
    typeset -a allargs
    for fname in ${(k)args}; do
        allargs=($allargs $args[$fname] $fname)
    done
    $cmd -f server-start $allargs &
fi

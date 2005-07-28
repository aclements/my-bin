#!/bin/zsh

if [ -e "~/.emacs_server" -o -e "/tmp/esrv${UID}-$(hostname)" ]; then
    # The emacs server appears to be running.  Call upon it
    exec emacsclient $* > /dev/null
else
    # Which emacs should I run?
    if whence -p emacs21 &> /dev/null; then
        if [[ ! `whence -p emacs21` -ef `whence -p emacs` ]]; then
            exec emacs21 -f server-start $*
        fi
    fi
    exec emacs -f server-start $*
fi

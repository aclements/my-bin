#!/usr/bin/python

import sys, os, random

if len(sys.argv) not in (2, 3) or not os.path.isdir(sys.argv[-1]):
    print "Usage: walld.py [-d] <directory>"
    print "  -d causes walld to stay in the foreground"
    print "  by default, walld will semi-daemonize after the first wallpaper"
    sys.exit(1)

if sys.argv[1] == "-d":
    daemonize = False
    del sys.argv[1]
else:
    daemonize = True
wdir = os.path.abspath(sys.argv[1])

xsc = None
event = None
while event != "":
    if event is None or event.startswith("LOCK "):
        f = os.path.join(wdir, random.choice(os.listdir(wdir)))
        if not daemonize:
            print "Setting wallpaper: %s" % f
        os.spawnlp(os.P_WAIT, "Esetroot", "Esetroot", "-center", f)
        if daemonize and not event:
            # Pseudo-daemonize
            if os.fork():
                sys.exit(0)
            os.setpgrp()
            os.chdir("/")
            if os.fork():
                sys.exit(0)
        if not event:
            xsc = os.popen("xscreensaver-command -watch")
    event = xsc.readline()
    if not daemonize:
        print "Event: %s" % event.strip()

if not daemonize:
    print "xscreensaver went away, terminating"

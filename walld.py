#!/usr/bin/python

# Whether or not to change the XFCE terminal background darkness
XFCETERMINAL = True

import sys, os, random, subprocess, re

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

def getMean(f):
    try:
        meanp = subprocess.Popen(("convert", f, "-type", "Grayscale", "-format", "%[mean]", "info:"),
                                 stdout = subprocess.PIPE)
        mean = meanp.stdout.read()
        if meanp.wait() != 0:
            print "convert failed:", meanp.returncode
            return None
    except OSError, e:
        print "convert failed:", e
        return

    try:
        mean = float(mean.strip())/65536.0
    except ValueError, e:
        print "failed to parse mean:", e
        return
    return mean

def setXFCETerminalDarkness(val):
    path = os.path.join(os.environ["HOME"], ".config", "Terminal", "terminalrc")
    try:
        f = open(path, "r")
        data = f.read()
        f.close()
    except IOError, e:
        print "failed to read %s:" % path, e
        return
    data = re.sub("(BackgroundDarkness=)[0-9]+(\\.[0-9]*)?", "\\g<1>%f" % val, data)
    try:
        f = open(path + ".new", "w")
        f.write(data)
        f.close()
    except IOError, e:
        print "failed to write %s.new:" % path, e
        return
    try:
        os.rename(path, path + ".old")
        os.rename(path + ".new", path)
    except OSError, e:
        print "failed to rename %s:" % path, e

xsc = None
event = None
while event != "":
    if event is None or event.startswith("LOCK "):
        f = os.path.join(wdir, random.choice(os.listdir(wdir)))
        if not daemonize:
            print "Setting wallpaper: %s" % f
        os.spawnlp(os.P_WAIT, "Esetroot", "Esetroot", "-center", f)
        if XFCETERMINAL:
            mean = getMean(f)
            if mean:
                setXFCETerminalDarkness(0.6 + mean*0.4)
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

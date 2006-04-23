#!/usr/bin/python
#
# powerd - Basic power management
# Austin Clements <amdragon@mit.edu>

import os, time
import logging

log = None

class CheckError(RuntimeError):
    def __init__(self, msg):
        RuntimeError.__init__(self, msg)

def suspend():
    log.info("Suspending")
    os.system("/etc/acpi/actions/atc_suspend.sh")

def readProcFile(filename):
    if not os.path.isfile(filename):
        return None
    fp = file(filename, "r")
    ret = {}
    for line in fp:
        p = line.find(":")
        if p < 0:
            log.error("Proc file %s contains malformed line %s" %
                      (filename, `line`))
        else:
            ret[line[:p]] = line[p+1:].strip()
    fp.close()
    return ret

def decodeInt(s, unit):
    if not s.endswith(" " + unit):
        raise CheckError, "Expected %s, got '%s'" % (unit, s)
    t = s[0:len(s) - (len(unit) + 1)]
    if not t.isdigit():
        raise CheckError, "Expected integer, got '%s'" % t
    return int(t)

def checkSuspend():
    base = "/proc/acpi/battery/BAT0/"
    if not os.path.isdir(base):
        raise CheckError, "%s not found" % base
    info = readProcFile(base + "info")
    if info is None:
        raise CheckError, "%s/info not found" % base
    state = readProcFile(base + "state")
    if state is None:
        raise CheckError, "%s/state not found" % base

    try:
        capacity = decodeInt(info["last full capacity"], "mWh")
        charging = state["charging state"]
        rate = decodeInt(state["present rate"], "mW")
        remaining = decodeInt(state["remaining capacity"], "mWh")
    except KeyError, e:
        raise CheckError, "Battery property '%s' not found" % e.args[0]
    
    log.debug("Capacity: %d", capacity)
    log.debug("Charging: %s", charging)
    log.debug("Rate: %d", rate)
    log.debug("Remaining: %d", remaining)

    if charging in ("charged", "charging"):
        return
    if charging != "discharging":
        log.warning("Unrecognized charging state '%s'" % charging)

    percentage = min(max(float(remaining)/float(capacity), 0), 1)
    minutes = float(remaining)/float(rate)*60

    log.debug("Percentage: %f", percentage)
    log.debug("Minutes: %f", minutes)

    if percentage <= 0.02 or minutes <= 5:
        return True

    return False

def loop():
    log.debug("Entering loop")
    while True:
        try:
            if checkSuspend():
                suspend()
            else:
                time.sleep(30)
        except CheckError, e:
            log.error(e.args)
            time.sleep(120)
        except:
            log.error(e.args)
            time.sleep(120)

def setupHotkeys():
    log.info("Enabling IBM hotkeys")
    name = "/proc/acpi/ibm/hotkey"
    if not os.access(name, os.W_OK):
        log.fatal("Cannot write to %s" % name)
        return
    fp = file(name,"w")
    fp.write("enable")
    fp.close()
    fp = file(name,"w")
    fp.write("0xffff")
    fp.close()
    log.info("Enabled IBM hotkeys")

def setupLog():
    global log
    log = logging.getLogger("powerd")
    handler = logging.StreamHandler()
    log.addHandler(handler)
    log.setLevel(logging.INFO)

def main():
    setupLog()
    setupHotkeys()
    loop()
    
if __name__ == "__main__":
    main()

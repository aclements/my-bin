#!/usr/bin/python

import sys, re, subprocess

cmds = ["set style data linespoints"]
funcs = []
nData = 0

args = sys.argv[1:]
while args:
    arg = args.pop(0)
    m = re.match("--([^=]+)=(.*)", arg)
    if m:
        cmds.append("set %s %s" % m.group(1,2))
        continue

    m = re.match("--([^=]+)", arg)
    if m:
        if not len(args):
            print >> sys.stderr, "Missing value for %s" % arg
            sys.exit(2)
        cmds.append("set %s %s" % (m.group(1), args.pop(0)))
        continue

    if arg == "-":
        arg = "1:2"
    if ":" in arg:
        nData += 1
    funcs.append(arg)

if not funcs:
    funcs.append("1:2")
    nData += 1

if nData:
    blocks = map(str.strip, sys.stdin.read().split("\n\n"))
    blocks = [b for b in blocks if b]
    nfuncs = []
    for func in funcs:
        if ":" not in func:
            nfuncs.append(func)
            continue
        for block in blocks:
            if block.startswith("#"):
                # XXX Escaping
                nfuncs.append("'-' using %s title %r" % (func, block.split("\n", 1)[0][1:].strip()))
            else:
                nfuncs.append("'-' using %s" % func)
    funcs = nfuncs

cmds.append("plot %s" % ",".join(funcs))
for copy in range(nData):
    for block in blocks:
        cmds.append("%s\ne\n" % block)

gnuplot = subprocess.Popen(["gnuplot", "-persist"],
                           stdin = subprocess.PIPE)
gnuplot.stdin.write("\n".join(cmds))
gnuplot.stdin.close()
sys.exit(gnuplot.wait())

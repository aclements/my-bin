from __future__ import with_statement

import sys
import socket
import threading

SOCKNAME = "/tmp/rbackupd-sock"

DEBUG = False

class ProtocolError(RuntimeError):
    pass

class Protocol(object):
    def __init__(self, sock = None):
        if sock:
            self.__sock = sock
        else:
            self.__sock = socket.socket(socket.AF_UNIX)
            self.__sock.connect(SOCKNAME)
        self.__lock = threading.Lock()

    def send(self, *msg):
        with self.__lock:
            sock = self.__sock
            if DEBUG:
                print >> sys.__stderr__, "Sending message %s" % `msg`
            sock.send("%d\n" % len(msg))
            for a in msg:
                if isinstance(a, str):
                    if "\"" in a:
                        sock.send("{%d}%s\n" % (len(a), a))
                    else:
                        sock.send("\""+a+"\"\n")
                elif isinstance(a, int):
                    sock.send("%d\n" % a)
                else:
                    raise RuntimeError, \
                        "Argument is not a string or number: %s" % `a`
        return self

    def recv(self, expect=None):
        while True:
            msg = self.__recv()
            if DEBUG:
                print >> sys.__stderr__, "Got message %s" % `msg`
            if msg == None:
                return None
            if hasattr(self, "cmd_" + msg[0]):
                getattr(self, "cmd_" + msg[0])(*msg[1:])
            elif expect == None or expect == msg[0]:
                return msg
            else:
                raise ProtocolError, "Unexpected message %s waiting for %s" % \
                    (`msg`, expect)

    def __recv(self):
        with self.__lock:
            sock = self.__sock
            cnt = self.__readTo("\n", True)
            if cnt == None:
                return None
            res = []
            for i in range(int(cnt)):
                ch = sock.recv(1)
                if ch == "\"":
                    res.append(self.__readTo("\""))
                    self.__expect("\n")
                elif ch == "{":
                    chs = int(self.__readTo("}"))
                    buf = ""
                    while len(buf) < chs:
                        new = sock.recv(chs-len(buf))
                        if not len(new):
                            raise ProtocolError, "EOF reading string"
                        buf += new
                    res.append(buf)
                    self.__expect("\n")
                else:
                    n = self.__readTo("\n")
                    res.append(int(ch+n))
        return res

    def finish(self):
        while True:
            msg = self.recv()
            if msg == None:
                self.__close()
                return
            else:
                raise ProtocolError, "Unexpected message %s" % `msg`

    def __close(self):
        sock = self.__sock
        sock.shutdown(socket.SHUT_RDWR)
        sock.close()

    def __expect(self, expect):
        ch = self.__sock.recv(1)
        if ch != expect:
            raise ProtocolError, "Expected %s, got %s" % (`expect`, `ch`)

    def __readTo(self, expect, eofokay = False):
        sock = self.__sock
        buf = ""
        while True:
            ch = sock.recv(1)
            if ch == expect:
                return buf
            elif len(ch):
                buf += ch
            elif not len(buf) and eofokay:
                return None
            else:
                raise ProtocolError, "EOF waiting for %s" % `expect`

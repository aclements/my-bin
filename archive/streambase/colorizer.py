import os, sys, errno, select
import signal
from popen2 import Popen3

class Color(object):
    class _Color(object):
        def __init__(self, base):
            self.base = base
    class _Attr(object):
        def __init__(self, base):
            self.base = base

    BLACK = _Color(0)
    RED = _Color(1)
    GREEN = _Color(2)
    YELLOW = _Color(3)
    BLUE = _Color(4)
    MAGENTA = _Color(5)
    CYAN = _Color(6)
    WHITE = _Color(7)
    NONE = None

    BRIGHT = _Attr(1)
    if os.getenv("TERM") == "screen" and False:
        # Stupid GNU screen.  I hacked my .screenrc to enable dimmed
        # text under xterm's.  Enable this otherwise.  Unfortunately,
        # there's no easy programmatic way to determine if this hack
        # is enabled.
        DIM = _Attr(-1)
    else:
        DIM = _Attr(2)
    UNDERSCORE = _Attr(4)
    BLINK = _Attr(5)
    REVERSE = _Attr(7)
    HIDDEN = _Attr(8)

    def __init__(self, fg = None, bg = None, *extra):
        assert fg == None or isinstance(fg, self._Color)
        assert bg == None or isinstance(bg, self._Color)
        for e in extra:
            assert isinstance(e, self._Attr)
        self.__fg = fg
        self.__bg = bg
        self.__extra = extra

    def __call__(self, s):
        if not len(s):
            return ""
        codes = []
        if self.__fg:
            codes.append("3%d" % self.__fg.base)
        if self.__bg:
            codes.append("4%d" % self.__bg.base)
        for extra in self.__extra:
            if extra.base != -1:
                codes.append(str(extra.base))
        ret = ""
        if codes:
            ret += "\x1B[%sm" % ";".join(codes)
        ret += s
        if codes:
            ret += "\x1B[0m"
        return ret

    def __add__(self, attr):
        assert isinstance(attr, self._Attr)
        return Color(self.__fg, self.__bg, *(self.__extra + (attr,)))

CHILD = Color(Color.WHITE, Color.RED, Color.BRIGHT)

def shellEscape(cmd):
    okayChars = ".-_"
    escaped = []
    for arg in cmd:
        escapedarg = []
        for ch in arg:
            if ch.isalnum() or ch in okayChars:
                escapedarg.append(ch)
            else:
                escapedarg.append("\\" + ch)
        escaped.append("".join(escapedarg))
    return " ".join(escaped)

class SIGINTHandler:
    def __init__(self):
        self.dieOnNext = True
        self.child = 0

    def setChild(self, pid):
        self.child = pid
        self.dieOnNext = False

    def __call__(self, signum, stack):
        if self.dieOnNext:
            if self.child:
                self.msg("Killing child %d with signal 9" % self.child)
                os.kill(-self.child, signal.SIGKILL)
            signal.default_int_handler()
        else:
            self.dieOnNext = True
            if self.child:
                self.msg("Interrupt: Waiting for child %s" % self.child)
                os.kill(-self.child, signal.SIGINT)

    def msg(self, msg):
        pad = (79 - len(msg))/2
        print >> sys.stdout
        print >> sys.stdout, CHILD("-"*pad + msg + "-"*pad)

class Colorizer(object):
    def __init__(self, args = sys.argv[1:]):
        if "--" in args:
            cmdpos = args.index("--")
            self.__cmd = args[cmdpos+1:]
            pargs = args[:cmdpos]
        else:
            self.__cmd = None
            pargs = args[:]
        self.processArgs(pargs)

    def start(self):
        if self.__cmd:
            handler = SIGINTHandler()
            signal.signal(signal.SIGINT, handler)
            env = self.getExtraEnvironment()
#             pre = " ".join(["%s=%s" % (name, val)
#                             for name, val in env.iteritems()])
#             child = Popen4(pre + " " + shellEscape(self.__cmd))
#             fin = child.fromchild
#             pid = child.pid
            import pty
            pid, mfd = pty.fork()
            if pid == 0:
                import resource
                maxfd = resource.getrlimit(resource.RLIMIT_NOFILE)[0]
                for fd in range(3, maxfd):
                    try:
                        os.close(fd)
                    except OSError:
                        pass
                newenv = os.environ.copy()
                newenv.update(env)
                os.execvpe(self.__cmd[0], self.__cmd, newenv)
            fin = os.fdopen(mfd)
            handler.setChild(pid)
        else:
            fin = sys.stdin
            pid = None

        fout = sys.stdout

        try:
            self.run(fin, fout)
        except KeyboardInterrupt:
            pass

        # Reap child
        if pid:
            rval = os.waitpid(pid, 0)[1]
        else:
            rval = 0
        sys.exit(rval)

    def processArgs(self, pargs):
        pass

    def getExtraEnvironment(self):
        return {}

    def run(self, fin, fout):
        pass

def disableLineBuffering(fd):
    from termios import ECHO, ICANON, ISIG, VMIN, VTIME, TCSANOW, \
         tcgetattr, tcsetattr
    mode = tcgetattr(fd)
    LFLAG = 3
    CC = 6
    mode[LFLAG] = (mode[LFLAG] & ~(ECHO | ICANON)) | ISIG
    mode[CC][VMIN] = 0
    mode[CC][VTIME] = 0
    tcsetattr(fd, TCSANOW, mode)

def nonblock(fd):
    import fcntl, os
    flags = fcntl.fcntl(fd, fcntl.F_GETFL)
    fcntl.fcntl(fd, fcntl.F_SETFD, flags | os.O_NONBLOCK)

class CharColorizer(Colorizer):
    def __init__(self, *args, **kwargs):
        Colorizer.__init__(self, *args, **kwargs)
        self.__outBuf = []

    def run(self, fin, fout):
        if fin.isatty():
            disableLineBuffering(fin)
            nonblock(fin)

        self.onStart(fin.isatty())

        buf = ""
        while True:
            if self.__outBuf:
                fout.write("".join(self.__outBuf))
                fout.flush()
                self.__outBuf = []
            new = self.__readAvail(fin, self.getTimeout())
            if new == None:
                self.onTimeout()
                continue
            if not new:
                self.onEOF()
                return
            buf += new
            del new
            while buf:
                nl = buf.find("\n")
                if nl == -1:
                    if buf.endswith("\r"):
                        # Next character might be \n, but we don't
                        # know yet
                        self.onInput(buf[:-1])
                        buf = buf[-1]
                        break
                    else:
                        self.onInput(buf)
                        buf = ""
                else:
                    if nl > 0:
                        if buf[nl-1] == "\r":
                            self.onInput(buf[:nl-1])
                        else:
                            self.onInput(buf[:nl])
                    self.onNewline()
                    buf = buf[nl+1:]

    def onStart(self, isatty):
        pass

    def getTimeout(self):
        return None

    def onTimeout(self):
        pass

    def onEOF(self):
        pass

    def onInput(self, data):
        self.output(data)

    def onNewline(self):
        self.output("\n")

    def output(self, s):
        self.__outBuf.append(s)

    def __readAvail(self, fd, timeout):
        # Block until we get a character (ignore EINTR)
        while True:
            try:
                inset, _, _ = select.select([fd], [], [], timeout)
            except select.error, e:
                if e.args[0] == errno.EINTR:
                    continue
                else:
                    raise
            break
        if not inset:
            return None
        buf = []
        while inset and len(buf) < 10:
            try:
                ch = os.read(fd.fileno(), 1024)
            except OSError, e:
                # When the child of a PTY goes away, read returns EIO
                if e.errno == errno.EINTR or e.errno == errno.EIO:
                    break
                raise
            if not ch:
                break
            buf.append(ch)
            try:
                inset, _, _ = select.select([fd], [], [], 0)
            except select.error, e:
                if e.args[0] == errno.EINTR:
                    break
                raise
        return "".join(buf)

class LineColorizer(CharColorizer):
    def __init__(self, *args, **kwargs):
        CharColorizer.__init__(self, *args, **kwargs)
        self.__line = ""

    def onInput(self, data):
        self.__line += data

    def onNewline(self):
        self.onLine(self.__line)
        self.__line = ""

    def onLine(self, line):
        self.output(line + "\n")

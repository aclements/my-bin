#!/usr/bin/python

import sys, re, time, md5, errno, os
from xml.sax.saxutils import escape

#
# Atom feeds
#

class AtomThing(object):
    def __init__(self, id, title, updated, links = []):
        self.__id = id
        self.__title = title
        self.__updated = time.strftime("%Y-%m-%dT%H:%M:%S",
                                       time.localtime(updated))
        tzhrs = (time.altzone if time.daylight else time.timezone) / 60
        self.__updated += "-" if tzhrs >= 0 else "+"
        self.__updated += "%02d:%02d" % (abs(tzhrs) / 60, abs(tzhrs) % 60)
        self.__links = links

    def _render(self, indent):
        pre = " "*indent
        out = []
        for tag, val in [("id", self.__id), ("title", self.__title),
                         ("updated", self.__updated)]:
            out.append(pre + '<%s>%s</%s>' % (tag, val, tag))
        for rel, href in self.__links:
            txt = pre + '<link '
            if rel:
                txt += 'rel="%s"' % rel
            txt += 'href="%s" />' % href
            out.append(txt)
        return "\n".join(out)

class Feed(AtomThing):
    def __init__(self, id, title, updated, base = None, link = None):
        AtomThing.__init__(self, id, title, updated, [(None, link)] if link else [])
        self.__base = base
        self.__entries = []

    def __iadd__(self, entry):
        self.__entries.append(entry)
        return self

    def __str__(self):
        out = []
        out.append('<?xml version="1.0" encoding="utf-8"?>')
        out.append('<feed xmlns="http://www.w3.org/2005/Atom"')
        out.append('      xml:lang="en"')
        if self.__base:
            out.append('      xml:base="%s"' % self.__base)
        out.append('>')
        out.append(self._render(2))
        for e in self.__entries:
            out.append(str(e))
        out.append('</feed>')
        return "\n".join(out)

class Entry(AtomThing):
    def __init__(self, id, title, updated, content = None, contentType = "html"):
        AtomThing.__init__(self, id, title, updated)
        self.__content = content
        self.__contentType = contentType

    def __str__(self):
        out = []
        out.append('  <entry>')
        out.append(self._render(4))
        if self.__content:
            out.append('    <content type="%s">' % self.__contentType)
            out.append(escape(self.__content))
            out.append('    </content>')
        out.append('  </entry>')
        return "\n".join(out)

def tbEntry(id):
    import traceback
    return Entry(id, "Unhandled exception", time.time(),
                 "<pre>" + escape(traceback.format_exc()) + "</pre>")
    

#
# Input processing
#

def find(name, regexp):
    r = re.compile(regexp)
    all = []
    for l in sys.stdin:
        m = r.search(l)
        all.append(l)
        if m:
            return m
    raise LookupError, "Failed to find " + name + " in " + "".join(all)

def consume(regexp):
    r = re.compile(regexp)
    out = []
    for l in sys.stdin:
        if r.search(l):
            break
        out.append(l)
    return "".join(out)

#
# Date guessing
#

def getDate(dbName, id, default = None, count = 25):
    fname = os.path.expanduser("~/.rssify/dates-%s" % dbName)
    dates = []
    try:
        db = file(fname, "r")
    except IOError, err:
        if err.errno != errno.ENOENT:
            raise err
        try:
            os.mkdir(os.path.dirname(fname))
        except OSError, err:
            if err.errno != errno.EEXIST:
                raise err
    else:
        r = re.compile("([0-9]+) (.+)$")
        for l in db:
            g = r.match(l)
            if not g:
                raise ValueError, "Unable to read date database %s" % dbName
            dates.append((g.group(2), int(g.group(1))))
        db.close()

    ddates = dict(dates)
    if id in ddates:
        return ddates[id]
    if default == None:
        raise LookupError, \
            "Unable to find %s in date database %s" % (`id`, dbName)

    default = int(default)
    while len(dates) >= count:
        dates.pop(0)
    dates.append((id, default))
    db = file(fname, "w")
    for id, date in dates:
        print >> db, "%d %s" % (date, id)
    db.close()
    return default


#
# Processors
#

def dmfa():
    f = Feed("http://www.missmab.com/ local feed", "DMFA", time.time(),
             "http://www.missmab.com", "/")

    try:
        title = find("title", "<TITLE>(DMFA: .*?)</TITLE>").group(1)
    except LookupError:
        return
    try:
        img = find("comic image", "<p><IMG SRC=\"(.*?)\">").group(1)
        subtitle = find("subtitle", "<I>(.*?)</I>").group(1)
        find("rant", "<TD Class=\"rant\"")
        rant = consume("^\\s*</TD></TR>\\s*$")
        try:
            meter = find("donation meter",
                         "^\\s*<IMG SRC=\"(.*?)\"><br>\\s*$").group(1)
        except LookupError:
            meter = None
        if title.startswith("DMFA: "):
            title = title[6:]

        now = time.time()

        comicID = "http://www.missmab.com/comics/" + md5.md5(img).hexdigest()
        comicHTML = """<table width="100%%"><tr><td align="center">
  <img src="%s">
</td></tr></table>""" % img
        comicDate = getDate("dmfa-comics", comicID, now)
        f += Entry(comicID, subtitle, comicDate, comicHTML)

        rantID = "http://www.missmab.com/rants/" + md5.md5(rant).hexdigest()
        rant = rant.replace("</p>", "</p><p>")
        if meter:
            rantHTML = """<table><tr>
  <td valign="top">%s</td>
  <td valign="top" bgcolor="#322C66"><img src="%s"></td>
</tr></table>""" % (rant, meter)
        else:
            rantHTML = rant
        rantDate = getDate("dmfa-rants", rantID, now - 1)
        f += Entry(rantID, title, rantDate, rantHTML)
    except:
        f += tbEntry("http://www.missmab.com/err")

    print str(f)

def abelsstory():
    f = Feed("http://www.missmab.com/Ab local feed", "DMFA Abel's Story", time.time(),
             "http://www.missmab.com/Comics/", "http://www.missmab.com/Comics/Arch_Ab.php")

    try:
        find("title", "<TITLE>Bonus")
    except LookupError:
        return
    try:
        find("table start", "<IMG SRC=\"Header_Ab.gif\"")
        count = 0
        now = time.time()
        while True:
            link = find("table entry",
                        "^\\s*<br>\\s*$|<A HREF=\"(.*?)\\.php\">(.*?)<")
            name, title = link.group(1, 2)
            if not name:
                break
            title = title.strip()

            comicID = "http://www.missmab.com/Ab/" + md5.md5(name).hexdigest()
            comicHTML = """<table width="100%%"><tr><td align="center">
  <img src="%s.jpg">
</td></tr></table>""" % name
            comicDate = getDate("abelsstory-comics", comicID, now + count,
                                count = 100)
            f += Entry(comicID, title, comicDate, comicHTML)
            count += 1
    except:
        f += tbEntry("http://www.missmab.com/Ab/err")

    print str(f)

handlers = {"dmfa" : dmfa, "abelsstory" : abelsstory}

if len(sys.argv) != 2 or sys.argv[1] not in handlers:
    print "Usage: %s %s" % (sys.argv[0], "|".join(handlers.keys()))
    sys.exit(1)
handlers[sys.argv[1]]()

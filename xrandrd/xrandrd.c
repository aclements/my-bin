// Run "xauto" to reconfigure xrandr whenever the physical screen
// configuration changes.

// Recent xf86-video-intel drivers (2.13.0+) generate the necessary
// Xrandr events on monitor hot plug.  See
// http://www.spinics.net/lists/dri-devel/msg04479.html,

// See also http://git.gnome.org/browse/gnome-settings-daemon/tree/plugins/xrandr/gsd-xrandr-manager.c

#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <X11/Xlib.h>
#include <X11/extensions/Xrandr.h>

struct randrResources
{
        XRRScreenResources *res;
        XRROutputInfo **outputs;
};

static Display *dpy;
static Window root;

struct randrResources *
getRandrResources(void)
{
        struct randrResources *r;
        if (!(r = malloc(sizeof *r)) ||
            !(r->res = XRRGetScreenResources(dpy, root)) ||
            !(r->outputs = malloc(r->res->noutput * sizeof *r->outputs)))
                goto bad;
        for (int i = 0; i < r->res->noutput; ++i) {
                r->outputs[i] = XRRGetOutputInfo(dpy, r->res,
                                                 r->res->outputs[i]);
                if (!r->outputs[i])
                        goto bad;
                printf("%s %d\n", r->outputs[i]->name, r->outputs[i]->connection);
        }
        return r;

bad:
        fprintf(stderr, "Failed to get screen resources\n");
        exit(1);
}

bool
randrResourcesEqual(struct randrResources *a, struct randrResources *b)
{
        if (a->res->noutput != b->res->noutput)
                return false;
        for (int i = 0; i < a->res->noutput; ++i) {
                XRROutputInfo *ao = a->outputs[i], *bo = b->outputs[i];
                if (strcmp(ao->name, bo->name) ||
                    ao->nmode != bo->nmode ||
                    ao->npreferred != bo->npreferred ||
                    memcmp(ao->modes, bo->modes,
                           sizeof(*ao->modes) * ao->nmode))
                        return false;
        }
        return true;
}

void
freeRandrResources(struct randrResources *r)
{
        for (int i = 0; i < r->res->noutput; ++i)
                XRRFreeOutputInfo(r->outputs[i]);
        free(r->outputs);
        XRRFreeScreenResources(r->res);
        free(r);
}

void
handleChange(void)
{
        static struct randrResources *prev = NULL;
        struct randrResources *now = getRandrResources();
        if (!prev || !randrResourcesEqual(prev, now)) {
                printf("Resources differ\n");
                system("xauto");
        } else
                printf("Resources do not differ\n");
        if (prev)
                freeRandrResources(prev);
        prev = now;
}

int
main(int argc, char **argv)
{
        int rrEvent, rrError;
        int r;

        // Open the X display and check for Xrandr
        dpy = XOpenDisplay(NULL);
        if (!dpy) {
                fprintf(stderr, "Can't open display %s\n", XDisplayName(NULL));
                exit(1);
        }
        root = RootWindow(dpy, DefaultScreen(dpy));
        int major, minor;
        if (!XRRQueryVersion (dpy, &major, &minor)) {
                fprintf(stderr, "RandR extension missing\n");
                exit(1);
        }
        if (major < 0 || (major == 1 && minor < 2)) {
                fprintf(stderr, "Requires RandR >= 1.2\n");
                exit(1);
        }

        // Get Xrandr event base
        if (!XRRQueryExtension(dpy, &rrEvent, &rrError)) {
                fprintf(stderr, "Failed to query RandR extension\n");
                exit(1);
        }

        // Assume things are initially configured incorrectly
        handleChange();

        // Monitor xrandr events on the root window
        XRRSelectInput(dpy, root, RROutputChangeNotifyMask);

        // Handle events
        while (1) {
                XEvent ev;
                XNextEvent(dpy, &ev);
                if (ev.type == rrEvent + RRNotify_OutputChange) {
                        handleChange();
                } else {
                        printf("Unexpected X event: %d\n", ev.type);
                }
        }

        XCloseDisplay(dpy);
}

// Run "xauto" to reconfigure xrandr whenever the physical screen
// configuration changes.  Inspired by
//   http://unix.stackexchange.com/questions/4489/a-tool-for-automatically-applying-randr-configuration-when-external-display-is-pl

// See also http://www.spinics.net/lists/dri-devel/msg04479.html,
// which generates Xrandr RROutputChangeNotify (I think) events on
// monitor hotplug.  xf86-video-intel 2.13.0 probably has this.


#include <libudev.h>
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

int
main(int argc, char **argv)
{
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
        struct randrResources *prev = NULL;

        // Create a udev monitor
        struct udev *udev;
        struct udev_monitor *mon;
        if (!(udev = udev_new()) ||
            !(mon = udev_monitor_new_from_netlink(udev, "udev")) ||
            udev_monitor_filter_add_match_subsystem_devtype(mon, "drm", NULL) ||
            udev_monitor_enable_receiving(mon)) {
                fprintf(stderr, "Failed to create udev event monitor\n");
                exit(1);
        }

        // Respond to udev events
        while (1) {
                struct randrResources *now = getRandrResources();
                if (!prev || !randrResourcesEqual(prev, now)) {
                        printf("Resources differ\n");
                        system("xauto");
                } else
                        printf("Resources do not differ\n");
                if (prev)
                        freeRandrResources(prev);
                prev = now;

                struct udev_device *dev = udev_monitor_receive_device(mon);
                if (!dev) {
                        fprintf(stderr, "Failed to receive udev event\n");
                        exit(1);
                }
                printf("%s\n", udev_device_get_syspath(dev));
                udev_device_unref(dev);
        }

        // Clean up
        udev_monitor_unref(mon);
        udev_unref(udev);
        XCloseDisplay(dpy);
}

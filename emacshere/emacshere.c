#define _GNU_SOURCE /* get_current_dir_name */
#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <sys/utsname.h>

#include <pth.h>
#include <xcb/xcb.h>
#include <xcb/xcb_event.h>

#define DEBUG 0

void
panic(const char *str)
{
        fprintf(stderr, "%s\n", str);
        abort();
}

//////////////////////////////////////////////////////////////////
// XCB/Pth
//

struct xcb_task
{
        pth_uctx_t ctx;
        struct xcb_task *next, *prev;

        void (*start)(void*);
        void *arg;
        bool dead;
};

static struct xcb_task xcb_task_main;
static struct xcb_task *xcb_task_cur;

void xcb_wait(void);

static void
xcb_spawn_trampoline(void *opaque)
{
        xcb_task_cur->start(xcb_task_cur->arg);
        xcb_task_cur->dead = true;
        xcb_wait();
        panic("Dead thread executed");
}

void
xcb_spawn(void (*func)(void *), void *arg)
{
        // Set up main task if necessary
        if (!xcb_task_cur) {
                if (!pth_uctx_create(&xcb_task_main.ctx))
                        panic("main pth_uctx_create failed");
                if (!pth_uctx_make(xcb_task_main.ctx, NULL, 16*1024, NULL,
                                   (void(*)(void*))(-1), NULL, NULL))
                        panic("main pth_uctx_make failed");
                xcb_task_main.next = xcb_task_main.prev = &xcb_task_main;
                xcb_task_cur = &xcb_task_main;
        }

        // Allocate a task
        struct xcb_task *task = calloc(1, sizeof *task);
        if (!task)
                panic("failed to malloc task");

        task->start = func;
        task->arg = arg;
        if (!pth_uctx_create(&task->ctx))
                panic("pth_uctx_create failed");
        if (!pth_uctx_make(task->ctx, NULL, 16*1024, NULL,
                           xcb_spawn_trampoline, NULL, NULL))
                panic("pth_uctx_make failed");

        // Execute the child up to its first wait (or exit).  This is
        // good for efficiency, but also important to make the
        // children send their initial requests in order.  First,
        // schedule a little ring of just this task and the child.
        struct xcb_task *prev = xcb_task_cur->prev,
                *next = xcb_task_cur->next;
        xcb_task_cur->next = xcb_task_cur->prev = task;
        task->next = task->prev = xcb_task_cur;
        xcb_wait();
        // Then piece that ring back into the bigger task ring with
        // the added task (or tasks, if the child spawned, too) behind
        // this task.
        xcb_task_cur->next = next;
        xcb_task_cur->prev->prev = prev;
        prev->next = xcb_task_cur->prev;
}

void
xcb_wait(void)
{
        if (!xcb_task_cur)
                return;

        // Reap threads
        while (xcb_task_cur->next->dead) {
                struct xcb_task *reap = xcb_task_cur->next;
                reap->next->prev = reap->prev;
                reap->prev->next = reap->next;
                pth_uctx_destroy(reap->ctx);
                free(reap);
        }
        if (xcb_task_cur == xcb_task_cur->next)
                return;

        // XXX Testing synchronous execution
//        if (!xcb_task_cur->dead && xcb_task_cur != &xcb_task_main)
//                return;

        // Roll the task ring
        xcb_task_cur = xcb_task_cur->next;

        // Switch.  Has to be the last thing we do in case the task
        // we're switching into is a new thread.
        pth_uctx_switch(xcb_task_cur->prev->ctx, xcb_task_cur->ctx);
}

// Ah, the wonders of CPP.  We need a few levels to get it to expand
// __LINE__.
#define XCB_ASYNC __XCB_ASYNC(__LINE__)
#define __XCB_ASYNC(uniq) ____XCB_ASYNC(uniq)
#define ____XCB_ASYNC(uniq)                                 \
        auto void __xcb_async_thread##uniq(void *__opaque); \
        xcb_spawn(__xcb_async_thread##uniq, NULL);          \
        void __xcb_async_thread##uniq(void *__opaque)

//////////////////////////////////////////////////////////////////
// XCB util
//

static xcb_connection_t *conn;
static const xcb_setup_t *setup;
static xcb_screen_t *screen;

xcb_atom_t
__get_atom(const char *name, xcb_atom_t *store)
{
        if (!(*store)) {
                xcb_intern_atom_cookie_t ia =
                        xcb_intern_atom(conn, false, strlen(name), name);
                xcb_wait();
                xcb_intern_atom_reply_t *iar =
                        xcb_intern_atom_reply(conn, ia, NULL);
                *store = iar->atom;
                free(iar);
        }
        return *store;
}

#define ATOM(name) ({static xcb_atom_t __atom; __get_atom(name, &__atom);})

bool
has_property(xcb_window_t win, xcb_atom_t atom)
{
        xcb_get_property_cookie_t gp =
                xcb_get_property(conn, false, win, atom,
                                 XCB_GET_PROPERTY_TYPE_ANY, 0, 0);
        xcb_wait();
        xcb_get_property_reply_t *gpr = xcb_get_property_reply(conn, gp, NULL);
        bool res = gpr->type != 0;
        free(gpr);
        return res;
}

void *
get_property(xcb_window_t win, xcb_atom_t atom, xcb_atom_t type, int format, void *out, int max)
{
        xcb_get_property_cookie_t gp =
                xcb_get_property(conn, false, win, atom, type, 0, max);
        xcb_wait();
        xcb_get_property_reply_t *gpr = xcb_get_property_reply(conn, gp, NULL);
        void *res = NULL;
        if (gpr->type == type && gpr->format == format) {
                int len = xcb_get_property_value_length(gpr);
                if (out)
                        res = out;
                else
                        res = malloc(len);
                if (!res) {
                        perror("malloc");
                        abort();
                }
                memmove(res, xcb_get_property_value(gpr), len);
        }
        free(gpr);
        return res;
}

char *
string_property(xcb_window_t win, xcb_atom_t atom)
{
        return get_property(win, atom, XCB_ATOM_STRING, 8, NULL, 4096);
}

uint32_t
card32_property(xcb_window_t win, xcb_atom_t atom)
{
        uint32_t res = 0;
        get_property(win, atom, XCB_ATOM_CARDINAL, 32, &res, sizeof(res));
        return res;
}

xcb_atom_t
atom_property(xcb_window_t win, xcb_atom_t atom)
{
        xcb_atom_t res = 0;
        get_property(win, atom, XCB_ATOM_ATOM, 32, &res, sizeof(res));
        return res;
}

//////////////////////////////////////////////////////////////////
// Main
//

static bool found_some, found_visible;
static uint32_t best_user_time;
static xcb_window_t best_window;

void
consider_target(xcb_window_t win)
{
        // Just Emacs windows
        char *cls = string_property(win, XCB_ATOM_WM_CLASS);
        if (!(cls && strcmp(cls, "emacs") == 0)) {
                free(cls);
                return;
        }
        free(cls);
        found_some = true;

        // Consider only visible windows
        xcb_get_window_attributes_cookie_t wa =
                xcb_get_window_attributes(conn, win);
        xcb_wait();
        xcb_get_window_attributes_reply_t *war =
                xcb_get_window_attributes_reply (conn, wa, NULL);
        if (war->map_state != XCB_MAP_STATE_VIEWABLE) {
                free(war);
                return;
        }
        free(war);
        found_visible = true;

        // Get access time (XXX support _NET_WM_USER_TIME_WINDOW)
        uint32_t user_time = card32_property(win, ATOM("_NET_WM_USER_TIME"));
#if DEBUG
        printf("Visible %#x user_time %u\n", win, user_time);
#endif
        if (!best_user_time || user_time > best_user_time) {
                best_user_time = user_time;
                best_window = win;
        }
}

void
get_top_level_windows(xcb_window_t win)
{
        // Is win top-level?  According to the ICCCM, top-level
        // windows have a WM_STATE property.  See also
        // XmuClientWindow.
        if (has_property(win, ATOM("WM_STATE"))) {
                consider_target(win);
                return;
        }

        // Not a top-level window.  Get its children.
        xcb_query_tree_cookie_t qt = xcb_query_tree(conn, win);
        xcb_wait();
        xcb_query_tree_reply_t *qtr = xcb_query_tree_reply(conn, qt, NULL);

        xcb_window_t *children = xcb_query_tree_children(qtr);
        int nc = xcb_query_tree_children_length(qtr);
        for (int i = 0; i < nc; ++i) {
                void t(void *op)
                {
                        get_top_level_windows(*((xcb_window_t*)op));
                }
                xcb_spawn(t, &children[i]);
        }

        // Let the children get their windows before freeing the reply
        xcb_wait();
        free(qtr);
}

void
xcb_wait_and_check(xcb_void_cookie_t cookie, const char *info)
{
        xcb_wait();
        xcb_generic_error_t *err = xcb_request_check(conn, cookie);
        if (err) {
                fprintf(stderr, "X error %s: %s\n", info,
                        xcb_event_get_error_label(err->error_code));
                free(err);
                exit(1);
        }
}

void
do_dnd(xcb_window_t target, const char *paste)
{
        xcb_window_t source = xcb_generate_id(conn);
        xcb_void_cookie_t v;

        xcb_atom_t XdndAware, XdndSelection,
                XdndEnter, XdndPosition, XdndStatus, XdndDrop, XdndFinished,
                textUriList, XdndActionCopy;
        XCB_ASYNC {XdndAware = ATOM("XdndAware");}
        XCB_ASYNC {XdndSelection = ATOM("XdndSelection");}
        XCB_ASYNC {XdndEnter = ATOM("XdndEnter");}
        XCB_ASYNC {XdndPosition = ATOM("XdndPosition");}
        XCB_ASYNC {XdndStatus = ATOM("XdndStatus");}
        XCB_ASYNC {XdndDrop = ATOM("XdndDrop");}
        XCB_ASYNC {XdndFinished = ATOM("XdndFinished");}
        XCB_ASYNC {textUriList = ATOM("text/uri-list");}
        XCB_ASYNC {XdndActionCopy = ATOM("XdndActionCopy");}
        // The following requires knowing that all of the above do at
        // most one xcb_wait, as do the other synchronizing xcb_wait's.
        xcb_wait();

        // Check for XDND target support
        int version = atom_property(target, XdndAware);
        if (!version) {
                fprintf(stderr, "Target window does not support drag-and-drop\n");
                exit(1);
        }
        if (version > 5)
                version = 5;

        // Create XDND source window
        XCB_ASYNC {
                v = xcb_create_window_checked
                        (conn, 0, source, screen->root,
                         0, 0, 1, 1, 0, XCB_WINDOW_CLASS_INPUT_ONLY,
                         screen->root_visual, 0, NULL);
                xcb_wait_and_check(v, "creating DND source window");
        }
        XCB_ASYNC {
                v = xcb_set_selection_owner_checked
                        (conn, source, XdndSelection,
                         XCB_CURRENT_TIME);
                xcb_wait_and_check(v, "setting selection owner");
        }
        xcb_wait();

        // Send enter event
        XCB_ASYNC {
                xcb_client_message_event_t msg;
                memset(&msg, 0, sizeof msg);
                msg.response_type = XCB_CLIENT_MESSAGE;
                msg.window = target;
                msg.type = XdndEnter;
                msg.format = 32;
                msg.data.data32[0] = source;
                msg.data.data32[1] = version << 24;
                msg.data.data32[2] = textUriList;
                v = xcb_send_event_checked(conn, 0, target, 0, (char*)&msg);
                xcb_wait_and_check(v, "Sending XdndEnter event");
        }
        // Send position event
        XCB_ASYNC {
                xcb_client_message_event_t msg;
                memset(&msg, 0, sizeof msg);
                msg.response_type = XCB_CLIENT_MESSAGE;
                msg.window = target;
                msg.type = XdndPosition;
                msg.format = 32;
                msg.data.data32[0] = source;
                msg.data.data32[1] = 0;
                // Emacs blows up if you send position (0,0)
                msg.data.data32[2] = (1 << 16) | 1;
                msg.data.data32[3] = XCB_CURRENT_TIME;
                msg.data.data32[4] = XdndActionCopy;
                v = xcb_send_event_checked(conn, 0, target, 0, (char*)&msg);
                xcb_wait_and_check(v, "Sending XdndPosition event");
        }
        xcb_wait();

        // Event loop
        while (1) {
                // XXX Timeout
                xcb_generic_event_t *ev = xcb_wait_for_event(conn);
                xcb_client_message_event_t *cev =
                        (xcb_client_message_event_t*)ev;
                xcb_selection_request_event_t *sev =
                        (xcb_selection_request_event_t*)ev;
                int typ = ev->response_type & XCB_EVENT_RESPONSE_TYPE_MASK;

                if (typ == XCB_CLIENT_MESSAGE &&
                    cev->type == XdndStatus) {
                        if (!(cev->data.data32[1] & 1)) {
                                // XXX Send XdndLeave
                                fprintf(stderr, "Target not accepting drag-and-drop\n");
                                exit(1);
                        }
                        // Send drop
                        xcb_client_message_event_t msg;
                        memset(&msg, 0, sizeof msg);
                        msg.response_type = XCB_CLIENT_MESSAGE;
                        msg.window = target;
                        msg.type = XdndDrop;
                        msg.format = 32;
                        msg.data.data32[0] = source;
                        msg.data.data32[1] = 0;
                        msg.data.data32[2] = XCB_CURRENT_TIME;
                        v = xcb_send_event_checked(conn, 0, target, 0, (char*)&msg);
                        xcb_wait_and_check(v, "Sending XdndDrop event");

                        uint32_t conf[] = {XCB_STACK_MODE_ABOVE};
                        xcb_configure_window(conn, target,
                                             XCB_CONFIG_WINDOW_STACK_MODE,
                                             conf);
                        xcb_set_input_focus(conn, XCB_INPUT_FOCUS_POINTER_ROOT,
                                            target, XCB_CURRENT_TIME);
                        // XXX
                        xcb_warp_pointer(conn, XCB_WINDOW_NONE, screen->root,
                                         0, 0, 0, 0, 0, 0);
                        xcb_warp_pointer(conn, XCB_WINDOW_NONE, target,
                                         0, 0, 0, 0, 10, 10);
                } else if (typ == XCB_SELECTION_REQUEST) {
                        if (sev->selection != XdndSelection) {
                                fprintf(stderr,
                                        "Request for unknown selection %d\n",
                                        sev->selection);
                                continue;
                        }
                        if (sev->target != textUriList) {
                                fprintf(stderr,
                                        "Request for unknown target type %d\n",
                                        sev->target);
                                continue;
                        }
                        if (sev->property == 0) {
                                fprintf(stderr,
                                        "Old-style selection protocol not supported\n");
                                exit(1);
                        }

                        // Expand paste to an file-DND-friendly URI.
                        // This is subtler than it looks.  We pass a
                        // "local" path, even if we actually want a
                        // "remote" (say, Tramp) path.  Remote paths
                        // are opened using url-handler-mode, which
                        // will use FTP.  So, to get Emacs' usual path
                        // handling, we pass a "local" path, so that
                        // dnd-open-file just strips the file:// part,
                        // and then invokes the usual
                        // file-name-handler-alist mechanism.
                        char *uri;
                        asprintf(&uri, "file://%s", paste);
                        // Respond with selection contents
                        XCB_ASYNC {
                                v = xcb_change_property_checked
                                        (conn, XCB_PROP_MODE_REPLACE, sev->requestor,
                                         sev->property, XCB_ATOM_STRING, 8,
                                         strlen(uri), uri);
                                xcb_wait_and_check(v, "returning selection");
                        }
                        XCB_ASYNC {
                                xcb_selection_notify_event_t smsg = {};
                                smsg.response_type = XCB_SELECTION_NOTIFY;
                                smsg.time = sev->time;
                                smsg.requestor = sev->requestor;
                                smsg.selection = sev->selection;
                                smsg.target = sev->target;
                                smsg.property = sev->property;
                                v = xcb_send_event_checked(conn, 0, target, 0, (char*)&smsg);
                                xcb_wait_and_check(v, "sending selection notify");
                        }
                        xcb_wait();
                        free(uri);
                } else if (typ == XCB_CLIENT_MESSAGE &&
                           cev->type == XdndFinished) {
                        break;
                } else {
                        fprintf(stderr,
                                "Received unexpected event type %d\n",
                                ev->response_type);
                }
        }
}

char *
parse_args(int argc, char **argv)
{
        char *path;
        char *cwd = get_current_dir_name();
        if (argc == 1) {
                path = strdup(cwd);
        } else if (argc == 2) {
                if (argv[1][0] == '/')
                        path = strdup(argv[1]);
                else if (strcmp(argv[1], ".") == 0)
                        path = strdup(cwd);
                else
                        asprintf(&path, "%s/%s", cwd, argv[1]);
        } else {
                printf("usage: %s [path]", argv[0]);
                exit(2);
        }
        free(cwd);

        // X-forwarded paths
        // XXX Make this configurable
        // XXX Allow tramp, or some path template, or even external script
        // XXX Detect this by comparing our hostname to the found
        // window's hostname
        if (getenv("SSH_CONNECTION")) {
                char *oldpath = path;
                // XXX This is what GNU hostname -s does.
                struct utsname un;
                if (uname(&un) < 0)
                        panic("uname");
                // XXX Hmm.  Would be nice to explicitly allow
                // home-relative paths for cases like this.
                asprintf(&path, "/home/amthrax/ssh/%s%s", un.nodename, oldpath);
                free(oldpath);
        }

        return path;
}

int
main(int argc, char **argv)
{
        char *path = parse_args(argc, argv);

        pth_init();

        conn = xcb_connect(NULL, NULL);
        if (xcb_connection_has_error(conn)) {
                fprintf(stderr, "Error opening display\n");
                exit(1);
        }
        setup = xcb_get_setup(conn);
        screen = xcb_setup_roots_iterator(setup).data;

        get_top_level_windows(screen->root);
        int rounds = 0;
        while (xcb_task_main.next != &xcb_task_main) {
                rounds++;
                xcb_wait();
        }
#if DEBUG
        printf("Window lookup took %d rounds\n", rounds);
#endif

        if (!found_some) {
                fprintf(stderr, "No Emacs windows found\n");
                exit(1);
        } else if (!found_visible) {
                fprintf(stderr, "No Emacs windows are visible\n");
                exit(1);
        }

#if DEBUG
        printf("Best %#x\n", best_window);
#endif

        do_dnd(best_window, path);

        xcb_disconnect(conn);
        free(path);
        return 0;
}


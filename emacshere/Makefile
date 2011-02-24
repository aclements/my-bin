CFLAGS += -O2 -g

ifeq ($(shell which pth-config),)
$(error GNU Pth not found.  Please install libpth-dev.)
endif
CFLAGS += $(shell pth-config --cflags)
LDFLAGS += $(shell pth-config --ldflags --libs)

ifeq ($(shell pkg-config --exists xcb || echo no),no)
$(error libxcb not found.  Please install libxcb1-dev.)
endif
ifeq ($(shell pkg-config --exists xcb-event || echo no),no)
$(error libxcb-event not found.  Please install libxcb-event1-dev.)
endif
CFLAGS += $(shell pkg-config --cflags xcb xcb-event)
LDFLAGS += $(shell pkg-config --libs xcb xcb-event)

emacshere: emacshere.o

emacshere.o: CFLAGS += -std=gnu99

clean:
	rm -f emacshere emacshere.o
.PHONY: clean

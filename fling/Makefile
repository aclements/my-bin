CFLAGS += -O2 -g

ifeq ($(shell which pth-config),)
MISSING = 1
$(warning GNU Pth not found.  Please install libpth-dev)
endif
ifeq ($(shell pkg-config --exists xcb || echo no),no)
MISSING = 1
$(warning libxcb not found.  Please install libxcb1-dev)
endif
ifeq ($(shell pkg-config --exists xcb-event || echo no),no)
MISSING = 1
$(warning libxcb-event not found.  Please install libxcb-event1-dev)
endif

ifeq (${MISSING},1)
$(error Required libraries are missing)
endif

CFLAGS += $(shell pth-config --cflags)
LDFLAGS += $(shell pth-config --ldflags --libs)

CFLAGS += $(shell pkg-config --cflags xcb xcb-event)
LDFLAGS += $(shell pkg-config --libs xcb xcb-event)

fling: fling.o

fling.o: CFLAGS += -std=gnu99

clean:
	rm -f fling fling.o
.PHONY: clean

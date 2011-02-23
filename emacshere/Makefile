CFLAGS += -O2 -g

CFLAGS += $(shell pth-config --cflags)
LDFLAGS += $(shell pth-config --ldflags --libs)

LDFLAGS += -lxcb -lxcb-event

emacshere: emacshere.o

emacshere.o: CFLAGS += -std=gnu99

clean:
	rm -f emacshere emacshere.o
.PHONY: clean

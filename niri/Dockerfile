# Build niri and xwayland-satellite for Debian testing.
#
# Debian has no package for either, and building them on the host would mean
# installing ~16 -dev packages there permanently. Doing it in a container keeps
# the host clean and makes the build reproducible from a checkout.
#
# The build happens inside the image rather than into a bind mount, so the
# expensive layers cache across runs and nothing writes to the host as root.
# ./build extracts /out afterwards.

FROM debian:testing

ARG NIRI_VERSION=v26.04
ARG XWS_VERSION=v0.8.2

# niri's dependencies come from the wiki's Building section, translated from its
# Ubuntu/Fedora lists to Debian names. libxcb1-dev and libxcb-cursor-dev are for
# xwayland-satellite, which documents its own (xcb, xcb-util-cursor, clang).
# Debian's rustc is 1.95, comfortably past niri's 1.85 minimum, so there is no
# need to pull in rustup.
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates git make pkg-config gcc clang cargo rustc golang-go \
      libudev-dev libgbm-dev libxkbcommon-dev libegl1-mesa-dev libwayland-dev \
      libinput-dev libdbus-1-dev libsystemd-dev libseat-dev libpipewire-0.3-dev \
      libpango1.0-dev libcairo2-dev libdisplay-info-dev \
      libxcb1-dev libxcb-cursor-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src

# --locked builds exactly the dependency set the release tag was cut with. If it
# ever conflicts with Debian's Rust, dropping the flag is the fallback.
#
# Do NOT add --all-features. The wiki warns that some features are for
# development only, including one that collects profiling data into a buffer
# that grows until the machine runs out of memory.
RUN git clone --depth=1 --branch "$NIRI_VERSION" \
        https://github.com/niri-wm/niri.git niri \
    && cd niri && cargo build --release --locked

RUN git clone --depth=1 --branch "$XWS_VERSION" \
        https://github.com/Supreeeme/xwayland-satellite.git xws \
    && cd xws && cargo build --release --locked

# Stage everything configure-niri installs. default-config.kdl ships here too so
# it is always the one matching this niri binary; the config includes it.
RUN mkdir -p /out \
    && cp niri/target/release/niri /out/ \
    && cp xws/target/release/xwayland-satellite /out/ \
    && cp niri/resources/niri-session \
          niri/resources/niri.desktop \
          niri/resources/niri-portals.conf \
          niri/resources/niri.service \
          niri/resources/niri-shutdown.target \
          niri/resources/default-config.kdl \
          /out/

#
# DankMaterialShell and the tools it needs that Debian does not package.
# DMS replaces waybar, fuzzel, mako, swaybg, swaylock, swayidle and the polkit
# agent with a single shell. quickshell itself comes from Debian; the QML tree
# staged below is what quickshell runs, so it is copied rather than compiled.
#

ARG MATUGEN_VERSION=v4.2.0
ARG DGOP_VERSION=v0.2.3
ARG DANKSEARCH_VERSION=v0.3.2
ARG DMS_VERSION=v1.5.3

# Debian's golang is 1.26.7, which satisfies DMS's "go 1.26.1". Pin the
# toolchain so the build never reaches out for a different one.
ENV GOTOOLCHAIN=local

# matugen derives the palette DMS themes everything from, out of the wallpaper.
RUN git clone --depth=1 --branch "$MATUGEN_VERSION" \
        https://github.com/InioX/matugen.git matugen \
    && cd matugen && cargo build --release --locked \
    && cp target/release/matugen /out/

RUN git clone --depth=1 --branch "$DGOP_VERSION" \
        https://github.com/AvengeMedia/dgop.git dgop \
    && cd dgop && make build && cp bin/dgop /out/

RUN git clone --depth=1 --branch "$DANKSEARCH_VERSION" \
        https://github.com/AvengeMedia/danksearch.git danksearch \
    && cd danksearch && make build && cp bin/dsearch /out/

# Only the Go CLI is compiled. The QML tree and assets are staged for
# configure-niri to place, rather than using DMS's own make install, so that
# everything lands under $HOME the way the rest of this setup does.
RUN git clone --depth=1 --branch "$DMS_VERSION" \
        https://github.com/AvengeMedia/DankMaterialShell.git dms \
    && cd dms && make -C core build \
    && cp core/bin/dms /out/ \
    && cp assets/systemd/dms.service /out/ \
    && cp assets/danklogo.svg /out/ \
    && mkdir -p /out/dms-desktop && cp assets/*.desktop /out/dms-desktop/ \
    && cp -r quickshell /out/dms-shell

# Which Debian packages provide the shared libraries these binaries need. A
# hand-written list cannot work: the container has every -dev package, so a
# library missing on the host stays invisible until something fails to start.
# The Go binaries are static, so ldd reports nothing for them.
RUN { for b in /out/niri /out/xwayland-satellite /out/matugen \
                /out/dms /out/dgop /out/dsearch; do ldd "$b" || true; done; } \
    | sed -n 's/.*=> \(\/[^ ]*\).*/\1/p' | sort -u \
    | xargs dpkg -S | cut -d: -f1 | sort -u > /out/runtime-deps.txt

# Record what was actually built, so configure-niri can tell a current out/ from
# one left over from an older pin. RUN does no Docker-side substitution; the
# shell expands these, reading the environment variables the ARGs above create.
RUN printf 'NIRI_VERSION=%s\nXWS_VERSION=%s\nMATUGEN_VERSION=%s\nDGOP_VERSION=%s\nDANKSEARCH_VERSION=%s\nDMS_VERSION=%s\n' \
      "$NIRI_VERSION" "$XWS_VERSION" "$MATUGEN_VERSION" "$DGOP_VERSION" \
      "$DANKSEARCH_VERSION" "$DMS_VERSION" \
      > /out/build-info.txt

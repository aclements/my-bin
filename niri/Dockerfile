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
      ca-certificates git pkg-config gcc clang cargo rustc \
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

# Record which Debian packages provide the shared libraries these binaries need,
# so configure-niri can ensure the host has them. This cannot be a hand-written
# list: the build container has every -dev package installed, so a runtime
# library that is missing on the host stays invisible until niri fails to start.
# No shell variables here -- Dockerfile would substitute them as build args.
RUN { ldd /out/niri; ldd /out/xwayland-satellite; } \
    | sed -n 's/.*=> \(\/[^ ]*\).*/\1/p' | sort -u \
    | xargs dpkg -S | cut -d: -f1 | sort -u > /out/runtime-deps.txt

# Record what was actually built, so configure-niri can tell a current out/ from
# one left over from an older pin. Here the substitution is Docker's, and
# deliberate: these are the build args used above.
RUN printf 'NIRI_VERSION=%s\nXWS_VERSION=%s\n' "$NIRI_VERSION" "$XWS_VERSION" \
      > /out/build-info.txt

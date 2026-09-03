"""Shared plumbing for the configure-* desktop setup scripts.

These scripts all follow the same shape: a list of "category" functions named
set_<category>, driven by --check (dry run, the default), --apply, or --list.
This module provides that driver plus the GSettings helpers, so configure-gnome
and configure-niri can share them.

Only Gio/GLib are imported here. Anything needing Gtk, Gdk, or dbus belongs in
the individual scripts.
"""

import argparse
import os.path
import subprocess

from gi.repository import Gio, GLib

# "check" (dry run), "apply", or "list". Set by main().
mode = "check"

def main(cats, add_args=None, post_parse=None):
    """Run the category functions selected on the command line.

    cats is a list of functions named set_<category>. add_args, if given, is
    called with the ArgumentParser to register script-specific options;
    post_parse, if given, is called with the parsed arguments.
    """
    parser = argparse.ArgumentParser()
    action = parser.add_mutually_exclusive_group()
    action.add_argument("--check", action="store_const", dest="mode", const="check",
                        help="report what would change (default)")
    action.add_argument("--apply", action="store_const", dest="mode", const="apply",
                        help="make the changes")
    action.add_argument("--list", action="store_const", dest="mode", const="list",
                        help="report every managed setting, changed or not")
    parser.set_defaults(mode="check")
    if add_args is not None:
        add_args(parser)
    cat_names = ["all"] + [cat.__name__[4:] for cat in cats]
    parser.add_argument("categories", nargs="*", choices=cat_names)
    args = parser.parse_args()

    global mode
    mode = args.mode
    if post_parse is not None:
        post_parse(args)

    do_cats = set(args.categories)
    if "all" in do_cats or len(do_cats) == 0:
        do_cats = set(cat_names[1:])
    for cat in cats:
        if cat.__name__[4:] in do_cats:
            cat()

def logDo(fmt):
    """Report an action. Returns True if it should actually be performed."""
    if mode == "apply":
        print(fmt)
        return True
    else:
        print(f"{fmt} (dry run)")
        return False

def checkApply(what):
    """Refuse to change anything outside --apply.

    logDo already gates every call site, but that only holds as long as each one
    remembers to ask. Primitives that actually modify the system call this too,
    so a missed guard is a crash during --check rather than a surprise change.
    """
    if mode != "apply":
        raise AssertionError(f"would modify the system in {mode} mode: {what}")

def run(cmd):
    """Run a command that changes the system. Refuses outside --apply."""
    checkApply(" ".join(cmd))
    subprocess.run(cmd, check=True)

def query(cmd):
    """Run a command that only reports state. Safe during --check."""
    return subprocess.run(cmd, capture_output=True, text=True)

def sudo(cmd):
    """Wrap cmd in sudo so it prompts every time and leaves no credit behind.

    Per sudo(8), -k with a command makes sudo ignore any cached credentials --
    so this always prompts -- and "will not update the user's cached
    credentials", so running this script never leaves a window in which
    something else can use sudo unprompted. -N would skip the update but still
    accept an existing cache, which is not what we want.

    sudoers keeps a separate credential record per terminal by default, so this
    does not disturb sudo in your other terminals either.
    """
    return ["sudo", "-k", "--"] + cmd

#
# GSettings
#

def get_settings(schemaID, path=None, extension=None):
    # Useful example: https://docs.gtk.org/gio/struct.SettingsSchema.html?q=
    schema = lookup_schema(schemaID, extension)
    if schema is None:
        raise ValueError(f"unknown schema {schemaID}")
    return Gio.Settings.new_full(schema, None, path)

def lookup_schema(schemaID, extension=None):
    """Return the GSettingsSchema for schemaID, or None if it isn't installed."""
    if extension is not None:
        # Combine home directory
        dir = os.path.join(os.path.expanduser("~"), ".local/share/gnome-shell/extensions", extension, "schemas")
        sss = Gio.SettingsSchemaSource.new_from_directory(dir, None, True)
    else:
        # In this case we could just use Gio.Settings.new, but we do this more
        # explicit approach to unify the path more with extensions.
        sss = Gio.SettingsSchemaSource.get_default()
    return sss.lookup(schemaID, True)

def has_schema(schemaID, extension=None):
    """Report whether schemaID is installed, so callers can skip settings for
    software that isn't present on this machine."""
    return lookup_schema(schemaID, extension) is not None

def get(schemaID, key, path=None, extension=None):
    s = get_settings(schemaID, path, extension)
    return s.get_value(key)

def gset(schemaID, *args, path=None, extension=None):
    if len(args) % 2 != 0:
        raise ValueError("expected an even number of arguments")

    s = get_settings(schemaID, path, extension)
    for i in range(0, len(args), 2):
        k, v = args[i], args[i + 1]
        if v is None:
            # Reset to default
            gv = s.get_default_value(k)
        else:
            gv = python_to_gvariant(v)
        have = s.get_value(k)
        gv = retype(gv, v, have.get_type_string())
        if have != gv or mode == "list":
            print(f"{schemaID}{':'+path if path else ''} {k}")
            print(f"  Default: {s.get_default_value(k)}")
            print(f"  Current: {have}")
            print(f"  New:     {gv}")
        if have != gv and mode == "apply":
            checkApply(f"{schemaID} {k}")
            if v is None:
                s.reset(k)
            else:
                s.set_value(k, gv)

# GSettings distinguishes integer widths that Python does not. python_to_gvariant
# can only guess from the Python type, and guesses int32; a key declared uint32,
# such as Ptyxis's default-columns, rejects that. gset consults the schema and
# rebuilds the value at the declared width.
_NUMERIC_VARIANTS = {
    "y": GLib.Variant.new_byte,
    "n": GLib.Variant.new_int16,
    "q": GLib.Variant.new_uint16,
    "i": GLib.Variant.new_int32,
    "u": GLib.Variant.new_uint32,
    "x": GLib.Variant.new_int64,
    "t": GLib.Variant.new_uint64,
    "d": GLib.Variant.new_double,
}

def retype(gv, value, want):
    """Rebuild gv as the type string want, when that is a numeric width we can
    convert to. Returns gv unchanged if not."""
    if gv.get_type_string() == want or want not in _NUMERIC_VARIANTS:
        return gv
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        return gv
    return _NUMERIC_VARIANTS[want](
        float(value) if want == "d" else int(value))

def python_to_gvariant(value):
    """Converts a Python value to a GVariant."""
    if isinstance(value, str):
        return GLib.Variant.new_string(value)
    elif isinstance(value, bool):
        return GLib.Variant.new_boolean(value)
    elif isinstance(value, int):
        return GLib.Variant.new_int32(value)  # "i" code
    elif isinstance(value, float):
        return GLib.Variant.new_double(value)
    elif isinstance(value, list):
        if all(isinstance(item, str) for item in value):
            return GLib.Variant("as", value)
        elif all(isinstance(item, int) for item in value):
            return GLib.Variant("ai", value)
        else:
            raise TypeError("Unsupported list element type")
    elif isinstance(value, dict):
        gvariant_dict_entries = {}
        for key, val in value.items():
            if not isinstance(key, str):
                raise TypeError("Dictionary keys must be strings")
            gvariant_dict_entries[key] = python_to_gvariant(val)
        # I'm not sure what happens if something is expecting a more specific
        # type, like "a{si}".
        return GLib.Variant("a{sv}", gvariant_dict_entries)
    elif isinstance(value, GLib.Variant):
        return value
    else:
        raise TypeError(f"Unsupported type: {type(value)}")

#
# Settings shared between window managers
#

def set_common():
    """Desktop settings that apply regardless of the window manager.

    Under niri these reach GTK and Flatpak apps through the Settings interface
    of xdg-desktop-portal-{gtk,gnome}, so they're worth setting there too.
    """
    # Dark mode. The niri wiki calls this out specifically: with
    # xdg-desktop-portal-gnome, Flatpak apps read the GNOME UI settings.
    gset("org.gnome.desktop.interface", "color-scheme", "prefer-dark")

    # Default monospace font
    gset("org.gnome.desktop.interface", "monospace-font-name", "Source Code Pro 9")

    # Ptyxis is the terminal x-terminal-emulator prefers.
    if not has_schema("org.gnome.Ptyxis"):
        print("Ptyxis not installed; skipping its settings")
        return
    ptyxisUUID = get("org.gnome.Ptyxis", "default-profile-uuid").get_string()
    if not ptyxisUUID:
        # Ptyxis hasn't been run yet, so it has no profile to configure. The
        # settings path would be .../Profiles//, which GSettings rejects.
        print("Ptyxis has no default profile yet; run it once, then re-run this")
        return
    gset(f"org.gnome.Ptyxis.Profile", "opacity", 0.95,
         path=f"/org/gnome/Ptyxis/Profiles/{ptyxisUUID}/")

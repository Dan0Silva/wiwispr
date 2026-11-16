#!/usr/bin/env python3
"""
wiwispr Floating Microphone & Sound-Wave Overlay
- Always on top of all windows (WindowType.POPUP + keep_above + X11 override-redirect)
- Never steals keyboard or mouse focus (accept_focus=False, focus_on_map=False, input_shape_combine_region empty)
- No window decorations (no close/minimize/maximize buttons, no borders, transparent background)
- Smooth 60 FPS Telegram/WhatsApp-style voice waveform animation + microphone icon
"""

import os
import sys
import math
import threading

# Force X11 backend (XWayland on KDE Wayland) so WindowType.POPUP (override_redirect)
# is guaranteed to stay on top of every window without ever stealing keyboard focus.
os.environ["GDK_BACKEND"] = "x11"

import gi
gi.require_version("Gtk", "3.0")
gi.require_version("Gdk", "3.0")
from gi.repository import Gtk, Gdk, GLib
import cairo


class OverlayHUD(Gtk.Window):
    WIDTH = 220
    HEIGHT = 54
    NUM_BARS = 14

    def __init__(self):
        # Gtk.WindowType.POPUP creates an X11 override-redirect window:
        # 1. Completely bypasses window manager titlebars/buttons
        # 2. Renders on top of all normal/fullscreen windows
        # 3. Never takes keyboard focus away from the active app!
        super().__init__(type=Gtk.WindowType.POPUP)

        self.set_title("wiwispr-overlay")
        self.set_decorated(False)
        self.set_resizable(False)
        self.set_keep_above(True)
        self.set_accept_focus(False)
        self.set_focus_on_map(False)
        self.set_can_focus(False)
        self.set_skip_taskbar_hint(True)
        self.set_skip_pager_hint(True)
        self.set_type_hint(Gdk.WindowTypeHint.NOTIFICATION)
        self.set_default_size(self.WIDTH, self.HEIGHT)
        self.set_size_request(self.WIDTH, self.HEIGHT)

        # Enable RGBA transparency
        screen = self.get_screen()
        visual = screen.get_rgba_visual()
        if visual is not None and screen.is_composited():
            self.set_visual(visual)
        self.set_app_paintable(True)

        self.current_level = 0.05
        self.target_level = 0.05
        self.speaking = False
        self.state = "LISTENING"  # "LISTENING" or "TRANSCRIBING"
        self.phase = 0.0
        self.bar_heights = [0.15] * self.NUM_BARS

        self.drawing_area = Gtk.DrawingArea()
        self.drawing_area.connect("draw", self.on_draw)
        self.add(self.drawing_area)

        self.connect("realize", self.on_realize)
        self.position_top_center()

        # 60 FPS animation timer (~16ms)
        GLib.timeout_add(16, self.on_tick)

    def position_top_center(self):
        display = Gdk.Display.get_default()
        if display is None:
            return
        monitor = display.get_primary_monitor() or display.get_monitor(0)
        if monitor is None:
            return
        geom = monitor.get_geometry()
        x = geom.x + (geom.width - self.WIDTH) // 2
        y = geom.y + 42  # 42px from the top edge of the screen
        self.move(x, y)

    def on_realize(self, widget):
        gdk_win = self.get_window()
        if gdk_win is not None:
            gdk_win.set_override_redirect(True)
            gdk_win.set_accept_focus(False)
            gdk_win.set_focus_on_map(False)
            gdk_win.set_keep_above(True)
            # Make window click-through so mouse clicks pass right through it
            empty_region = cairo.Region(cairo.RectangleInt(0, 0, 0, 0))
            gdk_win.input_shape_combine_region(empty_region, 0, 0)

    def on_tick(self):
        self.phase += 0.16
        # Smooth interpolation toward target audio level
        if self.target_level > self.current_level:
            self.current_level = self.current_level * 0.55 + self.target_level * 0.45
        else:
            self.current_level = self.current_level * 0.82 + self.target_level * 0.18

        # Update each sound-wave bar height (WhatsApp/Telegram voice note style)
        for i in range(self.NUM_BARS):
            center_dist = abs(i - (self.NUM_BARS - 1) / 2.0) / ((self.NUM_BARS - 1) / 2.0)
            envelope = math.cos(center_dist * math.pi * 0.45)

            if self.state == "TRANSCRIBING":
                # Traveling wave animation during transcription
                wave = 0.45 + 0.40 * math.sin(self.phase * 1.8 - i * 0.45)
                self.bar_heights[i] = max(0.16, wave * envelope)
            else:
                # Organic multi-frequency oscillation modulated by real microphone volume
                osc = (
                    0.50 * math.sin(self.phase * 1.4 + i * 0.65)
                    + 0.35 * math.cos(self.phase * 2.3 - i * 0.9)
                    + 0.15 * math.sin(self.phase * 3.1 + i * 1.3)
                )
                idle_ripple = 0.12 + 0.05 * math.sin(self.phase * 0.9 + i * 0.5)
                voice_amp = self.current_level * (0.65 + 0.35 * abs(osc)) * envelope
                target_h = max(idle_ripple, min(1.0, idle_ripple + voice_amp * 1.15))
                self.bar_heights[i] = self.bar_heights[i] * 0.45 + target_h * 0.55

        self.drawing_area.queue_draw()
        return True

    def on_draw(self, widget, cr):
        w = self.WIDTH
        h = self.HEIGHT

        # Clear surface to transparent
        cr.save()
        cr.set_operator(cairo.OPERATOR_SOURCE)
        cr.set_source_rgba(0, 0, 0, 0)
        cr.paint()
        cr.restore()

        # Draw sleek dark pill background with subtle border
        pad = 3
        pill_w = w - pad * 2
        pill_h = h - pad * 2
        radius = pill_h / 2.0

        self.draw_rounded_rect(cr, pad, pad, pill_w, pill_h, radius)
        cr.set_source_rgba(0.08, 0.09, 0.12, 0.92)
        cr.fill_preserve()

        # Subtle glowing border (emerald/cyan when speaking, soft blue when transcribing)
        if self.state == "TRANSCRIBING":
            cr.set_source_rgba(0.38, 0.65, 0.98, 0.75)
        elif self.speaking or self.current_level > 0.12:
            cr.set_source_rgba(0.18, 0.85, 0.55, 0.70)
        else:
            cr.set_source_rgba(1.0, 1.0, 1.0, 0.18)
        cr.set_line_width(1.4)
        cr.stroke()

        # Left indicator: Microphone icon inside a glowing circle
        cx = pad + 26
        cy = h / 2.0

        # Subtle pulsing halo behind mic icon when voice is active
        pulse_r = 14.0 + min(5.0, self.current_level * 6.0)
        cr.arc(cx, cy, pulse_r, 0, 2 * math.pi)
        if self.state == "TRANSCRIBING":
            cr.set_source_rgba(0.38, 0.65, 0.98, 0.22)
        else:
            cr.set_source_rgba(0.16, 0.82, 0.52, 0.18 + min(0.25, self.current_level * 0.3))
        cr.fill()

        # Solid inner circle
        cr.arc(cx, cy, 13.0, 0, 2 * math.pi)
        if self.state == "TRANSCRIBING":
            cr.set_source_rgba(0.23, 0.51, 0.96, 0.95)
        else:
            cr.set_source_rgba(0.12, 0.76, 0.46, 0.95)
        cr.fill()

        # Draw clean vector microphone icon in white
        self.draw_mic_icon(cr, cx, cy)

        # Draw sound wave bars on the right side of the microphone icon
        bars_start_x = cx + 24
        bars_end_x = w - pad - 18
        total_bars_w = bars_end_x - bars_start_x
        bar_w = 3.6
        gap = (total_bars_w - (self.NUM_BARS * bar_w)) / max(1, self.NUM_BARS - 1)
        max_bar_h = pill_h - 16.0

        for i, norm_h in enumerate(self.bar_heights):
            bh = max(4.0, norm_h * max_bar_h)
            bx = bars_start_x + i * (bar_w + gap)
            by = cy - bh / 2.0

            self.draw_rounded_rect(cr, bx, by, bar_w, bh, bar_w / 2.0)
            if self.state == "TRANSCRIBING":
                alpha = 0.65 + 0.35 * norm_h
                cr.set_source_rgba(0.55, 0.78, 1.0, alpha)
            elif self.speaking or self.current_level > 0.10:
                alpha = 0.70 + 0.30 * norm_h
                cr.set_source_rgba(0.25, 0.92, 0.62, alpha)
            else:
                cr.set_source_rgba(0.82, 0.85, 0.90, 0.55)
            cr.fill()

        return False

    @staticmethod
    def draw_mic_icon(cr, cx, cy):
        cr.save()
        cr.set_source_rgba(1.0, 1.0, 1.0, 0.98)

        # Microphone capsule (rounded rectangle)
        cap_w = 5.6
        cap_h = 9.6
        OverlayHUD.draw_rounded_rect(cr, cx - cap_w / 2.0, cy - 6.8, cap_w, cap_h, cap_w / 2.0)
        cr.fill()

        # U-shaped cradle around capsule
        cr.set_line_width(1.6)
        cr.set_line_cap(cairo.LINE_CAP_ROUND)
        cr.arc(cx, cy - 1.5, 5.2, 0.05 * math.pi, 0.95 * math.pi)
        cr.stroke()

        # Stem + base
        cr.move_to(cx, cy + 3.8)
        cr.line_to(cx, cy + 6.8)
        cr.stroke()

        cr.move_to(cx - 3.2, cy + 6.8)
        cr.line_to(cx + 3.2, cy + 6.8)
        cr.stroke()
        cr.restore()

    @staticmethod
    def draw_rounded_rect(cr, x, y, width, height, radius):
        r = min(radius, width / 2.0, height / 2.0)
        cr.new_sub_path()
        cr.arc(x + width - r, y + r, r, -math.pi / 2, 0)
        cr.arc(x + width - r, y + height - r, r, 0, math.pi / 2)
        cr.arc(x + r, y + height - r, r, math.pi / 2, math.pi)
        cr.arc(x + r, y + r, r, math.pi, 3 * math.pi / 2)
        cr.close_path()


def stdin_reader(hud):
    """Reads real-time commands from the Go parent process via stdin:
    - LEVEL <0.0..1.0> <0|1>
    - STATE TRANSCRIBING
    - QUIT
    """
    try:
        for line in sys.stdin:
            line = line.strip()
            if not line:
                continue
            if line == "QUIT":
                GLib.idle_add(Gtk.main_quit)
                return
            if line.startswith("STATE "):
                st = line.split(" ", 1)[1].strip()
                def update_st(s=st):
                    hud.state = s
                    return False
                GLib.idle_add(update_st)
            elif line.startswith("LEVEL "):
                parts = line.split()
                if len(parts) >= 2:
                    try:
                        val = float(parts[1])
                        spk = len(parts) >= 3 and parts[2] == "1"
                        def update_lvl(v=val, s=spk):
                            hud.target_level = max(0.02, min(1.0, v))
                            hud.speaking = s
                            return False
                        GLib.idle_add(update_lvl)
                    except ValueError:
                        pass
    except Exception:
        pass
    finally:
        # When Go closes stdin (or exits), immediately close the overlay
        GLib.idle_add(Gtk.main_quit)


def main():
    hud = OverlayHUD()
    hud.show_all()

    t = threading.Thread(target=stdin_reader, args=(hud,), daemon=True)
    t.start()

    try:
        Gtk.main()
    except KeyboardInterrupt:
        pass


if __name__ == "__main__":
    main()

#!/usr/bin/env python3
"""Print the hanging logo's alignment numbers (Exh-bv8l).

Parses internal/api/logo-hanging.svg and reports the x of the frame centre,
nail and wire apex, and where the wire's legs land relative to the frame's
outer top edge. Exits non-zero if the axis or the leg symmetry is off.
"""
import re, sys, pathlib

svg = pathlib.Path(__file__).resolve().parent.parent.joinpath("internal/api/logo-hanging.svg").read_text()

frame = re.search(r'<rect style="fill:#ffffff;stroke:#000000;stroke-width:([\d.]+)[^"]*" width="([\d.]+)" height="[\d.]+" x="([\d.]+)" y="([\d.]+)"', svg)
stroke, w, x, y = map(float, frame.groups())
frame_cx = x + w / 2
outer_top = y - stroke / 2

nail = float(re.search(r'class="logo-nail" cx="([\d.]+)"', svg).group(1))
wire = re.search(r'class="logo-wire" d="M ([\d.]+),([\d.]+) ([\d.]+),([\d.]+) ([\d.]+),([\d.]+)"', svg)
lx, ly, ax, ay, rx, ry = map(float, wire.groups())
wire_w = float(re.search(r'logo-wire"[^>]*stroke-width:([\d.]+)', svg).group(1))

print(f"frame centre x   {frame_cx:.5f}")
print(f"nail x           {nail:.5f}")
print(f"wire apex x      {ax:.5f}")
print(f"left leg lands   x={lx:.5f}  ({frame_cx - lx:.5f} left of centre)  y={ly:.5f}")
print(f"right leg lands  x={rx:.5f}  ({rx - frame_cx:.5f} right of centre)  y={ry:.5f}")
print(f"frame outer top y {outer_top:.5f}")
print(f"wire stroke {wire_w}  frame stroke {stroke}")

eps = 1e-3
ok = (abs(nail - frame_cx) < eps and abs(ax - frame_cx) < eps
      and abs((frame_cx - lx) - (rx - frame_cx)) < eps
      and abs(ly - outer_top) < eps and abs(ry - outer_top) < eps
      and abs(wire_w - stroke) < eps)
print("OK" if ok else "MISALIGNED")
sys.exit(0 if ok else 1)

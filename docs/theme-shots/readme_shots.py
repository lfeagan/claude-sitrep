#!/usr/bin/env python3
"""Turns the frames that TestWriteReadmeShots writes (SITREP_README_DIR) into the pictures of the README: each frame in a window with a
title bar, on a plain page. The first line of a frame ("# label") is the window's title. cvd-dark.ans is drawn three times, as seen with normal
sight, protanopia and deuteranopia. Each picture is then rewritten as a palette PNG, which is far smaller. Needs Python 3 and rsvg-convert; `make screenshots` runs it."""
import os, struct, subprocess, sys, zlib
from collections import Counter
from xml.sax.saxutils import escape
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import ansi2png as a

CW, CH, FS, PAD, BAR, GAP = a.CW, a.CH, a.FS, 14, 30, 22
DOTS = [(255, 95, 87), (254, 188, 46), (40, 200, 64)] # the window's dots are small, and still have to keep their colours
FONT = "Menlo, DejaVu Sans Mono, monospace"

def window(parts, x0, y0, lines, label, dfg, dbg, m, page):
    cols = max(sum(c[1] for c in a.parse(l, dfg, dbg)) for l in lines)
    w, h = cols * CW + 2 * PAD, len(lines) * CH + 2 * PAD + BAR
    parts.append(f'<rect x="{x0}" y="{y0}" width="{w}" height="{h}" rx="9" fill="{a.hexs(a.cvd(dbg, m))}"/>')
    bar = a.cvd(tuple(max(0, min(255, v + (14 if sum(dbg) < 300 else -14))) for v in dbg), m)
    parts.append(f'<path d="M{x0} {y0+BAR} V{y0+9} a9 9 0 0 1 9 -9 H{x0+w-9} a9 9 0 0 1 9 9 V{y0+BAR} Z" fill="{a.hexs(bar)}"/>')
    for i, c in enumerate(("#ff5f57", "#febc2e", "#28c840")):
        parts.append(f'<circle cx="{x0+16+i*18}" cy="{y0+BAR//2}" r="5.5" fill="{a.hexs(a.cvd(tuple(int(c[j:j+2], 16) for j in (1, 3, 5)), m))}"/>')
    tf = a.cvd(tuple(int(v * .62 + (255 - v) * .0) if sum(dbg) < 300 else int(v + 90) % 256 for v in dfg), m) if False else a.cvd(dfg, m)
    parts.append(f'<text x="{x0+w/2}" y="{y0+BAR//2+5}" text-anchor="middle" font-family="{FONT}" font-size="13" fill="{a.hexs(tf)}" fill-opacity="0.7">{escape(label)}</text>')
    y = y0 + BAR + PAD
    for line in lines:
        cells = a.parse(line, dfg, dbg)
        x, run, key = 0, [], None
        def flush(run, x0c):
            if not run: return
            text = "".join(c[0] for c in run); ncols = sum(c[1] for c in run)
            _, _, fg, bg, bold, ul, rev, faint = run[0]
            f, b = fg or dfg, bg or dbg
            if rev: f, b = (bg or dbg), (fg or dfg)
            if faint: f = tuple((p + q) // 2 for p, q in zip(f, b))
            f, b = a.cvd(f, m), a.cvd(b, m)
            px = x0 + PAD + x0c * CW
            if b != a.cvd(dbg, m):
                if x0c == 0: px, ncols_px = x0, ncols * CW + PAD
                else: ncols_px = ncols * CW
                end = x0c + ncols
                if end * CW + 2 * PAD >= w - 0 and end >= cols: ncols_px = x0 + w - px
                parts.append(f'<rect x="{px}" y="{y}" width="{ncols_px}" height="{CH}" fill="{a.hexs(b)}"/>')
            # a terminal paints the block characters over the whole cell, and a font's glyphs leave gaps between lines: draw the bars of the cards,
            # and the rail of the selection, as shapes
            col, drawn = x0c, []
            for c in run:
                ch = c[0]
                cx = x0 + PAD + col * CW
                mid, t = cx + CW / 2, 3 # the middle of the cell, and the weight of a heavy line
                if ch == "\u2503": parts.append(f'<rect x="{mid-t/2}" y="{y}" width="{t}" height="{CH}" fill="{a.hexs(f)}"/>')
                elif ch == "\u250f": parts.append(f'<rect x="{mid-t/2}" y="{y+CH/2-t/2}" width="{t}" height="{CH/2+t/2}" fill="{a.hexs(f)}"/><rect x="{mid-t/2}" y="{y+CH/2-t/2}" width="{CW/2+t/2}" height="{t}" fill="{a.hexs(f)}"/>')
                elif ch == "\u2517": parts.append(f'<rect x="{mid-t/2}" y="{y}" width="{t}" height="{CH/2+t/2}" fill="{a.hexs(f)}"/><rect x="{mid-t/2}" y="{y+CH/2-t/2}" width="{CW/2+t/2}" height="{t}" fill="{a.hexs(f)}"/>')
                elif ch == "\u2502": parts.append(f'<rect x="{mid-0.75}" y="{y}" width="1.5" height="{CH}" fill="{a.hexs(f)}"/>')
                elif ch == "\u2575": parts.append(f'<rect x="{mid-0.75}" y="{y}" width="1.5" height="{CH/2}" fill="{a.hexs(f)}"/>')
                drawn.append(" " if ch in "\u2503\u250f\u2517\u2502\u2575" else ch)
                col += c[1]
            text = "".join(drawn)
            if text.strip():
                parts.append(f'<text x="{x0+PAD+x0c*CW}" y="{y+CH-5}" font-family="{FONT}" font-size="{FS}" textLength="{ncols*CW}" lengthAdjust="spacing" xml:space="preserve" fill="{a.hexs(f)}"' + (' font-weight="bold"' if bold else '') + (' text-decoration="underline"' if ul else '') + f'>{escape(text)}</text>')
        for c in cells:
            k = c[2:]
            if key is not None and k != key:
                flush(run, x - sum(r[1] for r in run)); run = []
            key = k; run.append(c); x += c[1]
        flush(run, x - sum(r[1] for r in run))
        y += CH
    return w, h

def draw(frames, side, out, matrices=(None,), titles=None):
    dfg, dbg = ((230, 230, 230), (30, 30, 30)) if side == "dark" else ((26, 26, 26), (255, 255, 255))
    page = (34, 36, 46) if side == "dark" else (222, 225, 233)
    parts, y, width = [], GAP, 0
    items = []
    for fi, f in enumerate(frames):
        lines = f.rstrip("\n").split("\n")
        label = lines[0][2:] if lines[0].startswith("# ") else ""
        items.append((label, lines[1:] if lines[0].startswith("# ") else lines))
    for mi, m in enumerate(matrices):
        for label, lines in items:
            title = titles[mi] if titles else label
            w, h = window(parts, GAP, y, lines, (label + " - " + title) if titles else label, dfg, dbg, m, page)
            width = max(width, w); y += h + GAP
    W, H = width + 2 * GAP, y
    svg = f'<svg xmlns="http://www.w3.org/2000/svg" width="{W}" height="{H}" viewBox="0 0 {W} {H}"><rect width="100%" height="100%" fill="{a.hexs(page)}"/>' + "".join(parts) + "</svg>"
    open(out + ".svg", "w").write(svg)
    subprocess.run(["rsvg-convert", "-o", out, out + ".svg"], check=True)
    os.remove(out + ".svg")

def read_png(path):
    """Reads the 8-bit RGB or RGBA PNG that rsvg-convert writes, and returns (width, height, rows of RGB tuples)."""
    data = open(path, "rb").read()
    pos, idat, w, h = 8, b"", 0, 0
    while pos < len(data):
        n, kind = struct.unpack(">I4s", data[pos:pos + 8])
        body = data[pos + 8:pos + 8 + n]
        if kind == b"IHDR": w, h, depth, ctype = struct.unpack(">IIBB", body[:10]); assert depth == 8 and ctype in (2, 6), "expects 8-bit RGB or RGBA"
        elif kind == b"IDAT": idat += body
        pos += 12 + n
    bpp = 4 if ctype == 6 else 3
    raw, stride = zlib.decompress(idat), w * bpp
    rows, prev = [], bytearray(stride)
    for y in range(h):
        f, line = raw[y * (stride + 1)], bytearray(raw[y * (stride + 1) + 1:(y + 1) * (stride + 1)])
        for i in range(stride):
            l = line[i - bpp] if i >= bpp else 0
            u, ul = prev[i], (prev[i - bpp] if i >= bpp else 0)
            if f == 1: line[i] = (line[i] + l) & 255
            elif f == 2: line[i] = (line[i] + u) & 255
            elif f == 3: line[i] = (line[i] + (l + u) // 2) & 255
            elif f == 4:
                pa, pb, pc = abs(u - ul), abs(l - ul), abs(l + u - 2 * ul)
                line[i] = (line[i] + (l if pa <= pb and pa <= pc else (u if pb <= pc else ul))) & 255
        rows.append([(line[i], line[i + 1], line[i + 2]) for i in range(0, stride, bpp)])
        prev = line
    return w, h, rows

def palette(counts, n):
    """Median cut: n colours that stand for the colours of the picture, weighted by how many pixels have them."""
    boxes = [list(counts.items())]
    while len(boxes) < n:
        boxes.sort(key=lambda b: -sum(c for _, c in b) * (len(b) > 1))
        box = boxes.pop(0)
        if len(box) < 2: boxes.append(box); break
        ch = max(range(3), key=lambda k: max(c[0][k] for c in box) - min(c[0][k] for c in box))
        box.sort(key=lambda e: e[0][ch]); total, acc, cut = sum(c for _, c in box), 0, 1
        for i, (_, c) in enumerate(box):
            acc += c
            if acc * 2 >= total: cut = max(1, min(i + 1, len(box) - 1)); break
        boxes += [box[:cut], box[cut:]]
    return [tuple(round(sum(col[k] * c for col, c in b) / sum(c for _, c in b)) for k in range(3)) for b in boxes]

def shrink(path, colours=96):
    """Rewrites a PNG as a palette PNG with filtered rows: a picture of text is flat colour, and this is a fraction of the size."""
    w, h, rows = read_png(path)
    counts = Counter(px for r in rows for px in r)
    # the colours that carry many pixels (background, text, row colours, the dots of the window) are kept exactly; the antialiasing blends share the rest
    exact = [c for c in DOTS if c in counts] + [c for c, n in counts.most_common(colours // 2) if n >= 150 and c not in DOTS]
    rest = Counter({c: n for c, n in counts.items() if c not in exact})
    pal = exact + (palette(rest, max(colours - len(exact), 1)) if rest else [])
    near = {}
    def idx(px):
        if px not in near: near[px] = min(range(len(pal)), key=lambda i: sum((a - b) ** 2 for a, b in zip(pal[i], px)))
        return near[px]
    out = bytearray()
    prev = bytearray(w)
    for r in rows:
        line = bytearray(idx(px) for px in r)
        best = min(((0, line), (1, bytearray((line[i] - (line[i - 1] if i else 0)) & 255 for i in range(w))), (2, bytearray((line[i] - prev[i]) & 255 for i in range(w)))),
                   key=lambda t: sum(min(v, 256 - v) for v in t[1]))
        out += bytes([best[0]]) + best[1]; prev = line
    def chunk(kind, body): return struct.pack(">I", len(body)) + kind + body + struct.pack(">I", zlib.crc32(kind + body) & 0xffffffff)
    png = b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", w, h, 8, 3, 0, 0, 0)) + chunk(b"PLTE", b"".join(bytes(c) for c in pal)) \
        + chunk(b"IDAT", zlib.compress(bytes(out), 9)) + chunk(b"IEND", b"")
    open(path, "wb").write(png)

def social(frame, out):
    """The 1280 by 640 picture for GitHub's social preview: the name and the tagline over the top of the Cards window, which runs off the bottom."""
    dfg, dbg = (230, 230, 230), (30, 30, 30)
    page = (34, 36, 46)
    lines = frame.rstrip("\n").split("\n")[1:23]  # whole rows only: the window runs off the bottom of the card, and no row is cut in half
    cols = max(sum(c[1] for c in a.parse(l, dfg, dbg)) for l in lines)
    w = cols * CW + 2 * PAD
    parts = [f'<text x="640" y="82" text-anchor="middle" font-family="{FONT}" font-size="54" font-weight="bold" fill="#e6e6e6">claude-sitrep</text>',
             f'<text x="640" y="128" text-anchor="middle" font-family="{FONT}" font-size="22" fill="#a78bfa">A to-do list for each Claude Code session,</text>',
             f'<text x="640" y="158" text-anchor="middle" font-family="{FONT}" font-size="22" fill="#a78bfa">and a dashboard that shows which one is waiting for you.</text>']
    window(parts, (1280 - w) // 2, 196, lines, "sitrep dash --demo", dfg, dbg, None, page)
    svg = f'<svg xmlns="http://www.w3.org/2000/svg" width="1280" height="640" viewBox="0 0 1280 640"><rect width="100%" height="100%" fill="{a.hexs(page)}"/>' + "".join(parts) + "</svg>"
    open(out + ".svg", "w").write(svg)
    subprocess.run(["rsvg-convert", "-o", out, out + ".svg"], check=True)
    os.remove(out + ".svg")
    shrink(out)

if __name__ == "__main__":
    src, dst = sys.argv[1], sys.argv[2]
    os.makedirs(dst, exist_ok=True)
    for fn in sorted(os.listdir(src)):
        if not fn.endswith(".ans"): continue
        base = fn[:-4]; name, side = base.rsplit("-", 1)
        frames = open(os.path.join(src, fn), encoding="utf-8").read().split("\n=====\n")
        out = os.path.join(dst, (base if name == "themes" else name) + ".png")
        if name == "cvd":
            draw(frames, side, out, (None, a.CVD["protanopia (no red)"], a.CVD["deuteranopia (no green)"]), ("normal sight", "protanopia (no red)", "deuteranopia (no green)"))
        else:
            draw(frames, side, out)
        shrink(out)
        print(os.path.basename(out))
        if name == "hero-cards":
            social(frames[0], os.path.join(dst, "social-preview.png")); print("social-preview.png")

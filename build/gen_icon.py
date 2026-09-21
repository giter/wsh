#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""生成 wsh 的桌面应用图标。

只用 Python 标准库：光栅化（SDF + 解析抗锯齿）与 PNG / ICO 编码都是手写的，
因此不需要 Pillow / ImageMagick 之类的外部依赖。

    python3 build/gen_icon.py            # 生成 build/icon.ico、build/icon.png
    python3 build/gen_icon.py --sheet    # 额外生成 build/icon-sheet.png（尺寸对照图）

造型与 build/icon.svg 保持一致：深色圆角方块 + 绿蓝渐变的终端提示符 `>_`。
"""

import math
import os
import struct
import sys
import zlib

# ---------------------------------------------------------------- 造型参数
# 全部按画布边长取比例，任何尺寸都用同一套几何，缩小时造型不会走样。

MARGIN = 0.045          # 圆角方块到画布边缘的留白
RADIUS = 0.2184         # 圆角半径
STROKE = 0.055          # 提示符笔画半宽

CHEVRON = ((0.300, 0.295), (0.475, 0.500), (0.300, 0.705))  # `>` 的三个折点
CURSOR = ((0.600, 0.705), (0.690, 0.705))                   # `_` 光标的胶囊两端

BG_TOP = (0x2E, 0x2E, 0x3C)     # 背景渐变（左上 → 右下）
BG_BOTTOM = (0x14, 0x14, 0x1B)
FG_FROM = (0x34, 0xD3, 0x99)    # 前景渐变：应用主色绿 → 蓝
FG_TO = (0x60, 0xA5, 0xFA)

ICO_SIZES = (16, 24, 32, 48, 64, 128, 256)
EMBED_SIZE = 512                # 供 go:embed 的 PNG（macOS / Linux 用）
SHEET_SIZES = (256, 128, 64, 48, 32, 24, 16)

HERE = os.path.dirname(os.path.abspath(__file__))


# ---------------------------------------------------------------- 光栅化

def _lerp(c0, c1, t):
    return (c0[0] + (c1[0] - c0[0]) * t,
            c0[1] + (c1[1] - c0[1]) * t,
            c0[2] + (c1[2] - c0[2]) * t)


def _proj(px, py, a, d):
    """点 (px,py) 在射线 a→d 上的投影参数，截断到 [0,1]。"""
    t = ((px - a[0]) * d[0] + (py - a[1]) * d[1]) / (d[0] * d[0] + d[1] * d[1])
    return 0.0 if t < 0.0 else (1.0 if t > 1.0 else t)


def _sd_round_rect(px, py, cx, cy, hx, hy, r):
    """圆角矩形的有符号距离（负数在内部）。"""
    qx = abs(px - cx) - (hx - r)
    qy = abs(py - cy) - (hy - r)
    return math.hypot(max(qx, 0.0), max(qy, 0.0)) + min(max(qx, qy), 0.0) - r


def _sd_capsule(px, py, a, b):
    """线段 ab 的有符号距离（到线段的距离），圆头笔画靠它得到圆角端点。"""
    dx = b[0] - a[0]
    dy = b[1] - a[1]
    t = ((px - a[0]) * dx + (py - a[1]) * dy) / (dx * dx + dy * dy)
    t = 0.0 if t < 0.0 else (1.0 if t > 1.0 else t)
    return math.hypot(px - (a[0] + t * dx), py - (a[1] + t * dy))


def render(n):
    """把图标光栅化成 n×n 的 RGBA 字节串（每像素 1px 覆盖度由 SDF 解析求得）。"""
    s = float(n)
    buf = bytearray(n * n * 4)

    cx = cy = s / 2.0
    half = s * (1.0 - 2.0 * MARGIN) / 2.0
    radius = RADIUS * s
    stroke = STROKE * s

    chev = [(x * s, y * s) for x, y in CHEVRON]
    cur = [(x * s, y * s) for x, y in CURSOR]
    fg_a = (0.245 * s, 0.240 * s)       # 前景渐变沿字形外接矩形的对角线
    fg_d = (0.500 * s, 0.520 * s)

    for y in range(n):
        py = y + 0.5
        for x in range(n):
            px = x + 0.5

            a_bg = 0.5 - _sd_round_rect(px, py, cx, cy, half, half, radius)
            if a_bg <= 0.0:
                continue
            if a_bg > 1.0:
                a_bg = 1.0

            r, g, b = _lerp(BG_TOP, BG_BOTTOM, (px + py) / (2.0 * s))

            d_fg = min(_sd_capsule(px, py, chev[0], chev[1]),
                       _sd_capsule(px, py, chev[1], chev[2]),
                       _sd_capsule(px, py, cur[0], cur[1])) - stroke
            a_fg = 0.5 - d_fg
            if a_fg > 0.0:
                if a_fg > 1.0:
                    a_fg = 1.0
                fr, fg, fb = _lerp(FG_FROM, FG_TO, _proj(px, py, fg_a, fg_d))
                r += (fr - r) * a_fg
                g += (fg - g) * a_fg
                b += (fb - b) * a_fg

            i = (y * n + x) * 4
            buf[i] = int(r + 0.5)
            buf[i + 1] = int(g + 0.5)
            buf[i + 2] = int(b + 0.5)
            buf[i + 3] = int(a_bg * 255.0 + 0.5)
    return buf


# ---------------------------------------------------------------- 编码

def _chunk(tag, data):
    return (struct.pack(">I", len(data)) + tag + data
            + struct.pack(">I", zlib.crc32(tag + data) & 0xFFFFFFFF))


def png_bytes(n, rgba):
    raw = bytearray()
    stride = n * 4
    for y in range(n):
        raw.append(0)                                   # 过滤器：None
        raw += rgba[y * stride:(y + 1) * stride]
    return (b"\x89PNG\r\n\x1a\n"
            + _chunk(b"IHDR", struct.pack(">IIBBBBB", n, n, 8, 6, 0, 0, 0))
            + _chunk(b"IDAT", zlib.compress(bytes(raw), 9))
            + _chunk(b"IEND", b""))


def dib_bytes(n, rgba):
    """ICO 内嵌的 32 位 BMP（BITMAPINFOHEADER + BGRA + AND 掩码）。"""
    header = struct.pack("<IiiHHIIiiII", 40, n, n * 2, 1, 32, 0, 0, 0, 0, 0, 0)

    pixels = bytearray()
    for y in range(n - 1, -1, -1):                      # BMP 自下而上
        row = rgba[y * n * 4:(y + 1) * n * 4]
        for x in range(n):
            r, g, b, a = row[x * 4:x * 4 + 4]
            pixels += bytes((b, g, r, a))

    # 掩码：每行按 4 字节对齐，透明处为 1。32 位图标其实用 alpha 通道，
    # 但保留掩码能让老程序（以及部分缩略图组件）正确显示。
    mask_stride = ((n + 31) // 32) * 4
    mask = bytearray()
    for y in range(n - 1, -1, -1):
        bits = bytearray(mask_stride)
        for x in range(n):
            if rgba[(y * n + x) * 4 + 3] < 128:
                bits[x // 8] |= 0x80 >> (x % 8)
        mask += bits

    return header + bytes(pixels) + bytes(mask)


def ico_bytes(images):
    """images: [(size, payload_bytes)]，顺序即 ICO 目录顺序。"""
    count = len(images)
    offset = 6 + 16 * count
    directory = bytearray()
    for size, payload in images:
        directory += struct.pack("<BBBBHHII",
                                 0 if size >= 256 else size,
                                 0 if size >= 256 else size,
                                 0, 0, 1, 32, len(payload), offset)
        offset += len(payload)
    body = b"".join(payload for _, payload in images)
    return struct.pack("<HHH", 0, 1, count) + bytes(directory) + body


# ---------------------------------------------------------------- 输出

def build_sheet(images):
    """把各尺寸按原始像素并排画出来，方便检查小尺寸是否还看得清。"""
    gap = 18
    rows = ((0x24, 0x24, 0x2C), (0xF0, 0xF0, 0xF4))     # 深色 / 浅色两种底
    width = gap + sum(s + gap for s in SHEET_SIZES)
    height = gap
    for bg in rows:
        height += max(SHEET_SIZES) + gap

    px = bytearray()
    for bg in rows:
        for _ in range(max(SHEET_SIZES) + gap):
            for _ in range(width):
                px += bytes(bg + (255,))

    y = gap
    for bg in rows:
        x = gap
        for size in SHEET_SIZES:
            rgba = images[size]
            top = y + max(SHEET_SIZES) - size           # 底部对齐，便于比大小
            for sy in range(size):
                for sx in range(size):
                    i = (sy * size + sx) * 4
                    a = rgba[i + 3] / 255.0
                    if a <= 0.0:
                        continue
                    d = ((top + sy) * width + x + sx) * 4
                    for c in range(3):
                        px[d + c] = int(rgba[i + c] * a + px[d + c] * (1.0 - a) + 0.5)
            x += size + gap
        y += max(SHEET_SIZES) + gap

    return png_bytes(width, px), width, height


def main():
    images = {}
    for size in sorted(set(ICO_SIZES) | {EMBED_SIZE}):
        images[size] = render(size)

    entries = []
    for size in ICO_SIZES:
        payload = dib_bytes(size, images[size]) if size <= 64 else png_bytes(size, images[size])
        entries.append((size, payload))

    ico_path = os.path.join(HERE, "icon.ico")
    with open(ico_path, "wb") as f:
        f.write(ico_bytes(entries))

    png_path = os.path.join(HERE, "icon.png")
    with open(png_path, "wb") as f:
        f.write(png_bytes(EMBED_SIZE, images[EMBED_SIZE]))

    print("已生成 %s（%s）" % (ico_path, ", ".join("%dpx" % s for s in ICO_SIZES)))
    print("已生成 %s（%dpx）" % (png_path, EMBED_SIZE))

    if "--sheet" in sys.argv:
        sheet, w, h = build_sheet(images)
        sheet_path = os.path.join(HERE, "icon-sheet.png")
        with open(sheet_path, "wb") as f:
            f.write(sheet)
        print("已生成 %s（%dx%d）" % (sheet_path, w, h))


if __name__ == "__main__":
    main()

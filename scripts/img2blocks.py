#!/usr/bin/env python3
"""Convert an image to half-block pixel art.

Usage:
    python img2blocks.py <image_path> [--width 40] [--threshold 128]

Uses Unicode half-block characters (▀ ▄ █) to render 2 vertical pixels
per character cell. No row gaps — seamless vertical rendering.

Each character cell represents 1×2 pixels:
  - Both on  → █ (full block)
  - Top only → ▀ (upper half)
  - Bot only → ▄ (lower half)
  - Both off → (space)
"""
from __future__ import annotations

import argparse
import sys
from pathlib import Path

from PIL import Image


def image_to_blocks(
    image_path: str,
    width: int = 40,
    threshold: int = 128,
    invert: bool = False,
    yscale: float = 1.0,
) -> str:
    img = Image.open(image_path).convert("L")

    pixel_width = width
    aspect = img.height / img.width
    pixel_height = int(pixel_width * aspect * yscale)
    # Round to even (2 rows per character)
    pixel_height = ((pixel_height + 1) // 2) * 2

    img = img.resize((pixel_width, pixel_height), Image.Resampling.LANCZOS)
    pixels = img.load()

    lines = []
    for y in range(0, pixel_height, 2):
        line = []
        for x in range(pixel_width):
            top_on = (pixels[x, y] > threshold) if invert else (pixels[x, y] < threshold)
            bot_on = False
            if y + 1 < pixel_height:
                bot_on = (pixels[x, y + 1] > threshold) if invert else (pixels[x, y + 1] < threshold)

            if top_on and bot_on:
                line.append("█")
            elif top_on:
                line.append("▀")
            elif bot_on:
                line.append("▄")
            else:
                line.append(" ")
        lines.append("".join(line).rstrip())

    return "\n".join(lines)


def main() -> None:
    parser = argparse.ArgumentParser(description="Convert image to half-block pixel art")
    parser.add_argument("image", help="Path to source image")
    parser.add_argument("--width", type=int, default=40, help="Output width in chars (default: 40)")
    parser.add_argument("--threshold", type=int, default=128, help="Grayscale threshold 0-255 (default: 128)")
    parser.add_argument("--invert", action="store_true", help="Invert: light pixels become blocks")
    parser.add_argument("--yscale", type=float, default=1.0, help="Vertical scale factor")
    parser.add_argument("--output", "-o", help="Write to file instead of stdout")
    args = parser.parse_args()

    if not Path(args.image).is_file():
        print(f"Error: {args.image} not found", file=sys.stderr)
        sys.exit(1)

    result = image_to_blocks(args.image, args.width, args.threshold, args.invert, args.yscale)

    if args.output:
        Path(args.output).write_text(result + "\n", encoding="utf-8")
        print(f"Written to {args.output}")
    else:
        print(result)


if __name__ == "__main__":
    main()

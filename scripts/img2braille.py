#!/usr/bin/env python3
"""Convert an image to Unicode Braille art.

Usage:
    python img2braille.py <image_path> [--width 60] [--threshold 128] [--invert]

Each Braille character encodes a 2x4 pixel block using Unicode U+2800-U+28FF.
The 8 dots in a Braille cell map to pixel positions:

    (0,0) (1,0)     dot1 dot4
    (0,1) (1,1)     dot2 dot5
    (0,2) (1,2)     dot3 dot6
    (0,3) (1,3)     dot7 dot8

Braille codepoint = 0x2800 + sum of enabled dot bits.
"""
from __future__ import annotations

import argparse
import sys
from pathlib import Path

from PIL import Image, ImageFilter

# Braille dot bit positions for each (x, y) offset within a 2x4 cell
BRAILLE_MAP = {
    (0, 0): 0x01,  # dot 1
    (0, 1): 0x02,  # dot 2
    (0, 2): 0x04,  # dot 3
    (1, 0): 0x08,  # dot 4
    (1, 1): 0x10,  # dot 5
    (1, 2): 0x20,  # dot 6
    (0, 3): 0x40,  # dot 7
    (1, 3): 0x80,  # dot 8
}


def image_to_braille(
    image_path: str,
    width: int = 60,
    threshold: int = 128,
    invert: bool = False,
    clean: int = 0,
    yscale: float = 1.0,
    dilate: int = 0,
) -> str:
    """Convert an image file to a string of Braille characters.

    Args:
        image_path: Path to the source image.
        width: Output width in Braille characters (each = 2 pixels wide).
        threshold: Grayscale threshold (0-255). Pixels darker than this are "on".
        invert: If True, light pixels are "on" instead of dark ones.
        clean: Number of erosion+dilation passes to remove noise (0=off).
        dilate: Number of dilation passes to thicken strokes (0=off).

    Returns:
        Multi-line string of Braille Unicode characters.
    """
    img = Image.open(image_path).convert("L")  # grayscale

    # Resize: each Braille char encodes a 2px wide x 4px tall block,
    # but on screen a monospace character cell is roughly 1:2 (w:h).
    # The ratio of (cell_width / cell_height) to (pixel_width / pixel_height)
    # gives us the correction factor. Typical monospace: ~0.5:1 cell ratio,
    # pixel ratio per char: 2:4 = 0.5:1. These cancel out, so no correction
    # is needed for most monospace fonts. Use --yscale to fine-tune.
    pixel_width = width * 2
    aspect = img.height / img.width
    pixel_height = int(pixel_width * aspect * yscale)
    # Round up to multiple of 4 (Braille cell height)
    pixel_height = ((pixel_height + 3) // 4) * 4

    img = img.resize((pixel_width, pixel_height), Image.Resampling.LANCZOS)

    # Morphological cleanup: binarize, then erode (remove tiny dots) + dilate (restore strokes)
    if clean > 0:
        img = img.point(lambda v: 0 if v < threshold else 255)
        for _ in range(clean):
            img = img.filter(ImageFilter.MinFilter(3))  # erode (shrink dark regions)
        for _ in range(clean):
            img = img.filter(ImageFilter.MaxFilter(3))  # dilate (restore them)

    # Thicken strokes to fill gaps between Braille rows
    if dilate > 0:
        img = img.point(lambda v: 0 if v < threshold else 255)
        for _ in range(dilate):
            img = img.filter(ImageFilter.MinFilter(3))  # MinFilter on dark=0 expands dark regions

    pixels = img.load()

    lines = []
    for cy in range(0, pixel_height, 4):
        line = []
        for cx in range(0, pixel_width, 2):
            codepoint = 0x2800
            for (dx, dy), bit in BRAILLE_MAP.items():
                px = cx + dx
                py = cy + dy
                if px < pixel_width and py < pixel_height:
                    val = pixels[px, py]
                    is_on = val > threshold if invert else val < threshold
                    if is_on:
                        codepoint |= bit
            line.append(chr(codepoint))
        lines.append("".join(line))

    return "\n".join(lines)


def main() -> None:
    parser = argparse.ArgumentParser(description="Convert image to Braille art")
    parser.add_argument("image", help="Path to source image")
    parser.add_argument("--width", type=int, default=60, help="Output width in chars (default: 60)")
    parser.add_argument("--threshold", type=int, default=128, help="Grayscale threshold 0-255 (default: 128)")
    parser.add_argument("--invert", action="store_true", help="Invert: light pixels become dots")
    parser.add_argument("--clean", type=int, default=0, help="Noise cleanup passes (0=off, 1-2 recommended)")
    parser.add_argument("--yscale", type=float, default=1.0, help="Vertical scale factor (default: 1.0, try 0.6-0.8 if too tall)")
    parser.add_argument("--dilate", type=int, default=0, help="Thicken strokes to fill row gaps (0=off, 1-2 recommended)")
    parser.add_argument("--output", "-o", help="Write to file instead of stdout")
    args = parser.parse_args()

    if not Path(args.image).is_file():
        print(f"Error: {args.image} not found", file=sys.stderr)
        sys.exit(1)

    result = image_to_braille(args.image, args.width, args.threshold, args.invert, args.clean, args.yscale, args.dilate)

    if args.output:
        Path(args.output).write_text(result + "\n", encoding="utf-8")
        print(f"Written to {args.output}")
    else:
        print(result)


if __name__ == "__main__":
    main()

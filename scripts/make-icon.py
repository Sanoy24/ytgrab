"""Draws the YTGrab icon: a white download arrow over a tray on a red rounded square.

    python scripts/make-icon.py

Writes packaging/icon/ytgrab.png (512 px) and packaging/icon/ytgrab.ico (16-256 px).
Needs Pillow. Each size is drawn at 8x and scaled down, so small sizes stay sharp.
"""
from pathlib import Path

from PIL import Image, ImageDraw

RED = (196, 48, 43, 255)  # --accent in web/static/app.css
WHITE = (255, 255, 255, 255)
OUT = Path(__file__).resolve().parent.parent / "packaging" / "icon"


def draw(size):
    s = size * 8
    img = Image.new("RGBA", (s, s), (0, 0, 0, 0))
    d = ImageDraw.Draw(img)
    u = s / 100  # work in a 100x100 grid
    d.rounded_rectangle((0, 0, s - 1, s - 1), radius=22 * u, fill=RED)
    # Arrow: shaft and head.
    d.rectangle((44 * u, 18 * u, 56 * u, 48 * u), fill=WHITE)
    d.polygon([(28 * u, 44 * u), (72 * u, 44 * u), (50 * u, 66 * u)], fill=WHITE)
    # Tray: an open box under the arrow.
    w = 10 * u
    d.rectangle((20 * u, 60 * u, 20 * u + w, 82 * u), fill=WHITE)
    d.rectangle((80 * u - w, 60 * u, 80 * u, 82 * u), fill=WHITE)
    d.rectangle((20 * u, 82 * u - w, 80 * u, 82 * u), fill=WHITE)
    return img.resize((size, size), Image.LANCZOS)


OUT.mkdir(parents=True, exist_ok=True)
draw(512).save(OUT / "ytgrab.png")
sizes = [256, 128, 64, 48, 32, 24, 16]
frames = [draw(n) for n in sizes]
frames[0].save(OUT / "ytgrab.ico", sizes=[(n, n) for n in sizes], append_images=frames[1:])
print("wrote", OUT / "ytgrab.png", "and", OUT / "ytgrab.ico")

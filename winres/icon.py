# Draws the ssh-agent-totp icon: a yellow stopwatch whose crown is a padlock
# shackle, on a black rounded tile (vauth-desktop family, colours swapped).
# Regenerates icon.png (1024 px), icon.ico and the tray frames.
from PIL import Image, ImageDraw

S, YELLOW, BLACK = 1024, "#FFD814", "#111111"
image = Image.new("RGBA", (S, S), (0, 0, 0, 0))
draw = ImageDraw.Draw(image)
draw.rounded_rectangle((0, 0, S - 1, S - 1), radius=S * 0.16, fill=BLACK)
# Fill the tile: the tray shows it at 16-24 px.
cx, cy, r = S / 2, 612, 388
# Shackle crown.
half, width = 150, 90
draw.arc((cx - half, 34, cx + half, 34 + 2 * half), 180, 360, fill=YELLOW, width=width)
draw.rectangle((cx - half, 184, cx - half + width, cy - r + 40), fill=YELLOW)
draw.rectangle((cx + half - width, 184, cx + half, cy - r + 40), fill=YELLOW)
# Dial: yellow ring, elapsed sector, centre pin.
draw.ellipse((cx - r, cy - r, cx + r, cy + r), fill=YELLOW)
draw.ellipse((cx - r + 84, cy - r + 84, cx + r - 84, cy + r - 84), fill=BLACK)
draw.pieslice((cx - r + 136, cy - r + 136, cx + r - 136, cy + r - 136), -90, 30, fill=YELLOW)
draw.ellipse((cx - 40, cy - 40, cx + 40, cy + 40), fill=YELLOW)
image.save("icon.png")
image.resize((256, 256), Image.LANCZOS).save("icon.ico", sizes=[(s, s) for s in (16, 20, 24, 32, 40, 48, 64, 128, 256)])

# The existing dial becomes a 30-second tray spinner; keep the tile and crown.
# Small PNG frames, embedded by internal/tray, avoid runtime rendering.
from pathlib import Path
frames = Path("../internal/tray/frames")
frames.mkdir(exist_ok=True)
for old in frames.glob("*.png"):
    old.unlink()

def save_menu_frame(second):
    # Same ring geometry as ui/app.js: white surface, gray track, soft blue arc.
    menu = Image.new("RGBA", (S, S), "#FFFFFF")
    ring = ImageDraw.Draw(menu)
    radius, stroke = S * 8 / 22, round(S * 3 / 22)
    bounds = (S / 2 - radius, S / 2 - radius, S / 2 + radius, S / 2 + radius)
    ring.ellipse(bounds, outline="#DFE3E8", width=stroke)
    if second:
        ring.arc(bounds, -90, -90 + 360 * second / 30, fill="#6B92BD", width=stroke)
    menu.resize((16, 16), Image.LANCZOS).save(frames / f"menu-{second:02}.png")

bounds = (cx - r + 136, cy - r + 136, cx + r - 136, cy + r - 136)
for second in range(30):
    frame = image.copy()
    dial = ImageDraw.Draw(frame)
    dial.ellipse(bounds, fill=BLACK)
    if second:
        dial.pieslice(bounds, -90, -90 + second * 12, fill=YELLOW)
    dial.ellipse((cx - 40, cy - 40, cx + 40, cy + 40), fill=YELLOW)
    frame.resize((32, 32), Image.LANCZOS).save(frames / f"{second:02}.png")
    save_menu_frame(second)
frame = image.copy()
dial = ImageDraw.Draw(frame)
dial.ellipse(bounds, fill=BLACK)
dial.line([(320, 610), (460, 760), (725, 450)], fill=YELLOW, width=80)
frame.resize((32, 32), Image.LANCZOS).save(frames / "copied.png")

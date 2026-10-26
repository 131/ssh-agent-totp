# Draws the ssh-agent-totp icon: a yellow stopwatch whose crown is a padlock
# shackle, on a black rounded tile (vauth-desktop family, colours swapped).
# Regenerates icon.png (1024 px) and icon.ico.
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

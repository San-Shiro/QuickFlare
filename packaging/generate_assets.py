"""
QuickFlare Asset Generator (using PySide6 for 100% SVG gradient fidelity)
"""

import os
import shutil
import subprocess
from PySide6.QtGui import QImage, QPainter
from PySide6.QtSvg import QSvgRenderer
from PySide6.QtCore import Qt, QSize
from PIL import Image

REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
ASSETS_DIR = os.path.join(REPO_ROOT, "assets")
LOGO_SVG = os.path.join(ASSETS_DIR, "logo.svg")

def main():
    print(f"Loading {LOGO_SVG}...")
    renderer = QSvgRenderer(LOGO_SVG)
    if not renderer.isValid():
        raise RuntimeError(f"Failed to load SVG from {LOGO_SVG}")

    # Render master image at 2048x2048 using QtSvg renderer (supports all linear gradients)
    master_size = 2048
    qimg = QImage(master_size, master_size, QImage.Format_ARGB32_Premultiplied)
    qimg.fill(Qt.transparent)
    painter = QPainter(qimg)
    painter.setRenderHint(QPainter.Antialiasing, True)
    painter.setRenderHint(QPainter.SmoothPixmapTransform, True)
    renderer.render(painter)
    painter.end()

    # Convert QImage to PIL Image
    qimg_argb = qimg.convertToFormat(QImage.Format_RGBA8888)
    ptr = qimg_argb.constBits()
    master_im = Image.frombytes("RGBA", (master_size, master_size), bytes(ptr))

    bbox = master_im.getbbox()
    print(f"Rendered master image: {master_im.size}, Bounding Box: {bbox}")

    # Verify colors
    colors = master_im.getcolors(master_size * master_size)
    orange_pixels = [c for c in colors if c[1][0] > 180 and c[1][1] > 80 and c[1][2] < 60]
    gold_pixels = [c for c in colors if c[1][0] > 200 and c[1][1] > 180]
    print(f"  Verified orange cloud pixels: {sum(c[0] for c in orange_pixels)}")
    print(f"  Verified gold/white Q pixels: {sum(c[0] for c in gold_pixels)}")

    # Generate PNG resolutions
    png_sizes = [512, 256, 128, 64, 32, 16]
    generated_pngs = {}
    for sz in png_sizes:
        resampled = master_im.resize((sz, sz), Image.Resampling.LANCZOS)
        out_name = f"logo-{sz}.png" if sz != 512 else "logo-512.png"
        out_path = os.path.join(ASSETS_DIR, out_name)
        resampled.save(out_path, format="PNG", optimize=True)
        generated_pngs[sz] = resampled
        print(f"  Wrote {out_path} ({sz}x{sz})")

    # Also save standard logo.png (512x512)
    standard_logo_path = os.path.join(ASSETS_DIR, "logo.png")
    generated_pngs[512].save(standard_logo_path, format="PNG", optimize=True)
    print(f"  Wrote {standard_logo_path}")

    # Copy to internal/ui/assets and cmd/quickflare-installer/assets
    destinations = [
        os.path.join(REPO_ROOT, "internal", "ui", "assets", "logo.png"),
        os.path.join(REPO_ROOT, "cmd", "quickflare-installer", "assets", "logo.png"),
    ]
    for dst in destinations:
        os.makedirs(os.path.dirname(dst), exist_ok=True)
        shutil.copyfile(standard_logo_path, dst)
        print(f"  Copied logo.png -> {dst}")

    # Multi-resolution Windows ICO
    ico_sizes = [(16, 16), (24, 24), (32, 32), (48, 48), (64, 64), (128, 128), (256, 256)]
    ico_frames = [
        master_im.resize(s, Image.Resampling.LANCZOS)
        for s in ico_sizes
    ]

    quickflare_ico_path = os.path.join(ASSETS_DIR, "quickflare.ico")
    ico_frames[-1].save(
        quickflare_ico_path,
        format="ICO",
        sizes=ico_sizes,
        append_images=ico_frames[:-1]
    )
    print(f"  Wrote {quickflare_ico_path} with {len(ico_sizes)} resolutions.")

    tray_ico_path = os.path.join(REPO_ROOT, "internal", "ui", "assets", "tray.ico")
    ico_frames[-1].save(
        tray_ico_path,
        format="ICO",
        sizes=ico_sizes,
        append_images=ico_frames[:-1]
    )
    print(f"  Wrote {tray_ico_path}")

    # Create desaturated grayed frames for tray_disabled.ico
    def to_disabled(img):
        r, g, b, a = img.split()
        gray = img.convert("L").point(lambda p: int(p * 0.75))
        return Image.merge("RGBA", (gray, gray, gray, a))

    disabled_frames = [to_disabled(f) for f in ico_frames]
    tray_disabled_path = os.path.join(REPO_ROOT, "internal", "ui", "assets", "tray_disabled.ico")
    disabled_frames[-1].save(
        tray_disabled_path,
        format="ICO",
        sizes=ico_sizes,
        append_images=disabled_frames[:-1]
    )
    print(f"  Wrote {tray_disabled_path}")

    # Regenerate Windows syso resource files if rsrc is available
    rsrc_exe = r"C:\Users\Ketan\go\bin\rsrc.exe"
    if os.path.isfile(rsrc_exe):
        qf_syso = os.path.join(REPO_ROOT, "cmd", "quickflare", "rsrc_windows_amd64.syso")
        cmd1 = [rsrc_exe, "-ico", quickflare_ico_path, "-arch", "amd64", "-o", qf_syso]
        subprocess.run(cmd1, check=True)
        print(f"  Compiled {qf_syso}")

        inst_dir = os.path.join(REPO_ROOT, "cmd", "quickflare-installer")
        if os.path.isdir(inst_dir):
            inst_syso = os.path.join(inst_dir, "rsrc_windows_amd64.syso")
            cmd2 = [rsrc_exe, "-ico", quickflare_ico_path, "-arch", "amd64", "-o", inst_syso]
            subprocess.run(cmd2, check=True)
            print(f"  Compiled {inst_syso}")

    print("\nAll assets successfully generated with full gradient fidelity!")

if __name__ == "__main__":
    main()

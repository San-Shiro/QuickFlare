"""
QuickFlare Asset Generator
Generates all transparent PNG sizes, Windows ICO multi-resolution icons,
and embedded app assets from assets/logo.svg.
"""

import os
import shutil
import subprocess
import fitz
from PIL import Image

REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
ASSETS_DIR = os.path.join(REPO_ROOT, "assets")
LOGO_SVG = os.path.join(ASSETS_DIR, "logo.svg")

def main():
    print(f"Loading {LOGO_SVG}...")
    with open(LOGO_SVG, "r", encoding="utf-8") as f:
        svg_raw = f.read()

    # Step 1: Optical centering & framing
    # Measure raw vector artwork bounds
    doc_raw = fitz.open(stream=svg_raw.encode("utf-8"), filetype="svg")
    pix_raw = doc_raw[0].get_pixmap(alpha=True)
    im_raw = Image.frombytes("RGBA", [pix_raw.width, pix_raw.height], pix_raw.samples)
    bbox = im_raw.getbbox()
    print(f"Raw artwork bounds: {bbox}")

    w = bbox[2] - bbox[0]
    h = bbox[3] - bbox[1]
    cx = (bbox[0] + bbox[2]) / 2.0
    cy = (bbox[1] + bbox[3]) / 2.0
    print(f"Artwork dimensions: {w}x{h}, Center: ({cx:.1f}, {cy:.1f})")

    # Center at (256, 256) and scale to fill ~88% of 512 canvas (width = 450)
    target_w = 450.0
    scale = target_w / w
    tx = 256.0 - cx * scale
    ty = 256.0 - cy * scale
    print(f"Centering transform: translate({tx:.2f}, {ty:.2f}) scale({scale:.4f})")

    defs_idx = svg_raw.find("</defs>")
    if defs_idx != -1:
        before = svg_raw[:defs_idx + 7]
        after = svg_raw[defs_idx + 7:].rstrip()
        if after.endswith("</svg>"):
            after_body = after[:-6]
        else:
            after_body = after
        
        final_svg = (
            f'{before}\n'
            f'  <g transform="translate({tx:.2f}, {ty:.2f}) scale({scale:.4f})">\n'
            f'{after_body}\n'
            f'  </g>\n'
            f'</svg>\n'
        )
    else:
        final_svg = svg_raw

    # Write formatted SVG back to assets/logo.svg
    with open(LOGO_SVG, "w", encoding="utf-8") as f:
        f.write(final_svg)
    print(f"Updated {LOGO_SVG} with centered 512x512 transparent canvas.")

    # Step 2: Render 2048x2048 master raster for ultra-crisp downsampling
    doc = fitz.open(stream=final_svg.encode("utf-8"), filetype="svg")
    scale_factor = 2048.0 / 512.0
    mat = fitz.Matrix(scale_factor, scale_factor)
    pix = doc[0].get_pixmap(matrix=mat, alpha=True)
    master_im = Image.frombytes("RGBA", [pix.width, pix.height], pix.samples)
    print(f"Rendered master image: {master_im.size}, Bounding Box: {master_im.getbbox()}")

    # Step 3: Generate PNG resolutions
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

    # Step 4: Multi-resolution Windows ICO
    ico_sizes = [(16, 16), (24, 24), (32, 32), (48, 48), (64, 64), (128, 128), (256, 256)]
    ico_frames = [
        master_im.resize(s, Image.Resampling.LANCZOS)
        for s in ico_sizes
    ]

    # Save assets/quickflare.ico
    quickflare_ico_path = os.path.join(ASSETS_DIR, "quickflare.ico")
    ico_frames[-1].save(
        quickflare_ico_path,
        format="ICO",
        sizes=ico_sizes,
        append_images=ico_frames[:-1]
    )
    print(f"  Wrote {quickflare_ico_path} with {len(ico_sizes)} resolutions.")

    # Save internal/ui/assets/tray.ico
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

    # Step 5: Regenerate Windows syso resource files if rsrc is available
    rsrc_exe = r"C:\Users\Ketan\go\bin\rsrc.exe"
    if os.path.isfile(rsrc_exe):
        # cmd/quickflare/rsrc_windows_amd64.syso
        qf_syso = os.path.join(REPO_ROOT, "cmd", "quickflare", "rsrc_windows_amd64.syso")
        cmd1 = [rsrc_exe, "-ico", quickflare_ico_path, "-arch", "amd64", "-o", qf_syso]
        subprocess.run(cmd1, check=True)
        print(f"  Compiled {qf_syso}")

        # cmd/quickflare-installer/rsrc_windows_amd64.syso
        inst_dir = os.path.join(REPO_ROOT, "cmd", "quickflare-installer")
        if os.path.isdir(inst_dir):
            inst_syso = os.path.join(inst_dir, "rsrc_windows_amd64.syso")
            cmd2 = [rsrc_exe, "-ico", quickflare_ico_path, "-arch", "amd64", "-o", inst_syso]
            subprocess.run(cmd2, check=True)
            print(f"  Compiled {inst_syso}")

    print("\nAll assets successfully generated!")

if __name__ == "__main__":
    main()

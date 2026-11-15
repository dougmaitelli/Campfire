#!/usr/bin/env python3
"""Render an editorial Campfire product film from repository UI screenshots.

No live interactions or performance measurements are implied by these captures.
The music is synthesized locally; no external recordings or samples are used.
"""

import argparse
from functools import lru_cache
import json
import math
from pathlib import Path
import subprocess
import wave

import imageio_ffmpeg
import numpy as np
from PIL import Image, ImageDraw, ImageFont

ROOT = Path(__file__).resolve().parents[2]
W, H = 1920, 1080
DURATIONS = [4, 6, 4, 4, 4, 6, 6, 6, 4, 6, 6, 6, 6, 4, 6]
STARTS = [sum(DURATIONS[:i]) for i in range(len(DURATIONS))]
DURATION = sum(DURATIONS)
ORANGE = (255, 163, 83)
WHITE = (244, 238, 225)
MUTED = (176, 177, 162)
SCENES = [
    dict(name="Meet Campfire", title="Your servers. Your rules.", subtitle="Self-hosted game server management."),
    dict(name="Overview", title="See the whole picture.", subtitle="Server status. Resource usage. One dashboard.",
         source="screenshots/pages/dashboard-wide.png", accent="ONE PANEL. MULTIPLE HOSTS."),
    dict(name="Create / Game & host", title="Start your next world.", subtitle="Choose an Ember, name your server, and pick a Spark.",
         source="scripts/demo/assets/create-general.png", accent="YOUR GAME. YOUR HOST."),
    dict(name="Create / Configuration", title="Make it yours.", subtitle="Set the game variables before you launch.",
         source="scripts/demo/assets/create-variables.png", accent="CONFIGURE WITH CONFIDENCE."),
    dict(name="Create / Review", title="Review. Then create.", subtitle="Check the configuration in one place.",
         source="scripts/demo/assets/create-review.png", accent="READY FOR YOUR NEXT SERVER."),
    dict(name="Console", title="Stay in command.", subtitle="Live console output. Commands from your browser.",
         source="screenshots/pages/server-console.png", accent="LESS GUESSWORK. MORE CONTROL."),
    dict(name="Files", title="Make it your world.", subtitle="Browse, upload, and manage your game files.",
         source="screenshots/pages/server-files.png", accent="YOUR FILES. WITHIN REACH."),
    dict(name="Backups", title="Protect what you build.", subtitle="Full-server or save-data backups. Optional off-host storage.",
         source="screenshots/pages/server-backups.png", accent="BUILD. BACK UP. KEEP PLAYING."),
    dict(name="Transfers / Destination", title="Move to the right host.", subtitle="Choose a destination Spark for your server.",
         source="scripts/demo/assets/transfer-destination.png", accent="MOVE BETWEEN YOUR SPARKS."),
    dict(name="Schedules", title="Put routine on repeat.", subtitle="Schedule server actions. Follow every execution.",
         source="screenshots/pages/schedules.png", accent="SET THE SCHEDULE. RECLAIM YOUR TIME."),
    dict(name="Discord / Publishing", title="Choose what to share.", subtitle="Select notification categories, event types, and a destination channel.",
         source="scripts/demo/assets/publishing-settings.png", accent="KEEP YOUR COMMUNITY CONNECTED."),
    dict(name="Discord / Card preview", title="A live view for your community.", subtitle="Choose the fields on your persistent Discord server card.",
         source="scripts/demo/assets/server-card-preview.png", accent="PREVIEW BEFORE YOU PUBLISH."),
    dict(name="Game activity", title="Follow the action.", subtitle="Player arrivals, achievements, and triggered notifications in one view.",
         source="scripts/demo/assets/game-activity.png", accent="FROM CONFIGURATION TO ACTIVITY."),
    dict(name="Game activity / Notifications", title="See where it went.", subtitle="Open an event to see its details and delivered notifications.",
         source="scripts/demo/assets/game-event.png", accent="ONE EVENT. MULTIPLE DESTINATIONS."),
    dict(name="Get started", title="Less managing.\nMore playing.", subtitle="Meet your next game server panel."),
]


def ease(value):
    value = max(0.0, min(1.0, value))
    return 1 - (1 - value) ** 3


def smooth(value):
    value = max(0.0, min(1.0, value))
    return value * value * (3 - 2 * value)


@lru_cache(maxsize=64)
def font(size, bold=False):
    return ImageFont.truetype(str(FONT_PATH), size, index=BOLD_INDEX if bold else REGULAR_INDEX)


def text(image, value, xy, size, color=WHITE, bold=False, opacity=1.0):
    layer = Image.new("RGBA", (W, H))
    draw = ImageDraw.Draw(layer)
    draw.multiline_text(xy, value, font=font(size, bold), fill=(*color, int(255 * max(0, min(1, opacity)))), spacing=4, anchor="la")
    image.alpha_composite(layer)


def reveal(image, value, xy, size, local, delay=0, color=WHITE, bold=False):
    p = ease((local - delay) / 0.65)
    text(image, value, (xy[0], xy[1] + 24 * (1 - p)), size, color, bold, p)


def centered_reveal(image, value, y, size, local, delay=0, color=WHITE, bold=False):
    x = (W - font(size, bold).getlength(value)) / 2
    reveal(image, value, (x, y), size, local, delay, color, bold)


def build_background():
    y, x = np.mgrid[0:H, 0:W].astype(np.float32)
    glow = np.exp(-((x - 1540) ** 2 / 900 ** 2 + (y - 290) ** 2 / 650 ** 2))
    lower = np.exp(-((x - 200) ** 2 / 900 ** 2 + (y - 1100) ** 2 / 500 ** 2))
    pixels = np.zeros((H, W, 3), dtype=np.float32) + [17, 19, 17]
    pixels += glow[..., None] * [27, 12, 4]
    pixels += lower[..., None] * [6, 8, 5]
    return Image.fromarray(pixels.astype(np.uint8)).convert("RGBA")


BACKGROUND = build_background()
ASSETS = {}
LOGO = Image.open(ROOT / "web/public/brand/campfire.png").convert("RGBA")
LOGO = LOGO.crop(LOGO.getbbox())


@lru_cache(maxsize=12)
def logo(size):
    height = round(size * LOGO.height / LOGO.width)
    return LOGO.resize((size, height), Image.Resampling.LANCZOS)


def background(t, chapter):
    image = BACKGROUND.copy()
    draw = ImageDraw.Draw(image)
    # Restrained drifting embers, not confetti.
    for n in range(18):
        x = 1070 + (n * 137.3 + math.sin(t * 0.16 + n) * 35) % 800
        y = (960 - n * 63 - t * (6 + n % 4)) % 1000
        brightness = int(40 + 23 * math.sin(t * 0.7 + n) ** 2)
        draw.ellipse((x, y, x + 2, y + 2), fill=(brightness + 38, brightness, 30))
    image.alpha_composite(logo(28), (80, 53))
    text(image, "Campfire", (125, 50), 30, bold=True)
    text(image, "SELF-HOSTED. GAME-READY.", (125, 91), 14, MUTED)
    text(image, f"{chapter:02d}  /  {len(SCENES):02d}", (1740, 62), 20, MUTED)
    draw.line((80, 1022, 1840, 1022), fill=(57, 54, 46), width=1)
    draw.line((80, 1022, 80 + 1760 * t / DURATION, 1022), fill=ORANGE, width=2)
    text(image, "CAMPFIRE  /  PRODUCT TOUR", (80, 1040), 14, MUTED)
    return image


def source(scene):
    key = scene["source"]
    if key not in ASSETS:
        ASSETS[key] = Image.open(ROOT / key).convert("RGB")
    return ASSETS[key]


@lru_cache(maxsize=16)
def page_view(path, width, height):
    src = source({"source": path})
    scale = min(width / src.width, height / src.height)
    # Fit the complete screenshot, including navigation and page controls.
    # No source cropping, reconstructed browser chrome, or camera zoom.
    return src.resize(
        (round(src.width * scale), round(src.height * scale)),
        Image.Resampling.LANCZOS,
    ).convert("RGBA")


def screen(image, scene, local, rect=(580, 145, 1850, 980)):
    x0, y0, x1, y1 = rect
    view = page_view(scene["source"], x1 - x0, y1 - y0).copy()
    view.putalpha(round(255 * ease(local / 0.65)))
    x = x0 + (x1 - x0 - view.width) // 2
    y = y0 + (y1 - y0 - view.height) // 2
    image.alpha_composite(view, (x, y))


def wrapped(value, size, width, bold=False):
    lines = []
    for paragraph in value.split("\n"):
        line = ""
        for word in paragraph.split():
            candidate = f"{line} {word}".strip()
            if line and font(size, bold).getlength(candidate) > width:
                lines.append(line)
                line = word
            else:
                line = candidate
        lines.append(line)
    return "\n".join(lines)


def scene_frame(index, local):
    scene = SCENES[index]
    image = background(STARTS[index] + local, index + 1)
    if index == 0:
        reveal(image, "YOUR GAME SERVER PANEL", (84, 251), 22, local, color=ORANGE, bold=True)
        reveal(image, "Your servers.", (76, 326), 118, local, 0.12, bold=True)
        reveal(image, "Your rules.", (76, 463), 118, local, 0.34, ORANGE, True)
        reveal(image, "Meet Campfire.", (84, 660), 40, local, 0.65)
        reveal(image, "Self-hosted game server management.", (84, 723), 27, local, 0.85, MUTED)
        p = ease(local / 1.1)
        mark = logo(round(340 + 24 * p)).copy()
        mark.putalpha(mark.getchannel("A").point(lambda a: int(a * p)))
        image.alpha_composite(mark, (1335, round(297 + 25 * (1 - p))))
        draw = ImageDraw.Draw(image)
        draw.line((84, 830, 84 + 410 * ease((local - 1.1) / 0.8), 830), fill=ORANGE, width=3)
    elif index == len(SCENES) - 1:
        image.alpha_composite(logo(160), (880, 145))
        centered_reveal(image, "Less managing.", 332, 108, local, 0.1, bold=True)
        centered_reveal(image, "More playing.", 461, 108, local, 0.35, ORANGE, True)
        centered_reveal(image, "Self-hosted. Open source. Campfire.", 632, 33, local, 0.6)
        p = ease((local - 1) / 0.6)
        button = Image.new("RGBA", (W, H))
        bd = ImageDraw.Draw(button)
        bd.rounded_rectangle((490, 743, 1430, 834), radius=16, fill=(*ORANGE, round(255 * p)))
        image.alpha_composite(button)
        url = "github.com/dougmaitelli/Campfire"
        text(image, url, ((W - font(35, True).getlength(url)) / 2, 767), 35, (25, 26, 22), True, p)
        centered_reveal(image, "Bring your next world to life.", 888, 26, local, 1.4, MUTED)
    else:
        reveal(image, scene["name"].upper(), (84, 230), 20, local, color=ORANGE, bold=True)
        reveal(image, wrapped(scene["title"], 58, 455, True), (78, 306), 58, local, 0.12, bold=True)
        reveal(image, wrapped(scene["subtitle"], 27, 440), (84, 570), 27, local, 0.35, MUTED)
        screen(image, scene, local)
        reveal(image, wrapped(scene["accent"], 15, 440, True), (84, 850), 15, local, 0.7, ORANGE, True)
    return image.convert("RGB")


def frame(t):
    index = max(i for i, start in enumerate(STARTS) if start <= t)
    local = t - STARTS[index]
    image = scene_frame(index, local)
    # Six-frame editorial dissolve at 30 fps; nothing flashes to white.
    if index and local < 0.2:
        previous = scene_frame(index - 1, STARTS[index] - STARTS[index - 1] - 1 / 30)
        image = Image.blend(previous, image, smooth(local / 0.2))
    if t < 0.3:
        image = Image.blend(Image.new("RGB", (W, H), (17, 19, 17)), image, ease(t / 0.3))
    if t > DURATION - 0.5:
        image = Image.blend(image, Image.new("RGB", (W, H), (17, 19, 17)), smooth((t - DURATION + 0.5) / 0.5))
    return image


def make_music(path):
    """Original 120 BPM D-minor electronic cue, deterministic and sample-free."""
    rate = 48000
    audio = np.zeros((DURATION * rate, 2), dtype=np.float32)
    rng = np.random.default_rng(2718)

    def place(signal, start, gain=1.0, pan=0.0):
        offset = round(start * rate)
        length = min(len(signal), len(audio) - offset)
        if length <= 0:
            return
        stereo = np.array([math.sqrt((1 - pan) / 2), math.sqrt((1 + pan) / 2)])
        audio[offset:offset + length] += signal[:length, None] * stereo * gain

    def hz(midi):
        return 440 * 2 ** ((midi - 69) / 12)

    chords = [(50, 57, 62, 65), (46, 53, 58, 62), (41, 53, 57, 60), (48, 55, 60, 64)]
    outro = STARTS[-1]
    for bar in range(math.ceil(DURATION / 4)):
        chord = chords[0] if bar * 4 >= outro else chords[bar % 4]
        start = bar * 4
        t = np.arange(round(4.4 * rate)) / rate
        env = np.minimum(t / 0.28, 1) * np.minimum((4.4 - t) / 0.8, 1)
        for j, midi in enumerate(chord[1:]):
            tone = (np.sin(2 * np.pi * hz(midi) * t) + 0.22 * np.sin(2 * np.pi * hz(midi) * 2 * t)) * env
            place(tone, start, 0.035, (j - 1) * 0.45)
        for eighth in range(16):
            when = start + eighth * 0.25
            if when >= outro + 2:
                continue
            note = chord[[1, 2, 3, 2, 1, 3, 2, 3][eighth % 8]] + 12
            t = np.arange(round(0.7 * rate)) / rate
            env = (1 - np.exp(-t * 450)) * np.exp(-t * 8)
            tone = (np.sin(2 * np.pi * hz(note) * t) + 0.2 * np.sin(2 * np.pi * hz(note) * 3 * t)) * env
            pan = -0.35 if eighth % 2 == 0 else 0.35
            place(tone, when, 0.095, pan)
            place(tone, when + 0.375, 0.024, -pan)
        for beat in range(8):
            when = start + beat * 0.5
            if not 4 <= when < outro:
                continue
            t = np.arange(round(0.42 * rate)) / rate
            bass = np.sin(2 * np.pi * hz(chord[0] - 12) * t) * np.minimum(t / 0.008, 1) * np.exp(-t * 6)
            place(bass, when + 0.25, 0.13)

    for beat in range(DURATION * 2):
        when = beat * 0.5
        if not 4 <= when < outro:
            continue
        t = np.arange(round(0.3 * rate)) / rate
        phase = 2 * np.pi * (48 * t + 90 * 0.018 * (1 - np.exp(-t / 0.018)))
        kick = np.sin(phase) * np.exp(-t * 18) * np.minimum(t / 0.002, 1)
        place(kick, when, 0.38)
        if beat % 2:
            t = np.arange(round(0.17 * rate)) / rate
            noise = rng.normal(0, 1, len(t))
            noise = np.convolve(noise, np.ones(5) / 5, mode="same")
            snare = (noise * 0.6 + np.sin(2 * np.pi * 185 * t) * 0.3) * np.exp(-t * 28) * np.minimum(t / 0.001, 1)
            place(snare, when, 0.20, 0.07)
        for off in (0, 0.25):
            t = np.arange(round(0.075 * rate)) / rate
            noise = rng.normal(0, 1, len(t))
            hat = np.diff(noise, prepend=noise[0]) * np.exp(-t * 70) * np.minimum(t / 0.001, 1)
            place(hat, when + off, 0.025 if off == 0 else 0.036, -0.3 if off == 0 else 0.3)
    # A resolved final brand chord, not an abrupt loop ending.
    t = np.arange(6 * rate) / rate
    for midi in (50, 57, 62, 65, 69):
        place(np.sin(2 * np.pi * hz(midi) * t) * np.exp(-t * 0.6) * np.minimum(t / 0.012, 1), outro, 0.075)
    timeline = np.arange(len(audio)) / rate
    envelope = np.minimum(timeline / 0.35, 1) * np.minimum((DURATION - timeline) / 2, 1)
    audio *= envelope[:, None]
    peak = float(np.abs(audio).max())
    audio *= 0.78 / max(peak, 0.01)
    pcm = (np.clip(audio, -1, 1) * 32767).astype("<i2")
    with wave.open(str(path), "wb") as output:
        output.setnchannels(2)
        output.setsampwidth(2)
        output.setframerate(rate)
        output.writeframes(pcm.tobytes())


def timecode(seconds):
    return f"00:{int(seconds) // 60:02d}:{int(seconds) % 60:02d},000"


def write_supporting_files(output):
    frame(2.7).save(output / "campfire-demo-poster.jpg", quality=95)
    sheet = Image.new("RGB", (1280, math.ceil(len(SCENES) / 2) * 392), (17, 19, 17))
    draw = ImageDraw.Draw(sheet)
    for i, start in enumerate(STARTS):
        preview = frame(start + 2).resize((640, 360), Image.Resampling.LANCZOS)
        x, y = (i % 2) * 640, (i // 2) * 392
        sheet.paste(preview, (x, y))
        draw.text((x + 16, y + 364), f"{start:02d}s  /  {SCENES[i]['name']}", font=font(16), fill=WHITE)
    sheet.save(output / "campfire-demo-storyboard.jpg", quality=94)
    captions = []
    for i, scene in enumerate(SCENES):
        end = STARTS[i + 1] if i + 1 < len(STARTS) else DURATION
        title = scene["title"].replace("\n", " ")
        subtitle = scene["subtitle"].replace("\n", " ")
        captions.append(f"{i + 1}\n{timecode(STARTS[i])} --> {timecode(end)}\n{title}\n{subtitle}\n")
    (output / "campfire-demo.srt").write_text("\n".join(captions))
    (output / "manifest.json").write_text(json.dumps({
        "duration_seconds": DURATION, "dimensions": [W, H],
        "footage": "Animated repository screenshots with fixture data; not a live session recording.",
        "audio": "Original locally synthesized instrumental; no external samples or recordings.",
        "scenes": [dict(start=STARTS[i], **scene) for i, scene in enumerate(SCENES)],
    }, indent=2) + "\n")


def main():
    global FONT_PATH, BOLD_INDEX, REGULAR_INDEX
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, default=ROOT / "dist/demo")
    parser.add_argument("--fps", type=int, default=60, choices=[24, 30, 60])
    parser.add_argument("--preview", action="store_true", help="Render poster and storyboard only")
    parser.add_argument("--font", type=Path, default=Path("/System/Library/Fonts/Avenir Next.ttc"))
    parser.add_argument("--regular-index", type=int, default=7)
    parser.add_argument("--bold-index", type=int, default=0)
    args = parser.parse_args()
    FONT_PATH, BOLD_INDEX, REGULAR_INDEX = args.font, args.bold_index, args.regular_index
    if not FONT_PATH.is_file():
        parser.error("Pass --font with a local font file and the appropriate --regular-index/--bold-index.")
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=True)
    write_supporting_files(output)
    if args.preview:
        print(output / "campfire-demo-storyboard.jpg")
        return
    music = output / "campfire-demo-music.wav"
    make_music(music)
    ffmpeg = imageio_ffmpeg.get_ffmpeg_exe()
    target = output / "campfire-demo.mp4"
    command = [ffmpeg, "-y", "-hide_banner", "-loglevel", "warning",
               "-f", "rawvideo", "-vcodec", "rawvideo", "-pix_fmt", "rgb24",
               "-s", f"{W}x{H}", "-r", str(args.fps), "-i", "pipe:0", "-i", str(music),
               "-map", "0:v:0", "-map", "1:a:0", "-c:v", "libx264", "-preset", "medium", "-crf", "18",
               "-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "192k",
               "-af", "loudnorm=I=-18:TP=-1.5:LRA=7", "-ar", "48000",
               "-t", str(DURATION), "-movflags", "+faststart",
               "-metadata", "title=Campfire — Your servers. Your rules.",
               "-metadata", "comment=Product tour using UI screenshots and demo data. Original synthesized music.", str(target)]
    with (output / "encode.log").open("w") as log:
        process = subprocess.Popen(command, stdin=subprocess.PIPE, stderr=log)
        try:
            for n in range(DURATION * args.fps):
                process.stdin.write(frame(n / args.fps).tobytes())
                if n % (args.fps * 4) == 0:
                    print(f"Rendered {n // args.fps:02d}/{DURATION}s", flush=True)
            process.stdin.close()
            if process.wait() != 0:
                raise RuntimeError(f"FFmpeg failed; inspect {output / 'encode.log'}")
        except BaseException:
            process.kill()
            process.wait()
            raise
    # Decode every frame and audio packet; a successful encode is not enough.
    subprocess.run([ffmpeg, "-v", "error", "-i", str(target), "-f", "null", "-"], check=True)
    print(f"Verified: {target} ({target.stat().st_size / 1024 ** 2:.1f} MiB)")


if __name__ == "__main__":
    main()

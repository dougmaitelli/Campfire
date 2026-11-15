# Campfire product demo

A 78-second, 1920×1080 product film with animated typography, full UI screenshots,
short dissolves, a restrained ember background, and an original 120 BPM electronic
music bed. The default export is 60 fps H.264/AAC, with web-friendly fast start.
Pages are fitted in full beside the captions and held steady for readability,
without cropping or zooming. Creation, transfers, activity, and Discord settings
use full-viewport captures from the real UI running against the disposable E2E
backend. The Discord card shot is Campfire's built-in preview, not the Discord app.

## What the footage shows

This is an **edited screenshot-based product tour**, not a recording of a live
session. UI imagery combines existing browser-test screenshots with captures in
`assets/` produced by `capture.mjs`. All contain fixture data. There is no on-screen
capture label. The creation wizard is exercised through successful submission in
the disposable backend; transfer progress uses the same deterministic fixture as
the browser tests and is not a throughput or completion-time claim. The card's
fresh game-state values also follow the integration tests' fixture. No production
services or real Discord accounts are used. The activity event is published
through the test backend's real notification queue to two mock destinations;
both deliveries must succeed before the event drawer is captured.

| Time  | Sequence                                                      |
| ----- | ------------------------------------------------------------- |
| 00–04 | Your servers. Your rules.                                     |
| 04–10 | Full overview page                                            |
| 10–22 | Create a server: game/host, variables, and review             |
| 22–28 | Full server console page                                      |
| 28–34 | Full file-management page                                     |
| 34–40 | Server backups                                                |
| 40–44 | Transfer destination                                          |
| 44–50 | Scheduled operations and execution history                    |
| 50–56 | Discord notification categories, event types, and destination |
| 56–62 | Persistent server-card settings and preview                   |
| 62–68 | Game activity with triggered notification counts              |
| 68–72 | Event details with two delivered notifications                |
| 72–78 | Less managing. More playing. / project URL                    |

The music is synthesized from oscillators and deterministic noise in `render.py`.
It contains no downloaded recordings, stock music, third-party samples, or voice.
The Campfire logo and screenshots of Campfire are the only visual source assets.

## Render

From the repository root:

```sh
python3 -m venv .cache/demo/venv
.cache/demo/venv/bin/pip install -r scripts/demo/requirements.txt
.cache/demo/venv/bin/python scripts/demo/render.py
```

The default font is the locally installed macOS Avenir Next collection. The font
file is **not** redistributed. On another platform, supply a local font file and
its collection indices, for example:

```sh
.cache/demo/venv/bin/python scripts/demo/render.py \
  --font /path/to/font.ttf --regular-index 0 --bold-index 0
```

Use `--preview` to generate the poster and storyboard without encoding a video.
Use `--output /path/to/output` or `--fps 24` to change export settings. FFmpeg is
provided by `imageio-ffmpeg`; no global FFmpeg installation is necessary.

## Refresh the UI captures

The included source assets can be rendered without running Campfire. To refresh
them, first start the disposable browser-test backend at `http://127.0.0.1:8099`.
On Linux, `node e2e/scripts/start.mjs` builds and runs it after installing the web
and E2E dependencies. On macOS, the Linux backend can run in Docker with a local
port forward; this does not require a real Spark or game server.

With that test backend running and Google Chrome installed:

```sh
pnpm --dir e2e install --frozen-lockfile
node scripts/demo/capture.mjs
```

**This resets the E2E fixture database.** The capture script is pinned to the local
test URL and checks its test-only reset endpoint before signing in. It uses fake
credentials and a mock Discord provider. It never contacts Discord itself.
Re-render the film after refreshing the assets.

## Outputs

All generated assets go into the gitignored `dist/demo/` directory:

- `campfire-demo.mp4`: finished video with the original instrumental soundtrack.
- `campfire-demo-poster.jpg`: full-resolution poster.
- `campfire-demo-storyboard.jpg`: fifteen-scene contact sheet.
- `campfire-demo.srt`: optional subtitle track reproducing the on-screen copy;
  there is no spoken narration.
- `campfire-demo-music.wav`: uncompressed original music, before loudness normalization.
- `manifest.json`: scene timing, copy, and source paths.
- `encode.log`: encoder diagnostics.

The soundtrack in the MP4 is normalized toward −18 LUFS with a −1.5 dBTP ceiling.
After encoding, the renderer decodes the entire video and audio stream with FFmpeg
and fails if decoding reports an error. Review the actual export before publishing;
this technical check does not replace editorial review or listening to the audio.

To make a silent copy without re-encoding the picture:

```sh
FFMPEG=$(.cache/demo/venv/bin/python -c 'import imageio_ffmpeg; print(imageio_ffmpeg.get_ffmpeg_exe())')
"$FFMPEG" -i dist/demo/campfire-demo.mp4 -c:v copy -an -movflags +faststart \
  dist/demo/campfire-demo-silent.mp4
```

The renderer does not start an app server, contact production services, or publish
anything. The separate capture script interacts only with the local E2E harness. Edit the scene definitions in `render.py` to change the story or wording.

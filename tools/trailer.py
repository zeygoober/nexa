#!/usr/bin/env python3
"""Raw footage of Nexa for a trailer, filmed from the app running for real.

Every clip is a person using Nexa on a throwaway server full of made-up people:
the cursor glides from place to place, words are typed at a person's pace, and
nothing is cut. Headless Chrome films its own tab (getDisplayMedia with
preferCurrentTab, then MediaRecorder) at 2560x1440, so no screen-recording
permission is involved and nothing appears on screen, and ffmpeg turns each
take into a 1920x1080, 60 fps H.264 file without sound. Phone clips are
1080 wide and portrait.

Clips that show something Discord does not offer in any form go to unique/,
everyday use to general/, and README.txt in the output folder says what each
one shows.

  tools/.venv/bin/python tools/trailer.py [--out examplevideos] [--only clip,clip] [--list]

KLIPY_API_KEY and TELEGRAM_BOT_TOKEN are read from chatd.env when it has them,
for the GIF search and Telegram import clips (both need the internet). E2E_CHATD
and E2E_WEB_DIST (see tools/e2e.py) film a build made somewhere else.
"""
import argparse, asyncio, math, os, random, shutil, socket, ssl, subprocess, sys, threading, time, traceback
from contextlib import contextmanager
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(ROOT / "tools"))
from e2e import Server, Session  # noqa: E402
from playwright.sync_api import sync_playwright  # noqa: E402

# The layout is 1280x720 CSS pixels, drawn at twice that and delivered at
# 1920x1080: big enough to read on a phone, sharper than drawing at 1080p.
VIEW, SCALE, FINAL = (1280, 720), 2, (1920, 1080)
# Chrome films a tab at no more than twice its CSS size, so a phone is drawn
# 540 CSS pixels wide at twice that: 1080 pixels across, as delivered.
PHONE, PHONE_SCALE = (540, 1170), 2
# The address the footage shows: the throwaway server answers as Nexa's own,
# through TLS on a local port that headless Chrome sends nexa.pm to.
ADDRESS = "https://nexa.pm"
WORK = Path(os.environ.get("TRAILER_WORK", "/tmp/nexa-trailer"))
random.seed(7)

# ---------------------------------------------------------------------------
# In the page: a cursor people can see, and the recorder
# ---------------------------------------------------------------------------

# Headless Chrome has no pointer of its own on screen, so the page draws one
# that follows the mouse: an arrow, a hand over things that can be clicked and
# a text beam over text boxes, hidden while typing as macOS does. Phones show a
# touch instead. The recorder films the tab it runs in.
FILM_JS = r"""
(() => {
  if (window.top !== window || window.__film) return;
  const md = navigator.mediaDevices;
  const realGDM = md && md.getDisplayMedia ? md.getDisplayMedia.bind(md) : null;
  const TOUCH = %TOUCH%;
  const INK = '#15131b';
  // macOS's own pointers at the size macOS draws them: the arrow is 18 CSS
  // pixels tall, about 27 in the 1080p film, as on a Mac's screen
  const SHAPES = {
    arrow: { x: 1.2, y: 1.2, svg: `<svg xmlns="http://www.w3.org/2000/svg" width="13" height="19" viewBox="0 0 13.7 20"><path d="M1.3 1.3v15.4l3.7-3.6 2.6 6 2.6-1.1-2.5-5.9h5.2z" fill="${INK}" stroke="#fff" stroke-width="1.35" stroke-linejoin="round" paint-order="stroke"/></svg>` },
    hand: { x: 6.1, y: 1.4, svg: `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="18.5" viewBox="0 0 26 30"><path d="M10 2.2c-1.2 0-2.1.9-2.1 2.1v10.6l-1.5-1.6c-.9-.9-2.3-.9-3.1 0-.8.8-.8 2-.1 2.9l4.8 6.2c1.4 1.8 3.5 2.8 5.8 2.8h2.9c3.4 0 6.1-2.7 6.1-6.1v-5.7c0-1.1-.9-1.9-1.9-1.9-1.1 0-1.9.9-1.9 1.9v-.9c0-1.1-.9-1.9-1.9-1.9-1.1 0-1.9.9-1.9 1.9v-.6c0-1.1-.9-1.9-1.9-1.9s-1.9.9-1.9 1.9V4.3c0-1.2-.9-2.1-2.1-2.1z" fill="#fff" stroke="${INK}" stroke-width="1.7" stroke-linejoin="round"/><path d="M12.1 13.4v4.4M16 13.8v4M19.8 14.4v3.4" stroke="${INK}" stroke-width="1.5" stroke-linecap="round"/></svg>` },
    beam: { x: 5.5, y: 8.5, svg: `<svg xmlns="http://www.w3.org/2000/svg" width="11" height="17" viewBox="0 0 20 26"><g fill="none" stroke-linecap="round" stroke-linejoin="round"><path d="M6 3h1.4c1.2 0 2.1.6 2.6 1.5.5-.9 1.4-1.5 2.6-1.5H14M10 5v16M6 23h1.4c1.2 0 2.1-.6 2.6-1.5.5.9 1.4 1.5 2.6 1.5H14" stroke="#fff" stroke-width="4"/><path d="M6 3h1.4c1.2 0 2.1.6 2.6 1.5.5-.9 1.4-1.5 2.6-1.5H14M10 5v16M6 23h1.4c1.2 0 2.1-.6 2.6-1.5.5.9 1.4 1.5 2.6 1.5H14" stroke="${INK}" stroke-width="1.8"/></g></svg>` },
  };
  const CSS = `
*{-webkit-tap-highlight-color:transparent!important}
#__film-cursor{position:fixed;left:0;top:0;width:0;height:0;z-index:2147483647;pointer-events:none;opacity:0;transition:opacity 140ms ease;contain:layout style}
#__film-cursor[data-on]{opacity:1}
#__film-cursor .__film-shape{position:absolute;display:none;filter:drop-shadow(0 1px 1.2px rgba(0,0,0,.32));transition:transform 70ms ease}
#__film-cursor .__film-shape[data-on]{display:block}
#__film-cursor[data-down] .__film-shape{transform:scale(.94)}
#__film-cursor .__film-ring{position:absolute;left:-17px;top:-17px;width:34px;height:34px;border-radius:50%;border:2px solid rgba(255,255,255,.8);box-shadow:0 0 0 1px rgba(0,0,0,.2);opacity:0;pointer-events:none}
#__film-cursor .__film-ring[data-go]{display:none}
@keyframes __film-ring{from{opacity:.9;transform:scale(.4)}to{opacity:0;transform:scale(1.15)}}
#__film-touch{position:fixed;left:0;top:0;width:44px;height:44px;margin:-22px 0 0 -22px;border-radius:50%;background:rgba(255,255,255,.38);box-shadow:0 0 0 1.5px rgba(255,255,255,.7),0 4px 14px rgba(0,0,0,.3);z-index:2147483647;pointer-events:none;opacity:0;transform:scale(.6);transition:opacity 160ms ease,transform 160ms ease}
#__film-touch[data-on]{opacity:1;transform:scale(1)}`;
  let root = null, touchDot = null, shapes = {}, ring = null, kind = 'arrow', x = -200, y = -200;
  function build() {
    if (root && root.isConnected) return;
    const host = document.documentElement;
    if (!host) return;
    if (!document.getElementById('__film-style')) {
      const st = document.createElement('style');
      st.id = '__film-style';
      st.textContent = CSS;
      host.appendChild(st);
    }
    if (TOUCH) {
      touchDot = document.createElement('div');
      touchDot.id = '__film-touch';
      root = touchDot;
      host.appendChild(touchDot);
      return;
    }
    root = document.createElement('div');
    root.id = '__film-cursor';
    for (const [name, s] of Object.entries(SHAPES)) {
      const el = document.createElement('div');
      el.className = '__film-shape';
      el.style.left = `${-s.x}px`;
      el.style.top = `${-s.y}px`;
      el.style.transformOrigin = `${s.x}px ${s.y}px`;
      el.innerHTML = s.svg;
      shapes[name] = el;
      root.appendChild(el);
    }
    ring = document.createElement('div');
    ring.className = '__film-ring';
    root.appendChild(ring);
    shapes[kind].setAttribute('data-on', '');
    host.appendChild(root);
  }
  function place() {
    build();
    if (root) root.style.transform = `translate3d(${x}px, ${y}px, 0)`;
  }
  function shapeFor(t) {
    if (!t || t.id === '__film-arm') return 'arrow';
    const c = getComputedStyle(t).cursor;
    if (c === 'pointer' || c === 'grab' || c === 'grabbing') return 'hand';
    if (c === 'text' || c === 'vertical-text') return 'beam';
    if (c === 'auto') {
      const tag = t.tagName;
      if ((tag === 'INPUT' && !/^(button|submit|checkbox|radio|range|color|file|reset|image)$/i.test(t.type)) || tag === 'TEXTAREA' || t.isContentEditable) return 'beam';
    }
    return 'arrow';
  }
  function setShape(k) {
    if (TOUCH || k === kind) return;
    shapes[kind] && shapes[kind].removeAttribute('data-on');
    kind = k;
    shapes[kind] && shapes[kind].setAttribute('data-on', '');
  }
  addEventListener('mousemove', (e) => {
    if (TOUCH) return;
    x = e.clientX; y = e.clientY;
    place();
    root.setAttribute('data-on', '');
    setShape(shapeFor(document.elementFromPoint(x, y)));
  }, { capture: true, passive: true });
  addEventListener('mousedown', (e) => {
    if (TOUCH || (e.target && e.target.id === '__film-arm')) return;
    build();
    root.setAttribute('data-down', '');
    ring.removeAttribute('data-go');
    void ring.offsetWidth;
    ring.setAttribute('data-go', '');
  }, { capture: true, passive: true });
  addEventListener('mouseup', () => { if (root && !TOUCH) root.removeAttribute('data-down'); }, { capture: true, passive: true });
  // macOS hides the pointer while you type, until the mouse moves again
  addEventListener('keydown', () => { if (root && !TOUCH) root.removeAttribute('data-on'); }, { capture: true, passive: true });
  function touchAt(t) { x = t.clientX; y = t.clientY; build(); touchDot.style.left = `${x}px`; touchDot.style.top = `${y}px`; }
  addEventListener('touchstart', (e) => { if (!TOUCH || !e.touches[0]) return; touchAt(e.touches[0]); touchDot.setAttribute('data-on', ''); }, { capture: true, passive: true });
  addEventListener('touchmove', (e) => { if (TOUCH && e.touches[0]) touchAt(e.touches[0]); }, { capture: true, passive: true });
  addEventListener('touchend', () => { if (TOUCH && touchDot) setTimeout(() => touchDot.removeAttribute('data-on'), 90); }, { capture: true, passive: true });
  new MutationObserver(() => { if (root && !root.isConnected) { root = null; place(); } }).observe(document, { childList: true, subtree: false });

  const film = {
    ready: null,
    // getDisplayMedia needs a click: the next one lands on this see-through
    // cover, under the pointer wherever it already is, and starts the camera
    arm() {
      const b = document.createElement('button');
      b.id = '__film-arm';
      b.style.cssText = 'position:fixed;inset:0;width:100vw;height:100vh;z-index:2147483646;opacity:0;border:0;padding:0;margin:0;background:transparent;cursor:default';
      b.addEventListener('click', (e) => { e.preventDefault(); e.stopPropagation(); film.ready = film.begin(); b.remove(); }, { once: true, capture: true });
      document.documentElement.appendChild(b);
    },
    async begin() {
      const w = Math.round(innerWidth * devicePixelRatio), h = Math.round(innerHeight * devicePixelRatio);
      const stream = await realGDM({ video: { frameRate: { ideal: 60, max: 60 }, width: { ideal: w, max: w }, height: { ideal: h, max: h }, resizeMode: 'none' }, audio: false, preferCurrentTab: true, selfBrowserSurface: 'include', surfaceSwitching: 'exclude' });
      // the first frames can come at the page's CSS size: wait for the real one
      const v = document.createElement('video');
      v.muted = true; v.srcObject = stream;
      await v.play().catch(() => {});
      for (let i = 0; i < 120 && v.videoWidth !== w; i++) await new Promise((r) => setTimeout(r, 25));
      const size = [v.videoWidth, v.videoHeight];
      v.srcObject = null;
      const rec = new MediaRecorder(stream, { mimeType: 'video/mp4;codecs=avc1.640034', videoBitsPerSecond: 40000000 });
      film.chunks = [];
      rec.ondataavailable = (e) => { if (e.data.size) film.chunks.push(e.data); };
      rec.start(1000);
      film.rec = rec; film.stream = stream;
      return size;
    },
    async finish(name) {
      const rec = film.rec;
      if (!rec) return 0;
      await new Promise((r) => { rec.onstop = r; rec.stop(); });
      film.stream.getTracks().forEach((t) => t.stop());
      const blob = new Blob(film.chunks, { type: rec.mimeType });
      film.rec = null; film.chunks = [];
      const a = document.createElement('a');
      a.href = URL.createObjectURL(blob);
      a.download = name || 'take.mp4';
      a.style.display = 'none';
      document.documentElement.appendChild(a);
      a.click();
      a.remove();
      setTimeout(() => URL.revokeObjectURL(a.href), 120000);
      return blob.size;
    },
  };
  Object.defineProperty(window, '__film', { value: film });
})();
"""

# What someone sharing their screen shows: a plain moving picture, standing in
# for the window a person would pick.
SHARE_JS = r"""
(() => {
  if (window.top !== window || !navigator.mediaDevices) return;
  navigator.mediaDevices.getDisplayMedia = async () => {
    const c = document.createElement('canvas');
    c.width = 1920; c.height = 1080;
    const g = c.getContext('2d');
    const t0 = performance.now();
    const hills = [['#3b2a6b', 0.25, 700, 80], ['#2a1f55', 0.55, 790, 60], ['#181238', 1, 880, 44]];
    const draw = (now) => {
      const t = (now - t0) / 1000;
      const sky = g.createLinearGradient(0, 0, 0, 1080);
      sky.addColorStop(0, '#1b1240'); sky.addColorStop(0.55, '#6d2f8f'); sky.addColorStop(1, '#f08a6c');
      g.fillStyle = sky; g.fillRect(0, 0, 1920, 1080);
      g.fillStyle = '#ffd38a';
      g.beginPath(); g.arc(1300, 640 + Math.sin(t * 0.5) * 18, 150, 0, Math.PI * 2); g.fill();
      for (const [col, speed, base, amp] of hills) {
        g.fillStyle = col; g.beginPath(); g.moveTo(0, 1080);
        for (let px = 0; px <= 1920; px += 16) g.lineTo(px, base + Math.sin((px + t * 140 * speed) / 170) * amp + Math.sin((px + t * 70 * speed) / 61) * amp * 0.3);
        g.lineTo(1920, 1080); g.closePath(); g.fill();
      }
      requestAnimationFrame(draw);
    };
    requestAnimationFrame(draw);
    return c.captureStream(60);
  };
})();
"""

# ---------------------------------------------------------------------------
# https://nexa.pm, here
# ---------------------------------------------------------------------------


class TlsFront(threading.Thread):
    """TLS on a local port with a certificate made up for nexa.pm, the bytes
    passed to and from chatd as they are, so links in the footage read as
    Nexa's own address. Only the browsers launched here are sent to it."""

    def __init__(self, target_port):
        super().__init__(daemon=True)
        self.target = target_port
        s = socket.socket()
        s.bind(("127.0.0.1", 0))
        self.port = s.getsockname()[1]
        s.close()
        tls = WORK / "tls"
        tls.mkdir(parents=True, exist_ok=True)
        subprocess.run(["openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "2", "-subj", "/CN=nexa.pm",
                        "-addext", "subjectAltName=DNS:nexa.pm", "-keyout", str(tls / "key.pem"), "-out", str(tls / "cert.pem")],
                       check=True, capture_output=True)
        self.ctx = ssl.create_default_context(ssl.Purpose.CLIENT_AUTH)
        self.ctx.load_cert_chain(str(tls / "cert.pem"), str(tls / "key.pem"))
        self.ctx.set_alpn_protocols(["http/1.1"])
        self.ready = threading.Event()

    async def pipe(self, reader, writer):
        try:
            while True:
                data = await reader.read(65536)
                if not data:
                    break
                writer.write(data)
                await writer.drain()
        except Exception:
            pass
        finally:
            try:
                writer.close()
            except Exception:
                pass

    async def serve(self, reader, writer):
        try:
            up_reader, up_writer = await asyncio.open_connection("127.0.0.1", self.target)
        except Exception:
            writer.close()
            return
        await asyncio.gather(self.pipe(reader, up_writer), self.pipe(up_reader, writer))

    def run(self):
        loop = asyncio.new_event_loop()
        asyncio.set_event_loop(loop)
        server = loop.run_until_complete(asyncio.start_server(self.serve, "127.0.0.1", self.port, ssl=self.ctx))
        self.ready.set()
        loop.run_until_complete(server.serve_forever())


# ---------------------------------------------------------------------------
# People, and how a person moves
# ---------------------------------------------------------------------------


# The visible part of the nearest list around an element that scrolls.
SCROLL_AREA_JS = """(e) => {
    for (let p = e.parentElement; p && p !== document.body; p = p.parentElement) {
        const cs = getComputedStyle(p);
        if (/(auto|scroll|overlay)/.test(cs.overflowY) && p.scrollHeight > p.clientHeight + 2) {
            const r = p.getBoundingClientRect();
            return [Math.max(0, r.left), Math.max(0, r.top), Math.min(innerWidth, r.right), Math.min(innerHeight, r.bottom)];
        }
    }
    return null;
}"""


def _ease(u):
    """Minimum jerk: how a hand speeds up and settles."""
    return u * u * u * (10 - 15 * u + 6 * u * u)


class Actor(Session):
    """One person in their own browser tab, who moves and types like a person."""

    def __init__(self, browser, base, name, display, phone=False, share=False):
        self.base, self.name, self.display, self.out = base, name, display, WORK
        self.phone = phone
        w, h = PHONE if phone else VIEW
        self.size = (w, h)
        self.ctx = browser.new_context(
            viewport={"width": w, "height": h},
            device_scale_factor=PHONE_SCALE if phone else SCALE,
            is_mobile=phone, has_touch=phone,
            permissions=["microphone", "camera", "clipboard-read", "clipboard-write"],
            accept_downloads=True,
            ignore_https_errors=True,
        )
        self.ctx.add_init_script(FILM_JS.replace("%TOUCH%", "true" if phone else "false"))
        if share:
            self.ctx.add_init_script(SHARE_JS)
        self.page = self.ctx.new_page()
        self.errors, self.trace = [], []
        self.page.on("pageerror", lambda e: self.errors.append(f"{name}: {e}"))
        self.mouse = (w * 0.66, h * 0.6)
        self.filming = False

    # --- where things are ---------------------------------------------------

    def loc(self, target):
        return self.page.locator(target).first if isinstance(target, str) else target

    def at(self, target, fx=0.5, fy=0.5, timeout=20000):
        """A point inside the target, a little off its centre as a hand would land.
        Something scrolled out of sight is brought in first, with a turn of the
        wheel over the list it is in."""
        el = self.loc(target)
        el.wait_for(state="visible", timeout=timeout)
        vw, vh = self.size
        box = el.bounding_box()
        for _ in range(4):
            if box is not None and box["width"] > 0 and box["y"] >= 2 and box["y"] + box["height"] <= vh - 2:
                break
            area = el.evaluate(SCROLL_AREA_JS)
            if area is None or self.phone:
                el.scroll_into_view_if_needed(timeout=5000)
            else:
                cx = min(max(box["x"] + box["width"] / 2, area[0] + 12), area[2] - 12)
                self.glide(cx, (area[1] + area[3]) / 2)
                delta = (box["y"] + box["height"] / 2) - (area[1] + area[3]) / 2
                self.wheel(delta, dur=min(1.0, 0.35 + abs(delta) / 1500))
            time.sleep(0.3)
            box = el.bounding_box()
        jx = max(-6.0, min(6.0, (random.random() - 0.5) * 0.25 * box["width"]))
        jy = max(-3.0, min(3.0, (random.random() - 0.5) * 0.2 * box["height"]))
        return box["x"] + box["width"] * fx + jx, box["y"] + box["height"] * fy + jy

    # --- moving and clicking --------------------------------------------------

    def open_settings(self, nav):
        # on a phone User Settings lives in the navigation drawer
        if self.phone and self.page.locator("[role=dialog][aria-label='User Settings']").count() == 0:
            self.page.click("button[aria-label='Open Navigation']")
            self.page.wait_for_timeout(500)
        super().open_settings(nav)

    def close_settings(self):
        super().close_settings()
        if self.phone and self.page.locator("button[aria-label='User Settings']").first.is_visible():
            self.page.touchscreen.tap(PHONE[0] * 0.92, PHONE[1] * 0.45)
            self.page.wait_for_timeout(400)

    def glide(self, x, y, dur=None, settle=True):
        """Moves the pointer as a hand on a mouse does: one quick, slightly
        curved throw that lands a little short or long of the target, then a
        small slow correction onto it. Timed by Fitts's law, with the speed
        peaking before halfway and a faint tremor along the way."""
        x0, y0 = self.mouse
        dist = math.hypot(x - x0, y - y0)
        if dist < 2:
            self.page.mouse.move(x, y)
            self.mouse = (x, y)
            return
        if dur is None:
            dur = (0.16 + 0.12 * math.log2(dist / 26 + 1)) * random.uniform(0.9, 1.12)
        ux, uy = (x - x0) / dist, (y - y0) / dist
        # the throw ends off the target, mostly along the way it was going
        if settle and dist > 60:
            along = random.choice((-1, 1)) * random.uniform(0.025, 0.06) * dist
            side = random.uniform(-0.02, 0.02) * dist
            mx, my = x + ux * along - uy * side, y + uy * along + ux * side
        else:
            mx, my = x, y
        self._move(x0, y0, mx, my, dur)
        if (mx, my) != (x, y):
            time.sleep(random.uniform(0.01, 0.05))
            self._move(mx, my, x, y, random.uniform(0.13, 0.2), bend_scale=0.2)
        self.mouse = (x, y)

    def _move(self, x0, y0, x, y, dur, bend_scale=1.0):
        dist = math.hypot(x - x0, y - y0)
        # a wrist pivots, so long moves bow the same way more often than not
        bend = random.uniform(-0.35, 1.0) * min(36.0, dist * 0.08) * bend_scale
        nx, ny = -(y - y0) / max(dist, 1e-6), (x - x0) / max(dist, 1e-6)
        cx, cy = (x0 + x) / 2 + nx * bend, (y0 + y) / 2 + ny * bend
        phase = random.uniform(0, 6.3)
        steps = max(2, int(dur * 60))
        start = time.perf_counter()
        for i in range(1, steps + 1):
            u = i / steps
            e = _ease(u ** 0.82)  # quicker to get going than to stop
            px = (1 - e) ** 2 * x0 + 2 * (1 - e) * e * cx + e * e * x
            py = (1 - e) ** 2 * y0 + 2 * (1 - e) * e * cy + e * e * y
            wob = math.sin(u * math.pi) * 0.7 * math.sin(phase + u * 9.0)
            self.page.mouse.move(px + nx * wob, py + ny * wob)
            wait = start + i / 60 - time.perf_counter()
            if wait > 0:
                time.sleep(wait)

    def hover(self, target, fx=0.5, fy=0.5, dur=None):
        x, y = self.at(target, fx, fy)
        if self.phone:
            self.mouse = (x, y)
            return
        self.glide(x, y, dur)

    def click(self, target, fx=0.5, fy=0.5, button="left", pause=None, after=0.2, dur=None):
        x, y = self.at(target, fx, fy)
        if self.phone:
            self.mouse = (x, y)
            time.sleep(0.12 if pause is None else pause)
            self.page.touchscreen.tap(x, y)
            time.sleep(after)
            return
        self.glide(x, y, dur)
        time.sleep(random.uniform(0.07, 0.16) if pause is None else pause)
        self.page.mouse.down(button=button)
        time.sleep(0.075)
        self.page.mouse.up(button=button)
        time.sleep(after)

    def drag(self, src, dst, dur=0.9, hold=0.25):
        x0, y0 = self.at(src)
        self.glide(x0, y0)
        time.sleep(0.12)
        self.page.mouse.down()
        time.sleep(hold)
        x1, y1 = dst if isinstance(dst, tuple) else self.at(dst)
        self.glide(x1, y1, dur)
        time.sleep(0.2)
        self.page.mouse.up()
        time.sleep(0.2)

    def wheel(self, dy, dur=0.8, target=None):
        if target is not None:
            self.hover(target)
        n = max(2, int(dur * 60))
        start = time.perf_counter()
        for i in range(1, n + 1):
            # most of the travel early, as a flick of a wheel or trackpad does
            e0, e1 = _ease((i - 1) / n), _ease(i / n)
            self.page.mouse.wheel(0, dy * (e1 - e0))
            wait = start + i / 60 - time.perf_counter()
            if wait > 0:
                time.sleep(wait)

    def rest(self, s):
        time.sleep(s)

    # --- keys -----------------------------------------------------------------

    def type(self, text, speed=1.0):
        """Types at a person's pace: quick runs, a beat after commas and full stops."""
        for ch in text:
            if ch == "\n":
                self.page.keyboard.press("Shift+Enter")
            else:
                self.page.keyboard.type(ch)
            d = random.uniform(0.04, 0.11)
            if ch == " ":
                d += 0.025
            elif ch in ",.!?":
                d += random.uniform(0.1, 0.22)
            time.sleep(d / speed)

    def key(self, combo, after=0.25):
        self.page.keyboard.press(combo)
        time.sleep(after)

    def paste(self, text, after=0.3):
        """Puts text in at once, as pasting does."""
        self.page.keyboard.insert_text(text)
        time.sleep(after)

    # --- filming --------------------------------------------------------------

    def record_start(self):
        p = self.page
        p.evaluate("() => window.__film.arm()")
        x, y = self.mouse
        p.mouse.click(x, y)
        got = p.evaluate("() => window.__film.ready")
        w, h = self.size
        scale = PHONE_SCALE if self.phone else SCALE
        if got[0] != round(w * scale):
            raise RuntimeError(f"{self.name}: the recording came out {got}, not {w * scale}x{h * scale}")
        if not self.phone:
            p.mouse.move(x + 0.5, y)
        self.filming = True
        self.started, self.cut_at = time.time(), 0.0

    def cut(self):
        """Marks where the delivered clip starts, when the take began earlier."""
        self.cut_at = max(0.0, time.time() - self.started - 0.05)

    def record_stop(self, rel):
        raw = WORK / "raw" / f"{rel}.mp4"
        raw.parent.mkdir(parents=True, exist_ok=True)
        with self.page.expect_download(timeout=240000) as dl:
            self.page.evaluate("(n) => window.__film.finish(n)", raw.name)
        dl.value.save_as(str(raw))
        self.filming = False
        return raw

    def close(self):
        try:
            self.ctx.close()
        except Exception:
            pass


# ---------------------------------------------------------------------------
# Takes
# ---------------------------------------------------------------------------

TAKES = []  # (rel, raw path, phone, seconds cut from the start)


@contextmanager
def take(*views, lead=1.2, tail=1.6):
    """Films each (actor, clip name) at once, from a still moment before the
    action to one after it. A take that fails is kept aside, never delivered."""
    started = []
    try:
        for actor, rel in views:
            actor.record_start()
            started.append((actor, rel))
        time.sleep(lead)
        yield
        time.sleep(tail)
    except BaseException:
        for actor, rel in started:
            try:
                actor.record_stop("failed/" + rel)
            except Exception:
                pass
        raise
    for actor, rel in started:
        TAKES.append((rel, actor.record_stop(rel), actor.phone, actor.cut_at))
        print(f"    filmed {rel}")


def deliver(raw, final, phone, cut=0.0):
    """One take as the editor gets it: 1080p, a steady 60 fps, no sound."""
    final.parent.mkdir(parents=True, exist_ok=True)
    size = "scale=1080:-2:flags=lanczos" if phone else f"scale={FINAL[0]}:{FINAL[1]}:flags=lanczos"
    subprocess.run([
        "ffmpeg", "-y", "-v", "error", "-i", str(raw), "-ss", f"{cut:.3f}", "-vf", f"{size},fps=60",
        "-c:v", "libx264", "-preset", "slow", "-crf", "14", "-profile:v", "high", "-pix_fmt", "yuv420p",
        "-an", "-movflags", "+faststart", str(final),
    ], check=True)


def duration(path):
    out = subprocess.run(["ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", str(path)], capture_output=True, text=True)
    try:
        return float(out.stdout.strip())
    except ValueError:
        return 0.0


# ---------------------------------------------------------------------------
# The world the clips happen in
# ---------------------------------------------------------------------------

# Made-up people, each with an emoji for a face
PROFILES = {
    "maya": ("she/her", "illustrator, night owl, collects way too many stickers 🦊", "#ff8a3d", "#c23b7a"),
    "theo": ("he/him", "will cook for compliments. currently learning guitar (badly) 🎸", "#2fa36b", "#1d5f8a"),
    "juno": ("she/they", "deep sea enthusiast 🐙 ask me about octopuses", "#b44fd1", "#4b2bd1"),
    "kai": ("he/him", "snacks guy. movie nights every friday 🍿", "#3d6dff", "#29b6c8"),
    "rin": ("they/them", "streams cozy games on sundays 🐼", "#2a9d8f", "#8bc34a"),
}
CAST = [
    ("maya", "Maya", "1f98a", ("#ffbf73", "#ff6a3d")),
    ("theo", "Theo", "1f438", ("#a6eba9", "#2fa36b")),
    ("juno", "Juno", "1f419", ("#f9b2e0", "#b44fd1")),
    ("kai", "Kai", "1f427", ("#a6d4ff", "#3d6dff")),
    ("rin", "Rin", "1f43c", ("#c2f0e6", "#2a9d8f")),
]
SERVER = "Night Owls"
# a personal sticker pack's pictures, named as the stickers will be
STICKERS = [("Hi", "1f989"), ("Coffee", "2615"), ("Party", "1f389"), ("Sleepy", "1f634")]
CHANNELS = ["photos", "music", "gaming"]
EMOJI = [("owlhype", "1f989", "#6d4cff"), ("cheers", "1f942", "#ffb020"), ("pizzatime", "1f355", "#ff6b4a"),
         ("gg", "1f3c6", "#2fbf71"), ("sleepy", "1f634", "#4f7cff"), ("lfg", "1f680", "#e8467c")]
GENERAL_TALK = [
    ("theo", "who's around tonight? 🌙"),
    ("juno", "me! just got off work"),
    ("kai", "same, but i need food first"),
    ("rin", "pizza run? i can order for everyone 🍕"),
    ("theo", "yes please. pineapple on half 👀"),
    ("juno", "theo no"),
    ("maya", "theo yes, actually"),
    ("kai", "ok after food, movie in the voice channel at 9?"),
    ("rin", "i'll share my screen, bring snacks"),
    ("juno", "see you all at 9 💜"),
]
DM_TALK = [
    ("theo", "did you see the photos from saturday?"),
    ("maya", "yes!! the one at the pier is so good"),
    ("theo", "i'll send you the rest later"),
]
PHOTOS = {
    "sunset-pier": "linear-gradient(180deg,#2b1a55 0%,#8a3f86 42%,#f2876a 63%,#ffd18f 70%,#3a3d7a 70.2%,#1d1f4a 100%)",
    "lake": "linear-gradient(180deg,#9ed2ff 0%,#d9efff 48%,#5b8fb8 48.3%,#2f5f86 100%)",
    "city-night": "linear-gradient(180deg,#0b0d24 0%,#1d2150 60%,#3a2c6e 100%)",
    "beach": "linear-gradient(180deg,#7ec8ff 0%,#c9ecff 50%,#2fb3c7 50.2%,#1f8fb0 66%,#f3d9a4 66.2%,#e8c27f 100%)",
}


class World:
    def __init__(self, browser, srv, base):
        self.browser, self.srv, self.base = browser, srv, base
        self.people = {}
        self.assets = WORK / "assets"
        self.server_id = None
        self.channels = {}

    def __getitem__(self, name):
        return self.people[name]

    def channel_url(self, name):
        return f"{self.base}/channels/{self.server_id}/{self.channels[name]}"

    def go(self, actor, url):
        actor.page.goto(url, wait_until="networkidle")
        actor.page.wait_for_selector("[data-testid=user-settings], textarea, [data-testid=notes-page]", timeout=30000)
        actor.page.wait_for_timeout(700)


def render(browser, html, w, h, path, transparent=False):
    page = browser.new_page(viewport={"width": w, "height": h})
    page.set_content(f"<!doctype html><html><body style='margin:0;background:transparent'>{html}</body></html>", wait_until="load")
    page.wait_for_timeout(150)
    page.screenshot(path=str(path), omit_background=transparent)
    page.close()


def make_assets(w):
    """Faces, an icon, emoji, stickers and a few pictures, drawn by the browser."""
    a = w.assets
    a.mkdir(parents=True, exist_ok=True)
    tw = lambda code: f"{w.srv.base}/twemoji/{code}.svg"  # noqa: E731
    for name, _, code, (c1, c2) in CAST:
        render(w.browser, f"<div style='width:512px;height:512px;background:linear-gradient(145deg,{c1},{c2});display:grid;place-items:center'><img src='{tw(code)}' style='width:330px;height:330px'></div>", 512, 512, a / f"avatar-{name}.png")
    render(w.browser, f"<div style='width:512px;height:512px;background:linear-gradient(160deg,#4b2bd1,#1a1250);display:grid;place-items:center'><img src='{tw('1f989')}' style='width:340px;height:340px'></div>", 512, 512, a / "server-icon.png")
    for name, code, color in EMOJI:
        render(w.browser, f"<div style='width:128px;height:128px;display:grid;place-items:center'><div style='width:124px;height:124px;border-radius:50%;background:{color};display:grid;place-items:center'><img src='{tw(code)}' style='width:84px;height:84px'></div></div>", 128, 128, a / f"{name}.png", transparent=True)
    outline = "filter:drop-shadow(5px 0 0 #fff) drop-shadow(-5px 0 0 #fff) drop-shadow(0 5px 0 #fff) drop-shadow(0 -5px 0 #fff) drop-shadow(0 8px 12px rgba(0,0,0,.28))"
    (a / "stickers").mkdir(exist_ok=True)
    for name, code in STICKERS:
        render(w.browser, f"<div style='width:320px;height:320px;display:grid;place-items:center'><img src='{tw(code)}' style='width:240px;height:240px;{outline}'></div>", 320, 320, a / "stickers" / f"{name}.png", transparent=True)
    for name, bg in PHOTOS.items():
        extra = ""
        if name == "sunset-pier":
            extra = "<div style='position:absolute;left:58%;top:52%;width:220px;height:220px;margin:-110px;border-radius:50%;background:radial-gradient(circle,#fff3d4,#ffc27a 60%,rgba(255,190,120,0) 72%)'></div><div style='position:absolute;left:8%;right:48%;top:66%;height:14px;background:#1b1238'></div>"
        elif name == "lake":
            extra = "<div style='position:absolute;inset:0;background:linear-gradient(180deg,transparent 0,transparent 30%,#3f6d55 30.2%,transparent 48%);clip-path:polygon(0 48%,12% 22%,26% 40%,41% 14%,58% 38%,72% 20%,88% 36%,100% 26%,100% 48%)'></div>"
        elif name == "city-night":
            extra = "".join(f"<div style='position:absolute;left:{x}%;bottom:0;width:{wd}%;height:{ht}%;background:#141634'></div>" for x, wd, ht in ((2, 9, 44), (12, 7, 62), (20, 10, 38), (31, 8, 71), (40, 11, 50), (52, 7, 64), (60, 12, 41), (73, 8, 58), (82, 13, 47)))
            extra += "<div style='position:absolute;inset:0;background-image:radial-gradient(circle,#ffd98a 1.5px,transparent 2px);background-size:22px 26px;mask-image:linear-gradient(0deg,#000 0,#000 55%,transparent 70%);opacity:.8'></div>"
        elif name == "beach":
            extra = "<div style='position:absolute;left:18%;top:20%;width:150px;height:150px;border-radius:50%;background:#fff6c8;box-shadow:0 0 60px #fff3a8'></div>"
        render(w.browser, f"<div style='position:relative;width:1600px;height:1000px;background:{bg};overflow:hidden'>{extra}</div>", 1600, 1000, a / f"{name}.jpg")


def set_avatar(actor, path):
    actor.open_settings("profiles")
    p = actor.page
    p.locator("[role=dialog][aria-label='User Settings'] input[type=file]").first.set_input_files(str(path))
    p.click("[data-testid=crop-apply]")
    p.wait_for_selector("[data-testid=crop-apply]", state="detached", timeout=20000)
    p.wait_for_timeout(1500)
    pronouns, about, c1, c2 = PROFILES[actor.name]
    p.fill("[role=dialog] input[placeholder='Add your pronouns']", pronouns)
    p.fill("[data-testid=about-me-editor] textarea", about)
    colors = p.locator("[role=dialog] input[type=color]")
    colors.nth(0).fill(c1)
    p.wait_for_timeout(200)
    colors.nth(1).fill(c2)
    p.wait_for_timeout(300)
    p.locator("[role=dialog] button:has-text('Save Changes')").last.click()
    p.wait_for_timeout(1200)
    actor.close_settings()


def invite_link(actor):
    p = actor.page
    p.click("button[aria-label='Invite People']")
    box = p.locator("[data-testid=invite-link]")
    box.wait_for(timeout=15000)
    link = ""
    for _ in range(80):
        link = box.input_value()
        if "#k=" in link:
            break
        time.sleep(0.1)
    p.keyboard.press("Escape")
    return link


def channel_ids(page):
    return page.evaluate("""() => {
        const out = {};
        for (const el of document.querySelectorAll('[data-channel]')) {
            const name = [...el.querySelectorAll('span')].map((s) => s.textContent.trim()).find((t) => t) || el.textContent.trim();
            if (!(name in out)) out[name] = el.getAttribute('data-channel');
        }
        return out;
    }""")


def say(actor, text):
    """Sends a message quickly, while nothing is filmed, and waits until it is there."""
    actor.send(text)
    actor.page.wait_for_function("() => (document.querySelector('textarea[aria-label^=\"Message\"]')?.value ?? 'x') === ''", timeout=20000)
    actor.page.wait_for_timeout(250)


def seed(w):
    print("seeding: pictures")
    make_assets(w)
    # the first account becomes the instance's admin and wears its crown; it stays out of shot
    owner = Actor(w.browser, w.base, "hostadmin", "Host")
    owner.register("Host")
    owner.close()
    for name, display, _, _ in CAST:
        print("seeding:", name)
        a = Actor(w.browser, w.base, name, display)
        a.register(display)
        set_avatar(a, w.assets / f"avatar-{name}.png")
        w.people[name] = a
    maya = w["maya"]
    for other in ("theo", "juno", "kai", "rin"):
        maya.add_friend(other)
        w[other].accept_friend("Maya")
    print("seeding: the server")
    p = maya.page
    p.goto(w.base + "/channels/@me", wait_until="networkidle")
    p.click("button[aria-label='Add a Server']")
    p.set_input_files("[data-testid=server-icon-file]", str(w.assets / "server-icon.png"))
    try:
        p.click("[data-testid=crop-apply]", timeout=5000)
    except Exception:
        pass
    p.fill("[data-testid=server-name]", SERVER)
    p.click("[data-testid=create-server-submit]")
    p.wait_for_url("**/channels/*/*", timeout=30000)
    p.wait_for_selector("textarea[aria-label='Message #general']", timeout=20000)
    w.server_id = p.url.split("/channels/")[1].split("/")[0]
    for n in CHANNELS:
        p.click("button[aria-label='Create Channel']")
        p.fill("[data-testid=channel-name]", n)
        p.click("button:has-text('Create Channel') >> nth=-1")
        p.wait_for_selector(f"[data-channel]:has(span:text-is('{n}'))", timeout=15000)
        time.sleep(0.2)
    link = invite_link(maya)
    for other in ("theo", "juno", "kai", "rin"):
        o = w[other].page
        o.goto(link, wait_until="networkidle")
        o.click("[data-testid=invite-join]")
        o.wait_for_url(f"**/channels/{w.server_id}/*", timeout=30000)
    p.click(f"button:has-text('{SERVER}')")
    p.click("text=Server Settings")
    p.click("[data-nav=emoji]")
    p.set_input_files("[data-testid=emoji-file]", [str(w.assets / f"{n}.png") for n, _, _ in EMOJI])
    p.wait_for_selector(f"text=:{EMOJI[-1][0]}:", timeout=30000)
    p.keyboard.press("Escape")
    p.wait_for_timeout(500)
    w.channels = channel_ids(p)
    print("seeding: talk")
    for name in ("maya", "theo", "juno", "kai", "rin"):
        w.go(w[name], w.channel_url("general"))
    for who, text in GENERAL_TALK:
        say(w[who], text)
    w["theo"].open_dm_with("Maya")
    w.dm_theo = w["theo"].page.url.rstrip("/").split("/")[-1]
    for who, text in DM_TALK:
        a = w[who]
        w.go(a, f"{w.base}/channels/@me/{w.dm_theo}")
        say(a, text)
    for name in ("maya", "theo", "juno", "kai", "rin"):
        w.go(w[name], w.channel_url("general"))


# ---------------------------------------------------------------------------
# The clips
# ---------------------------------------------------------------------------

CLIPS = {}


def clip(folder, name, about, discord=None):
    """Registers a clip: where it goes, what it shows, and for unique/ what Discord lacks."""
    def wrap(fn):
        CLIPS[name] = {"folder": folder, "fn": fn, "about": about, "discord": discord}
        return fn
    return wrap


def spa(actor, path):
    """Goes to a page of the app the way its own links do, without loading it again."""
    actor.page.evaluate("(p) => { history.pushState(null, '', p); dispatchEvent(new PopStateEvent('popstate', { state: null })); }", path)


def end_of(actor, text, selector="body"):
    """Where the caret goes to type straight after `text`: the end of its last letter."""
    r = actor.page.evaluate("""([text, sel]) => {
        const root = document.querySelector(sel) || document.body;
        const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
        for (let n = walker.nextNode(); n; n = walker.nextNode()) {
            const i = n.data.indexOf(text);
            if (i < 0) continue;
            const r = document.createRange();
            r.setStart(n, i + text.length - 1);
            r.setEnd(n, i + text.length);
            const b = r.getBoundingClientRect();
            return [b.right, b.top + b.height / 2];
        }
        return null;
    }""", [text, selector])
    if r is None:
        raise RuntimeError(f"not on the page: {text!r}")
    return r[0] + 1, r[1]


def click_point(actor, x, y, pause=0.12):
    actor.glide(x, y)
    time.sleep(pause)
    actor.page.mouse.down()
    time.sleep(0.07)
    actor.page.mouse.up()
    time.sleep(0.2)


@clip("unique", "e2ee-what-the-server-sees",
      "The front page shows one chat twice: as the people in it read it, and as Nexa's server holds it, which is scrambled. Dragging the line between them compares the two. Messages and files are end-to-end encrypted, so the server can never read them.",
      discord="Discord does not end-to-end encrypt text messages or files at all (only voice and video calls, since 2026). It said on 18 May 2026 it has no plans to do so for text. Sources: discord.com/blog/every-voice-and-video-call-on-discord-is-now-end-to-end-encrypted, discord.com/blog/meet-dave-e2ee-for-audio-video")
def c_front_duo(w):
    v = Actor(w.browser, w.base, "visitor", "Visitor")
    try:
        p = v.page
        p.goto(w.base + "/", wait_until="networkidle")
        knob = "figure:has(input[aria-label^='Move the line']) span[class*=knob]"
        p.wait_for_selector(knob, timeout=20000)
        # away and back inside the app, so the page draws itself again on camera
        spa(v, "/terms")
        p.wait_for_selector("figure:has(input[aria-label^='Move the line'])", state="detached", timeout=15000)
        p.wait_for_timeout(800)
        v.mouse = (VIEW[0] * 0.72, VIEW[1] * 0.86)
        with take((v, "unique/e2ee-what-the-server-sees"), lead=0.3):
            spa(v, "/")
            v.cut()
            p.wait_for_selector(knob, timeout=15000)
            v.rest(3.4)  # the line sweeps in by itself
            stage = v.loc("figure:has(input[aria-label^='Move the line']) div[data-settled]")
            box = stage.bounding_box()
            v.hover(knob)
            v.rest(0.35)
            kx, ky = v.mouse
            p.mouse.down()
            v.rest(0.15)
            v.glide(box["x"] + box["width"] * 0.1, ky + 4, dur=2.3)
            v.rest(0.7)
            v.glide(box["x"] + box["width"] * 0.9, ky - 3, dur=3.0)
            v.rest(0.7)
            v.glide(box["x"] + box["width"] * 0.52, ky, dur=1.7)
            v.rest(0.2)
            p.mouse.up()
            v.rest(0.5)
            v.glide(box["x"] + box["width"] * 0.7, box["y"] + box["height"] + 70, dur=0.9)
            v.rest(1.2)
    finally:
        v.close()


@clip("unique", "notes-writing-together",
      "Two people write in the same note at once. Each sees the other's caret and name move as they type, a picture one adds appears for the other straight away, and it is all end-to-end encrypted. Filmed from both sides at the same moment: -maya-view and -theo-view line up.",
      discord="Discord has no shared documents or notes that several people edit together. The nearest thing is the Whiteboard activity, a drawing canvas inside a call. Source: discord.com/blog/server-activities-games-voice-watch-together")
def c_notes(w):
    maya, theo = w["maya"], w["theo"]
    m, t = maya.page, theo.page
    surface = "[data-testid=note-surface]"
    m.goto(w.base + "/channels/@me/notes", wait_until="networkidle")
    m.click("[data-testid=note-new]")
    m.wait_for_selector("[data-testid=note-title]")
    m.fill("[data-testid=note-title]", "Weekend at the Lake")
    m.click(surface)
    for i, line in enumerate(["We leave Friday after work. Bring a jacket, it gets cold by the water.", "Packing list:", "- sunscreen", "snacks for the drive", "the speaker"]):
        if i:
            m.keyboard.press("Enter")
        m.keyboard.type(line, delay=3)
    m.wait_for_timeout(1500)
    note = m.url
    m.click("[data-testid=note-share]")
    m.wait_for_selector("[data-testid=share-note]")
    m.click("[data-testid=note-invite-role-2]")
    m.click("[data-testid=note-invite-row][data-user=theo] button")
    m.wait_for_timeout(800)
    m.keyboard.press("Escape")
    t.goto(w.base + "/channels/@me/notes", wait_until="networkidle")
    t.click("[data-testid=note-request-accept]")
    t.wait_for_timeout(800)
    t.goto(note, wait_until="networkidle")
    t.wait_for_selector(surface)
    m.wait_for_selector("text=/Sent Theo a request/", state="detached", timeout=15000)
    maya.mouse = (VIEW[0] * 0.8, VIEW[1] * 0.75)
    theo.mouse = (VIEW[0] * 0.75, VIEW[1] * 0.8)
    for a in (maya, theo):
        a.page.mouse.move(*a.mouse)
    with take((maya, "unique/notes-writing-together-maya-view"), (theo, "unique/notes-writing-together-theo-view")):
        # Theo goes to the end of the list, Maya to her first line
        click_point(theo, *end_of(theo, "the speaker", surface))
        theo.rest(0.5)
        click_point(maya, *end_of(maya, "after work.", surface))
        maya.rest(0.6)
        theo.key("Enter", after=0.3)
        hers, his = " The cabin has a canoe we can use!", "board games"
        for i in range(max(len(hers), len(his))):
            if i < len(hers):
                m.keyboard.type(hers[i])
            if i < len(his):
                t.keyboard.type(his[i])
            time.sleep(random.uniform(0.06, 0.12))
        time.sleep(0.9)
        # Theo adds a picture under the list, from the toolbar
        theo.key("Enter", after=0.1)
        theo.key("Enter", after=0.3)
        add = "button[aria-label='Add Picture or Video']"
        theo.hover(add)
        theo.rest(0.3)
        with t.expect_file_chooser() as fc:
            click_point(theo, *theo.mouse)
        fc.value.set_files(str(w.assets / "lake.jpg"))
        m.wait_for_selector("[data-testid=note-file][data-state=ready]", timeout=30000)
        theo.rest(0.8)
        theo.type("Same spot as last year 🛶")
        theo.rest(0.6)
        # Maya makes the picture smaller by its edge
        maya.hover("[data-testid=note-file][data-visual]")
        maya.rest(0.6)
        grip = maya.loc("[data-testid=note-file-grip-end]")
        maya.drag(grip, (maya.at(grip)[0] - 180, maya.at(grip)[1]), dur=0.9)
        maya.glide(VIEW[0] * 0.86, VIEW[1] * 0.3)
        maya.rest(1.2)
    w.note_url = note


@clip("unique", "subchat-with-a-password",
      "A side conversation inside a direct message, locked with its own password. Maya makes one; Theo, in the same conversation, has to type the password before he can read it.",
      discord="Discord has no password-protected chats of any kind, and no sub-conversations inside a DM or group DM (threads exist only in servers and follow channel permissions). Sources: support.discord.com/hc/en-us/articles/4403205878423-Threads-FAQ, support.discord.com/hc/en-us/community/posts/360029990411")
def c_subchat(w):
    maya, theo = w["maya"], w["theo"]
    dm = f"{w.base}/channels/@me/{w.dm_theo}"
    w.go(maya, dm)
    w.go(theo, dm)
    maya.mouse = (VIEW[0] * 0.62, VIEW[1] * 0.55)
    maya.page.mouse.move(*maya.mouse)
    with take((maya, "unique/subchat-with-a-password-maya-makes-it")):
        maya.click("[data-testid=subchats-button]")
        maya.rest(0.6)
        maya.click("[data-testid=subchat-with-password]")
        maya.rest(0.4)
        maya.click("[data-testid=subchat-modal-name]")
        maya.type("birthday surprise for juno 🎂")
        maya.rest(0.3)
        maya.click("[role=dialog] label:has(input[role=switch])")
        maya.rest(0.4)
        maya.click("[data-testid=subchat-modal-password]")
        maya.type("candles", speed=0.8)
        maya.click("[role=dialog] input[type=password] >> nth=1")
        maya.type("candles", speed=0.8)
        maya.rest(0.3)
        maya.click("[data-testid=subchat-modal-save]")
        maya.page.wait_for_selector("[role=dialog] [data-testid=subchat-modal-save]", state="detached", timeout=20000)
        maya.rest(0.8)
        maya.click(maya.page.locator(":text('birthday surprise for'):visible").first)
        maya.page.wait_for_selector("[data-testid=subchat-parent]", timeout=20000)
        maya.rest(0.8)
        maya.click("textarea[aria-label^='Message']")
        maya.type("ok nobody tell juno. cake from the place on 5th?")
        maya.key("Enter", after=1.0)
    w.go(theo, dm)
    theo.mouse = (VIEW[0] * 0.6, VIEW[1] * 0.5)
    theo.page.mouse.move(*theo.mouse)
    with take((theo, "unique/subchat-with-a-password-theo-opens-it")):
        theo.click("[data-testid=subchats-button]")
        theo.rest(0.7)
        theo.click("[data-subchat]")
        theo.page.wait_for_selector("[data-testid=locked-subchat]", timeout=20000)
        theo.rest(1.0)
        theo.click("[data-testid=subchat-password]")
        theo.type("candles", speed=0.8)
        theo.rest(0.3)
        theo.click("[data-testid=subchat-unlock]")
        theo.page.wait_for_selector("text=ok nobody tell juno", timeout=20000)
        theo.rest(0.9)
        theo.click("textarea[aria-label^='Message']")
        theo.type("my lips are sealed 🤐 the chocolate one!")
        theo.key("Enter", after=1.0)


@clip("unique", "telegram-sticker-pack-import",
      "Bringing a whole Telegram sticker pack into Nexa by pasting its link (Settings, Emoji & Stickers, Create Sticker Pack, Import Telegram Sticker Pack), then sending one of its animated stickers.",
      discord="Discord cannot import Telegram sticker packs; only third-party bots and converters do. Source: support.discord.com/hc/en-us/community/posts/360050868734-Custom-sticker-packs")
def c_telegram(w):
    maya = w["maya"]
    dm = f"{w.base}/channels/@me/{w.dm_theo}"
    w.go(maya, dm)
    maya.mouse = (VIEW[0] * 0.55, VIEW[1] * 0.5)
    maya.page.mouse.move(*maya.mouse)
    with take((maya, "unique/telegram-sticker-pack-import")):
        maya.click("[data-testid=user-settings]")
        maya.rest(0.5)
        maya.click("[data-nav=stickers]")
        maya.rest(0.8)
        maya.click("[data-testid=create-pack]")
        maya.rest(0.6)
        maya.click("[data-testid=telegram-import-open]")
        maya.rest(0.5)
        maya.click("[data-testid=telegram-import-url]")
        maya.paste("https://t.me/addstickers/UtyaDuck")
        maya.rest(0.6)
        maya.click(maya.page.get_by_role("button", name="Import", exact=True).last)
        maya.page.wait_for_selector("[data-testid=pack-name]", timeout=180000)
        maya.rest(3.0)
        maya.wheel(500, dur=1.2, target="[data-testid=pack-name]")
        maya.rest(1.2)
        maya.key("Escape", after=0.6)
        if maya.page.locator("[role=dialog][aria-label='User Settings']").count():
            maya.key("Escape", after=0.6)
        maya.click("button[aria-label='Select Emoji']")
        maya.rest(0.4)
        maya.click("[role=dialog][aria-label='Expression Picker'] button:has-text('Stickers')")
        maya.rest(1.2)
        maya.click("[role=dialog][aria-label='Expression Picker'] button[data-sticker] >> nth=3")
        maya.rest(2.5)


@clip("unique", "app-lock-passcode",
      "Locking the app with a passcode: set one in Settings, Security, lock Nexa, and get back in with it. It can also lock itself after a few idle minutes or when you switch away.",
      discord="No Discord app has a passcode or PIN lock of its own (people have asked for one for years). Source: support.discord.com/hc/en-us/community/posts/360036829831-App-Passcode")
def c_app_lock(w):
    maya = w["maya"]
    w.go(maya, w.channel_url("general"))
    maya.mouse = (VIEW[0] * 0.6, VIEW[1] * 0.55)
    maya.page.mouse.move(*maya.mouse)
    with take((maya, "unique/app-lock-passcode")):
        maya.click("[data-testid=user-settings]")
        maya.rest(0.5)
        maya.click("[data-nav=security]")
        maya.rest(0.8)
        maya.click("[data-testid=lock-set]")
        maya.rest(0.5)
        maya.click("[data-testid=passcode-new]")
        maya.type("246813", speed=0.7)
        maya.click("[data-testid=passcode-confirm]")
        maya.type("246813", speed=0.7)
        maya.rest(0.3)
        maya.click("[data-testid=passcode-save]")
        maya.page.wait_for_selector("[data-testid=lock-row]:has-text('Passcode is on')", timeout=30000)
        maya.rest(1.0)
        maya.click("[data-testid=lock-now]")
        maya.page.wait_for_selector("[data-testid=passcode-input]", timeout=20000)
        maya.rest(1.6)
        maya.click("[data-testid=passcode-input]")
        maya.type("246813", speed=0.7)
        maya.rest(0.3)
        maya.click("[data-testid=passcode-submit]")
        maya.page.wait_for_selector("[data-testid=passcode-input]", state="detached", timeout=45000)
        maya.rest(0.8)
        if maya.page.locator("[role=dialog][aria-label='User Settings']").count():
            maya.key("Escape", after=0.5)
        maya.page.wait_for_selector("textarea[aria-label^='Message']", timeout=45000)
        maya.rest(1.0)
    # the passcode goes again, so the clips after this one are not locked
    maya.open_settings("security")
    maya.page.click("[data-testid=lock-remove]")
    maya.page.click("[role=dialog] button:has-text('Remove it')")
    maya.page.wait_for_selector("[data-testid=lock-row]:has-text('No passcode')", timeout=20000)
    maya.close_settings()


CSS_DEMO = """:root {
  --brand: #ff4f8b;
  --bg-base: #2a1426;
  --bg-raised: #331a2f;
}
"""


@clip("unique", "custom-css",
      "Restyling the whole app with custom CSS in Settings, Custom CSS: the colours change as the CSS is typed. It follows the account to every device.",
      discord="Discord's official apps do not take custom CSS; restyling needs a third-party client mod such as BetterDiscord or Vencord, which break Discord's terms. Discord itself offers only preset colour themes (custom colours with Nitro). Sources: support.discord.com/hc/en-us/articles/207260127, discord.com/safety/platform-manipulation-policy-explainer")
def c_custom_css(w):
    maya = w["maya"]
    w.go(maya, w.channel_url("general"))
    maya.mouse = (VIEW[0] * 0.6, VIEW[1] * 0.55)
    maya.page.mouse.move(*maya.mouse)
    with take((maya, "unique/custom-css")):
        maya.click("[data-testid=user-settings]")
        maya.rest(0.5)
        maya.click("[data-nav=customCss]")
        maya.rest(0.8)
        sw = maya.loc("[role=dialog] label:has(input[role=switch])")
        if not maya.page.locator("[role=dialog] input[role=switch]").first.is_checked():
            maya.click(sw)
            maya.rest(0.5)
        maya.click("[data-testid=css-snippet]")
        maya.type(CSS_DEMO, speed=1.4)
        maya.rest(1.0)
        maya.click("[data-testid=save]")
        maya.rest(1.0)
        maya.key("Escape", after=1.5)
        maya.hover("[data-channel]:has(span:text-is('music'))")
        maya.rest(0.4)
        maya.click("[data-channel]:has(span:text-is('music'))")
        maya.rest(1.0)
        maya.click("[data-channel]:has(span:text-is('general'))")
        maya.rest(1.4)
    # back to Nexa's own colours for the rest
    maya.open_settings("customCss")
    if maya.page.locator("[role=dialog] input[role=switch]").first.is_checked():
        maya.page.locator("[role=dialog] label:has(input[role=switch])").first.click()
    maya.page.locator("[data-testid=css-snippet]").first.fill("")
    try:
        maya.page.click("[data-testid=save]", timeout=3000)
    except Exception:
        pass
    maya.page.wait_for_timeout(800)
    maya.close_settings()


PLUGIN_DEMO = """export default {
  name: "Dice Roller",
  description: "Adds /roll to the message box.",
  version: "1.0.0",
  author: "Maya",
  start(api) {
    api.commands.register({
      name: "roll",
      description: "Roll a die (a d20 unless you say otherwise)",
      run(args) {
        const sides = Math.max(2, parseInt(args, 10) || 20);
        const n = 1 + Math.floor(Math.random() * sides);
        return `🎲 rolled a **${n}** on a d${sides}`;
      },
    });
    api.toast("Dice Roller is ready. Try /roll");
  },
};
"""


@clip("unique", "plugins",
      "Adding a plugin by pasting its code in Settings, Plugins, after a clear warning, then using the /roll command it adds. Plugins and their settings follow the account.",
      discord="Discord's official apps cannot install plugins; that needs a third-party client mod (BetterDiscord, Vencord), which breaks Discord's terms. Sources: discord.com/safety/platform-manipulation-policy-explainer")
def c_plugins(w):
    maya = w["maya"]
    w.go(maya, w.channel_url("gaming"))
    maya.mouse = (VIEW[0] * 0.6, VIEW[1] * 0.55)
    maya.page.mouse.move(*maya.mouse)
    with take((maya, "unique/plugins")):
        maya.click("[data-testid=user-settings]")
        maya.rest(0.5)
        maya.click("[data-nav=plugins]")
        maya.rest(0.8)
        maya.click("[data-testid=plugin-open-code]")
        maya.rest(0.5)
        maya.click("[data-testid=plugin-code]")
        maya.paste(PLUGIN_DEMO, after=0.8)
        maya.click("[data-testid=plugin-agree]")
        maya.rest(0.4)
        maya.click("[data-testid=plugin-add]")
        maya.page.wait_for_selector("[data-testid=plugin-item]", timeout=20000)
        maya.rest(1.6)
        maya.key("Escape", after=0.6)
        if maya.page.locator("[role=dialog][aria-label='User Settings']").count():
            maya.key("Escape", after=0.6)
        maya.click("textarea[aria-label^='Message']")
        maya.type("/roll")
        maya.rest(0.9)
        maya.key("Enter", after=0.6)
        if maya.page.locator("textarea[aria-label^='Message']").first.input_value().strip():
            maya.key("Enter", after=0.4)
        maya.page.wait_for_selector("[data-id]:has-text('rolled a')", timeout=15000)
        maya.rest(1.5)
    # The plugin goes again, or every later clip opens with its toast. The
    # account's settings are saved after a pause, so each removal is given
    # time to reach the server and checked after a reload.
    for _ in range(3):
        maya.open_settings("plugins")
        items = maya.page.locator("[data-testid=plugin-item]")
        if items.count() == 0:
            maya.close_settings()
            break
        try:
            items.first.locator("[data-testid=plugin-remove]").click(timeout=5000)
            # the confirmation is the newest dialog; its button's words sit in
            # an inner element, so a button:text-is() selector never matches it
            maya.page.locator("[role=dialog]").last.get_by_role("button", name="Remove", exact=True).click(timeout=5000)
        except Exception:
            traceback.print_exc()
        maya.page.wait_for_timeout(2500)
        maya.close_settings()
        maya.page.reload(wait_until="networkidle")
        maya.page.wait_for_timeout(1000)
    else:
        raise RuntimeError("the Dice Roller plugin would not go")


@clip("unique", "select-several-messages-and-delete",
      "Picking several messages and deleting them in one go (right-click, Select Messages, tick, Delete Messages).",
      discord="Discord's apps have no way for a person to select several messages and delete them together; only bots can bulk-delete. Sources: docs.discord.com/developers/resources/message, support.discord.com/hc/en-us/community/posts/360059780531")
def c_bulk_delete(w):
    maya = w["maya"]
    w.go(maya, w.channel_url("gaming"))
    for text in ("test", "testing the new bot", "is this thing on?", "oops, wrong channel 🙃"):
        say(maya, text)
    maya.mouse = (VIEW[0] * 0.75, VIEW[1] * 0.4)
    maya.page.mouse.move(*maya.mouse)
    with take((maya, "unique/select-several-messages-and-delete")):
        row = maya.page.locator("[data-id]:has-text('testing the new bot')").last
        maya.hover(row, fx=0.4)
        maya.rest(0.4)
        x, y = maya.mouse
        maya.page.mouse.click(x, y, button="right")
        maya.rest(0.6)
        maya.click("[role=menu] >> text=Select Messages")
        maya.rest(0.6)
        rows = maya.page.locator("[data-id]")
        n = rows.count()
        for i in range(n - 4, n):
            target = rows.nth(i).locator("[data-testid=pick-message]")
            if target.get_attribute("aria-checked") != "true":
                maya.click(target)
                maya.rest(0.3)
        maya.rest(0.6)
        maya.click("[data-testid=delete-selected]")
        maya.rest(0.6)
        maya.click("[role=dialog] button:has-text('Delete')")
        maya.page.wait_for_selector("[data-id]:has-text('testing the new bot')", state="detached", timeout=15000)
        maya.rest(1.2)


@clip("unique", "coloured-text",
      "Colouring words inside ordinary messages: [words]{red} colours them, [words]{on yellow} highlights them, and the colours adjust themselves to stay readable in every theme.",
      discord="Discord's formatting has no text colour. The only workaround is an undocumented ANSI escape trick inside a code block, desktop only. Source: support.discord.com/hc/en-us/articles/210298617-Markdown-Text-101-Chat-Formatting-Bold-Italic-Underline")
def c_colours(w):
    maya = w["maya"]
    w.go(maya, w.channel_url("general"))
    maya.mouse = (VIEW[0] * 0.6, VIEW[1] * 0.55)
    maya.page.mouse.move(*maya.mouse)
    with take((maya, "unique/coloured-text")):
        maya.click("textarea[aria-label^='Message']")
        maya.type("the sunset tonight was [unreal]{orange} 🌅")
        maya.key("Enter", after=0.9)
        maya.type("movie at 9 is [CONFIRMED]{green}, snacks are [your problem]{pink}")
        maya.key("Enter", after=0.9)
        maya.type("[no spoilers]{on red} for the finale please")
        maya.key("Enter", after=1.6)


# --- general/ ------------------------------------------------------------------

def react(actor, text, query):
    """Somebody else's reaction, quickly, off camera."""
    p = actor.page
    row = p.locator(f"[data-id]:has-text('{text}')").last
    row.hover()
    row.locator("button[aria-label='Add Reaction']").first.click()
    p.fill("input[aria-label='search emoji' i]", query)
    p.locator("[role=dialog][aria-label='Expression Picker'] button[data-emoji]").first.click()
    p.wait_for_timeout(300)
    if p.locator("[role=dialog][aria-label='Expression Picker']").count():
        p.keyboard.press("Escape")


def typed(actor, text, delay=55):
    """Somebody else typing a message, off camera, so their typing shows for whoever watches."""
    box = actor.page.locator("textarea[aria-label^='Message']").first
    box.click()
    actor.page.keyboard.type(text, delay=delay)
    actor.page.keyboard.press("Enter")


def english(actor):
    """Back to English, if a clip that went wrong left someone in another language."""
    p = actor.page
    if p.evaluate("() => document.documentElement.lang") == "en":
        return
    p.goto(actor.base + "/channels/@me?settings=language", wait_until="networkidle")
    p.click("[data-testid=language-en]")
    p.wait_for_function("() => document.documentElement.lang === 'en'", timeout=15000)
    p.keyboard.press("Escape")


def start(actor, x=0.62, y=0.55):
    """Where the pointer rests when a take begins."""
    actor.mouse = (actor.size[0] * x, actor.size[1] * y)
    if not actor.phone:
        actor.page.mouse.move(*actor.mouse)


@clip("general", "sign-up-with-just-a-username",
      "Making an account with a username and a password, nothing else: no email, no phone number. The recovery key shown at the end can restore everything if the password is ever lost.")
def g_signup(w):
    sam = Actor(w.browser, w.base, "sam", "Sam")
    p = sam.page
    p.goto(w.base + "/register", wait_until="networkidle")
    start(sam, 0.75, 0.8)
    with take((sam, "general/sign-up-with-just-a-username")):
        sam.click("input[autocomplete=username]")
        sam.type("sam")
        sam.click("input[autocomplete=nickname]")
        sam.type("Sam")
        pw = p.locator("input[type=password]")
        sam.click(pw.nth(0))
        sam.type("correct horse battery staple", speed=1.8)
        sam.click(pw.nth(1))
        sam.type("correct horse battery staple", speed=1.8)
        sam.rest(0.3)
        sam.click("button[type=submit]")
        p.wait_for_selector("[data-testid=recovery-key]", timeout=60000)
        sam.rest(2.2)
        sam.click("[data-testid=recovery-saved]")
        sam.rest(0.3)
        sam.click("[data-testid=recovery-done]")
        p.wait_for_url("**/channels/@me", timeout=30000)
        try:
            p.wait_for_selector("[data-testid=terms-gate]", timeout=6000)
            sam.rest(1.2)
            sam.click("[data-testid=terms-check]")
            sam.click("[data-testid=terms-agree]")
        except Exception:
            pass
        sam.rest(1.5)
    w.people["sam"] = sam


@clip("general", "chatting-in-a-server",
      "A server's channel while friends talk: messages arrive as they are sent, you see who is typing, reactions land, and Maya joins in.")
def g_live_chat(w):
    for n in ("theo", "juno", "kai", "rin", "maya"):
        w.go(w[n], w.channel_url("general"))
    maya = w["maya"]
    start(maya)
    with take((maya, "general/chatting-in-a-server")):
        typed(w["juno"], "ok who's bringing the popcorn 🍿")
        maya.rest(1.2)
        typed(w["kai"], "i've got popcorn AND the good chocolate")
        maya.rest(0.8)
        react(w["theo"], "the good chocolate", "fire")
        maya.rest(0.6)
        react(w["juno"], "the good chocolate", "heart")
        maya.rest(0.8)
        maya.click("textarea[aria-label^='Message']")
        maya.type("kai you're the best")
        maya.key("Enter", after=1.0)
        typed(w["rin"], "screen share is ready whenever you are 🎬")
        maya.rest(1.5)


@clip("general", "reply-react-edit",
      "Replying to a message, reacting with an emoji, and fixing a message after sending it (up arrow to edit).")
def g_reply(w):
    maya = w["maya"]
    w.go(maya, w.channel_url("general"))
    start(maya)
    with take((maya, "general/reply-react-edit")):
        row = maya.page.locator("[data-id]:has-text('pizza run?')").last
        maya.hover(row, fx=0.45)
        maya.rest(0.5)
        maya.click(row.locator("button[aria-label='Reply']"))
        maya.rest(0.3)
        maya.type("yes!! extra cheese please")
        maya.key("Enter", after=0.9)
        row = maya.page.locator("[data-id]:has-text('see you all at 9')").last
        maya.hover(row, fx=0.45)
        maya.rest(0.4)
        maya.click(row.locator("button[aria-label='Add Reaction']").first)
        maya.rest(0.4)
        maya.click("input[aria-label='search emoji' i]")
        maya.type("purple heart")
        maya.rest(0.5)
        maya.click("[role=dialog][aria-label='Expression Picker'] button[data-emoji]")
        maya.rest(0.9)
        if maya.page.locator("[role=dialog][aria-label='Expression Picker']").count():
            maya.key("Escape")
        maya.click("textarea[aria-label^='Message']")
        maya.key("ArrowUp", after=0.6)
        maya.key("End", after=0.2)
        maya.type(" and garlic bread 🧄")
        maya.key("Enter", after=1.6)


@clip("general", "photos-several-at-once",
      "Sending three photos in one message (each encrypted on the device before it leaves), the grid they land in, and flicking through them full screen.")
def g_photos(w):
    maya = w["maya"]
    w.go(maya, w.channel_url("photos"))
    start(maya)
    with take((maya, "general/photos-several-at-once")):
        maya.click("[data-testid=composer-plus]")
        maya.rest(0.4)
        with maya.page.expect_file_chooser() as fc:
            maya.click("text=Upload a File")
        fc.value.set_files([str(w.assets / f"{n}.jpg") for n in ("sunset-pier", "lake", "beach")])
        maya.rest(1.2)
        maya.click("textarea[aria-label^='Message']")
        maya.type("saturday at the lake 📸")
        maya.key("Enter")
        maya.page.wait_for_selector("[data-testid=picture-grid] img", timeout=60000)
        maya.rest(1.6)
        maya.click(maya.page.locator("[data-testid=picture-grid] img").first)
        maya.page.wait_for_selector("[data-testid=lightbox]", timeout=15000)
        maya.rest(1.4)
        for _ in range(2):
            maya.click("button[aria-label='Next Picture']")
            maya.rest(1.2)
        maya.key("Escape", after=1.0)


@clip("general", "gif-search",
      "Searching GIFs in the picker and sending one.")
def g_gifs(w):
    maya = w["maya"]
    w.go(maya, w.channel_url("general"))
    start(maya)
    with take((maya, "general/gif-search")):
        maya.click("button[aria-label='Select Emoji']")
        maya.rest(0.5)
        maya.click("[role=dialog][aria-label='Expression Picker'] button:has-text('GIFs')")
        maya.rest(1.5)
        maya.click("[role=dialog][aria-label='Expression Picker'] input")
        maya.type("popcorn")
        maya.page.wait_for_function("() => document.querySelectorAll(\"[role=dialog][aria-label='Expression Picker'] img\").length > 4", timeout=30000)
        maya.rest(2.2)
        maya.click(maya.page.locator("[role=dialog][aria-label='Expression Picker'] img").nth(1))
        maya.rest(2.5)


@clip("general", "custom-emoji",
      "A server's own emoji: picked as you type their name, sent, and named when you point at one. Servers can have as many as they like.")
def g_emoji(w):
    maya = w["maya"]
    w.go(maya, w.channel_url("general"))
    start(maya)
    with take((maya, "general/custom-emoji")):
        maya.click("textarea[aria-label^='Message']")
        maya.type("tonight is going to be great :owlh")
        maya.page.wait_for_selector("[role=listbox]", timeout=10000)
        maya.rest(0.8)
        maya.key("Enter", after=0.3)
        maya.type(" :chee")
        maya.page.wait_for_selector("[role=listbox]", timeout=10000)
        maya.rest(0.6)
        maya.key("Enter", after=0.3)
        maya.key("Enter", after=1.0)
        img = maya.page.locator("[data-id] img.md-emoji[alt=':owlhype:']").last
        maya.hover(img)
        maya.rest(2.0)
        maya.glide(VIEW[0] * 0.6, VIEW[1] * 0.4)
        maya.rest(0.8)


@clip("general", "sticker-pack-shared-by-link",
      "Making a personal sticker pack, sharing it with a link, and a friend installing it from that link. Packs belong to people, not servers, and work everywhere.",
      discord="Partly in Discord: custom stickers there belong to a server (5 slots free, more with boosts) and need Nitro to use elsewhere; there are no personal packs or pack links. Source: support.discord.com/hc/en-us/articles/4403089981975-Custom-Stickers-FAQ")
def g_pack(w):
    maya, theo = w["maya"], w["theo"]
    dm = f"{w.base}/channels/@me/{w.dm_theo}"
    w.go(maya, dm)
    start(maya)
    with take((maya, "general/sticker-pack-shared-by-link-maya-makes-it")):
        maya.click("[data-testid=user-settings]")
        maya.rest(0.5)
        maya.click("[data-nav=stickers]")
        maya.rest(0.8)
        maya.click("[data-testid=create-pack]")
        maya.rest(0.4)
        maya.click("[data-testid=create-pack-name]")
        maya.key("ControlOrMeta+a", after=0.15)
        maya.type("Owl Moods")
        maya.key("Enter", after=0.8)
        maya.page.wait_for_selector("[data-testid=pack-name]", timeout=15000)
        with maya.page.expect_file_chooser() as fc:
            maya.click("[data-drop-zone]")
        fc.value.set_files([str(w.assets / "stickers" / f"{n}.png") for n, _ in STICKERS])
        maya.page.wait_for_selector("[data-testid=upload-stickers]", timeout=15000)
        maya.rest(1.0)
        maya.click("[data-testid=upload-stickers]")
        maya.page.wait_for_selector("[data-testid=upload-stickers]", state="detached", timeout=60000)
        maya.rest(1.0)
        maya.click("[role=dialog] label:has-text('Public pack')")
        maya.rest(0.8)
        maya.click("[role=dialog] button:has-text('Copy Link')")
        maya.rest(0.8)
        link = maya.page.evaluate("() => navigator.clipboard.readText()")
        maya.key("Escape", after=0.6)
        if maya.page.locator("[role=dialog][aria-label='User Settings']").count():
            maya.key("Escape", after=0.6)
        maya.click("textarea[aria-label^='Message']")
        maya.paste(link)
        maya.type(" made these, take them 🦉")
        maya.key("Enter", after=1.6)
    w.go(theo, dm)
    start(theo, 0.5, 0.45)
    with take((theo, "general/sticker-pack-shared-by-link-theo-installs-it")):
        maya_link = theo.page.locator("[data-id] a[href*='/stickers/pack/']").last
        theo.click(maya_link)
        theo.page.wait_for_selector("button:has-text('Install Pack')", timeout=20000)
        theo.rest(1.6)
        theo.click("button:has-text('Install Pack')")
        theo.rest(1.8)


@clip("general", "polls",
      "A poll in a channel: asked, then answered by friends while the results fill in live.")
def g_poll(w):
    for n in ("theo", "juno", "kai", "rin", "maya"):
        w.go(w[n], w.channel_url("general"))
    maya = w["maya"]
    start(maya)
    with take((maya, "general/polls")):
        maya.click("[data-testid=composer-plus]")
        maya.rest(0.4)
        maya.click("text=Create Poll")
        maya.rest(0.6)
        maya.click("[data-testid=poll-question]")
        maya.type("what are we watching tonight? 🍿")
        answers = ["Spirited Away", "Knives Out", "The Princess Bride"]
        for i, a in enumerate(answers):
            inputs = maya.page.locator("[data-testid=poll-answer-input]")
            if i >= inputs.count():
                maya.click("[data-testid=poll-add-answer]")
                maya.rest(0.3)
            maya.click(maya.page.locator("[data-testid=poll-answer-input]").nth(i))
            maya.type(a)
        maya.rest(0.4)
        maya.click("[data-testid=poll-post]")
        maya.page.wait_for_selector("[data-testid=poll-answer]", timeout=20000)
        maya.rest(1.0)
        for who, pick in (("theo", 0), ("juno", 2), ("kai", 0), ("rin", 1)):
            o = w[who].page
            o.wait_for_selector("[data-testid=poll-answer]", timeout=20000)
            o.locator("[data-testid=poll-answer]").nth(pick).click()
            o.click("[data-testid=poll-vote]")
            maya.rest(1.0)
        maya.click(maya.page.locator("[data-testid=poll-answer]").nth(0))
        maya.rest(0.3)
        maya.click("[data-testid=poll-vote]")
        maya.rest(2.2)


@clip("general", "youtube-video-in-chat",
      "A YouTube link becomes a card with its title, channel and length, and plays inside the chat. The video comes through Nexa, so YouTube never sees who is watching.")
def g_youtube(w):
    maya = w["maya"]
    w.go(maya, w.channel_url("music"))
    start(maya)
    with take((maya, "general/youtube-video-in-chat")):
        maya.click("textarea[aria-label^='Message']")
        maya.paste("https://www.youtube.com/watch?v=aqz-KE-bpKQ")
        maya.type(" the short film i told you about")
        maya.key("Enter")
        maya.page.wait_for_selector("[data-testid=youtube-video]", timeout=90000)
        maya.rest(2.0)
        maya.click("[data-testid=youtube-video]")
        maya.page.wait_for_selector("[data-testid=youtube-frame]", timeout=30000)
        maya.rest(7.0)


def join_voice(actor, server_url):
    p = actor.page
    if not p.url.startswith(server_url.rsplit("/", 1)[0]):
        p.goto(server_url, wait_until="networkidle")
    p.locator("[data-channel]:has(span:text-is('General')) button").first.click()
    p.wait_for_selector("[data-testid=voice-status]", timeout=30000)


@clip("general", "voice-channel",
      "Joining a server's voice channel where friends already are: speaking rings light up as people talk, and muting is one click. Calls are end-to-end encrypted.")
def g_voice(w):
    url = w.channel_url("general")
    for n in ("theo", "juno", "kai", "rin"):
        w.go(w[n], url)
        join_voice(w[n], url)
    maya = w["maya"]
    w.go(maya, url)
    start(maya, 0.3, 0.4)
    with take((maya, "general/voice-channel")):
        maya.click("[data-channel]:has(span:text-is('General')) button")
        maya.page.wait_for_function("() => document.querySelectorAll('[data-peer-state=connected][data-user]').length >= 5", timeout=60000)
        maya.rest(4.0)
        maya.click("[data-testid=voice-mute]")
        maya.rest(1.5)
        maya.click("[data-testid=voice-mute]")
        maya.rest(3.0)
    w.in_voice = True


@clip("general", "screen-share-60fps",
      "Sharing a screen at full resolution and 60 frames a second, and watching it from the call. Nobody needs a subscription for it.",
      discord="In Discord, sharing above 720p at 30 fps needs Nitro (or a boosted server). Source: support.discord.com/hc/en-us/articles/360040816151-Go-Live-and-Screen-Share")
def g_share(w):
    url = w.channel_url("general")
    sharer = Actor(w.browser, w.base, "rin", "Rin", share=True)
    sharer.login()
    if not getattr(w, "in_voice", False):
        for n in ("theo", "juno", "kai"):
            w.go(w[n], url)
            join_voice(w[n], url)
    w.go(w["rin"], w.base + "/channels/@me")
    sharer.page.goto(url, wait_until="networkidle")
    join_voice(sharer, url)
    maya = w["maya"]
    if not maya.page.locator("[data-testid=voice-status]").count():
        w.go(maya, url)
        join_voice(maya, url)
    maya.page.wait_for_function("() => document.querySelectorAll('[data-peer-state=connected][data-user]').length >= 4", timeout=60000)
    start(maya, 0.5, 0.5)
    with take((maya, "general/screen-share-60fps")):
        sp = sharer.page
        sp.click("[data-testid=screen-share]")
        sp.click("[data-testid=share-res-source]")
        sp.click("[data-testid=share-fps-60]")
        sp.click("[data-testid=go-live]")
        maya.page.wait_for_selector("[data-testid=watch-stream]", timeout=30000)
        maya.rest(1.2)
        maya.click("[data-testid=watch-stream]")
        maya.rest(6.0)
        maya.click("[data-testid=stop-watching]")
        maya.rest(1.2)
    sharer.close()
    for n in ("maya", "theo", "juno", "kai"):
        try:
            if w[n].page.locator("[data-testid=voice-status-leave]").count():
                w[n].page.click("[data-testid=voice-status-leave]")
        except Exception:
            pass
    w.in_voice = False


@clip("general", "dm-call",
      "A friend calls: the call rings, is answered, and the conversation stays beside it.")
def g_dm_call(w):
    maya, theo = w["maya"], w["theo"]
    dm = f"{w.base}/channels/@me/{w.dm_theo}"
    w.go(maya, dm)
    w.go(theo, dm)
    start(maya, 0.55, 0.5)
    with take((maya, "general/dm-call")):
        theo.page.click("[data-testid=start-call]")
        maya.page.wait_for_selector("[data-testid=incoming-call]", timeout=20000)
        maya.rest(2.0)
        maya.click("[data-testid=accept-call]")
        maya.page.wait_for_function("() => document.querySelectorAll('[data-peer-state=connected][data-user]').length >= 2", timeout=40000)
        maya.rest(4.0)
    w.dm_call = True


@clip("general", "guest-call-link",
      "Someone without an account joins a call from a link: they pick a name and they are in. The link carries the call's key, so it stays end-to-end encrypted.")
def g_guest(w):
    maya, theo = w["maya"], w["theo"]
    if not getattr(w, "dm_call", False):
        dm = f"{w.base}/channels/@me/{w.dm_theo}"
        w.go(maya, dm)
        w.go(theo, dm)
        theo.page.click("[data-testid=start-call]")
        maya.page.wait_for_selector("[data-testid=incoming-call]", timeout=20000)
        maya.page.click("[data-testid=accept-call]")
    maya.page.wait_for_function("() => document.querySelectorAll('[data-peer-state=connected][data-user]').length >= 2", timeout=40000)
    maya.page.click("[data-testid=invite-guests]")
    box = maya.page.locator("[data-testid=guest-link]")
    box.wait_for(timeout=15000)
    link = ""
    for _ in range(100):
        link = box.input_value()
        if "#k=" in link:
            break
        time.sleep(0.1)
    maya.page.keyboard.press("Escape")
    guest = Actor(w.browser, w.base, "guest", "Alex")
    guest.page.goto(link, wait_until="networkidle")
    start(guest, 0.7, 0.75)
    with take((guest, "general/guest-call-link")):
        guest.click("[data-testid=guest-name]")
        guest.type("Alex")
        guest.rest(0.4)
        guest.click("[data-testid=guest-join]")
        guest.page.wait_for_selector("[data-testid=guest-call]", timeout=30000)
        guest.page.wait_for_function("() => document.querySelectorAll('[data-peer-state=connected][data-user]').length >= 3", timeout=60000)
        guest.rest(4.0)
    guest.close()
    for a in (maya, theo):
        try:
            a.page.click("[data-testid=voice-status-leave]", timeout=3000)
        except Exception:
            pass
    w.dm_call = False


THEME_TOUR = ["midnight", "forest", "rose", "amber", "ocean", "nexa-light", "sky", "rose-light"]


@clip("general", "colour-themes",
      "Going through Nexa's colour themes in Settings, Appearance; every one keeps text readable.")
def g_themes(w):
    maya = w["maya"]
    w.go(maya, w.channel_url("general"))
    maya.open_settings("appearance")
    first = maya.page.evaluate("() => (document.querySelector('[data-theme-id][aria-pressed=true], [data-theme-id][aria-checked=true], [data-theme-id][data-on=true]') || {}).getAttribute?.('data-theme-id') || 'dark'")
    maya.close_settings()
    start(maya)
    with take((maya, "general/colour-themes")):
        maya.click("[data-testid=user-settings]")
        maya.rest(0.5)
        maya.click("[data-nav=appearance]")
        maya.rest(1.0)
        for theme in THEME_TOUR:
            maya.click(f"[data-theme-id='{theme}']")
            maya.rest(1.1)
        maya.click(f"[data-theme-id='{first}']")
        maya.rest(0.8)
        maya.key("Escape", after=1.2)


@clip("general", "profiles",
      "People's profiles: a name, pronouns, an About Me, colours of their own, mutual friends and servers; and Maya editing her own.")
def g_profiles(w):
    maya = w["maya"]
    w.go(maya, w.channel_url("general"))
    start(maya)
    with take((maya, "general/profiles")):
        maya.click("[data-testid=member-list] [data-user]:has-text('Juno')")
        maya.page.wait_for_selector("[data-testid=profile-modal]", timeout=15000)
        maya.rest(2.4)
        maya.key("Escape", after=0.6)
        maya.click("[data-testid=member-list] [data-user]:has-text('Kai')")
        maya.page.wait_for_selector("[data-testid=profile-modal]", timeout=15000)
        maya.rest(2.2)
        maya.key("Escape", after=0.6)
        maya.click("[data-testid=user-settings]")
        maya.rest(0.4)
        maya.click("[data-nav=profiles]")
        maya.rest(1.0)
        maya.click("[data-testid=about-me-editor] textarea")
        maya.key("End", after=0.2)
        maya.type(" ✨")
        maya.rest(0.6)
        maya.click("[role=dialog] button:has-text('Save Changes')")
        maya.rest(1.2)
        maya.key("Escape", after=1.0)


@clip("general", "friend-request-with-a-note",
      "Adding a friend with a note that says who you are, and the request as they see it.")
def g_friend(w):
    maya = w["maya"]
    if "sam" not in w.people:
        sam = Actor(w.browser, w.base, "sam", "Sam")
        sam.register("Sam")
        w.people["sam"] = sam
    sam = w["sam"]
    w.go(maya, w.base + "/channels/@me")
    start(maya)
    with take((maya, "general/friend-request-with-a-note-sending")):
        maya.click("button:has-text('Add Friend') >> nth=0")
        maya.rest(0.5)
        maya.click("input[aria-label=Username]")
        maya.type("sam")
        maya.click("[data-testid=friend-request-add-note]")
        maya.click("[data-testid=friend-request-note-input]")
        maya.type("hey! it's maya from the climbing gym 🧗")
        maya.rest(0.4)
        maya.click("button:has-text('Send Friend Request')")
        maya.rest(1.6)
    w.go(sam, w.base + "/channels/@me")
    start(sam)
    with take((sam, "general/friend-request-with-a-note-receiving")):
        sam.click("button:has-text('Pending')")
        sam.rest(2.0)
        sam.click("[data-testid=friend-accept]")
        sam.rest(1.8)


def make_server(actor, name):
    p = actor.page
    p.goto(actor.base + "/channels/@me", wait_until="networkidle")
    p.click("button[aria-label='Add a Server']")
    p.fill("[data-testid=server-name]", name)
    p.click("[data-testid=create-server-submit]")
    p.wait_for_url("**/channels/*/*", timeout=30000)
    p.wait_for_timeout(800)
    return p.url.split("/channels/")[1].split("/")[0]


@clip("general", "server-folders",
      "Dropping one server onto another makes a folder on the server list; folders open and close with a click.")
def g_folders(w):
    maya = w["maya"]
    if not getattr(w, "extra_servers", None):
        w.extra_servers = [make_server(maya, "Study Group"), make_server(maya, "Book Club")]
    w.go(maya, w.channel_url("general"))
    start(maya, 0.4, 0.5)
    a, b = w.extra_servers
    with take((maya, "general/server-folders")):
        maya.drag(f"[data-server='{b}']", f"[data-server='{a}']", dur=1.0, hold=0.35)
        maya.rest(1.2)
        if maya.page.locator("[data-testid=folder-save]").count():
            maya.click("[data-testid=folder-name]")
            maya.type("School")
            maya.click("[data-testid=folder-save]")
            maya.rest(1.0)
        maya.click("[data-testid=folder-button]")
        maya.rest(1.2)
        maya.click("[data-testid=folder-button]")
        maya.rest(1.2)


@clip("general", "quick-switcher-and-search",
      "Jumping anywhere with Ctrl/Cmd+K, and searching every conversation's history, which is decrypted and indexed on the device.")
def g_switcher(w):
    maya = w["maya"]
    w.go(maya, w.channel_url("general"))
    start(maya)
    with take((maya, "general/quick-switcher-and-search")):
        maya.key("Meta+k", after=0.6)
        maya.page.wait_for_selector("[data-testid=switcher-input]", timeout=10000)
        maya.type("gam")
        maya.rest(0.8)
        maya.key("Enter", after=1.2)
        maya.key("Meta+k", after=0.6)
        maya.type("pizza")
        maya.page.wait_for_selector("[data-testid=switcher-hit]", timeout=15000)
        maya.rest(1.5)
        maya.click(maya.page.locator("[data-testid=switcher-hit]").first)
        maya.rest(2.0)


@clip("general", "safety-numbers",
      "Checking a friend's encryption key: the safety number and QR code both people can compare to be sure nobody is in between.")
def g_safety(w):
    maya = w["maya"]
    w.go(maya, f"{w.base}/channels/@me/{w.dm_theo}")
    start(maya)
    with take((maya, "general/safety-numbers")):
        maya.click("[data-testid=key-chip]")
        maya.rest(3.0)
        maya.key("Escape", after=1.0)


@clip("general", "eleven-languages",
      "Nexa in another language with one click (eleven of them), including Arabic, which turns the whole layout right to left.")
def g_languages(w):
    maya = w["maya"]
    w.go(maya, w.channel_url("general"))
    start(maya)
    with take((maya, "general/eleven-languages")):
        maya.click("[data-testid=user-settings]")
        maya.rest(0.5)
        maya.click("[data-nav=language]")
        maya.rest(1.0)
        try:
            for code in ("ja", "ar", "es"):
                maya.click(f"[data-testid=language-{code}]")
                maya.rest(1.8)
            maya.click("[data-testid=language-en]")
            maya.rest(1.0)
            maya.key("Escape", after=1.0)
        finally:
            english(maya)


MARKDOWN_DEMO = """> [!TIP]
> movie night checklist
- [x] popcorn
- [x] blankets
- [ ] pick the movie (vote above!)

| who | brings |
|---|---|
| Kai | snacks |
| Rin | the screen |"""


@clip("general", "rich-formatting",
      "Messages with a callout, a checklist and a table, written in markdown and drawn properly.")
def g_markdown(w):
    maya = w["maya"]
    w.go(maya, w.channel_url("general"))
    start(maya)
    with take((maya, "general/rich-formatting")):
        maya.click("textarea[aria-label^='Message']")
        maya.paste(MARKDOWN_DEMO, after=0.8)
        maya.key("Enter", after=2.5)


@clip("general", "tabs",
      "Keeping several conversations open in tabs (right-click, Open in New Tab) and moving between them.")
def g_tabs(w):
    maya = w["maya"]
    w.go(maya, w.channel_url("general"))
    start(maya, 0.5, 0.5)
    with take((maya, "general/tabs")):
        for ch in ("music", "gaming"):
            target = f"[data-channel]:has(span:text-is('{ch}'))"
            maya.hover(target)
            maya.rest(0.3)
            x, y = maya.mouse
            maya.page.mouse.click(x, y, button="right")
            maya.rest(0.5)
            maya.click("text=Open in New Tab")
            maya.rest(1.0)
        tabs = maya.page.locator("[role=tablist] [role=tab]")
        for i in (0, 1, 2):
            if i < tabs.count():
                maya.click(tabs.nth(i))
                maya.rest(1.0)


@clip("general", "custom-status",
      "Setting a custom status with an emoji, which friends see beside your name.")
def g_status(w):
    maya = w["maya"]
    w.go(maya, w.channel_url("general"))
    start(maya)
    with take((maya, "general/custom-status")):
        maya.click("button[aria-haspopup=menu]:has-text('Maya')")
        maya.rest(0.6)
        maya.click("text=Set a Custom Status")
        maya.rest(0.6)
        maya.click("[role=dialog] input[maxlength='128']")
        maya.type("movie night 🎬")
        maya.rest(0.4)
        maya.click("[role=dialog] button:has-text('Save')")
        maya.rest(2.0)


@clip("general", "on-a-phone",
      "Nexa on a phone: the drawer with servers and conversations, a channel, and sending a message.")
def g_phone(w):
    phone = Actor(w.browser, w.base, "juno", "Juno", phone=True)
    phone.login()
    phone.page.goto(w.channel_url("general"), wait_until="networkidle")
    phone.page.wait_for_timeout(1200)
    start(phone, 0.5, 0.5)
    with take((phone, "general/on-a-phone")):
        phone.rest(0.6)
        phone.click("button[aria-label='Open Navigation']")
        phone.rest(1.2)
        phone.click("[data-channel]:has(span:text-is('photos'))")
        phone.rest(1.6)
        phone.click("button[aria-label='Open Navigation']")
        phone.rest(1.0)
        phone.click("[data-channel]:has(span:text-is('general'))")
        phone.rest(1.0)
        phone.click("textarea[aria-label^='Message']")
        phone.type("on my way! 🏃")
        phone.click("button[aria-label='Send Message']")
        phone.rest(1.8)
    phone.close()


# ---------------------------------------------------------------------------
# Running it
# ---------------------------------------------------------------------------

README_HEAD = """Nexa: raw footage for the trailer
=================================

Every clip is the real app, recorded as it ran, on a throwaway server with
made-up people. Nothing is edited: each clip starts and ends on a still moment
so it can be cut anywhere. 1920x1080 (phone clips 1080 wide, portrait), 60 fps,
H.264, no sound.

unique/   things Discord does not have in any form, free or Nitro, on any of
          its apps, as of October 2026. Each entry says what Discord lacks and
          where that was checked.
general/  everyday use: chatting, calls, profiles, themes and so on. Some of
          it exists in Discord too, sometimes only with Nitro.

Things in some clips that belong to others, to check before a public trailer
uses them:
  general/gif-search         GIFs found through Klipy's search (their owners')
  unique/telegram-sticker-pack-import
                             Telegram's own "Utya Duck" sticker pack
  general/youtube-video-in-chat
                             "Big Buck Bunny" by the Blender Foundation, CC BY 3.0
                             (credit: Blender Foundation, peach.blender.org)
The people, the server, the messages and every other picture are made up.

"""


def describe(stem):
    best = None
    for name, c in CLIPS.items():
        if stem == name or stem.startswith(name + "-"):
            if best is None or len(name) > len(best[0]):
                best = (name, c)
    return best[1] if best else None


def write_readme(out):
    lines = [README_HEAD]
    for folder in ("unique", "general"):
        files = sorted((out / folder).glob("*.mp4")) if (out / folder).exists() else []
        if not files:
            continue
        lines.append(f"{folder}/\n" + "-" * (len(folder) + 1) + "\n")
        for f in files:
            c = describe(f.stem)
            lines.append(f"{f.name}  ({duration(f):.0f} s)")
            if c:
                lines.append("  " + c["about"])
                if c.get("discord"):
                    lines.append("  Not in Discord: " + c["discord"])
            lines.append("")
    (out / "README.txt").write_text("\n".join(lines).rstrip() + "\n")


def env_keys():
    """The GIF and Telegram keys from chatd.env, and nothing else from it."""
    keys = {}
    path = ROOT / "chatd.env"
    if path.exists():
        for line in path.read_text().splitlines():
            line = line.strip()
            if not line or line.startswith("#") or "=" not in line:
                continue
            k, v = line.split("=", 1)
            if k.strip() in ("KLIPY_API_KEY", "TELEGRAM_BOT_TOKEN") and v.strip():
                keys[k.strip()] = v.strip().strip('"').strip("'")
    return keys


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", default=str(ROOT / "examplevideos"))
    ap.add_argument("--only", default="", help="comma-separated clip names (see --list)")
    ap.add_argument("--list", action="store_true")
    args = ap.parse_args()
    if args.list:
        for name, c in CLIPS.items():
            print(f"{c['folder']:8} {name}")
        return 0
    chosen = [n.strip() for n in args.only.split(",") if n.strip()] or list(CLIPS)
    unknown = [n for n in chosen if n not in CLIPS]
    if unknown:
        print("no such clip:", ", ".join(unknown), file=sys.stderr)
        return 2
    if not shutil.which("ffmpeg"):
        print("ffmpeg is not on PATH", file=sys.stderr)
        return 1
    out = Path(args.out)
    for sub in ("raw", "assets"):
        shutil.rmtree(WORK / sub, ignore_errors=True)
    for old in WORK.glob("failed-*.png"):
        old.unlink()
    WORK.mkdir(parents=True, exist_ok=True)
    os.environ.update(env_keys())
    srv = Server(public_url=ADDRESS)
    front = TlsFront(srv.port)
    front.start()
    front.ready.wait(10)
    failed = []
    try:
        with sync_playwright() as pw:
            browser = pw.chromium.launch(channel="chromium", args=[
                f"--host-resolver-rules=MAP nexa.pm 127.0.0.1:{front.port}",
                "--ignore-certificate-errors",
                # the recorder films its own tab without asking
                "--auto-accept-this-tab-capture",
                # calls: a stand-in microphone and camera, allowed by the contexts' permissions
                "--use-fake-device-for-media-stream",
                "--autoplay-policy=no-user-gesture-required",
                # several people at once: nobody's tab is slowed for being out of sight
                "--disable-renderer-backgrounding",
                "--disable-background-timer-throttling",
                "--disable-backgrounding-occluded-windows",
            ])
            w = World(browser, srv, ADDRESS)
            seed(w)
            for name in chosen:
                print("clip:", name)
                try:
                    CLIPS[name]["fn"](w)
                except Exception:
                    traceback.print_exc()
                    failed.append(name)
                    for a in w.people.values():
                        try:
                            a.page.screenshot(path=str(WORK / f"failed-{name}-{a.name}.png"))
                            if a.filming:
                                a.record_stop("failed/" + name + "-" + a.name)
                            a.page.goto(w.base + "/channels/@me", wait_until="networkidle")
                            english(a)
                        except Exception:
                            pass
            browser.close()
    finally:
        srv.stop()
    print("delivering")
    for rel, raw, phone, cut in TAKES:
        final = out / f"{rel}.mp4"
        deliver(raw, final, phone, cut)
        print(f"  {rel}.mp4  {duration(final):.1f} s")
    write_readme(out)
    if failed:
        print("failed:", ", ".join(failed))
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())

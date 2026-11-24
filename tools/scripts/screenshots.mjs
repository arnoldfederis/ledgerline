#!/usr/bin/env bun
import { spawn } from "node:child_process";
import { existsSync, mkdirSync, rmSync } from "node:fs";
import { argv, exit } from "node:process";

// Captures the README screenshots from the running dev server.
//
//   bun tools/scripts/screenshots.mjs [--url http://localhost:5173] [--scale 2]
//
// Headless Chrome rather than a screenshot tool, for one reason: the page is
// rendered at `--force-device-scale-factor`, so a 1180px-wide layout comes out
// 2360px wide with text rasterised at that size. Resizing a 1x capture
// afterwards cannot recover those pixels.

const CHROME = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";

const WIDTH = 1180;
const HEIGHT = 1340;

const SHOTS = [
  { theme: "dark", out: "docs/dashboard-dark.png" },
  { theme: "light", out: "docs/dashboard-light.png" },
];

function flag(name, fallback) {
  const index = argv.indexOf(`--${name}`);
  return index >= 0 && argv[index + 1] ? argv[index + 1] : fallback;
}

const baseUrl = flag("url", "http://localhost:5173");
const scale = Number(flag("scale", "2"));

function run(command, args) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, { stdio: ["ignore", "ignore", "pipe"] });
    let stderr = "";

    child.stderr.on("data", (chunk) => {
      stderr += chunk;
    });
    child.on("error", reject);
    child.on("close", (code) => {
      if (code === 0) {
        resolve();
      } else {
        reject(new Error(`${command} exited ${code}\n${stderr.trim()}`));
      }
    });
  });
}

if (!existsSync(CHROME)) {
  console.error(`Google Chrome not found at ${CHROME}`);
  exit(1);
}

const reachable = await fetch(baseUrl)
  .then((r) => r.ok)
  .catch(() => false);

if (!reachable) {
  console.error(`No dev server at ${baseUrl}. Start it with: bun run dev`);
  exit(1);
}

mkdirSync("docs", { recursive: true });

// A throwaway profile keeps the capture independent of any local Chrome state.
const profile = `/tmp/ledgerline-screenshots-${Date.now()}`;

try {
  for (const { theme, out } of SHOTS) {
    await run(CHROME, [
      "--headless=new",
      `--user-data-dir=${profile}`,
      "--hide-scrollbars",
      "--no-first-run",
      "--disable-extensions",
      `--force-device-scale-factor=${scale}`,
      `--window-size=${WIDTH},${HEIGHT}`,
      // The fonts and the first report need a moment before the shutter.
      "--virtual-time-budget=6000",
      `--screenshot=${out}`,
      `${baseUrl}/?theme=${theme}`,
    ]);

    console.log(`${out}  ${WIDTH * scale}x${HEIGHT * scale}`);
  }
} finally {
  rmSync(profile, { recursive: true, force: true });
}

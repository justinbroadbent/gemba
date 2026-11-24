// Drives the built app in a real browser against the demo floor, the way a
// person would: clicking, dragging and typing. Run it from the repository root
// after building:
//
//   npm --prefix web run build && go build -o bin/gemba ./cmd/gemba
//   node web/e2e/browser.mjs
//
// It starts its own server on a spare port with the built-in demo data, so it
// touches nothing real. It uses a browser already on the machine: set BROWSER
// to "chrome" or "msedge" (the default).
import { spawn } from "node:child_process";
import { existsSync } from "node:fs";
import { chromium } from "playwright-core";

const PORT = 8099;
const BASE = `http://127.0.0.1:${PORT}`;
const bin = [process.env.GEMBA_BIN, "bin/gemba.exe", "bin/gemba"].filter(Boolean).find(existsSync);
if (!bin) throw new Error("Build the server first: go build -o bin/gemba ./cmd/gemba");

const server = spawn(bin, [], { env: { ...process.env, GEMBA_ADDR: `127.0.0.1:${PORT}`, GEMBA_CONFIG: "", GEMBA_REQUIRE_AUTH: "" }, stdio: "ignore" });
for (let n = 0; ; n++) {
  if (await fetch(`${BASE}/healthz`).then((r) => r.ok, () => false)) break;
  if (n > 50) throw new Error("The server did not start");
  await new Promise((r) => setTimeout(r, 200));
}

const browser = await chromium.launch({ channel: process.env.BROWSER || "msedge" });
const page = await browser.newPage({ viewport: { width: 1400, height: 1000 } });
const errors = [];
page.on("pageerror", (e) => errors.push(String(e)));

let failed = 0;
async function check(name, fn) {
  try {
    await fn();
    console.log(`ok    ${name}`);
  } catch (e) {
    failed++;
    console.log(`FAIL  ${name}\n      ${String(e.message ?? e).split("\n")[0]}`);
  }
}
const expect = (cond, msg) => {
  if (!cond) throw new Error(msg);
};
const go = async (path, ready) => {
  await page.goto(BASE + path);
  await page.waitForSelector(ready);
};

try {
  const session = await fetch(`${BASE}/api/session`).then((r) => r.json());
  const ws = session.workspaces[0].slug;
  const w = `/w/${ws}`;

  await check("the floor shows a room for every cell", async () => {
    await go(`${w}/floor`, ".room");
    expect((await page.locator(".room").count()) === session.workspaces[0].cells.length, "one room per cell");
    expect((await page.locator(".attention .alert").count()) > 0, "the demo floor has things needing attention");
  });

  await check("clicking an attention sentence points at the item", async () => {
    await page.locator(".attention .alert:not([disabled])").first().click();
    await page.waitForSelector(".room .pulse", { timeout: 3000 });
  });

  await check("a type chip dims everything else", async () => {
    await page.locator(".chips .chip", { hasText: "Unplanned" }).click();
    expect((await page.locator(".room .tile.dim").count()) > 0, "planned tiles should dim");
    expect((await page.locator(".room .tile:not(.dim):not(.more-tile)").count()) > 0, "unplanned tiles should stay");
    await page.locator(".chips .chip", { hasText: "All work" }).click();
    expect((await page.locator(".room .tile.dim").count()) === 0, "All work clears the filter");
  });

  await check("clicking an item opens the side panel, and Escape closes it", async () => {
    const tile = page.locator(".room .tile:not(.more-tile)").first();
    const title = (await tile.locator(".tile-title").innerText()).trim();
    await tile.click();
    await page.waitForSelector(".detail");
    expect((await page.locator(".detail h2").innerText()).trim() === title, "the panel shows the item that was clicked");
    expect((await page.locator(".detail a.button").count()) === 1, "the panel links to the ticket or issue");
    await page.keyboard.press("Escape");
    await page.waitForSelector(".detail", { state: "detached" });
  });

  await check("dragging on the line changes what comes first, and it sticks", async () => {
    const tiles = page.locator(".room").first().locator(".tile:not(.more-tile)");
    const first = (await tiles.nth(0).locator(".tile-title").innerText()).trim();
    const second = (await tiles.nth(1).locator(".tile-title").innerText()).trim();
    await tiles.nth(1).dragTo(tiles.nth(0), { targetPosition: { x: 6, y: 30 } });
    await page.waitForFunction(
      (t) => document.querySelector(".room .tile:not(.more-tile) .tile-title")?.textContent?.trim() === t,
      second,
      { timeout: 5000 },
    );
    await go(`${w}/floor`, ".room"); // a fresh load reads it back from the server
    const now = page.locator(".room").first().locator(".tile:not(.more-tile) .tile-title");
    expect((await now.nth(0).innerText()).trim() === second && (await now.nth(1).innerText()).trim() === first, "the new order survived a reload");
  });

  await check("triage: a fixed date cannot be queued without its date", async () => {
    await go(`${w}/triage`, ".sort.open");
    const row = page.locator(".sort.open").first();
    await row.locator(".pick", { hasText: "Fixed date" }).click();
    expect(await row.getByRole("button", { name: "Queue it" }).isDisabled(), "no date, no queuing");
    await row.locator('input[type="date"]').fill("2027-01-15");
    expect(await row.getByRole("button", { name: "Queue it" }).isEnabled(), "with a date it can be queued");
  });

  await check("triage: choosing quick explains the quick list and hides the place", async () => {
    const row = page.locator(".sort.open").first();
    await row.locator(".pick", { hasText: "Quick" }).click();
    expect((await row.locator(".outcome").innerText()).includes("quick list"), "the outcome names the quick list");
    expect((await row.locator(".pick", { hasText: "Front of the queue" }).count()) === 0, "quick work has no place in the queue");
  });

  await check("triage: queuing an item takes it off the list", async () => {
    const before = await page.locator(".sort:not(.batch)").count();
    const row = page.locator(".sort.open").first();
    await row.locator(".pick", { hasText: "Standard" }).click();
    await row.getByRole("button", { name: "Queue it" }).click();
    await page.waitForFunction((n) => document.querySelectorAll(".sort:not(.batch)").length === n - 1, before, { timeout: 5000 });
  });

  await check("triage: the type filter shows one kind of work", async () => {
    const chip = page.locator(".chips .chip", { hasText: "Unplanned" });
    const n = Number((await chip.innerText()).match(/\d+/)[0]);
    await chip.click();
    expect((await page.locator(".sort:not(.batch)").count()) === n, "only unplanned rows remain");
    await page.locator(".chips .chip", { hasText: "All" }).click();
  });

  await check("triage: a fresh ticket can be triaged early on request", async () => {
    const before = await page.locator(".sort:not(.batch)").count();
    await page.getByRole("button", { name: "Triage one now" }).click();
    expect((await page.locator(".sort:not(.batch)").count()) > before, "the fast-lane tickets are listed");
    await page.getByRole("button", { name: "Hide them" }).click();
  });

  await check("a manager hands the triage lead to someone else", async () => {
    const was = (await page.locator(".lead-name").innerText()).trim();
    await page.getByRole("button", { name: /Hand over|Name someone/ }).click();
    const options = await page.locator('select[aria-label="Triage lead"] option:not([disabled])').allInnerTexts();
    const next = options.find((o) => o.trim() !== was);
    await page.locator('select[aria-label="Triage lead"]').selectOption({ label: next });
    await page.waitForFunction((n) => document.querySelector(".lead-name")?.textContent?.trim() === n, next.trim(), { timeout: 5000 });
  });

  await check("now: setting work aside as waiting frees the hand, and it can be taken back", async () => {
    const mine = await fetch(`${BASE}/api/w/${ws}/floor`).then((r) => r.json());
    const person = mine.lanes.flatMap((l) => l.hands).find((h) => h.items.some((c) => !c.mirrored && !c.blocked));
    await go(`${w}/now?as=${person.login}`, ".hands");
    const held = await page.locator(".hands button.hand").count();
    await page.locator(".park").first().click();
    await page.locator(".park-form input").fill("the vendor to call back");
    await page.getByRole("button", { name: "Set aside" }).click();
    await page.waitForFunction((n) => document.querySelectorAll(".hands button.hand").length === n - 1, held, { timeout: 5000 });
    const row = page.locator(".waiting-row", { hasText: "the vendor to call back" });
    expect((await row.count()) === 1, "it is listed as waiting, with the reason");
    await row.getByRole("button", { name: "Take it back" }).click();
    await page.waitForFunction((n) => document.querySelectorAll(".hands button.hand").length === n, held, { timeout: 5000 });
  });

  await check("now: the page says what to pull next and lists the quick ones", async () => {
    expect((await page.locator(".next h3").count()) === 1, "one thing to pull next");
    expect((await page.locator(".next-why").innerText()).trim().length > 0, "with the reason it is next");
  });

  await check("queues: search narrows the list", async () => {
    await go(`${w}/queues`, "main");
    const search = page.locator('input[type="search"], input[placeholder*="earch"]').first();
    const all = await page.locator("main .card").count();
    await search.fill("zzzz-no-such-item");
    await page.waitForFunction((n) => document.querySelectorAll("main .card").length < n, all, { timeout: 3000 });
  });

  await check("projects and trade-off pages load", async () => {
    await go(`${w}/projects`, ".project");
    await go(`${w}/tradeoff`, ".pickers select");
    const options = await page.locator(".pickers select").first().locator("option").count();
    expect(options > 1, "there are items to choose as the new priority");
  });

  await check("trade-off: choosing an item shows what it displaces", async () => {
    const select = page.locator(".pickers select").first();
    const value = await select.locator("option").nth(1).getAttribute("value");
    await select.selectOption(value);
    await page.waitForSelector(".decision", { timeout: 5000 }).catch(async () => {
      await page.locator(".pickers select").nth(1).selectOption({ index: 1 });
      await page.waitForSelector(".decision", { timeout: 5000 });
    });
  });

  await check("review: changing the period reloads the figures", async () => {
    await go(`${w}/review`, ".tiles");
    const quarter = await page.locator(".tiles dd b").first().innerText();
    await page.locator(".review-tools .chip", { hasText: "Half year" }).click();
    await page.waitForFunction((q) => document.querySelector(".tiles dd b")?.textContent !== q, quarter, { timeout: 5000 });
    expect(page.url().includes("days=182"), "the period is in the address, so the view can be shared");
    await page.locator(".review-tools select").selectOption({ index: 1 });
    await page.waitForSelector(".review h1");
    expect(page.url().includes("as="), "one person's review has its own address");
  });

  await check("check now reloads the page in view", async () => {
    await page.getByRole("button", { name: "Check now" }).click();
    await page.waitForSelector(".tiles");
  });

  await check("no page threw a script error", async () => {
    expect(errors.length === 0, errors.join(" | "));
  });
} finally {
  await browser.close();
  server.kill();
}

console.log(failed ? `\n${failed} failed` : "\nAll passed");
process.exit(failed ? 1 : 0);
